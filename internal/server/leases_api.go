package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- tenants ----

type tenantIn struct {
	ID         int64   `json:"id"` // in a lease: an existing tenant
	Name       *string `json:"name"`
	Email      *string `json:"email"`
	Phone      *string `json:"phone"`
	ContactRef *string `json:"contact_ref"`
	Notes      *string `json:"notes"`
}

func (in tenantIn) apply(t *store.Tenant) error {
	trimP(&t.Name, in.Name)
	trimP(&t.Email, in.Email)
	trimP(&t.Phone, in.Phone)
	trimP(&t.ContactRef, in.ContactRef)
	trimP(&t.Notes, in.Notes)
	t.Email = strings.ToLower(t.Email)
	if t.Name == "" {
		t.Name = t.Email
	}
	t.Name = clip(t.Name, 160)
	switch {
	case t.Name == "":
		return &userErr{"say who the tenant is (a name or an email)"}
	case t.Email != "" && !reEmail.MatchString(t.Email):
		return &userErr{"that email address does not look right"}
	}
	return nil
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "tenants")
		return
	}
	ts, err := s.store.ListTenants(ctx, uid)
	if err != nil {
		s.fail(w, err, "tenants")
		return
	}
	byID := map[int64]*store.Tenant{}
	for _, t := range ts {
		t.ClientKey = ""
		byID[t.ID] = t
	}
	for _, l := range b.leases {
		for _, lt := range l.Tenants {
			t := byID[lt.ID]
			if t == nil {
				continue
			}
			t.Leases++
			if l.Status == store.LeaseActive {
				if t.LeaseID == 0 {
					t.Current, t.LeaseID = place(l.UnitName, l.PropertyName), l.ID
				}
				t.Balance += l.Balance
			}
		}
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in tenantIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t, err := s.makeTenant(ctx, actorID(ctx), in)
	if err != nil {
		s.userFail(w, err, "tenant")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// makeTenant adds a tenant, or finds the one with that email already.
func (s *Server) makeTenant(ctx context.Context, uid int64, in tenantIn) (*store.Tenant, error) {
	if in.ID > 0 {
		return s.store.GetTenant(ctx, uid, in.ID)
	}
	t := &store.Tenant{UserID: uid}
	if err := in.apply(t); err != nil {
		return nil, err
	}
	if t.Email != "" {
		if old, err := s.store.TenantByEmail(ctx, uid, t.Email); err == nil {
			return old, nil
		}
	}
	if err := s.store.CreateTenant(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetTenant(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "tenant")
		return
	}
	t.ClientKey = ""
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "tenant")
		return
	}
	leases := []*store.Lease{}
	for _, l := range b.leases {
		for _, lt := range l.Tenants {
			if lt.ID == t.ID {
				leases = append(leases, l)
			}
		}
	}
	contact := ""
	if t.ContactRef != "" {
		contact = s.siblingLink(ctx, uid, appContacts, "/contacts/"+t.ContactRef)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": t, "leases": leases, "contact_url": contact})
}

func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetTenant(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "tenant")
		return
	}
	var in tenantIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(t); err != nil {
		s.userFail(w, err, "tenant")
		return
	}
	if err := s.store.UpdateTenant(ctx, t); err != nil {
		s.fail(w, err, "tenant")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteTenant(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "tenant")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- leases ----

type leaseIn struct {
	UnitID       *int64     `json:"unit_id"`
	Status       *string    `json:"status"`
	StartDate    *string    `json:"start_date"`
	EndDate      *string    `json:"end_date"`
	ChargesFrom  *string    `json:"charges_from"`
	Rent         *int64     `json:"rent"`
	Currency     *string    `json:"currency"`
	Frequency    *string    `json:"frequency"`
	DueDay       *int       `json:"due_day"`
	GraceDays    *int       `json:"grace_days"`
	LateFee      *int64     `json:"late_fee"`
	NoticeDays   *int       `json:"notice_days"`
	Deposit      *int64     `json:"deposit"`
	DepositState *string    `json:"deposit_state"`
	Notes        *string    `json:"notes"`
	Tenants      []tenantIn `json:"tenants"`    // replaces who rents (existing by id, or new)
	TenantIDs    []int64    `json:"tenant_ids"` // the same, by id only
}

func (in leaseIn) apply(l *store.Lease) error {
	if in.UnitID != nil {
		l.UnitID = *in.UnitID
	}
	trimP(&l.Status, in.Status)
	trimP(&l.StartDate, in.StartDate)
	trimP(&l.EndDate, in.EndDate)
	trimP(&l.ChargesFrom, in.ChargesFrom)
	trimP(&l.Frequency, in.Frequency)
	trimP(&l.DepositState, in.DepositState)
	trimP(&l.Notes, in.Notes)
	if in.Currency != nil {
		l.Currency = cleanCurrency(*in.Currency)
	}
	if in.Rent != nil {
		l.Rent = *in.Rent
	}
	if in.DueDay != nil {
		l.DueDay = *in.DueDay
	}
	if in.GraceDays != nil {
		l.GraceDays = *in.GraceDays
	}
	if in.LateFee != nil {
		l.LateFee = max(*in.LateFee, 0)
	}
	if in.NoticeDays != nil {
		l.NoticeDays = max(*in.NoticeDays, 0)
	}
	if in.Deposit != nil {
		l.Deposit = max(*in.Deposit, 0)
	}
	if l.Deposit == 0 {
		l.DepositState = "none"
	} else if l.DepositState == "none" || l.DepositState == "" {
		l.DepositState = "due"
	}
	maxDay := 28
	if l.Frequency == "weekly" {
		maxDay = 7
	}
	switch {
	case l.UnitID == 0:
		return &userErr{"pick the unit this lease is for"}
	case !oneOf(l.Status, store.LeaseStates):
		return &userErr{"a lease is a draft, active or ended"}
	case l.StartDate == "" || !validDate(l.StartDate):
		return &userErr{"the lease starts on a day (YYYY-MM-DD)"}
	case !validDate(l.EndDate) || (l.EndDate != "" && l.EndDate < l.StartDate):
		return &userErr{"the lease ends on a day after it starts (or leave it empty when it runs until notice)"}
	case !validDate(l.ChargesFrom):
		return &userErr{"“charge rent from” is a day (YYYY-MM-DD)"}
	case l.Rent <= 0:
		return &userErr{"say what the rent is"}
	case l.Currency == "":
		return &userErr{"the currency is a three-letter code like EUR"}
	case !oneOf(l.Frequency, store.Frequencies):
		return &userErr{"rent is paid monthly, weekly, quarterly or yearly"}
	case l.DueDay < 1 || l.DueDay > maxDay:
		if l.Frequency == "weekly" {
			return &userErr{"for weekly rent the due day is a weekday, 1 (Monday) to 7 (Sunday)"}
		}
		return &userErr{"rent is due on a day from 1 to 28"}
	case l.GraceDays < 0 || l.GraceDays > 60:
		return &userErr{"rent may be 0 to 60 days late before it counts as late"}
	case !oneOf(l.DepositState, store.DepositStates):
		return &userErr{"the deposit is none, due, held, returned or part_returned"}
	}
	return nil
}

// termsKey changes when anything that decides the rent changes.
func termsKey(l *store.Lease) string {
	return strings.Join([]string{l.Status, l.StartDate, l.EndDate, l.ChargesFrom, itoa(l.Rent), l.Currency, l.Frequency, itoa(int64(l.DueDay))}, "|")
}

func (s *Server) handleListLeases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, err := s.loadBook(ctx, actorID(ctx), true)
	if err != nil {
		s.fail(w, err, "leases")
		return
	}
	q := r.URL.Query()
	status, prop := q.Get("status"), int64(queryInt(r, "property"))
	out := []*store.Lease{}
	for _, l := range b.leases {
		if (status == "" || l.Status == status) && (prop == 0 || l.PropertyID == prop) {
			out = append(out, l)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// setTenants saves who rents under a lease from the input.
func (s *Server) setTenants(ctx context.Context, uid, leaseID int64, in leaseIn) error {
	var ids []int64
	if in.Tenants != nil {
		for _, ti := range in.Tenants {
			t, err := s.makeTenant(ctx, uid, ti)
			if err != nil {
				return err
			}
			ids = append(ids, t.ID)
		}
	} else if in.TenantIDs != nil {
		ids = in.TenantIDs
	} else {
		return nil
	}
	return s.store.SetLeaseTenants(ctx, uid, leaseID, ids)
}

// clash says whether another active lease already covers the unit.
func (s *Server) clash(ctx context.Context, l *store.Lease) string {
	if l.Status != store.LeaseActive {
		return ""
	}
	ls, _ := s.store.ListLeases(ctx, l.UserID, store.LeaseFilter{UnitID: l.UnitID, Status: store.LeaseActive})
	for _, o := range ls {
		if o.ID == l.ID {
			continue
		}
		if (o.EndDate == "" || o.EndDate >= l.StartDate) && (l.EndDate == "" || l.EndDate >= o.StartDate) {
			who := tenantNames(o.Tenants)
			if who == "" {
				who = "someone"
			}
			return place(o.UnitName, o.PropertyName) + " is let to " + who + " already: end that lease first (or save this one as a draft)"
		}
	}
	return ""
}

func (s *Server) handleCreateLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in leaseIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	l, err := s.createLease(ctx, uid, in, linkApp(ctx))
	if err != nil {
		s.userFail(w, err, "lease")
		return
	}
	s.respondLease(w, r, l.ID, http.StatusCreated)
}

func (s *Server) createLease(ctx context.Context, uid int64, in leaseIn, app string) (*store.Lease, error) {
	st := s.settingsOf(ctx, uid)
	l := &store.Lease{UserID: uid, Status: store.LeaseActive, Currency: st.Currency, Frequency: "monthly", DueDay: st.DueDay, GraceDays: st.GraceDays, NoticeDays: st.NoticeDays,
		DepositState: "none", SourceApp: app}
	if in.UnitID != nil {
		if u, err := s.store.GetUnit(ctx, uid, *in.UnitID); err == nil {
			l.Rent, l.Deposit = u.Rent, u.Deposit
			if p, err := s.store.GetProperty(ctx, uid, u.PropertyID); err == nil && in.Currency == nil {
				l.Currency = p.Currency
			}
		} else {
			return nil, &userErr{"that unit is not there"}
		}
	}
	if err := in.apply(l); err != nil {
		return nil, err
	}
	if msg := s.clash(ctx, l); msg != "" {
		return nil, &userErr{msg}
	}
	if err := s.store.CreateLease(ctx, l); err != nil {
		return nil, err
	}
	if err := s.setTenants(ctx, uid, l.ID, in); err != nil {
		_ = s.store.DeleteLease(ctx, uid, l.ID)
		return nil, err
	}
	l, _ = s.store.GetLease(ctx, uid, l.ID)
	lid := l.ID
	line := "Lease made: " + fmtMoney(l.Rent, l.Currency) + " " + l.Frequency + " from " + niceDate(l.StartDate)
	if l.EndDate != "" {
		line += " to " + niceDate(l.EndDate)
	}
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: line, SourceApp: app})
	s.ensureLease(ctx, l)
	s.forgetEnsured(uid)
	if l.Status == store.LeaseActive {
		s.publish(ctx, uid, "rentals.lease_started", "lease:"+itoa(l.ID), s.leaseEvent(l))
	}
	go s.syncLeaseEvent(context.WithoutCancel(ctx), uid, l.ID)
	return l, nil
}

// leaseEvent is a lease on the bus.
func (s *Server) leaseEvent(l *store.Lease) map[string]any {
	var names, emails []string
	for _, t := range l.Tenants {
		names = append(names, t.Name)
		if t.Email != "" {
			emails = append(emails, t.Email)
		}
	}
	return map[string]any{"lease_id": l.ID, "place": place(l.UnitName, l.PropertyName), "property_id": l.PropertyID, "tenants": names, "emails": emails,
		"start": l.StartDate, "end": l.EndDate, "rent": l.Rent, "currency": l.Currency, "frequency": l.Frequency, "url": s.link("/leases/" + itoa(l.ID))}
}

// respondLease answers with a lease as its page shows it.
func (s *Server) respondLease(w http.ResponseWriter, r *http.Request, id int64, status int) {
	ctx := r.Context()
	uid := actorID(ctx)
	s.ensureUser(ctx, uid, false)
	l, cs, ps, err := s.leaseFull(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	moneyBase := s.siblingLink(ctx, uid, appMoney, "")
	for _, p := range ps {
		if p.MoneyTxRef != "" && moneyBase != "" {
			p.MoneyURL = moneyBase + "/transactions/" + p.MoneyTxRef
		}
	}
	tl, _ := s.store.ListTimeline(ctx, uid, store.TimelineFilter{LeaseID: id, Limit: 200})
	made, _ := s.store.ListMade(ctx, uid, "lease", id)
	reqs, _ := s.store.ListRequests(ctx, uid, store.RequestFilter{LeaseID: id})
	insp, _ := s.store.ListInspections(ctx, uid, 0, id)
	files, _ := s.store.ListFiles(ctx, uid, "lease", id)
	signURL := ""
	if l.SigningRef != "" {
		signURL = s.siblingLink(ctx, uid, appSign, "/documents/"+l.SigningRef)
	}
	writeJSON(w, status, map[string]any{"lease": l, "charges": cs, "payments": ps, "timeline": tl, "made": made, "requests": reqs, "inspections": insp, "files": files, "signing_url": signURL})
}

func (s *Server) handleGetLease(w http.ResponseWriter, r *http.Request) {
	s.respondLease(w, r, idParam(r, "id"), http.StatusOK)
}

func (s *Server) handleUpdateLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetLease(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	if app := linkApp(ctx); app != "" && l.SourceApp != app {
		writeErr(w, http.StatusForbidden, "this app may change only the leases it added")
		return
	}
	var in leaseIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	before, wasStatus := termsKey(l), l.Status
	if err := in.apply(l); err != nil {
		s.userFail(w, err, "lease")
		return
	}
	if msg := s.clash(ctx, l); msg != "" {
		writeErr(w, http.StatusConflict, msg)
		return
	}
	if err := s.store.UpdateLease(ctx, l); err != nil {
		s.fail(w, err, "lease")
		return
	}
	if err := s.setTenants(ctx, uid, l.ID, in); err != nil {
		s.userFail(w, err, "lease")
		return
	}
	if termsKey(l) != before {
		_ = s.store.DropFutureRent(ctx, uid, l.ID, today())
		s.ensureLease(ctx, l)
		s.forgetEnsured(uid)
	}
	if wasStatus == store.LeaseDraft && l.Status == store.LeaseActive {
		lid := l.ID
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: "Lease started"})
		full, _ := s.store.GetLease(ctx, uid, l.ID)
		s.publish(ctx, uid, "rentals.lease_started", "lease:"+itoa(l.ID), s.leaseEvent(full))
	}
	go s.syncLeaseEvent(context.WithoutCancel(ctx), uid, l.ID)
	s.respondLease(w, r, l.ID, http.StatusOK)
}

func (s *Server) handleDeleteLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetLease(ctx, uid, id); err != nil {
		s.fail(w, err, "lease")
		return
	}
	s.dropEvent(ctx, uid, "lease", id)
	files, _ := s.store.ListFiles(ctx, uid, "lease", id)
	if err := s.store.DeleteLease(ctx, uid, id); err != nil {
		s.fail(w, err, "lease")
		return
	}
	for _, f := range files {
		s.removeFile(uid, f.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEndLease ends a lease on a day: no rent after it, the deposit
// settled when asked, the unit free again.
func (s *Server) handleEndLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetLease(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	var in struct {
		On     string `json:"on"`
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.On == "" {
		in.On = today()
	}
	if !validDate(in.On) || in.On < l.StartDate {
		writeErr(w, http.StatusBadRequest, "the lease ends on a day after it started")
		return
	}
	l.Status, l.EndedOn, l.EndReason = store.LeaseEnded, in.On, clip(strings.TrimSpace(in.Reason), 300)
	if err := s.store.UpdateLease(ctx, l); err != nil {
		s.fail(w, err, "lease")
		return
	}
	_ = s.store.DropRentAfter(ctx, uid, l.ID, in.On)
	// the last part period is charged by the day up to the end
	s.ensureLease(ctx, l)
	lid := l.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: "Lease ended on " + niceDate(in.On), Body: l.EndReason})
	s.publish(ctx, uid, "rentals.lease_ended", "lease:"+itoa(l.ID), s.leaseEvent(l))
	s.dropEvent(ctx, uid, "lease", l.ID)
	s.respondLease(w, r, l.ID, http.StatusOK)
}

// handleRenewLease runs a lease on: a new end (or none) and, when it
// changes, a new rent for the periods not made yet.
func (s *Server) handleRenewLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetLease(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	var in struct {
		EndDate string `json:"end_date"`
		Rent    int64  `json:"rent"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !validDate(in.EndDate) || (in.EndDate != "" && in.EndDate <= today()) {
		writeErr(w, http.StatusBadRequest, "the new end is a day in the future (or empty: until notice)")
		return
	}
	old := l.Rent
	l.EndDate, l.Status, l.EndedOn = in.EndDate, store.LeaseActive, ""
	if in.Rent > 0 {
		l.Rent = in.Rent
	}
	if err := s.store.UpdateLease(ctx, l); err != nil {
		s.fail(w, err, "lease")
		return
	}
	_ = s.store.DropFutureRent(ctx, uid, l.ID, today())
	s.ensureLease(ctx, l)
	line := "Lease renewed until notice"
	if in.EndDate != "" {
		line = "Lease renewed until " + niceDate(in.EndDate)
	}
	if l.Rent != old {
		line += "; rent " + fmtMoney(old, l.Currency) + " → " + fmtMoney(l.Rent, l.Currency)
	}
	lid := l.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: line})
	go s.syncLeaseEvent(context.WithoutCancel(ctx), uid, l.ID)
	s.respondLease(w, r, l.ID, http.StatusOK)
}

// handleDeposit settles the deposit: given back in full or in part (what
// was kept and why), optionally booked in Pocket Money.
func (s *Server) handleDeposit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetLease(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	var in struct {
		State string `json:"state"` // held | returned | part_returned
		Back  int64  `json:"back"`
		Note  string `json:"note"`
		Date  string `json:"date"`
		Money bool   `json:"money"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !oneOf(in.State, []string{"held", "returned", "part_returned"}) {
		writeErr(w, http.StatusBadRequest, "the deposit is held, returned or part_returned")
		return
	}
	if in.State == "returned" && in.Back == 0 {
		in.Back = l.Deposit
	}
	if in.Back < 0 || in.Back > l.Deposit {
		writeErr(w, http.StatusBadRequest, "you can give back at most the whole deposit")
		return
	}
	if in.Date == "" {
		in.Date = today()
	}
	l.DepositState, l.DepositBack, l.DepositNote = in.State, in.Back, clip(strings.TrimSpace(in.Note), 2000)
	if in.State == "held" {
		l.DepositBack = 0
	}
	if err := s.store.UpdateLease(ctx, l); err != nil {
		s.fail(w, err, "lease")
		return
	}
	lid := l.ID
	out := map[string]any{}
	if in.State != "held" {
		line := "Deposit given back: " + fmtMoney(in.Back, l.Currency)
		if kept := l.Deposit - in.Back; kept > 0 {
			line += ", kept " + fmtMoney(kept, l.Currency)
		}
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "payment", Title: line, Body: l.DepositNote})
		if in.Money && in.Back > 0 {
			out["money"] = s.bookDepositBack(ctx, uid, l, in.Back, in.Date)
		}
	}
	out["ok"] = true
	writeJSON(w, http.StatusOK, out)
}

// ---- charges ----

type chargeIn struct {
	Kind    *string `json:"kind"`
	Title   *string `json:"title"`
	DueDate *string `json:"due_date"`
	Amount  *int64  `json:"amount"`
	Waived  *bool   `json:"waived"`
	Note    *string `json:"note"`
}

func (in chargeIn) apply(c *store.Charge) error {
	trimP(&c.Kind, in.Kind)
	trimP(&c.Title, in.Title)
	trimP(&c.DueDate, in.DueDate)
	trimP(&c.Note, in.Note)
	if in.Amount != nil {
		c.Amount = *in.Amount
	}
	if in.Waived != nil {
		c.Waived = *in.Waived
	}
	c.Title = clip(c.Title, 200)
	switch {
	case !oneOf(c.Kind, store.ChargeKinds):
		return &userErr{"a charge is rent, deposit, utilities, late_fee, repair or other"}
	case c.DueDate == "" || !validDate(c.DueDate):
		return &userErr{"say the day it is due"}
	case c.Amount <= 0:
		return &userErr{"say how much"}
	}
	if c.Title == "" {
		c.Title = map[string]string{"rent": "Rent", "deposit": "Deposit", "utilities": "Bills", "late_fee": "Late fee", "repair": "Repair", "other": "Charge"}[c.Kind]
	}
	return nil
}

func (s *Server) handleAddCharge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetLease(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	var in chargeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := &store.Charge{UserID: uid, LeaseID: l.ID, Kind: "other", Currency: l.Currency, DueDate: today()}
	if err := in.apply(c); err != nil {
		s.userFail(w, err, "charge")
		return
	}
	if _, err := s.store.AddCharge(ctx, c); err != nil {
		s.fail(w, err, "charge")
		return
	}
	lid := l.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "charge", Title: "Charge added: " + c.Title + ", " + fmtMoney(c.Amount, c.Currency), Body: c.Note})
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCharge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	c, err := s.store.GetCharge(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "charge")
		return
	}
	var in chargeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Kind = nil // a charge stays what it is
	wasWaived := c.Waived
	if err := in.apply(c); err != nil {
		s.userFail(w, err, "charge")
		return
	}
	if err := s.store.UpdateCharge(ctx, c); err != nil {
		s.fail(w, err, "charge")
		return
	}
	if c.Waived != wasWaived {
		lid := c.LeaseID
		t := "Waived: " + c.Title
		if !c.Waived {
			t = "Charged again: " + c.Title
		}
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "charge", Title: t})
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCharge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	c, err := s.store.GetCharge(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "charge")
		return
	}
	if c.Kind == "rent" && c.Period != "" {
		writeErr(w, http.StatusBadRequest, "rent comes back when deleted: waive it instead")
		return
	}
	if err := s.store.DeleteCharge(ctx, uid, c.ID); err != nil {
		s.fail(w, err, "charge")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- payments ----

type paymentIn struct {
	Date       *string `json:"date"`
	Amount     *int64  `json:"amount"`
	Method     *string `json:"method"`
	Note       *string `json:"note"`
	Money      *bool   `json:"money"`        // book it in Pocket Money (default: the setting)
	AccountID  int64   `json:"account_id"`   // the account to book it on
	MoneyTxRef *string `json:"money_tx_ref"` // or: it is this transaction already in Pocket Money
	ClientKey  string  `json:"client_key"`
}

func (in paymentIn) apply(p *store.Payment) error {
	trimP(&p.Date, in.Date)
	trimP(&p.Method, in.Method)
	trimP(&p.Note, in.Note)
	if in.Amount != nil {
		p.Amount = *in.Amount
	}
	p.Note = clip(p.Note, 1000)
	switch {
	case p.Date == "" || !validDate(p.Date):
		return &userErr{"say the day it came in"}
	case p.Amount <= 0:
		return &userErr{"say how much came in"}
	case !oneOf(p.Method, store.PayMethods):
		return &userErr{"it came by bank, cash, card or other"}
	}
	return nil
}

func (s *Server) handleAddPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetLease(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	var in paymentIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p, out, err := s.addPayment(ctx, uid, l, in, linkApp(ctx))
	if err != nil {
		s.userFail(w, err, "payment")
		return
	}
	out["payment"] = p
	writeJSON(w, http.StatusCreated, out)
}

// addPayment records money in for a lease (and books it in Pocket Money
// when asked); the answer says what happened there.
func (s *Server) addPayment(ctx context.Context, uid int64, l *store.Lease, in paymentIn, app string) (*store.Payment, map[string]any, error) {
	p := &store.Payment{UserID: uid, LeaseID: l.ID, Date: today(), Currency: l.Currency, Method: "bank", SourceApp: app, ClientKey: strings.TrimSpace(in.ClientKey)}
	if err := in.apply(p); err != nil {
		return nil, nil, err
	}
	if in.MoneyTxRef != nil && strings.TrimSpace(*in.MoneyTxRef) != "" {
		p.MoneyTxRef, p.MoneyFound = strings.TrimSpace(*in.MoneyTxRef), true
		if old, err := s.store.PaymentByMoney(ctx, uid, p.MoneyTxRef); err == nil {
			return old, map[string]any{"already": true}, nil
		}
	}
	if err := s.store.AddPayment(ctx, p); err != nil {
		return nil, nil, err
	}
	lid := l.ID
	who := tenantNames(l.Tenants)
	title := "Paid " + fmtMoney(p.Amount, p.Currency)
	if who != "" {
		title += " by " + who
	}
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "payment", Title: title, Body: p.Note, At: p.Date + "T12:00:00Z", SourceApp: app})
	out := map[string]any{}
	st := s.settingsOf(ctx, uid)
	book := st.MoneyBook
	if in.Money != nil {
		book = *in.Money
	}
	if !p.MoneyFound && book && app == "" {
		out["money"] = s.bookPayment(ctx, uid, p, l, in.AccountID)
	}
	full, _, _, err := s.leaseFull(ctx, uid, l.ID)
	if err == nil {
		s.ensureLease(ctx, full)
		out["balance"] = full.Balance
	}
	s.publish(ctx, uid, "rentals.payment_recorded", "payment:"+itoa(p.ID), map[string]any{"lease_id": l.ID, "place": place(l.UnitName, l.PropertyName), "tenants": who,
		"date": p.Date, "amount": p.Amount, "currency": p.Currency, "url": s.link("/leases/" + itoa(l.ID))})
	return p, out, nil
}

func (s *Server) handleUpdatePayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPayment(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "payment")
		return
	}
	var in paymentIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(p); err != nil {
		s.userFail(w, err, "payment")
		return
	}
	if err := s.store.UpdatePayment(ctx, p); err != nil {
		s.fail(w, err, "payment")
		return
	}
	out := map[string]any{"payment": p}
	if p.MoneyTxRef != "" && !p.MoneyFound {
		if l, err := s.store.GetLease(ctx, uid, p.LeaseID); err == nil {
			out["money"] = s.syncPaymentMoney(ctx, uid, p, l)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeletePayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPayment(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "payment")
		return
	}
	out := map[string]any{"deleted": true}
	if p.MoneyTxRef != "" && !p.MoneyFound && queryBool(r, "money") {
		out["money"] = s.unbookTx(ctx, uid, p.MoneyTxRef)
	}
	if err := s.store.DeletePayment(ctx, uid, p.ID); err != nil {
		s.fail(w, err, "payment")
		return
	}
	lid := p.LeaseID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "payment", Title: "Payment removed: " + fmtMoney(p.Amount, p.Currency) + " of " + niceDate(p.Date)})
	writeJSON(w, http.StatusOK, out)
}

// ---- timeline ----

type entryIn struct {
	LeaseID    int64  `json:"lease_id"`
	PropertyID int64  `json:"property_id"`
	RequestID  int64  `json:"request_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	At         string `json:"at"`
}

func (s *Server) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in entryIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	e := &store.Entry{UserID: uid, Kind: firstNonEmpty(in.Kind, "note"), Title: clip(strings.TrimSpace(in.Title), 300), Body: clip(strings.TrimSpace(in.Body), 20000), SourceApp: linkApp(ctx)}
	if !oneOf(e.Kind, []string{"note", "call", "email", "visit"}) {
		writeErr(w, http.StatusBadRequest, "a line is a note, a call, an email or a visit")
		return
	}
	if e.Title == "" && e.Body == "" {
		writeErr(w, http.StatusBadRequest, "write something first")
		return
	}
	if in.At != "" {
		if len(in.At) == 10 && validDate(in.At) {
			e.At = in.At + "T12:00:00Z"
		} else {
			e.At = in.At
		}
	}
	switch {
	case in.LeaseID > 0:
		if _, err := s.store.GetLease(ctx, uid, in.LeaseID); err != nil {
			s.fail(w, err, "lease")
			return
		}
		e.LeaseID = &in.LeaseID
	case in.RequestID > 0:
		rq, err := s.store.GetRequest(ctx, uid, in.RequestID)
		if err != nil {
			s.fail(w, err, "repair")
			return
		}
		e.RequestID, e.PropertyID = &in.RequestID, &rq.PropertyID
	case in.PropertyID > 0:
		if _, err := s.store.GetProperty(ctx, uid, in.PropertyID); err != nil {
			s.fail(w, err, "place")
			return
		}
		e.PropertyID = &in.PropertyID
	default:
		writeErr(w, http.StatusBadRequest, "say which lease, place or repair it is about")
		return
	}
	if err := s.store.AddEntry(ctx, e); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteEntry(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := store.TimelineFilter{LeaseID: int64(queryInt(r, "lease")), PropertyID: int64(queryInt(r, "property")), RequestID: int64(queryInt(r, "request")), Limit: queryInt(r, "limit")}
	list, err := s.store.ListTimeline(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
