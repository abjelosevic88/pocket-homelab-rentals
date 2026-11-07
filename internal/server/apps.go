package server

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- rentals in the other Pocket apps ----
//
// From a lease, a repair, an inspection or a tenant, as their owner
// (their own Core link or direct token): the tenant into Pocket Contacts,
// a repair or a renewal as a task, inspections, visits and lease ends in
// Pocket Calendar, a report as a note, an email from Pocket Mail, the lease
// to sign through Pocket Sign, a draft invoice for a rent, a document from
// Pocket Documents attached, a finished repair in Pocket Home's log. Each
// thing made is remembered with a link; a failed try says why in plain words.
//
// Nothing here is needed for renting itself: when an app is not on the box,
// switched off or too old, the page greys the button and says what to do.

type makeIn struct {
	Owner      string `json:"owner"` // lease | request | inspection | property | tenant
	OwnerID    int64  `json:"owner_id"`
	Target     string `json:"target"` // contact | task | note | email | signing | invoice | document
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	Due        string `json:"due"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	To         string `json:"to"`
	TemplateID int64  `json:"template_id"`
	Send       bool   `json:"send"`
	ChargeID   int64  `json:"charge_id"`
	Ref        string `json:"ref"` // document: its id; contact: the card to link
}

var makeTargets = map[string]pocketApp{"contact": appContacts, "task": appTasks, "note": appNotes, "email": appMail, "signing": appSign,
	"invoice": appInvoicing, "document": appDocuments}

// subject is what a made thing is about, in words and as a link here.
type subject struct {
	title, link string
	lease       *store.Lease
	request     *store.Request
	inspection  *store.Inspection
	property    *store.Property
	tenant      *store.Tenant
}

func (s *Server) subjectOf(ctx context.Context, uid int64, owner string, id int64) (*subject, error) {
	sub := &subject{}
	switch owner {
	case "lease":
		l, err := s.store.GetLease(ctx, uid, id)
		if err != nil {
			return nil, err
		}
		sub.lease, sub.title, sub.link = l, place(l.UnitName, l.PropertyName), s.link("/leases/"+itoa(id))
	case "request":
		r, err := s.store.GetRequest(ctx, uid, id)
		if err != nil {
			return nil, err
		}
		sub.request, sub.title, sub.link = r, r.Title+" — "+place(r.UnitName, r.PropertyName), s.link("/repairs/"+itoa(id))
		if r.LeaseID != nil {
			sub.lease, _ = s.store.GetLease(ctx, uid, *r.LeaseID)
		}
	case "inspection":
		in, err := s.store.GetInspection(ctx, uid, id)
		if err != nil {
			return nil, err
		}
		sub.inspection, sub.title, sub.link = in, inspectionWords[in.Kind]+" — "+place(in.UnitName, in.PropertyName), s.link("/inspections/"+itoa(id))
		if in.LeaseID != nil {
			sub.lease, _ = s.store.GetLease(ctx, uid, *in.LeaseID)
		}
	case "property":
		p, err := s.store.GetProperty(ctx, uid, id)
		if err != nil {
			return nil, err
		}
		sub.property, sub.title, sub.link = p, p.Name, s.link("/properties/"+itoa(id))
	case "tenant":
		t, err := s.store.GetTenant(ctx, uid, id)
		if err != nil {
			return nil, err
		}
		sub.tenant, sub.title, sub.link = t, t.Name, s.link("/tenants/"+itoa(id))
	default:
		return nil, &userErr{"say what it is for: a lease, a repair, an inspection, a place or a tenant"}
	}
	return sub, nil
}

func (sub *subject) origin() string { return "From Pocket Rentals — " + sub.title + ": " + sub.link }

// make makes one thing in another app and remembers it.
func (s *Server) make(ctx context.Context, c *clients, in makeIn) (*store.Made, error) {
	a, ok := makeTargets[in.Target]
	if !ok {
		return nil, &userErr{"make a contact, task, note, email, document to sign, invoice, or attach a document"}
	}
	uid := c.u.ID
	sub, err := s.subjectOf(ctx, uid, in.Owner, in.OwnerID)
	if err != nil {
		return nil, err
	}
	m := &store.Made{Owner: in.Owner, OwnerID: in.OwnerID, Target: in.Target, App: a.slug}
	var (
		res  map[string]any
		path string
	)
	switch in.Target {
	case "contact":
		if sub.tenant == nil {
			return nil, &userErr{"contacts are made for tenants"}
		}
		return s.linkContact(ctx, c, sub.tenant, in.Ref)
	case "task":
		res, path, err = s.makeTask(ctx, c, sub, in)
	case "note":
		res, path, err = s.makeNote(ctx, c, sub, in)
	case "email":
		err = s.sendEmail(ctx, c, sub, in)
		m.Title = firstNonEmpty(in.Subject, "Email")
	case "signing":
		res, path, err = s.makeSigning(ctx, c, sub, in)
	case "invoice":
		res, path, err = s.makeInvoice(ctx, c, sub, in)
	case "document":
		res, path, err = s.attachDocument(ctx, c, in)
	}
	if err != nil {
		return nil, err
	}
	if res != nil {
		m.Ref = idOf(res)
		m.Title = firstNonEmpty(str(res, "title", "name", "number"), in.Title, m.Title)
		m.State = str(res, "status")
		if in.Target == "invoice" && m.Title != "" && !strings.HasPrefix(m.Title, "Invoice") {
			m.Title = "Invoice " + m.Title
		}
		if sib, _ := c.conn(ctx, a, a.scopes[0]); sib != nil && sib.public != "" && m.Ref != "" && path != "" {
			m.URL = strings.TrimRight(sib.public, "/") + path
		}
	}
	if in.Target != "email" {
		if err := s.store.AddMade(ctx, uid, m); err != nil {
			return nil, err
		}
	}
	s.madeLine(ctx, uid, sub, m, in)
	return m, nil
}

// madeLine writes what was made on the timeline it belongs to.
func (s *Server) madeLine(ctx context.Context, uid int64, sub *subject, m *store.Made, in makeIn) {
	e := &store.Entry{UserID: uid, Kind: "made", App: m.App, URL: m.URL}
	switch {
	case sub.request != nil:
		e.RequestID, e.PropertyID, e.LeaseID = &sub.request.ID, &sub.request.PropertyID, sub.request.LeaseID
	case sub.inspection != nil:
		e.PropertyID, e.LeaseID = &sub.inspection.PropertyID, sub.inspection.LeaseID
	case sub.lease != nil:
		e.LeaseID = &sub.lease.ID
	case sub.property != nil:
		e.PropertyID = &sub.property.ID
	default:
		return
	}
	switch m.Target {
	case "task":
		e.Title = "Task made: " + m.Title
	case "note":
		e.Title = "Saved as a note"
	case "email":
		e.Kind, e.Title, e.Body = "email", "Email sent: "+firstNonEmpty(in.Subject, "(no subject)"), clip(in.Body, 4000)
	case "signing":
		e.Title = "Sent to sign: " + m.Title
		if !in.Send {
			e.Title = "Made ready to sign: " + m.Title
		}
	case "invoice":
		e.Title = "Draft invoice made: " + m.Title
	case "document":
		e.Title = "Document attached: " + m.Title
	default:
		return
	}
	_ = s.store.AddEntry(ctx, e)
}

// plainErr is a failed try in a sentence.
func plainErr(a pocketApp, err error) string {
	msg := err.Error()
	var ue *userErr
	if isNotConnected(err) || errors.As(err, &ue) {
		return msg
	}
	return a.name + ": " + msg
}

// ---- Pocket Contacts ----

func splitName(n string) (given, family string) {
	parts := strings.Fields(n)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

// firstValue is the first email or phone of a card (strings or {value}).
func firstValue(v any) string {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return ""
	}
	switch x := l[0].(type) {
	case string:
		return x
	case map[string]any:
		return str(x, "value")
	}
	return ""
}

// linkContact links a tenant to their card in Pocket Contacts: the one
// picked, the one with their email, or a new one.
func (s *Server) linkContact(ctx context.Context, c *clients, t *store.Tenant, pick string) (*store.Made, error) {
	rs, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		return nil, err
	}
	m := &store.Made{Owner: "tenant", OwnerID: t.ID, Target: "contact", App: appContacts.slug}
	cardURL := func(ref string) string {
		if rs.public == "" || ref == "" {
			return ""
		}
		return strings.TrimRight(rs.public, "/") + "/contacts/" + ref
	}
	link := func(card map[string]any, added bool) (*store.Made, error) {
		ref := idOf(card)
		t.ContactRef = ref
		if t.Email == "" {
			t.Email = strings.ToLower(firstValue(card["emails"]))
		}
		if t.Phone == "" {
			t.Phone = firstValue(card["phones"])
		}
		_ = s.store.UpdateTenant(ctx, t)
		m.Ref, m.Title, m.URL = ref, firstNonEmpty(str(card, "fn", "name"), t.Name), cardURL(ref)
		if added {
			m.Title += " (added)"
		}
		_ = s.store.AddMade(ctx, c.u.ID, m)
		return m, nil
	}
	if pick != "" {
		v, err := getJSONFrom(ctx, rs, "/api/v1/contacts/"+url.PathEscape(pick), nil)
		if err != nil {
			return nil, err
		}
		card, _ := v.(map[string]any)
		if card == nil {
			return nil, &userErr{"that contact is not there any more"}
		}
		if inner := obj(card, "card"); len(inner) > 0 {
			inner["id"] = card["id"]
			card = inner
		}
		return link(card, false)
	}
	if t.Email != "" {
		if v, err := getJSONFrom(ctx, rs, "/api/v1/lookup", url.Values{"email": {t.Email}}); err == nil {
			if found := rows(v, "contacts"); len(found) > 0 {
				return link(found[0], false)
			}
		}
	}
	ws, err := c.conn(ctx, appContacts, scopeContactsWrite)
	if err != nil {
		return nil, err
	}
	given, family := splitName(t.Name)
	card := map[string]any{"kind": "individual", "fn": t.Name, "given": given, "family": family, "note": "Tenant — From Pocket Rentals: " + s.link("/tenants/"+itoa(t.ID)), "categories": []string{"Tenant"}}
	if t.Email != "" {
		card["emails"] = []map[string]any{{"label": "Home", "value": t.Email, "pref": true}}
	}
	if t.Phone != "" {
		card["phones"] = []map[string]any{{"label": "Mobile", "value": t.Phone}}
	}
	res, err := postJSONTo(ctx, ws, "/api/v1/contacts", map[string]any{"card": card})
	if err != nil {
		return nil, err
	}
	return link(res, true)
}

// handleContactSearch is the tenant picker: cards from Pocket Contacts.
func (s *Server) handleContactSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}})
		return
	}
	sib, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}, "limit": {"12"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": "Pocket Contacts: " + err.Error()})
		return
	}
	out := []map[string]any{}
	for _, x := range rows(v, "contacts") {
		if str(x, "kind") == "org" {
			continue
		}
		out = append(out, map[string]any{"id": idOf(x), "name": str(x, "fn"), "company": str(x, "org"), "email": firstValue(x["emails"]), "phone": firstValue(x["phones"]),
			"initials": str(x, "initials"), "color": str(x, "color")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// ---- Pocket Tasks ----

func (s *Server) makeTask(ctx context.Context, c *clients, sub *subject, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appTasks, scopeTasksWrite)
	if err != nil {
		return nil, "", err
	}
	title, due := strings.TrimSpace(in.Title), in.Due
	switch {
	case sub.request != nil:
		title = firstNonEmpty(title, "Fix: "+sub.request.Title+" — "+place(sub.request.UnitName, sub.request.PropertyName))
		due = firstNonEmpty(due, sub.request.PlannedOn)
		if due == "" && sub.request.Priority == "urgent" {
			due = today()
		}
	case sub.inspection != nil:
		title = firstNonEmpty(title, inspectionWords[sub.inspection.Kind]+" — "+place(sub.inspection.UnitName, sub.inspection.PropertyName))
		due = firstNonEmpty(due, sub.inspection.Date)
	case sub.lease != nil:
		l := sub.lease
		title = firstNonEmpty(title, "Renew or end the lease — "+place(l.UnitName, l.PropertyName))
		if due == "" && l.EndDate != "" {
			due = store.AddDays(l.EndDate, -max(l.NoticeDays, 0)-7)
			if due < today() {
				due = today()
			}
		}
	default:
		title = firstNonEmpty(title, sub.title)
	}
	body := map[string]any{"title": clip(title, 200), "notes": clip(joinNonEmpty("\n\n", in.Notes, sub.origin()), 20000), "url": sub.link}
	if sub.request != nil && sub.request.Details != "" {
		body["notes"] = clip(joinNonEmpty("\n\n", in.Notes, sub.request.Details, sub.origin()), 20000)
	}
	if due != "" {
		if !validDate(due) {
			return nil, "", &userErr{"the due day is YYYY-MM-DD"}
		}
		body["due"] = due
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/tasks", body)
	if err != nil {
		return nil, "", err
	}
	if sub.request != nil {
		sub.request.TaskRef = idOf(res)
		_ = s.store.UpdateRequest(ctx, sub.request)
	}
	return res, "/today?task=" + idOf(res), nil
}

// updateTask keeps a repair's task named and dated like the repair.
func (s *Server) updateTask(ctx context.Context, uid int64, rq *store.Request) {
	c := s.clients(ctx, uid)
	sib, err := optional(c.conn(ctx, appTasks, scopeTasksWrite))
	if err != nil || sib == nil {
		return
	}
	body := map[string]any{"title": clip("Fix: "+rq.Title+" — "+place(rq.UnitName, rq.PropertyName), 200)}
	if rq.PlannedOn != "" {
		body["due"] = rq.PlannedOn
	}
	var out map[string]any
	if err := sib.client.Do(ctx, http.MethodPut, "/api/v1/tasks/"+url.PathEscape(rq.TaskRef), nil, body, &out); err != nil {
		s.log.Debug("tasks: update repair", "request", rq.ID, "err", err)
	}
}

// completeTask ticks a task off in Pocket Tasks (the repair was done here).
func (s *Server) completeTask(ctx context.Context, uid int64, ref string) {
	c := s.clients(ctx, uid)
	sib, err := optional(c.conn(ctx, appTasks, scopeTasksWrite))
	if err != nil || sib == nil {
		return
	}
	var out map[string]any
	if err := sib.client.Do(ctx, http.MethodPost, "/api/v1/tasks/"+url.PathEscape(ref)+"/complete", nil, map[string]any{}, &out); err != nil {
		s.log.Debug("tasks: complete", "task", ref, "err", err)
	}
}

// ---- Pocket Calendar: kept in step when the person wants that ----

type eventSpec struct {
	title, description, date, time, link string
	minutes                              int
}

// syncEvent keeps one thing's event in Pocket Calendar: made, moved, or
// removed when spec is nil.
func (s *Server) syncEvent(ctx context.Context, uid int64, owner string, ownerID int64, spec *eventSpec) {
	made, _ := s.store.ListMade(ctx, uid, owner, ownerID)
	var cur *store.Made
	for _, m := range made {
		if m.Target == "event" && m.Ref != "" {
			cur = m
			break
		}
	}
	c := s.clients(ctx, uid)
	if spec != nil && !c.st.CalendarSync && cur == nil {
		return
	}
	sib, err := optional(c.conn(ctx, appCalendar, scopeCalendarWrite))
	if err != nil || sib == nil {
		return
	}
	if spec == nil {
		if cur != nil {
			_ = sib.client.Do(ctx, http.MethodDelete, "/api/v1/events/"+url.PathEscape(cur.Ref), nil, nil, nil)
			_ = s.store.DeleteMade(ctx, uid, cur.ID)
		}
		return
	}
	body := map[string]any{"title": clip(spec.title, 200), "description": spec.description, "url": spec.link, "tz": time.Local.String(), "categories": []string{"Rentals"}}
	if spec.time != "" {
		start, err := time.ParseInLocation("2006-01-02 15:04", spec.date+" "+spec.time, time.Local)
		if err != nil {
			return
		}
		body["start"], body["end"] = start.Format("2006-01-02T15:04"), start.Add(time.Duration(max(spec.minutes, 30))*time.Minute).Format("2006-01-02T15:04")
	} else {
		body["start"], body["all_day"], body["busy"] = spec.date, true, false
	}
	if cur != nil {
		var out map[string]any
		err := sib.client.Do(ctx, http.MethodPut, "/api/v1/events/"+url.PathEscape(cur.Ref), nil, body, &out)
		if err == nil {
			_ = s.store.SetMadeState(ctx, uid, cur.ID, "", "")
			return
		}
		_ = s.store.DeleteMade(ctx, uid, cur.ID) // gone there: make it again
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		s.log.Debug("calendar: event", "owner", owner, "id", ownerID, "err", err)
		return
	}
	ref := idOf(res)
	if v, ok := res["event_id"].(float64); ok && v > 0 {
		ref = strconv.FormatInt(int64(v), 10)
	}
	ref = strings.SplitN(ref, ":", 2)[0]
	m := &store.Made{Owner: owner, OwnerID: ownerID, Target: "event", App: "calendar", Ref: ref, Title: spec.title}
	if sib.public != "" && ref != "" {
		m.URL = strings.TrimRight(sib.public, "/") + "/event/" + ref
	}
	_ = s.store.AddMade(ctx, uid, m)
}

func (s *Server) dropEvent(ctx context.Context, uid int64, owner string, id int64) {
	s.syncEvent(ctx, uid, owner, id, nil)
}

func (s *Server) syncLeaseEvent(ctx context.Context, uid, id int64) {
	l, err := s.store.GetLease(ctx, uid, id)
	if err != nil {
		return
	}
	if l.Status == store.LeaseEnded || l.EndDate == "" {
		s.dropEvent(ctx, uid, "lease", id)
		return
	}
	who := tenantNames(l.Tenants)
	s.syncEvent(ctx, uid, "lease", id, &eventSpec{title: "Lease ends — " + place(l.UnitName, l.PropertyName), date: l.EndDate, link: s.link("/leases/" + itoa(id)),
		description: joinNonEmpty("\n", who, fmt.Sprintf("Notice: %d days.", l.NoticeDays), "From Pocket Rentals: "+s.link("/leases/"+itoa(id)))})
}

func (s *Server) syncRequestEvent(ctx context.Context, uid, id int64) {
	r, err := s.store.GetRequest(ctx, uid, id)
	if err != nil {
		return
	}
	if !r.Open() || r.PlannedOn == "" {
		s.dropEvent(ctx, uid, "request", id)
		return
	}
	s.syncEvent(ctx, uid, "request", id, &eventSpec{title: "Repair: " + r.Title + " — " + place(r.UnitName, r.PropertyName), date: r.PlannedOn, time: r.PlannedTime, minutes: 60,
		link: s.link("/repairs/" + itoa(id)), description: joinNonEmpty("\n\n", r.Details, joinNonEmpty(" · ", r.ProName, r.ProPhone), "From Pocket Rentals: "+s.link("/repairs/"+itoa(id)))})
}

func (s *Server) syncInspectionEvent(ctx context.Context, uid, id int64) {
	in, err := s.store.GetInspection(ctx, uid, id)
	if err != nil {
		return
	}
	if in.Status == "done" && in.Date < today() {
		return // keep what happened in the calendar
	}
	s.syncEvent(ctx, uid, "inspection", id, &eventSpec{title: inspectionWords[in.Kind] + " — " + place(in.UnitName, in.PropertyName), date: in.Date, time: in.Time, minutes: 60,
		link: s.link("/inspections/" + itoa(id)), description: "From Pocket Rentals: " + s.link("/inspections/"+itoa(id))})
}

// handleICS is something's day as a calendar file, for when Pocket
// Calendar is not connected: any calendar app opens it.
func (s *Server) handleICS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	owner, id := chiParam(r, "owner"), idParam(r, "id")
	var spec *eventSpec
	switch owner {
	case "lease":
		if l, err := s.store.GetLease(ctx, uid, id); err == nil && l.EndDate != "" {
			spec = &eventSpec{title: "Lease ends — " + place(l.UnitName, l.PropertyName), date: l.EndDate, link: s.link("/leases/" + itoa(id))}
		}
	case "request":
		if rq, err := s.store.GetRequest(ctx, uid, id); err == nil && rq.PlannedOn != "" {
			spec = &eventSpec{title: "Repair: " + rq.Title + " — " + place(rq.UnitName, rq.PropertyName), date: rq.PlannedOn, time: rq.PlannedTime, minutes: 60, link: s.link("/repairs/" + itoa(id))}
		}
	case "inspection":
		if in, err := s.store.GetInspection(ctx, uid, id); err == nil {
			spec = &eventSpec{title: inspectionWords[in.Kind] + " — " + place(in.UnitName, in.PropertyName), date: in.Date, time: in.Time, minutes: 60, link: s.link("/inspections/" + itoa(id))}
		}
	}
	if spec == nil {
		writeErr(w, http.StatusNotFound, "nothing with a day to put in a calendar")
		return
	}
	esc := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Pocket Rentals//EN\r\nBEGIN:VEVENT\r\n")
	fmt.Fprintf(&b, "UID:rentals-%s-%d@pocket-rentals\r\nDTSTAMP:%s\r\n", owner, id, time.Now().UTC().Format("20060102T150405Z"))
	if spec.time == "" {
		d, _ := time.Parse("2006-01-02", spec.date)
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\nDTEND;VALUE=DATE:%s\r\n", d.Format("20060102"), d.AddDate(0, 0, 1).Format("20060102"))
	} else {
		st, _ := time.ParseInLocation("2006-01-02 15:04", spec.date+" "+spec.time, time.Local)
		fmt.Fprintf(&b, "DTSTART:%s\r\nDTEND:%s\r\n", st.UTC().Format("20060102T150405Z"), st.Add(time.Hour).UTC().Format("20060102T150405Z"))
	}
	fmt.Fprintf(&b, "SUMMARY:%s\r\nDESCRIPTION:%s\r\nURL:%s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", esc(spec.title), esc("From Pocket Rentals: "+spec.link), spec.link)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%d.ics"`, owner, id))
	_, _ = w.Write([]byte(b.String()))
}

// ---- Pocket Notes ----

func (s *Server) makeNote(ctx context.Context, c *clients, sub *subject, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		return nil, "", err
	}
	title := firstNonEmpty(strings.TrimSpace(in.Title), sub.title)
	content := joinNonEmpty("\n\n", in.Notes, s.subjectMarkdown(ctx, c.u.ID, sub), "— "+sub.origin())
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(title, 200), "content": content})
	if err != nil {
		return nil, "", err
	}
	return res, "/note/" + idOf(res), nil
}

var conditionWords = map[string]string{"good": "Good", "fair": "Fair", "poor": "Poor", "damaged": "Damaged", "missing": "Missing", "": "—"}

// subjectMarkdown is a lease, an inspection or a repair as Markdown.
func (s *Server) subjectMarkdown(ctx context.Context, uid int64, sub *subject) string {
	var b strings.Builder
	switch {
	case sub.inspection != nil:
		in := sub.inspection
		fmt.Fprintf(&b, "**%s** on %s", inspectionWords[in.Kind], niceDate(in.Date))
		if sub.lease != nil && len(sub.lease.Tenants) > 0 {
			fmt.Fprintf(&b, " · %s", tenantNames(sub.lease.Tenants))
		}
		b.WriteString("\n\n")
		if in.Summary != "" {
			b.WriteString(in.Summary + "\n\n")
		}
		room := ""
		for _, it := range in.Items {
			if it.Room != room {
				room = it.Room
				fmt.Fprintf(&b, "\n### %s\n\n", firstNonEmpty(room, "Other"))
			}
			fmt.Fprintf(&b, "- **%s** — %s", firstNonEmpty(it.Item, "—"), conditionWords[it.Condition])
			if it.Note != "" {
				b.WriteString(": " + it.Note)
			}
			if len(it.Photos) > 0 {
				fmt.Fprintf(&b, " (%d %s)", len(it.Photos), plural(len(it.Photos), "photo", "photos"))
			}
			b.WriteString("\n")
		}
	case sub.request != nil:
		r := sub.request
		fmt.Fprintf(&b, "**Repair:** %s (%s)\n\n", r.Title, r.Priority)
		if r.Details != "" {
			b.WriteString(r.Details + "\n\n")
		}
		if r.ProName != "" {
			fmt.Fprintf(&b, "**Who fixes it:** %s\n\n", joinNonEmpty(" · ", r.ProName, r.ProPhone))
		}
		if r.Cost > 0 {
			fmt.Fprintf(&b, "**Cost:** %s\n\n", fmtMoney(r.Cost, r.Currency))
		}
	case sub.lease != nil:
		l := sub.lease
		fmt.Fprintf(&b, "**Tenants:** %s\n\n", firstNonEmpty(tenantNames(l.Tenants), "—"))
		fmt.Fprintf(&b, "**Rent:** %s %s, due on day %d\n\n", fmtMoney(l.Rent, l.Currency), l.Frequency, l.DueDay)
		end := "until notice"
		if l.EndDate != "" {
			end = "to " + niceDate(l.EndDate)
		}
		fmt.Fprintf(&b, "**Runs:** from %s %s (notice %d days)\n\n", niceDate(l.StartDate), end, l.NoticeDays)
		if l.Deposit > 0 {
			fmt.Fprintf(&b, "**Deposit:** %s (%s)\n\n", fmtMoney(l.Deposit, l.Currency), strings.ReplaceAll(l.DepositState, "_", " "))
		}
		if l.Notes != "" {
			b.WriteString(l.Notes + "\n\n")
		}
		if full, cs, _, err := s.leaseFull(ctx, uid, l.ID); err == nil {
			fmt.Fprintf(&b, "**Owed now:** %s\n\n### Rent\n\n", fmtMoney(max(full.Balance, 0), l.Currency))
			for i := len(cs) - 1; i >= 0 && i >= len(cs)-12; i-- {
				ch := cs[i]
				fmt.Fprintf(&b, "- %s — %s, %s\n", niceDate(ch.DueDate), ch.Title, fmtMoney(ch.Amount, ch.Currency)+" ("+ch.State+")")
			}
		}
	case sub.property != nil:
		p := sub.property
		fmt.Fprintf(&b, "**%s**, %s\n\n%s", p.Name, joinNonEmpty(", ", p.Address, p.City, p.Country), p.Notes)
	}
	return strings.TrimSpace(b.String())
}

// ---- Pocket Mail ----

func mailHTML(body string) string {
	var h strings.Builder
	h.WriteString(`<!doctype html><html><head><meta charset="utf-8"></head><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2433;line-height:1.55">`)
	for _, p := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		h.WriteString(`<p style="margin:0 0 12px">` + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(p)), "\n", "<br>") + `</p>`)
	}
	h.WriteString(`</body></html>`)
	return h.String()
}

// recipients are the tenants with an email, or the one address given.
func recipients(sub *subject, to string) []map[string]string {
	if to = strings.ToLower(strings.TrimSpace(to)); to != "" {
		return []map[string]string{{"addr": to}}
	}
	var out []map[string]string
	if sub.lease != nil {
		for _, t := range sub.lease.Tenants {
			if t.Email != "" {
				out = append(out, map[string]string{"name": t.Name, "addr": t.Email})
			}
		}
	}
	if sub.tenant != nil && sub.tenant.Email != "" {
		out = append(out, map[string]string{"name": sub.tenant.Name, "addr": sub.tenant.Email})
	}
	return out
}

func (s *Server) sendEmail(ctx context.Context, c *clients, sub *subject, in makeIn) error {
	to := recipients(sub, in.To)
	if len(to) == 0 {
		return &userErr{"nobody here has an email address to write to"}
	}
	for _, r := range to {
		if !reEmail.MatchString(r["addr"]) {
			return &userErr{"that email address does not look right"}
		}
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return &userErr{"write the email first"}
	}
	subjectLine := clip(firstNonEmpty(strings.TrimSpace(in.Subject), sub.title), 200)
	if s.cfg.DemoData {
		s.log.Debug("demo: email not sent", "to", to, "subject", subjectLine)
		return nil
	}
	sib, err := c.conn(ctx, appMail, scopeMailSend)
	if err != nil {
		return err
	}
	msg := map[string]any{"from": c.st.MailFrom, "to": to, "subject": subjectLine, "body_text": body, "body_html": mailHTML(body)}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out map[string]any
	return sib.client.Do(ctx, http.MethodPost, "/api/v1/send", nil, msg, &out)
}

// reminderText fills the person's rent reminder for a lease.
func (s *Server) reminderText(st store.Settings, l *store.Lease, amount int64, due string) (string, string) {
	t := firstNonEmpty(st.ReminderText, store.DefaultReminder)
	name := tenantNames(l.Tenants)
	if len(l.Tenants) > 0 {
		first, _ := splitName(l.Tenants[0].Name)
		if len(l.Tenants) == 1 {
			name = firstNonEmpty(first, name)
		}
	}
	r := strings.NewReplacer("{name}", firstNonEmpty(name, "there"), "{amount}", fmtMoney(amount, l.Currency), "{due}", niceDate(due), "{place}", place(l.UnitName, l.PropertyName))
	return "Rent for " + place(l.UnitName, l.PropertyName), r.Replace(t)
}

// handleReminder is the rent reminder for a lease, ready to send or to copy.
func (s *Server) handleReminder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, cs, _, err := s.leaseFull(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	amount, due := l.Rent, l.NextDue
	if l.Balance > 0 {
		amount = l.Balance
		for _, c := range cs {
			if c.State == "late" || c.State == "due" || c.State == "part" {
				due = c.DueDate
				break
			}
		}
	}
	subj, body := s.reminderText(s.settingsOf(ctx, uid), l, amount, firstNonEmpty(due, today()))
	var emails []string
	for _, t := range l.Tenants {
		if t.Email != "" {
			emails = append(emails, t.Email)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"subject": subj, "body": body, "to": emails})
}

// ---- Pocket Sign ----

func (s *Server) makeSigning(ctx context.Context, c *clients, sub *subject, in makeIn) (map[string]any, string, error) {
	if sub.lease == nil {
		return nil, "", &userErr{"leases are sent to sign"}
	}
	var tenants []*store.Tenant
	for _, t := range sub.lease.Tenants {
		if t.Email != "" {
			tenants = append(tenants, t)
		}
	}
	if len(tenants) == 0 {
		return nil, "", &userErr{"add an email address to the tenants first: Pocket Sign sends it to them"}
	}
	if in.TemplateID <= 0 {
		return nil, "", &userErr{"pick a template from Pocket Sign"}
	}
	rs, err := c.conn(ctx, appSign, scopeSigningRead)
	if err != nil {
		return nil, "", err
	}
	v, err := getJSONFrom(ctx, rs, "/api/v1/templates", nil)
	if err != nil {
		return nil, "", err
	}
	var roles []map[string]any
	for _, t := range rows(v, "templates") {
		if int64(num(t, "id")) == in.TemplateID {
			roles = rows(map[string]any{"items": t["roles"]})
		}
	}
	if len(roles) == 0 {
		return nil, "", &userErr{"that Pocket Sign template has nobody to sign it"}
	}
	// the roles without a fixed email are the tenants', in order
	var people []map[string]any
	ti := 0
	for _, rl := range roles {
		if str(rl, "email") != "" || ti >= len(tenants) {
			continue
		}
		people = append(people, map[string]any{"recipient_id": int64(num(rl, "id")), "name": tenants[ti].Name, "email": tenants[ti].Email})
		ti++
	}
	if len(people) == 0 {
		return nil, "", &userErr{"that template has no place for the tenants to sign (every signer has an email already)"}
	}
	ws, err := c.conn(ctx, appSign, scopeSigningWrite)
	if err != nil {
		return nil, "", err
	}
	title := firstNonEmpty(strings.TrimSpace(in.Title), "Lease — "+place(sub.lease.UnitName, sub.lease.PropertyName))
	res, err := postJSONTo(ctx, ws, fmt.Sprintf("/api/v1/templates/%d/use", in.TemplateID),
		map[string]any{"title": clip(title, 200), "message": strings.TrimSpace(in.Notes), "people": people, "send": in.Send})
	if err != nil {
		return nil, "", err
	}
	if doc, ok := res["document"].(map[string]any); ok {
		res = doc
	}
	state := firstNonEmpty(strings.ToLower(str(res, "status")), "draft")
	_ = s.store.SetSigning(ctx, c.u.ID, sub.lease.ID, idOf(res), state)
	return res, "/documents/" + idOf(res), nil
}

// ---- Pocket Invoicing ----

func (s *Server) makeInvoice(ctx context.Context, c *clients, sub *subject, in makeIn) (map[string]any, string, error) {
	if sub.lease == nil {
		return nil, "", &userErr{"invoices are made for a lease's rent"}
	}
	l := sub.lease
	ch, err := s.store.GetCharge(ctx, c.u.ID, in.ChargeID)
	if err != nil || ch.LeaseID != l.ID {
		return nil, "", &userErr{"pick the rent (or charge) to invoice"}
	}
	if ch.InvoiceRef != "" {
		return nil, "", &userErr{"that one has an invoice already"}
	}
	if st := c.status(ctx, appInvoicing); st.Via != "" && !st.Can(scopeInvoicesWrite) {
		return nil, "", &notConnected{"Pocket Invoicing does not make invoices for Rentals yet: " + firstNonEmpty(st.Fix, st.Detail)}
	}
	sib, err := c.conn(ctx, appInvoicing, scopeInvoicesWrite)
	if err != nil {
		return nil, "", err
	}
	if len(l.Tenants) == 0 {
		return nil, "", &userErr{"add the tenant first: the invoice needs a client"}
	}
	t := l.Tenants[0]
	clientID := int64(0)
	if v, err := getJSONFrom(ctx, sib, "/api/v1/clients", url.Values{"q": {t.Name}}); err == nil {
		for _, cl := range rows(v, "items", "clients") {
			if strings.EqualFold(strings.TrimSpace(str(cl, "name")), t.Name) || (t.Email != "" && strings.EqualFold(str(cl, "email"), t.Email)) {
				clientID = int64(num(cl, "id"))
				break
			}
		}
	}
	if clientID == 0 {
		cl, err := postJSONTo(ctx, sib, "/api/v1/clients", map[string]any{"name": t.Name, "contact_name": t.Name, "email": t.Email, "phone": t.Phone,
			"address": place(l.UnitName, l.PropertyName)})
		if err != nil {
			return nil, "", err
		}
		clientID = int64(num(cl, "id"))
	}
	body := map[string]any{"client_id": clientID, "currency": ch.Currency, "status": "draft", "due_date": ch.DueDate,
		"notes": clip(joinNonEmpty("\n", strings.TrimSpace(in.Notes), sub.origin()), 4000),
		"items": []map[string]any{{"description": clip(ch.Title+" — "+place(l.UnitName, l.PropertyName), 300), "quantity": 1, "unit_price": float64(ch.Amount) / 100}}}
	res, err := postJSONTo(ctx, sib, "/api/v1/invoices", body)
	if err != nil {
		return nil, "", err
	}
	ch.InvoiceRef = idOf(res)
	_ = s.store.UpdateCharge(ctx, ch)
	return res, "/invoices/" + idOf(res), nil
}

// ---- Pocket Documents ----

// attachDocument links a document from the archive (nothing is copied).
func (s *Server) attachDocument(ctx context.Context, c *clients, in makeIn) (map[string]any, string, error) {
	if in.Ref == "" {
		return nil, "", &userErr{"pick a document"}
	}
	sib, err := c.conn(ctx, appDocuments, scopeDocumentsRead)
	if err != nil {
		return nil, "", err
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/documents/"+url.PathEscape(in.Ref), nil)
	if err != nil {
		return nil, "", err
	}
	doc, _ := v.(map[string]any)
	if doc == nil {
		return nil, "", &userErr{"that document is not there"}
	}
	return map[string]any{"id": idOf(doc), "title": str(doc, "title", "original_name")}, "/documents/" + idOf(doc), nil
}

// handleDocumentSearch is the document picker: the person's archive.
func (s *Server) handleDocumentSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appDocuments, scopeDocumentsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"documents": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/documents", url.Values{"q": {strings.TrimSpace(r.URL.Query().Get("q"))}, "page_size": {"15"}, "limit": {"15"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"documents": []any{}, "off": "Pocket Documents: " + err.Error()})
		return
	}
	out := []map[string]any{}
	for _, d := range rows(v, "results", "documents") {
		out = append(out, map[string]any{"id": idOf(d), "title": str(d, "title", "original_name"), "created": day10(d["created"]), "url": linkTo(sib, "/documents/"+idOf(d))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": out})
}

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

// ---- Pocket Home ----

// logInHome writes a finished repair into the place's log in Pocket Home
// (when the place is linked to a home there). "" = nothing to say.
func (s *Server) logInHome(ctx context.Context, uid int64, rq *store.Request) string {
	if rq.HomeRecordRef != "" {
		return ""
	}
	p, err := s.store.GetProperty(ctx, uid, rq.PropertyID)
	if err != nil || p.HomeRef == "" {
		return ""
	}
	c := s.clients(ctx, uid)
	sib, err := optional(c.conn(ctx, appHome, scopeHomeWrite))
	if err != nil || sib == nil {
		return ""
	}
	hid, _ := strconv.ParseInt(p.HomeRef, 10, 64)
	body := map[string]any{"home_id": hid, "date": firstNonEmpty(rq.DoneOn, today()), "kind": "repair", "title": clip(joinNonEmpty(" — ", rq.Title, rq.UnitName), 200),
		"notes": joinNonEmpty("\n\n", rq.Details, "From Pocket Rentals: "+s.link("/repairs/"+itoa(rq.ID))), "cost": rq.Cost, "currency": rq.Currency,
		"external_id": fmt.Sprintf("rentals:request:%d", rq.ID), "money": false}
	if pro, _ := strconv.ParseInt(rq.ProRef, 10, 64); pro > 0 {
		body["pro_id"] = pro
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/records", body)
	if err != nil {
		return "Pocket Home: " + err.Error()
	}
	if rec := obj(res, "record"); len(rec) > 0 {
		res = rec
	}
	rq.HomeRecordRef = idOf(res)
	_ = s.store.UpdateRequest(ctx, rq)
	m := &store.Made{Owner: "request", OwnerID: rq.ID, Target: "record", App: "home", Ref: rq.HomeRecordRef, Title: "In the log of " + p.Name, URL: linkTo(sib, "/homes/"+p.HomeRef)}
	_ = s.store.AddMade(ctx, uid, m)
	rid, pid := rq.ID, rq.PropertyID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, RequestID: &rid, PropertyID: &pid, Kind: "made", Title: "Written into Pocket Home's log", App: "home", URL: m.URL})
	return ""
}

// ---- repairs from Pocket Forms ----

// handleFormAnswers lists the answers to the repair form not taken yet.
func (s *Server) handleFormAnswers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	c := s.clients(ctx, uid)
	st := c.status(ctx, appForms)
	if st.Via == "" || !st.Can(scopeFormsRead) {
		writeJSON(w, http.StatusOK, map[string]any{"state": st.State, "detail": st.Detail, "fix": st.Fix, "items": []any{}})
		return
	}
	sib, err := c.conn(ctx, appForms, scopeFormsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "off", "detail": err.Error(), "items": []any{}})
		return
	}
	q := url.Values{"status": {"open"}, "limit": {"60"}}
	if c.st.RepairForm != "" {
		q.Set("form", c.st.RepairForm)
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/responses", q)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "error", "detail": "Pocket Forms did not answer: " + err.Error(), "items": []any{}})
		return
	}
	items := []map[string]any{}
	for _, x := range rows(v, "items", "responses") {
		id := idOf(x)
		item := map[string]any{"id": id, "form": str(x, "form_title"), "name": str(x, "name"), "email": str(x, "email"), "preview": clip(str(x, "preview"), 300),
			"at": str(x, "created_at"), "url": linkTo(sib, "/answers/"+id)}
		if email := str(x, "email"); email != "" {
			if t, err := s.store.TenantByEmail(ctx, uid, email); err == nil {
				item["tenant"] = t.Name
			}
		}
		if rq, err := s.store.RequestBySource(ctx, uid, "forms:answer:"+id); err == nil {
			item["request_id"], item["request_title"] = rq.ID, rq.Title
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok", "items": items, "open_url": st.Open, "form": c.st.RepairForm})
}

// handleTakeAnswer turns one answer into a repair (once: again opens the same one).
func (s *Server) handleTakeAnswer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := chiParam(r, "id")
	ref := "forms:answer:" + id
	if rq, err := s.store.RequestBySource(ctx, uid, ref); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"request": rq})
		return
	}
	var in struct {
		PropertyID int64 `json:"property_id"`
		UnitID     int64 `json:"unit_id"`
	}
	_ = decode(r, &in)
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appForms, scopeFormsRead)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/responses/"+url.PathEscape(id), url.Values{"peek": {"1"}})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Forms: "+err.Error())
		return
	}
	view, _ := v.(map[string]any)
	resp, form := obj(view, "response"), obj(view, "form")
	role := map[string]string{}
	var lines []string
	for _, it := range rows(map[string]any{"x": view["items"]}, "x") {
		t := strings.TrimSpace(str(it, "text"))
		if t == "" || str(it, "type") == "section" {
			continue
		}
		if k := str(it, "role"); k != "" && role[k] == "" {
			role[k] = t
		}
		lines = append(lines, fmt.Sprintf("%s: %s", str(it, "label"), t))
	}
	name := firstNonEmpty(str(resp, "name"), role["name"])
	email := firstNonEmpty(str(resp, "email"), role["email"])
	rin := requestIn{}
	// the tenant tells us where: their lease
	if in.PropertyID == 0 && email != "" {
		if t, err := s.store.TenantByEmail(ctx, uid, email); err == nil {
			if ls, _ := s.store.ListLeases(ctx, uid, store.LeaseFilter{TenantID: t.ID, Status: store.LeaseActive}); len(ls) > 0 {
				in.PropertyID, in.UnitID = ls[0].PropertyID, ls[0].UnitID
				rin.LeaseID = &ls[0].ID
			}
			name = firstNonEmpty(t.Name, name)
		}
	}
	if in.PropertyID == 0 {
		writeErr(w, http.StatusBadRequest, "pick the place: the answer does not say which one, and its email is not a tenant's")
		return
	}
	title := clip(firstNonEmpty(role["subject"], firstLine(role["message"]), str(form, "title")), 160)
	details := strings.Join(lines, "\n")
	if u := linkTo(sib, "/answers/"+id); u != "" {
		details += "\n\nFrom Pocket Forms: " + u
	}
	prio := "normal"
	if strings.Contains(strings.ToLower(details), "urgent") || strings.Contains(strings.ToLower(details), "leak") || strings.Contains(strings.ToLower(details), "no heating") {
		prio = "urgent"
	}
	day := firstNonEmpty(day10(resp["created_at"]), today())
	rin.PropertyID, rin.Title, rin.Details, rin.ReportedBy, rin.ReportedOn, rin.Priority = &in.PropertyID, &title, &details, &name, &day, &prio
	if in.UnitID > 0 {
		rin.UnitID = &in.UnitID
	}
	rq, out, err := s.createRequest(ctx, uid, rin, "", ref)
	if err != nil {
		s.userFail(w, err, "repair")
		return
	}
	_ = s.store.AddMade(ctx, uid, &store.Made{Owner: "request", OwnerID: rq.ID, Target: "answer", App: "forms", Ref: id, Title: "Answer to “" + str(form, "title") + "”", URL: linkTo(sib, "/answers/"+id)})
	out["request"] = rq
	writeJSON(w, http.StatusCreated, out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".\n!?"); i > 0 {
		s = s[:i]
	}
	return clip(s, 120)
}

// ---- keeping up with the other apps ----

// syncAll looks, for everyone, at what is out in the other apps.
func (s *Server) syncAll(ctx context.Context) {
	if !s.syncMu.TryLock() {
		return
	}
	defer s.syncMu.Unlock()
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		s.syncUser(ctx, uid)
	}
}

// syncUser checks up to 20 linked things: a lease signed or declined, an
// invoice paid, a repair's task ticked off in Pocket Tasks.
func (s *Server) syncUser(ctx context.Context, uid int64) {
	list, err := s.store.MadeToCheck(ctx, uid, 20)
	if err != nil || len(list) == 0 {
		return
	}
	c := s.clients(ctx, uid)
	for _, m := range list {
		var (
			a     pocketApp
			scope string
			path  string
		)
		switch m.Target {
		case "signing":
			a, scope, path = appSign, scopeSigningRead, "/api/v1/documents/"+url.PathEscape(m.Ref)
		case "invoice":
			a, scope, path = appInvoicing, scopeInvoicesRead, "/api/v1/invoices/"+url.PathEscape(m.Ref)
		case "task":
			a, scope, path = appTasks, scopeTasksRead, "/api/v1/tasks/"+url.PathEscape(m.Ref)
		default:
			continue
		}
		sib, err := optional(c.conn(ctx, a, scope))
		if err != nil || sib == nil {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		v, err := getJSONFrom(ctx, sib, path, nil)
		if err != nil {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		o, _ := v.(map[string]any)
		if doc := obj(o, "document"); len(doc) > 0 {
			o = doc
		}
		if t := obj(o, "task"); len(t) > 0 {
			o = t
		}
		state := strings.ToLower(str(o, "status"))
		if m.Target == "task" && state == "" {
			if done, _ := o["completed"].(bool); done {
				state = "completed"
			}
		}
		if state == "" || state == m.State {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		_ = s.store.SetMadeState(ctx, uid, m.ID, state, "")
		s.madeChanged(ctx, uid, m, state)
	}
}

// madeChanged writes what changed elsewhere, and acts on it: a lease all
// signed starts; an invoice paid records the rent; a task ticked off
// finishes the repair.
func (s *Server) madeChanged(ctx context.Context, uid int64, m *store.Made, state string) {
	switch m.Target {
	case "signing":
		if m.Owner != "lease" {
			return
		}
		l, err := s.store.GetLease(ctx, uid, m.OwnerID)
		if err != nil {
			return
		}
		_ = s.store.SetSigning(ctx, uid, l.ID, m.Ref, state)
		line := ""
		switch state {
		case "completed":
			line = "Signed by everyone: " + m.Title
			if l.Status == store.LeaseDraft {
				l.Status = store.LeaseActive
				_ = s.store.UpdateLease(ctx, l)
				s.ensureLease(ctx, l)
				line += " — the lease is active now"
				s.publish(ctx, uid, "rentals.lease_started", "lease:"+itoa(l.ID), s.leaseEvent(l))
			}
		case "declined":
			line = "Declined to sign: " + m.Title
		case "voided", "expired":
			line = "Signing " + state + ": " + m.Title
		case "sent", "pending":
			line = "Sent to sign: " + m.Title
		}
		if line != "" {
			lid := l.ID
			_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "made", Title: line, App: "sign", URL: m.URL})
		}
	case "invoice":
		if m.Owner != "lease" || state != "paid" {
			return
		}
		lid := m.OwnerID
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "made", Title: "Invoice paid: " + m.Title + " (record the payment if it is not here yet)", App: "invoicing", URL: m.URL})
	case "task":
		if m.Owner != "request" || (state != "completed" && state != "done") {
			return
		}
		rq, err := s.store.GetRequest(ctx, uid, m.OwnerID)
		if err != nil || !rq.Open() || rq.TaskRef != m.Ref {
			return
		}
		was := *rq
		rq.Status, rq.DoneOn = "done", today()
		if err := s.store.UpdateRequest(ctx, rq); err == nil {
			rid, pid := rq.ID, rq.PropertyID
			_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, RequestID: &rid, PropertyID: &pid, Kind: "made", Title: "Ticked off in Pocket Tasks", App: "tasks", URL: m.URL})
			rq.TaskRef = "" // already ticked there
			s.afterRequestChange(ctx, uid, &was, rq)
		}
	}
}

// ---- the handlers ----

func (s *Server) makeFail(w http.ResponseWriter, a pocketApp, err error) {
	var ue *userErr
	switch {
	case errors.As(err, &ue), isNotConnected(err):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
	}
}

func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in makeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := s.clients(ctx, actorID(ctx))
	m, err := s.make(ctx, c, in)
	if err != nil {
		s.makeFail(w, makeTargets[in.Target], err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleListMade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListMade(ctx, actorID(ctx), r.URL.Query().Get("owner"), int64(queryInt(r, "owner_id")))
	if err != nil {
		s.fail(w, err, "made")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleDeleteMade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteMade(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "made")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSyncNow looks at linked things now (a page asks when it opens).
func (s *Server) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.syncUser(ctx, actorID(ctx))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleApps says per app whether Rentals can reach it, and if not why and
// what to do — the pages grey out what does not work.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

// handleAppOptions lists what to pick from in an app.
func (s *Server) handleAppOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	what := chiParam(r, "what")
	switch what {
	case "accounts", "income-categories", "expense-categories":
		sib, err := c.conn(ctx, appMoney, scopeAccountsRead)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if what == "accounts" {
			accts, err := sib.client.Accounts(ctx)
			if err != nil {
				writeErr(w, http.StatusBadGateway, "Pocket Money: "+err.Error())
				return
			}
			out := []map[string]any{}
			for _, a := range accts {
				out = append(out, map[string]any{"id": a.ID, "name": a.Name, "currency": a.Currency, "everyday": a.Everyday})
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		kind := strings.TrimSuffix(what, "-categories")
		cats, err := sib.client.Categories(ctx, int64(queryInt(r, "account")), kind)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "Pocket Money: "+err.Error())
			return
		}
		out := []map[string]any{}
		for _, ct := range cats {
			out = append(out, map[string]any{"id": ct.ID, "name": firstNonEmpty(ct.FullName, ct.Name)})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	var (
		a     pocketApp
		scope string
		path  string
		key   string
	)
	switch what {
	case "templates":
		a, scope, path, key = appSign, scopeSigningRead, "/api/v1/templates", "templates"
	case "homes":
		a, scope, path, key = appHome, scopeHomeRead, "/api/v1/homes", "homes"
	case "pros":
		a, scope, path, key = appHome, scopeHomeRead, "/api/v1/pros", "pros"
	case "forms":
		a, scope, path, key = appForms, scopeFormsRead, "/api/v1/forms", "forms"
	default:
		writeErr(w, http.StatusNotFound, "unknown list")
		return
	}
	sib, err := c.conn(ctx, a, scope)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, path, url.Values{"q": {r.URL.Query().Get("q")}})
	if err != nil {
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
		return
	}
	out := []map[string]any{}
	for _, x := range rows(v, key) {
		o := map[string]any{"id": idOf(x), "name": str(x, "name", "title")}
		switch key {
		case "templates":
			o["roles"] = x["roles"]
		case "pros":
			o["trade"], o["phone"], o["company"] = str(x, "trade"), str(x, "phone"), str(x, "company")
		case "homes":
			o["address"] = str(x, "address")
		}
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, out)
}
