package server

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- inspections ----

// defaultRooms is the checklist a new inspection starts with.
var defaultRooms = []struct {
	room  string
	items []string
}{
	{"Entrance", []string{"Front door and lock", "Keys", "Walls and floor"}},
	{"Living room", []string{"Walls", "Floor", "Windows", "Lights and sockets"}},
	{"Kitchen", []string{"Cupboards", "Sink and taps", "Stove and oven", "Fridge"}},
	{"Bathroom", []string{"Toilet", "Shower or bath", "Sink and taps", "Tiles"}},
	{"Bedroom", []string{"Walls", "Floor", "Windows", "Wardrobe"}},
	{"Meters", []string{"Electricity reading", "Water reading", "Heating or gas reading"}},
}

type inspectionIn struct {
	PropertyID *int64  `json:"property_id"`
	UnitID     *int64  `json:"unit_id"`
	LeaseID    *int64  `json:"lease_id"`
	Kind       *string `json:"kind"`
	Date       *string `json:"date"`
	Time       *string `json:"time"`
	Status     *string `json:"status"`
	Summary    *string `json:"summary"`
	FromID     int64   `json:"from_id"` // start from this inspection's lines
	Blank      bool    `json:"blank"`   // no checklist
}

func (in inspectionIn) apply(i *store.Inspection, keys map[string]bool) error {
	if in.PropertyID != nil {
		i.PropertyID = *in.PropertyID
	}
	if keys == nil || keys["unit_id"] {
		i.UnitID = nonZero(in.UnitID)
	}
	if keys == nil || keys["lease_id"] {
		i.LeaseID = nonZero(in.LeaseID)
	}
	trimP(&i.Kind, in.Kind)
	trimP(&i.Date, in.Date)
	trimP(&i.Time, in.Time)
	trimP(&i.Status, in.Status)
	trimP(&i.Summary, in.Summary)
	i.Summary = clip(i.Summary, 20000)
	switch {
	case i.PropertyID == 0:
		return &userErr{"say which place it is at"}
	case !oneOf(i.Kind, store.InspectionKind):
		return &userErr{"an inspection is at moving in, moving out, or a routine one"}
	case i.Date == "" || !validDate(i.Date):
		return &userErr{"say the day (YYYY-MM-DD)"}
	case i.Time != "" && !reTime.MatchString(i.Time):
		return &userErr{"the time is HH:MM"}
	case !oneOf(i.Status, []string{"planned", "done"}):
		return &userErr{"an inspection is planned or done"}
	}
	return nil
}

func (s *Server) checkInspectionRefs(ctx context.Context, uid int64, i *store.Inspection) error {
	if _, err := s.store.GetProperty(ctx, uid, i.PropertyID); err != nil {
		return &userErr{"that place is not there"}
	}
	if i.LeaseID != nil {
		l, err := s.store.GetLease(ctx, uid, *i.LeaseID)
		if err != nil || l.PropertyID != i.PropertyID {
			return &userErr{"that lease is not for this place"}
		}
		if i.UnitID == nil {
			i.UnitID = &l.UnitID
		}
	}
	if i.UnitID != nil {
		if u, err := s.store.GetUnit(ctx, uid, *i.UnitID); err != nil || u.PropertyID != i.PropertyID {
			return &userErr{"that unit is not at this place"}
		}
	}
	return nil
}

func (s *Server) handleListInspections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListInspections(ctx, actorID(ctx), int64(queryInt(r, "property")), int64(queryInt(r, "lease")))
	if err != nil {
		s.fail(w, err, "inspections")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateInspection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in inspectionIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	i := &store.Inspection{UserID: uid, Kind: "routine", Date: today(), Status: "planned"}
	if err := in.apply(i, nil); err != nil {
		s.userFail(w, err, "inspection")
		return
	}
	if err := s.checkInspectionRefs(ctx, uid, i); err != nil {
		s.userFail(w, err, "inspection")
		return
	}
	if err := s.store.CreateInspection(ctx, i); err != nil {
		s.fail(w, err, "inspection")
		return
	}
	var items []*store.InspectionItem
	if in.FromID > 0 {
		if from, err := s.store.GetInspection(ctx, uid, in.FromID); err == nil {
			for _, it := range from.Items {
				items = append(items, &store.InspectionItem{Room: it.Room, Item: it.Item})
			}
		}
	}
	if items == nil && !in.Blank {
		for _, r := range defaultRooms {
			for _, it := range r.items {
				items = append(items, &store.InspectionItem{Room: r.room, Item: it})
			}
		}
	}
	if len(items) > 0 {
		_ = s.store.SetItems(ctx, uid, i.ID, items)
	}
	s.inspectionLine(ctx, uid, i, "planned")
	go s.syncInspectionEvent(context.WithoutCancel(ctx), uid, i.ID)
	s.respondInspection(w, r, i.ID, http.StatusCreated)
}

var inspectionWords = map[string]string{"move_in": "Moving-in inspection", "move_out": "Moving-out inspection", "routine": "Inspection"}

func (s *Server) inspectionLine(ctx context.Context, uid int64, i *store.Inspection, what string) {
	title := inspectionWords[i.Kind] + " planned for " + niceDate(i.Date)
	if what == "done" {
		title = inspectionWords[i.Kind] + " done"
		if i.Issues > 0 {
			title += fmt.Sprintf(": %d %s to look at", i.Issues, plural(i.Issues, "thing", "things"))
		}
	}
	pid := i.PropertyID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: i.LeaseID, PropertyID: &pid, Kind: "inspection", Title: title, URL: "/inspections/" + itoa(i.ID)})
}

// respondInspection answers with an inspection, and for moving out the
// moving-in one beside it (how each line looked then).
func (s *Server) respondInspection(w http.ResponseWriter, r *http.Request, id int64, status int) {
	ctx := r.Context()
	uid := actorID(ctx)
	i, err := s.store.GetInspection(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "inspection")
		return
	}
	out := map[string]any{"inspection": i}
	if before := s.earlierInspection(ctx, uid, i); before != nil {
		was := map[string]map[string]any{}
		for _, it := range before.Items {
			was[strings.ToLower(it.Room+"|"+it.Item)] = map[string]any{"condition": it.Condition, "note": it.Note, "photos": it.Photos}
		}
		out["before"] = map[string]any{"id": before.ID, "kind": before.Kind, "date": before.Date, "items": was}
	}
	if i.LeaseID != nil {
		if l, err := s.store.GetLease(ctx, uid, *i.LeaseID); err == nil {
			out["lease"] = map[string]any{"id": l.ID, "title": place(l.UnitName, l.PropertyName), "tenants": l.Tenants, "deposit": l.Deposit, "deposit_state": l.DepositState, "currency": l.Currency}
		}
	}
	out["made"], _ = s.store.ListMade(ctx, uid, "inspection", id)
	writeJSON(w, status, out)
}

// earlierInspection is what a moving-out (or routine) inspection is
// compared with: the moving-in one of the same lease, else the last one done
// before it in the same unit.
func (s *Server) earlierInspection(ctx context.Context, uid int64, i *store.Inspection) *store.Inspection {
	if i.Kind == "move_in" {
		return nil
	}
	list, _ := s.store.ListInspections(ctx, uid, i.PropertyID, 0)
	var pick *store.Inspection
	for _, o := range list {
		if o.ID == i.ID || o.Status != "done" || o.Date > i.Date || !sameUnit(o.UnitID, i.UnitID) {
			continue
		}
		if i.LeaseID != nil && o.LeaseID != nil && *o.LeaseID == *i.LeaseID && o.Kind == "move_in" {
			pick = o
			break
		}
		if pick == nil || o.Date > pick.Date {
			pick = o
		}
	}
	if pick == nil {
		return nil
	}
	full, err := s.store.GetInspection(ctx, uid, pick.ID)
	if err != nil {
		return nil
	}
	return full
}

func sameUnit(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Server) handleGetInspection(w http.ResponseWriter, r *http.Request) {
	s.respondInspection(w, r, idParam(r, "id"), http.StatusOK)
}

func (s *Server) handleUpdateInspection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	i, err := s.store.GetInspection(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "inspection")
		return
	}
	var in inspectionIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	was := i.Status
	if err := in.apply(i, keys); err != nil {
		s.userFail(w, err, "inspection")
		return
	}
	if err := s.checkInspectionRefs(ctx, uid, i); err != nil {
		s.userFail(w, err, "inspection")
		return
	}
	if err := s.store.UpdateInspection(ctx, i); err != nil {
		s.fail(w, err, "inspection")
		return
	}
	if was != "done" && i.Status == "done" {
		full, _ := s.store.GetInspection(ctx, uid, i.ID)
		if full != nil {
			s.inspectionLine(ctx, uid, full, "done")
		}
	}
	go s.syncInspectionEvent(context.WithoutCancel(ctx), uid, i.ID)
	s.respondInspection(w, r, i.ID, http.StatusOK)
}

func (s *Server) handleSetItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetInspection(ctx, uid, id); err != nil {
		s.fail(w, err, "inspection")
		return
	}
	var in []*store.InspectionItem
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in) > 500 {
		writeErr(w, http.StatusBadRequest, "at most 500 lines")
		return
	}
	var items []*store.InspectionItem
	for _, it := range in {
		it.Room, it.Item, it.Note = clip(strings.TrimSpace(it.Room), 80), clip(strings.TrimSpace(it.Item), 120), clip(strings.TrimSpace(it.Note), 2000)
		if it.Item == "" && it.Room == "" {
			continue
		}
		if !oneOf(it.Condition, store.Conditions) {
			writeErr(w, http.StatusBadRequest, "a condition is good, fair, poor, damaged or missing")
			return
		}
		items = append(items, it)
	}
	if err := s.store.SetItems(ctx, uid, id, items); err != nil {
		s.fail(w, err, "inspection")
		return
	}
	s.cleanFiles(ctx)
	s.respondInspection(w, r, id, http.StatusOK)
}

func (s *Server) handleDeleteInspection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetInspection(ctx, uid, id); err != nil {
		s.fail(w, err, "inspection")
		return
	}
	s.dropEvent(ctx, uid, "inspection", id)
	files, err := s.store.DeleteInspection(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "inspection")
		return
	}
	for _, f := range files {
		s.removeFile(uid, f)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- photos and files ----

func (s *Server) filesDir() string { return filepath.Join(s.cfg.DataDir, "files") }

func (s *Server) filePath(uid, id int64) string {
	return filepath.Join(s.filesDir(), strconv.FormatInt(uid, 10), strconv.FormatInt(id, 10))
}

func (s *Server) removeFile(uid, id int64) { _ = os.Remove(s.filePath(uid, id)) }

// cleanFiles removes stored files nothing points at any more.
func (s *Server) cleanFiles(ctx context.Context) {
	ids, err := s.store.AllFileIDs(ctx)
	if err != nil {
		return
	}
	users, _ := os.ReadDir(s.filesDir())
	for _, u := range users {
		if !u.IsDir() {
			continue
		}
		fs, _ := os.ReadDir(filepath.Join(s.filesDir(), u.Name()))
		for _, f := range fs {
			id, err := strconv.ParseInt(f.Name(), 10, 64)
			if err == nil && !ids[id] {
				_ = os.Remove(filepath.Join(s.filesDir(), u.Name(), f.Name()))
			}
		}
	}
}

// ownerOK says whether the person owns what a file is put on.
func (s *Server) ownerOK(ctx context.Context, uid int64, owner string, id int64) bool {
	var err error
	switch owner {
	case "item":
		_, err = s.store.ItemOwner(ctx, uid, id)
	case "request":
		_, err = s.store.GetRequest(ctx, uid, id)
	case "lease":
		_, err = s.store.GetLease(ctx, uid, id)
	case "property":
		_, err = s.store.GetProperty(ctx, uid, id)
	case "inspection":
		_, err = s.store.GetInspection(ctx, uid, id)
	default:
		return false
	}
	return err == nil
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	owner, ownerID := r.URL.Query().Get("owner"), int64(queryInt(r, "owner_id"))
	if !s.ownerOK(ctx, uid, owner, ownerID) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "send the file as “file” (it may be too big)")
		return
	}
	defer f.Close()
	name := filepath.Base(strings.TrimSpace(hdr.Filename))
	ct := hdr.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" {
		ct = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	}
	if !strings.HasPrefix(ct, "image/") && ct != "application/pdf" {
		writeErr(w, http.StatusBadRequest, "photos (JPEG, PNG, HEIC, WebP) and PDFs only")
		return
	}
	rec := &store.File{UserID: uid, Owner: owner, OwnerID: ownerID, Name: clip(name, 200), Mime: ct}
	if err := s.store.AddFile(ctx, rec); err != nil {
		s.fail(w, err, "file")
		return
	}
	path := s.filePath(uid, rec.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		_ = s.store.DeleteFile(ctx, uid, rec.ID)
		s.fail(w, err, "file")
		return
	}
	out, err := os.Create(path)
	if err != nil {
		_ = s.store.DeleteFile(ctx, uid, rec.ID)
		s.fail(w, err, "file")
		return
	}
	n, err := io.Copy(out, f)
	out.Close()
	if err != nil {
		_ = os.Remove(path)
		_ = s.store.DeleteFile(ctx, uid, rec.ID)
		writeErr(w, http.StatusBadRequest, "the file did not arrive whole")
		return
	}
	rec.Size = n
	_, _ = s.store.DB.ExecContext(ctx, `UPDATE files SET size = ? WHERE id = ?`, n, rec.ID)
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f, err := s.store.GetFile(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "file")
		return
	}
	fh, err := os.Open(s.filePath(uid, f.ID))
	if err != nil {
		writeErr(w, http.StatusNotFound, "the file is gone")
		return
	}
	defer fh.Close()
	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disp := "inline"
	if queryBool(r, "download") {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, disp, f.Name))
	http.ServeContent(w, r, "", parseTime(f.CreatedAt), fh)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if err := s.store.DeleteFile(ctx, uid, id); err != nil {
		s.fail(w, err, "file")
		return
	}
	s.removeFile(uid, id)
	w.WriteHeader(http.StatusNoContent)
}
