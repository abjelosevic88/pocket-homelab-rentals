package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- shared bits ----

var (
	reHex   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reCur   = regexp.MustCompile(`^[A-Z]{3}$`)
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reTime  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// userErr is a reason the person can act on (400, not 502).
type userErr struct{ msg string }

func (e *userErr) Error() string { return e.msg }

// today is the local day for the person (the box's zone).
func today() string { return time.Now().Format("2006-01-02") }

func (s *Server) settingsOf(ctx context.Context, uid int64) store.Settings {
	st, _ := s.store.GetSettings(ctx, uid)
	return st
}

func (s *Server) userFail(w http.ResponseWriter, err error, what string) {
	var ue *userErr
	if errors.As(err, &ue) {
		writeErr(w, http.StatusBadRequest, ue.msg)
		return
	}
	s.fail(w, err, what)
}

// link is a page of this app, for links in other apps and messages.
func (s *Server) link(path string) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + path
}

// fmtMoney is 45000 cents EUR as "450 EUR" (cents only when there are some).
func fmtMoney(cents int64, cur string) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole, frac := cents/100, cents%100
	s := fmt.Sprintf("%d", whole)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if frac != 0 {
		s += fmt.Sprintf(".%02d", frac)
	}
	if neg {
		s = "-" + s
	}
	return s + " " + cur
}

// niceDate is 2026-11-05 as "5 Nov 2026".
func niceDate(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("2 Jan 2006")
}

func monthName(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("January 2006")
}

// daysBetween is b − a in days.
func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// tenantNames is "Ana Marić and Ivan Horvat".
func tenantNames(ts []*store.Tenant) string {
	var n []string
	for _, t := range ts {
		n = append(n, t.Name)
	}
	switch len(n) {
	case 0:
		return ""
	case 1:
		return n[0]
	}
	return strings.Join(n[:len(n)-1], ", ") + " and " + n[len(n)-1]
}

// place is "Flat 2, Linden Street 14" (just the property when the unit is the whole place).
func place(unit, property string) string {
	if unit == "" || strings.EqualFold(unit, property) || strings.EqualFold(unit, "Whole place") {
		return property
	}
	return unit + ", " + property
}

// ---- a person's book: places, units, leases with their money ----

type book struct {
	props    []*store.Property
	units    []*store.Unit
	leases   []*store.Lease
	charges  map[int64][]*store.Charge
	payments map[int64][]*store.Payment
	requests []*store.Request
	propByID map[int64]*store.Property
	unitByID map[int64]*store.Unit
	leaseBy  map[int64]*store.Lease
}

// loadBook reads everything one person has, with each lease's money worked out.
func (s *Server) loadBook(ctx context.Context, uid int64, archived bool) (*book, error) {
	s.ensureUser(ctx, uid, false)
	b := &book{charges: map[int64][]*store.Charge{}, payments: map[int64][]*store.Payment{}, propByID: map[int64]*store.Property{}, unitByID: map[int64]*store.Unit{}, leaseBy: map[int64]*store.Lease{}}
	var err error
	if b.props, err = s.store.ListProperties(ctx, uid, archived); err != nil {
		return nil, err
	}
	if b.units, err = s.store.ListUnits(ctx, uid, 0); err != nil {
		return nil, err
	}
	if b.leases, err = s.store.ListLeases(ctx, uid, store.LeaseFilter{}); err != nil {
		return nil, err
	}
	cs, err := s.store.ListCharges(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		b.charges[c.LeaseID] = append(b.charges[c.LeaseID], c)
	}
	ps, err := s.store.ListPayments(ctx, uid, 0, "", "")
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		b.payments[p.LeaseID] = append(b.payments[p.LeaseID], p)
	}
	if b.requests, err = s.store.ListRequests(ctx, uid, store.RequestFilter{}); err != nil {
		return nil, err
	}
	t := today()
	for _, p := range b.props {
		b.propByID[p.ID] = p
		p.Units = []*store.Unit{}
	}
	for _, u := range b.units {
		b.unitByID[u.ID] = u
	}
	for _, l := range b.leases {
		b.leaseBy[l.ID] = l
		decorateLease(l, b.charges[l.ID], b.payments[l.ID], t)
	}
	// each unit's lease: the active one, else the next draft
	for _, l := range b.leases {
		u := b.unitByID[l.UnitID]
		if u == nil {
			continue
		}
		switch {
		case l.Status == store.LeaseActive:
			if u.Lease == nil || u.Lease.Status != store.LeaseActive || l.StartDate > u.Lease.StartDate {
				u.Lease = l
			}
		case l.Status == store.LeaseDraft && u.Lease == nil:
			u.Lease = l
		}
	}
	for _, u := range b.units {
		p := b.propByID[u.PropertyID]
		if p == nil || u.Archived {
			continue
		}
		p.Units = append(p.Units, u)
		p.UnitCount++
		if u.Lease != nil && u.Lease.Status == store.LeaseActive {
			p.Let++
			p.RentRoll += monthly(u.Lease)
			p.Owed += u.Lease.Overdue
		}
	}
	for _, r := range b.requests {
		if p := b.propByID[r.PropertyID]; p != nil && r.Open() {
			p.OpenRepair++
		}
	}
	return b, nil
}

// monthly is what a lease brings in a month (weekly × 52 / 12…).
func monthly(l *store.Lease) int64 {
	switch l.Frequency {
	case "weekly":
		return l.Rent * 52 / 12
	case "quarterly":
		return l.Rent / 3
	case "yearly":
		return l.Rent / 12
	}
	return l.Rent
}

// decorateLease works out a lease's money, its next rent day and its title.
func decorateLease(l *store.Lease, cs []*store.Charge, ps []*store.Payment, t string) {
	l.ClientKey = ""
	l.Balance, l.Overdue = store.Ledger(cs, ps, t, l.GraceDays)
	if l.Status == store.LeaseActive {
		l.NextDue = store.NextDue(l, t)
	}
	if l.EndDate != "" && l.Status != store.LeaseEnded {
		d := daysBetween(t, l.EndDate)
		l.DaysLeft = &d
	}
	l.Title = place(l.UnitName, l.PropertyName)
	if n := tenantNames(l.Tenants); n != "" {
		l.Title += " — " + n
	}
}

// ---- making rent charges ----

// rentHorizon is how far ahead rent is made (so the next one shows).
func rentHorizon(t string) string { return store.AddDays(t, 35) }

// ensureUser makes the rent, deposits and late fees each lease is due (at
// most once an hour per person unless forced).
func (s *Server) ensureUser(ctx context.Context, uid int64, force bool) {
	s.ensureMu.Lock()
	last, ok := s.ensured[uid]
	if ok && !force && time.Since(last) < time.Hour {
		s.ensureMu.Unlock()
		return
	}
	s.ensured[uid] = time.Now()
	s.ensureMu.Unlock()
	ls, err := s.store.ListLeases(ctx, uid, store.LeaseFilter{})
	if err != nil {
		return
	}
	for _, l := range ls {
		if l.Status == store.LeaseDraft {
			continue
		}
		s.ensureLease(ctx, l)
	}
}

// ensureLease makes one lease's charges up to the horizon (never twice).
func (s *Server) ensureLease(ctx context.Context, l *store.Lease) {
	if l.Status == store.LeaseDraft {
		return
	}
	t := today()
	for _, p := range store.Periods(l, rentHorizon(t)) {
		title := "Rent " + monthName(p.Start)
		switch {
		case l.Frequency == "weekly":
			title = "Rent, week of " + niceDate(p.Start)
		case l.Frequency == "quarterly":
			title = "Rent, quarter from " + niceDate(p.Start)
		case l.Frequency == "yearly":
			title = "Rent, year from " + niceDate(p.Start)
		case p.Part:
			title = fmt.Sprintf("Rent %s – %s", niceDate(p.Start), niceDate(store.AddDays(p.End, -1)))
		}
		_, _ = s.store.AddCharge(ctx, &store.Charge{UserID: l.UserID, LeaseID: l.ID, Kind: "rent", Title: title, Period: p.Start, DueDate: p.Due, Amount: p.Amount, Currency: l.Currency})
	}
	if l.Deposit > 0 && l.DepositState == "due" && l.StartDate != "" {
		_, _ = s.store.AddCharge(ctx, &store.Charge{UserID: l.UserID, LeaseID: l.ID, Kind: "deposit", Title: "Deposit", Period: l.StartDate, DueDate: l.StartDate, Amount: l.Deposit, Currency: l.Currency})
	}
	cs, err := s.store.ListCharges(ctx, l.UserID, l.ID)
	if err != nil {
		return
	}
	ps, _ := s.store.ListPayments(ctx, l.UserID, l.ID, "", "")
	store.Ledger(cs, ps, t, l.GraceDays)
	for _, c := range cs {
		switch {
		case c.Kind == "deposit" && c.State == "paid" && l.DepositState == "due":
			l.DepositState = "held"
			_ = s.store.UpdateLease(ctx, l)
		case c.Kind == "rent" && c.State == "late" && l.LateFee > 0 && l.Status == store.LeaseActive && c.Period != "" && lateSince(c, l.GraceDays, l.CreatedAt, t):
			added, _ := s.store.AddCharge(ctx, &store.Charge{UserID: l.UserID, LeaseID: l.ID, Kind: "late_fee", Title: "Late fee, " + strings.TrimPrefix(c.Title, "Rent "), Period: c.Period,
				DueDate: t, Amount: l.LateFee, Currency: l.Currency})
			if added {
				lid := l.ID
				_ = s.store.AddEntry(ctx, &store.Entry{UserID: l.UserID, LeaseID: &lid, Kind: "charge", Title: "Late fee added: " + fmtMoney(l.LateFee, l.Currency), Body: c.Title + " was not paid in time."})
			}
		}
	}
}

// forgetEnsured makes the next look make charges again (a lease changed).
func (s *Server) forgetEnsured(uid int64) {
	s.ensureMu.Lock()
	delete(s.ensured, uid)
	s.ensureMu.Unlock()
}

// leaseFull is a lease with its money, for its page and for other apps.
func (s *Server) leaseFull(ctx context.Context, uid, id int64) (*store.Lease, []*store.Charge, []*store.Payment, error) {
	l, err := s.store.GetLease(ctx, uid, id)
	if err != nil {
		return nil, nil, nil, err
	}
	cs, err := s.store.ListCharges(ctx, uid, id)
	if err != nil {
		return nil, nil, nil, err
	}
	ps, err := s.store.ListPayments(ctx, uid, id, "", "")
	if err != nil {
		return nil, nil, nil, err
	}
	decorateLease(l, cs, ps, today())
	return l, cs, ps, nil
}

// sortLeases puts the leases that need you first: late, then ending soon, then the rest.
func sortLeases(ls []*store.Lease) {
	sort.SliceStable(ls, func(i, j int) bool {
		a, b := ls[i], ls[j]
		if (a.Overdue > 0) != (b.Overdue > 0) {
			return a.Overdue > 0
		}
		return false
	})
}

// parseTime reads a stored timestamp (zero when it is not one).
func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}

// lateSince says whether a rent went late while the app was keeping the
// lease, in the last month — rent brought in already late (an import, a
// lease added afterwards) never gets a fee made up for it.
func lateSince(c *store.Charge, grace int, created, t string) bool {
	late := store.AddDays(c.DueDate, grace+1)
	return late > firstNonEmpty(day10(created), t) && late >= store.AddDays(t, -31)
}
