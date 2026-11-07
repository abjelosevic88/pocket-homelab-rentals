package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- repairs ----

type requestIn struct {
	PropertyID  *int64  `json:"property_id"`
	UnitID      *int64  `json:"unit_id"`
	LeaseID     *int64  `json:"lease_id"`
	Title       *string `json:"title"`
	Details     *string `json:"details"`
	Priority    *string `json:"priority"`
	Status      *string `json:"status"`
	ReportedBy  *string `json:"reported_by"`
	ReportedOn  *string `json:"reported_on"`
	PlannedOn   *string `json:"planned_on"`
	PlannedTime *string `json:"planned_time"`
	DoneOn      *string `json:"done_on"`
	Cost        *int64  `json:"cost"`
	Currency    *string `json:"currency"`
	ProName     *string `json:"pro_name"`
	ProPhone    *string `json:"pro_phone"`
	ProRef      *string `json:"pro_ref"`
	TenantPays  *bool   `json:"tenant_pays"`
	Task        *bool   `json:"task"` // make it a task in Pocket Tasks (default: the setting)
	Ref         string  `json:"ref"`  // the same ref twice gives back the same repair (a retried flow, a form answer)
}

func (in requestIn) apply(r *store.Request, keys map[string]bool) error {
	if in.PropertyID != nil {
		r.PropertyID = *in.PropertyID
	}
	if keys == nil || keys["unit_id"] {
		r.UnitID = nonZero(in.UnitID)
	}
	if keys == nil || keys["lease_id"] {
		r.LeaseID = nonZero(in.LeaseID)
	}
	trimP(&r.Title, in.Title)
	trimP(&r.Details, in.Details)
	trimP(&r.Priority, in.Priority)
	trimP(&r.Status, in.Status)
	trimP(&r.ReportedBy, in.ReportedBy)
	trimP(&r.ReportedOn, in.ReportedOn)
	trimP(&r.PlannedOn, in.PlannedOn)
	trimP(&r.PlannedTime, in.PlannedTime)
	trimP(&r.DoneOn, in.DoneOn)
	trimP(&r.ProName, in.ProName)
	trimP(&r.ProPhone, in.ProPhone)
	trimP(&r.ProRef, in.ProRef)
	if in.Currency != nil {
		r.Currency = cleanCurrency(*in.Currency)
	}
	if in.Cost != nil {
		r.Cost = max(*in.Cost, 0)
	}
	if in.TenantPays != nil {
		r.TenantPays = *in.TenantPays
	}
	r.Title, r.Details = clip(r.Title, 200), clip(r.Details, 20000)
	if r.Status == "done" && r.DoneOn == "" {
		r.DoneOn = today()
	}
	if r.Status == "planned" && r.PlannedOn == "" {
		r.Status = "new"
	}
	switch {
	case r.PropertyID == 0:
		return &userErr{"say which place it is at"}
	case r.Title == "":
		return &userErr{"say what needs fixing"}
	case !oneOf(r.Priority, store.Priorities):
		return &userErr{"a repair is low, normal or urgent"}
	case !oneOf(r.Status, store.RequestStates):
		return &userErr{"a repair is new, planned, in_progress, waiting, done or cancelled"}
	case !validDate(r.ReportedOn) || !validDate(r.PlannedOn) || !validDate(r.DoneOn):
		return &userErr{"days are YYYY-MM-DD"}
	case r.PlannedTime != "" && !reTime.MatchString(r.PlannedTime):
		return &userErr{"the time is HH:MM"}
	case r.Currency == "":
		return &userErr{"the currency is a three-letter code like EUR"}
	}
	return nil
}

func nonZero(p *int64) *int64 {
	if p == nil || *p == 0 {
		return nil
	}
	return p
}

// checkRequestRefs makes sure the place, unit and lease are the person's own and fit together.
func (s *Server) checkRequestRefs(ctx context.Context, uid int64, r *store.Request) error {
	if _, err := s.store.GetProperty(ctx, uid, r.PropertyID); err != nil {
		return &userErr{"that place is not there"}
	}
	if r.LeaseID != nil {
		l, err := s.store.GetLease(ctx, uid, *r.LeaseID)
		if err != nil {
			return &userErr{"that lease is not there"}
		}
		if r.UnitID == nil {
			r.UnitID = &l.UnitID
		}
		if l.PropertyID != r.PropertyID {
			return &userErr{"that lease is for another place"}
		}
	}
	if r.UnitID != nil {
		u, err := s.store.GetUnit(ctx, uid, *r.UnitID)
		if err != nil || u.PropertyID != r.PropertyID {
			return &userErr{"that unit is not at this place"}
		}
		if r.LeaseID == nil {
			for _, l := range s.activeLeases(ctx, uid, u.ID) {
				id := l.ID
				r.LeaseID = &id
				if r.ReportedBy == "" {
					r.ReportedBy = tenantNames(l.Tenants)
				}
				break
			}
		}
	}
	return nil
}

func (s *Server) activeLeases(ctx context.Context, uid, unitID int64) []*store.Lease {
	ls, _ := s.store.ListLeases(ctx, uid, store.LeaseFilter{UnitID: unitID, Status: store.LeaseActive})
	return ls
}

func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.RequestFilter{Open: queryBool(r, "open"), Status: q.Get("status"), PropertyID: int64(queryInt(r, "property")), LeaseID: int64(queryInt(r, "lease")), Limit: queryInt(r, "limit")}
	list, err := s.store.ListRequests(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "repairs")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in requestIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid, app := actorID(ctx), linkApp(ctx)
	ref := clip(strings.TrimSpace(in.Ref), 200)
	if ref != "" && app != "" && !strings.HasPrefix(ref, app+":") {
		ref = app + ":" + ref
	}
	if ref != "" {
		if old, err := s.store.RequestBySource(ctx, uid, ref); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"request": old, "already": true})
			return
		}
	}
	rq, out, err := s.createRequest(ctx, uid, in, app, ref)
	if err != nil {
		s.userFail(w, err, "repair")
		return
	}
	out["request"] = rq
	writeJSON(w, http.StatusCreated, out)
}

// createRequest adds a repair (as a task too, when the person wants that).
func (s *Server) createRequest(ctx context.Context, uid int64, in requestIn, app, ref string) (*store.Request, map[string]any, error) {
	st := s.settingsOf(ctx, uid)
	rq := &store.Request{UserID: uid, Priority: "normal", Status: "new", ReportedOn: today(), Currency: st.Currency, SourceApp: app, SourceRef: ref}
	if in.PropertyID != nil {
		if p, err := s.store.GetProperty(ctx, uid, *in.PropertyID); err == nil {
			rq.Currency = p.Currency
		}
	}
	if err := in.apply(rq, nil); err != nil {
		return nil, nil, err
	}
	if rq.PlannedOn != "" && rq.Status == "new" {
		rq.Status = "planned"
	}
	if err := s.checkRequestRefs(ctx, uid, rq); err != nil {
		return nil, nil, err
	}
	if err := s.store.CreateRequest(ctx, rq); err != nil {
		return nil, nil, err
	}
	rq, _ = s.store.GetRequest(ctx, uid, rq.ID)
	rid, pid := rq.ID, rq.PropertyID
	line := "Repair reported: " + rq.Title
	if rq.ReportedBy != "" {
		line += " (by " + rq.ReportedBy + ")"
	}
	e := &store.Entry{UserID: uid, RequestID: &rid, PropertyID: &pid, LeaseID: rq.LeaseID, Kind: "repair", Title: line, Body: clip(rq.Details, 600), SourceApp: app}
	_ = s.store.AddEntry(ctx, e)
	out := map[string]any{}
	wantTask := st.RepairTasks
	if in.Task != nil {
		wantTask = *in.Task
	}
	if wantTask {
		c := s.clients(ctx, uid)
		if m, err := s.make(ctx, c, makeIn{Owner: "request", OwnerID: rq.ID, Target: "task"}); err == nil {
			out["task"] = m
		} else if !isNotConnected(err) {
			out["task_error"] = plainErr(appTasks, err)
		}
	}
	go s.syncRequestEvent(context.WithoutCancel(ctx), uid, rq.ID)
	s.publish(ctx, uid, "rentals.request_created", "request:"+itoa(rq.ID), s.requestEvent(rq))
	if rq.Priority == "urgent" && st.NotifyURL != "" && app != "" {
		s.push(ctx, uid, st, "Urgent repair: "+rq.Title, place(rq.UnitName, rq.PropertyName)+". "+clip(rq.Details, 300), s.link("/repairs/"+itoa(rq.ID)))
	}
	return rq, out, nil
}

func (s *Server) requestEvent(rq *store.Request) map[string]any {
	return map[string]any{"request_id": rq.ID, "title": rq.Title, "place": place(rq.UnitName, rq.PropertyName), "property_id": rq.PropertyID, "priority": rq.Priority,
		"status": rq.Status, "reported_by": rq.ReportedBy, "planned_on": rq.PlannedOn, "done_on": rq.DoneOn, "cost": rq.Cost, "currency": rq.Currency, "url": s.link("/repairs/" + itoa(rq.ID))}
}

func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rq, err := s.store.GetRequest(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "repair")
		return
	}
	tl, _ := s.store.ListTimeline(ctx, uid, store.TimelineFilter{RequestID: rq.ID})
	made, _ := s.store.ListMade(ctx, uid, "request", rq.ID)
	files, _ := s.store.ListFiles(ctx, uid, "request", rq.ID)
	out := map[string]any{"request": rq, "timeline": tl, "made": made, "files": files}
	if e, err := s.store.ExpenseForRequest(ctx, uid, rq.ID); err == nil {
		out["expense"] = e
	}
	if rq.LeaseID != nil {
		if l, err := s.store.GetLease(ctx, uid, *rq.LeaseID); err == nil {
			out["lease"] = map[string]any{"id": l.ID, "title": place(l.UnitName, l.PropertyName), "tenants": l.Tenants}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rq, err := s.store.GetRequest(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "repair")
		return
	}
	if app := linkApp(ctx); app != "" && rq.SourceApp != app {
		writeErr(w, http.StatusForbidden, "this app may change only the repairs it added")
		return
	}
	var in requestIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	was := *rq
	if err := in.apply(rq, keys); err != nil {
		s.userFail(w, err, "repair")
		return
	}
	if rq.PlannedOn != "" && rq.Status == "new" {
		rq.Status = "planned"
	}
	if err := s.checkRequestRefs(ctx, uid, rq); err != nil {
		s.userFail(w, err, "repair")
		return
	}
	if err := s.store.UpdateRequest(ctx, rq); err != nil {
		s.fail(w, err, "repair")
		return
	}
	out := s.afterRequestChange(ctx, uid, &was, rq)
	rq, _ = s.store.GetRequest(ctx, uid, rq.ID)
	out["request"] = rq
	writeJSON(w, http.StatusOK, out)
}

// afterRequestChange writes what changed, and when a repair is done: its
// cost as an expense, the tenant charged when they pay, the task ticked
// off, the repair in the place's log in Pocket Home.
func (s *Server) afterRequestChange(ctx context.Context, uid int64, was, rq *store.Request) map[string]any {
	out := map[string]any{}
	rid, pid := rq.ID, rq.PropertyID
	line := func(t string) {
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, RequestID: &rid, PropertyID: &pid, LeaseID: rq.LeaseID, Kind: "repair", Title: t})
	}
	if was.Status != rq.Status {
		switch rq.Status {
		case "planned":
			line("Planned for " + niceDate(rq.PlannedOn))
		case "in_progress":
			line("Work started")
		case "waiting":
			line("Waiting (parts, a quote, someone to come)")
		case "cancelled":
			line("Cancelled")
		case "done":
			line("Done on " + niceDate(rq.DoneOn))
		}
	} else if was.PlannedOn != rq.PlannedOn && rq.PlannedOn != "" {
		line("Moved to " + niceDate(rq.PlannedOn))
	}
	if rq.Status == "done" && was.Status != "done" {
		st := s.settingsOf(ctx, uid)
		if rq.Cost > 0 {
			if _, err := s.store.ExpenseForRequest(ctx, uid, rq.ID); err != nil {
				e := &store.Expense{UserID: uid, PropertyID: rq.PropertyID, UnitID: rq.UnitID, RequestID: &rid, Date: rq.DoneOn, Amount: rq.Cost, Currency: rq.Currency,
					Category: "repairs", Payee: rq.ProName, Note: rq.Title}
				if err := s.store.AddExpense(ctx, e); err == nil {
					out["expense"] = e
					if st.MoneyBook {
						out["money"] = s.bookExpense(ctx, uid, e, 0)
					}
				}
			}
			if rq.TenantPays && rq.LeaseID != nil {
				if l, err := s.store.GetLease(ctx, uid, *rq.LeaseID); err == nil {
					c := &store.Charge{UserID: uid, LeaseID: l.ID, Kind: "repair", Title: "Repair: " + rq.Title, DueDate: today(), Amount: rq.Cost, Currency: rq.Currency}
					if _, err := s.store.AddCharge(ctx, c); err == nil {
						lid := l.ID
						_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "charge", Title: "Charged for a repair: " + rq.Title + ", " + fmtMoney(rq.Cost, rq.Currency)})
					}
				}
			}
		}
		if rq.TaskRef != "" {
			go s.completeTask(context.WithoutCancel(ctx), uid, rq.TaskRef)
		}
		if st.HomeLog {
			if res := s.logInHome(ctx, uid, rq); res != "" {
				out["home"] = res
			}
		}
		s.publish(ctx, uid, "rentals.request_done", "request:"+itoa(rq.ID), s.requestEvent(rq))
	}
	if rq.TaskRef != "" && rq.Status != "done" && (was.Title != rq.Title || was.PlannedOn != rq.PlannedOn) {
		go s.updateTask(context.WithoutCancel(ctx), uid, rq)
	}
	go s.syncRequestEvent(context.WithoutCancel(ctx), uid, rq.ID)
	return out
}

func (s *Server) handleDeleteRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetRequest(ctx, uid, id); err != nil {
		s.fail(w, err, "repair")
		return
	}
	s.dropEvent(ctx, uid, "request", id)
	files, _ := s.store.ListFiles(ctx, uid, "request", id)
	if err := s.store.DeleteRequest(ctx, uid, id); err != nil {
		s.fail(w, err, "repair")
		return
	}
	for _, f := range files {
		s.removeFile(uid, f.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- expenses ----

type expenseIn struct {
	PropertyID *int64  `json:"property_id"`
	UnitID     *int64  `json:"unit_id"`
	Date       *string `json:"date"`
	Amount     *int64  `json:"amount"`
	Currency   *string `json:"currency"`
	Category   *string `json:"category"`
	Payee      *string `json:"payee"`
	Note       *string `json:"note"`
	Money      *bool   `json:"money"`
	AccountID  int64   `json:"account_id"`
	MoneyTxRef *string `json:"money_tx_ref"`
	ClientKey  string  `json:"client_key"`
}

func (in expenseIn) apply(e *store.Expense, keys map[string]bool) error {
	if in.PropertyID != nil {
		e.PropertyID = *in.PropertyID
	}
	if keys == nil || keys["unit_id"] {
		e.UnitID = nonZero(in.UnitID)
	}
	trimP(&e.Date, in.Date)
	trimP(&e.Category, in.Category)
	trimP(&e.Payee, in.Payee)
	trimP(&e.Note, in.Note)
	if in.Currency != nil {
		e.Currency = cleanCurrency(*in.Currency)
	}
	if in.Amount != nil {
		e.Amount = *in.Amount
	}
	e.Payee, e.Note = clip(e.Payee, 200), clip(e.Note, 2000)
	switch {
	case e.PropertyID == 0:
		return &userErr{"say which place it was for"}
	case e.Date == "" || !validDate(e.Date):
		return &userErr{"say the day (YYYY-MM-DD)"}
	case e.Amount <= 0:
		return &userErr{"say how much"}
	case e.Currency == "":
		return &userErr{"the currency is a three-letter code like EUR"}
	case !oneOf(e.Category, store.ExpenseKinds):
		return &userErr{"pick what kind of cost it is"}
	}
	return nil
}

func (s *Server) handleListExpenses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	list, err := s.store.ListExpenses(ctx, uid, int64(queryInt(r, "property")), q.Get("from"), q.Get("to"))
	if err != nil {
		s.fail(w, err, "expenses")
		return
	}
	if base := s.siblingLink(ctx, uid, appMoney, ""); base != "" {
		for _, e := range list {
			if e.MoneyTxRef != "" {
				e.MoneyURL = base + "/transactions/" + e.MoneyTxRef
			}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in expenseIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	e := &store.Expense{UserID: uid, Date: today(), Currency: s.settingsOf(ctx, uid).Currency, Category: "other", ClientKey: strings.TrimSpace(in.ClientKey)}
	if in.PropertyID != nil {
		if p, err := s.store.GetProperty(ctx, uid, *in.PropertyID); err == nil {
			e.Currency = p.Currency
		}
	}
	if err := in.apply(e, nil); err != nil {
		s.userFail(w, err, "expense")
		return
	}
	if _, err := s.store.GetProperty(ctx, uid, e.PropertyID); err != nil {
		writeErr(w, http.StatusBadRequest, "that place is not there")
		return
	}
	if e.UnitID != nil {
		if u, err := s.store.GetUnit(ctx, uid, *e.UnitID); err != nil || u.PropertyID != e.PropertyID {
			writeErr(w, http.StatusBadRequest, "that unit is not at this place")
			return
		}
	}
	if in.MoneyTxRef != nil && strings.TrimSpace(*in.MoneyTxRef) != "" {
		e.MoneyTxRef, e.MoneyFound = strings.TrimSpace(*in.MoneyTxRef), true
	}
	if err := s.store.AddExpense(ctx, e); err != nil {
		s.fail(w, err, "expense")
		return
	}
	out := map[string]any{"expense": e}
	book := s.settingsOf(ctx, uid).MoneyBook
	if in.Money != nil {
		book = *in.Money
	}
	if book && !e.MoneyFound && linkApp(ctx) == "" {
		out["money"] = s.bookExpense(ctx, uid, e, in.AccountID)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateExpense(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	e, err := s.store.GetExpense(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "expense")
		return
	}
	var in expenseIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(e, keys); err != nil {
		s.userFail(w, err, "expense")
		return
	}
	if _, err := s.store.GetProperty(ctx, uid, e.PropertyID); err != nil {
		writeErr(w, http.StatusBadRequest, "that place is not there")
		return
	}
	if err := s.store.UpdateExpense(ctx, e); err != nil {
		s.fail(w, err, "expense")
		return
	}
	out := map[string]any{"expense": e}
	if e.MoneyTxRef != "" && !e.MoneyFound {
		out["money"] = s.syncExpenseMoney(ctx, uid, e)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	e, err := s.store.GetExpense(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "expense")
		return
	}
	out := map[string]any{"deleted": true}
	if e.MoneyTxRef != "" && !e.MoneyFound && queryBool(r, "money") {
		out["money"] = s.unbookTx(ctx, uid, e.MoneyTxRef)
	}
	if err := s.store.DeleteExpense(ctx, uid, e.ID); err != nil {
		s.fail(w, err, "expense")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
