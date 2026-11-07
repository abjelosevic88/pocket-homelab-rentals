package server

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// money is amounts by currency (never added across currencies).
type money map[string]int64

func (m money) add(cur string, v int64) {
	if v != 0 {
		m[cur] += v
	}
}

// dueItem is one rent (or charge) on the Today page and in the rent list.
type dueItem struct {
	ChargeID int64           `json:"charge_id"`
	LeaseID  int64           `json:"lease_id"`
	Lease    string          `json:"lease"`
	Place    string          `json:"place"`
	Tenants  []*store.Tenant `json:"tenants"`
	Title    string          `json:"title"`
	Kind     string          `json:"kind"`
	DueDate  string          `json:"due_date"`
	Amount   int64           `json:"amount"`
	Paid     int64           `json:"paid"`
	Currency string          `json:"currency"`
	State    string          `json:"state"`
	PaidOn   string          `json:"paid_on,omitempty"`
	Days     int             `json:"days"` // days late (> 0) or until due (< 0)
}

func toDue(l *store.Lease, c *store.Charge, t string) dueItem {
	return dueItem{ChargeID: c.ID, LeaseID: l.ID, Lease: l.Title, Place: place(l.UnitName, l.PropertyName), Tenants: l.Tenants, Title: c.Title, Kind: c.Kind,
		DueDate: c.DueDate, Amount: c.Amount, Paid: c.Paid, Currency: c.Currency, State: c.State, PaidOn: c.PaidOn, Days: daysBetween(c.DueDate, t)}
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.loadBook(ctx, uid, false)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	st := s.settingsOf(ctx, uid)
	t := today()
	out := map[string]any{"properties": len(b.props), "units": 0, "let": 0, "vacant": 0, "late": 0, "due": 0, "repairs": 0, "urgent": 0, "ending": 0, "drafts": 0}
	roll, owed := money{}, money{}
	units, let := 0, 0
	for _, p := range b.props {
		units += p.UnitCount
		let += p.Let
		roll.add(p.Currency, p.RentRoll)
	}
	late, due, ending, drafts := 0, 0, 0, 0
	horizon := store.AddDays(t, max(st.RemindDays, 0))
	for _, l := range b.leases {
		switch l.Status {
		case store.LeaseDraft:
			drafts++
			continue
		case store.LeaseEnded:
			if l.Balance > 0 {
				owed.add(l.Currency, l.Balance)
			}
			continue
		}
		if l.Overdue > 0 {
			late++
		}
		if l.Balance > 0 {
			owed.add(l.Currency, l.Balance)
		}
		for _, c := range b.charges[l.ID] {
			if (c.State == "due" || c.State == "part" || c.State == "upcoming") && c.DueDate <= horizon {
				due++
			}
		}
		if l.DaysLeft != nil && *l.DaysLeft <= st.EndingDays {
			ending++
		}
	}
	repairs, urgent := 0, 0
	for _, rq := range b.requests {
		if rq.Open() {
			repairs++
			if rq.Priority == "urgent" {
				urgent++
			}
		}
	}
	out["units"], out["let"], out["vacant"] = units, let, units-let
	out["late"], out["due"], out["ending"], out["drafts"], out["repairs"], out["urgent"] = late, due, ending, drafts, repairs, urgent
	out["rent_roll"], out["owed"], out["currency"] = roll, owed, st.Currency
	writeJSON(w, http.StatusOK, out)
}

// handleToday is what needs you: rent late and due, leases ending, repairs,
// inspections coming, units standing empty.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.loadBook(ctx, uid, false)
	if err != nil {
		s.fail(w, err, "today")
		return
	}
	st := s.settingsOf(ctx, uid)
	t := today()
	horizon := store.AddDays(t, max(st.RemindDays, 0)+7)
	late, due := []dueItem{}, []dueItem{}
	ending := []*store.Lease{}
	expected, received := money{}, money{}
	month := t[:7]
	for _, l := range b.leases {
		if l.Status == store.LeaseDraft {
			continue
		}
		for _, c := range b.charges[l.ID] {
			if strings.HasPrefix(c.DueDate, month) && !c.Waived {
				expected.add(c.Currency, c.Amount)
			}
			if l.Status != store.LeaseActive && c.State != "late" {
				continue
			}
			switch {
			case c.State == "late":
				late = append(late, toDue(l, c, t))
			case (c.State == "due" || c.State == "part" || c.State == "upcoming") && c.DueDate <= horizon:
				due = append(due, toDue(l, c, t))
			}
		}
		for _, p := range b.payments[l.ID] {
			if strings.HasPrefix(p.Date, month) {
				received.add(p.Currency, p.Amount)
			}
		}
		if l.Status == store.LeaseActive && l.DaysLeft != nil && *l.DaysLeft <= st.EndingDays {
			ending = append(ending, l)
		}
	}
	sort.Slice(late, func(i, j int) bool { return late[i].DueDate < late[j].DueDate })
	sort.Slice(due, func(i, j int) bool { return due[i].DueDate < due[j].DueDate })
	sort.Slice(ending, func(i, j int) bool { return *ending[i].DaysLeft < *ending[j].DaysLeft })
	repairs := []*store.Request{}
	for _, rq := range b.requests {
		if rq.Open() && len(repairs) < 8 {
			repairs = append(repairs, rq)
		}
	}
	insp, _ := s.store.ListInspections(ctx, uid, 0, 0)
	soon := []*store.Inspection{}
	for _, i := range insp {
		if i.Status == "planned" && i.Date <= store.AddDays(t, 30) {
			soon = append(soon, i)
		}
	}
	vacant := []*store.Unit{}
	drafts := []*store.Lease{}
	for _, u := range b.units {
		if p := b.propByID[u.PropertyID]; p == nil || u.Archived {
			continue
		}
		if u.Lease == nil || u.Lease.Status != store.LeaseActive {
			vacant = append(vacant, u)
		}
	}
	for _, l := range b.leases {
		if l.Status == store.LeaseDraft {
			drafts = append(drafts, l)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": t, "late": late, "due": due, "ending": ending, "repairs": repairs, "inspections": soon, "vacant": vacant, "drafts": drafts,
		"month": map[string]any{"month": month, "expected": expected, "received": received}, "empty": len(b.props) == 0})
}

// handleRent is one month of rent: every charge due in it, and how far it is paid.
func (s *Server) handleRent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	month := r.URL.Query().Get("month")
	if _, err := time.Parse("2006-01", month); err != nil {
		month = today()[:7]
	}
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "rent")
		return
	}
	t := today()
	items := []dueItem{}
	expected, received, outstanding := money{}, money{}, money{}
	for _, l := range b.leases {
		for _, c := range b.charges[l.ID] {
			if !strings.HasPrefix(c.DueDate, month) {
				continue
			}
			items = append(items, toDue(l, c, t))
			if !c.Waived {
				expected.add(c.Currency, c.Amount)
				outstanding.add(c.Currency, c.Amount-c.Paid)
			}
		}
		for _, p := range b.payments[l.ID] {
			if strings.HasPrefix(p.Date, month) {
				received.add(p.Currency, p.Amount)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].DueDate != items[j].DueDate {
			return items[i].DueDate < items[j].DueDate
		}
		return items[i].Place < items[j].Place
	})
	writeJSON(w, http.StatusOK, map[string]any{"month": month, "items": items, "expected": expected, "received": received, "outstanding": outstanding})
}

// ---- reports ----

type propertyYear struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	Currency   string           `json:"currency"`
	Income     int64            `json:"income"`
	Expenses   int64            `json:"expenses"`
	Net        int64            `json:"net"`
	ByCategory map[string]int64 `json:"by_category"`
	Months     [12][2]int64     `json:"months"`    // income, expenses
	Occupancy  int              `json:"occupancy"` // % of unit-days let this year (so far)
	Owed       int64            `json:"owed"`
	Value      int64            `json:"value"`
	Yield      float64          `json:"yield"` // net a year / value, %
}

func (s *Server) reportYear(r *http.Request, year int) ([]*propertyYear, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		return nil, err
	}
	y := strconv.Itoa(year)
	from, to := y+"-01-01", y+"-12-31"
	exps, err := s.store.ListExpenses(ctx, uid, 0, from, to)
	if err != nil {
		return nil, err
	}
	out := []*propertyYear{}
	by := map[int64]*propertyYear{}
	for _, p := range b.props {
		py := &propertyYear{ID: p.ID, Name: p.Name, Currency: p.Currency, ByCategory: map[string]int64{}, Value: p.Value}
		by[p.ID] = py
		out = append(out, py)
	}
	month := func(d string) int { m, _ := strconv.Atoi(d[5:7]); return m - 1 }
	for _, l := range b.leases {
		py := by[l.PropertyID]
		if py == nil {
			continue
		}
		for _, p := range b.payments[l.ID] {
			if p.Date >= from && p.Date <= to && p.Currency == py.Currency {
				py.Income += p.Amount
				py.Months[month(p.Date)][0] += p.Amount
			}
		}
		if l.Balance > 0 && l.Currency == py.Currency {
			py.Owed += l.Balance
		}
	}
	for _, e := range exps {
		py := by[e.PropertyID]
		if py == nil || e.Currency != py.Currency {
			continue
		}
		py.Expenses += e.Amount
		py.ByCategory[e.Category] += e.Amount
		py.Months[month(e.Date)][1] += e.Amount
	}
	// occupancy: the days each unit was let, this year up to today
	t := today()
	end := to
	if t < end {
		end = t
	}
	days := daysBetween(from, end) + 1
	for _, p := range b.props {
		py := by[p.ID]
		total, let := 0, 0
		for _, u := range p.Units {
			total += days
			lets := map[string]bool{}
			for _, l := range b.leases {
				if l.UnitID != u.ID || l.Status == store.LeaseDraft {
					continue
				}
				a, z := maxS(l.StartDate, from), end
				if l.EndDate != "" && l.EndDate < z {
					z = l.EndDate
				}
				if l.EndedOn != "" && l.EndedOn < z {
					z = l.EndedOn
				}
				for d := a; d <= z; d = store.AddDays(d, 1) {
					lets[d] = true
				}
			}
			let += len(lets)
		}
		if total > 0 {
			py.Occupancy = let * 100 / total
		}
		py.Net = py.Income - py.Expenses
		if py.Value > 0 && days > 0 {
			py.Yield = float64(py.Net) * 365 / float64(days) / float64(py.Value) * 100
		}
	}
	return out, nil
}

func maxS(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	year := queryInt(r, "year")
	if year < 1990 || year > 2200 {
		year = time.Now().Year()
	}
	list, err := s.reportYear(r, year)
	if err != nil {
		s.fail(w, err, "reports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"year": year, "properties": list})
}

// handleYearCSV is a year's money in and out, line by line (for taxes or an accountant).
func (s *Server) handleYearCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	year := queryInt(r, "year")
	if year < 1990 || year > 2200 {
		year = time.Now().Year()
	}
	y := strconv.Itoa(year)
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	exps, _ := s.store.ListExpenses(ctx, uid, 0, y+"-01-01", y+"-12-31")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="rentals-%d.csv"`, year))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "type", "place", "unit", "who", "what", "amount", "currency"})
	type line struct{ rec []string }
	var lines []line
	for _, l := range b.leases {
		for _, p := range b.payments[l.ID] {
			if strings.HasPrefix(p.Date, y) {
				lines = append(lines, line{[]string{p.Date, "income", l.PropertyName, l.UnitName, tenantNames(l.Tenants), "Rent and charges", fmt.Sprintf("%.2f", float64(p.Amount)/100), p.Currency}})
			}
		}
	}
	for _, e := range exps {
		unit := ""
		if e.UnitID != nil {
			if u := b.unitByID[*e.UnitID]; u != nil {
				unit = u.Name
			}
		}
		lines = append(lines, line{[]string{e.Date, "expense", e.PropertyName, unit, e.Payee, joinNonEmpty(": ", expenseWords[e.Category], e.Note), fmt.Sprintf("%.2f", -float64(e.Amount)/100), e.Currency}})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].rec[0] < lines[j].rec[0] })
	for _, l := range lines {
		_ = cw.Write(l.rec)
	}
	cw.Flush()
}

// ---- for the rest of the box: search, Contacts, Pocket Day ----

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	type hit struct {
		Kind  string `json:"kind"`
		ID    int64  `json:"id"`
		Title string `json:"title"`
		Sub   string `json:"subtitle"`
		URL   string `json:"url"`
		Path  string `json:"path"`
	}
	out := []hit{}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": out})
		return
	}
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	has := func(parts ...string) bool {
		return strings.Contains(strings.ToLower(strings.Join(parts, " ")), q)
	}
	add := func(h hit) {
		if len(out) < limit {
			h.URL = s.link(h.Path)
			out = append(out, h)
		}
	}
	for _, p := range b.props {
		if has(p.Name, p.Address, p.City, p.Notes) {
			add(hit{Kind: "property", ID: p.ID, Title: p.Name, Sub: joinNonEmpty(", ", p.Address, p.City, fmt.Sprintf("%d of %d let", p.Let, p.UnitCount)), Path: "/properties/" + itoa(p.ID)})
		}
	}
	for _, l := range b.leases {
		if has(l.Title, l.Notes, tenantEmails(l.Tenants)) {
			sub := l.Status + " · " + fmtMoney(l.Rent, l.Currency) + " " + l.Frequency
			if l.Balance > 0 {
				sub += " · owes " + fmtMoney(l.Balance, l.Currency)
			}
			add(hit{Kind: "lease", ID: l.ID, Title: l.Title, Sub: sub, Path: "/leases/" + itoa(l.ID)})
		}
	}
	ts, _ := s.store.ListTenants(ctx, uid)
	for _, t := range ts {
		if has(t.Name, t.Email, t.Phone, t.Notes) {
			add(hit{Kind: "tenant", ID: t.ID, Title: t.Name, Sub: joinNonEmpty(" · ", t.Email, t.Phone), Path: "/tenants/" + itoa(t.ID)})
		}
	}
	for _, rq := range b.requests {
		if has(rq.Title, rq.Details, rq.ProName, rq.PropertyName, rq.UnitName) {
			add(hit{Kind: "repair", ID: rq.ID, Title: rq.Title, Sub: place(rq.UnitName, rq.PropertyName) + " · " + strings.ReplaceAll(rq.Status, "_", " "), Path: "/repairs/" + itoa(rq.ID)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func tenantEmails(ts []*store.Tenant) string {
	var e []string
	for _, t := range ts {
		e = append(e, t.Name, t.Email, t.Phone)
	}
	return strings.Join(e, " ")
}

// handleLookup is what Rentals knows about a person by email (Pocket
// Contacts shows it on their card): as a tenant, where and what they owe.
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	if email == "" {
		writeJSON(w, http.StatusOK, map[string]any{"tenant": nil, "leases": []any{}})
		return
	}
	t, err := s.store.TenantByEmail(ctx, uid, email)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"tenant": nil, "leases": []any{}})
		return
	}
	t.ClientKey = ""
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "lookup")
		return
	}
	type leaseLine struct {
		ID       int64  `json:"id"`
		Place    string `json:"place"`
		Status   string `json:"status"`
		Start    string `json:"start_date"`
		End      string `json:"end_date"`
		Rent     int64  `json:"rent"`
		Currency string `json:"currency"`
		Balance  int64  `json:"balance"`
		URL      string `json:"url"`
	}
	out := []leaseLine{}
	for _, l := range b.leases {
		for _, lt := range l.Tenants {
			if lt.ID == t.ID {
				out = append(out, leaseLine{ID: l.ID, Place: place(l.UnitName, l.PropertyName), Status: l.Status, Start: l.StartDate, End: firstNonEmpty(l.EndedOn, l.EndDate),
					Rent: l.Rent, Currency: l.Currency, Balance: l.Balance, URL: s.link("/leases/" + itoa(l.ID))})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": t, "leases": out, "url": s.link("/tenants/" + itoa(t.ID))})
}

// handleDay is one day for Pocket Day's journal: rent that came in, repairs
// reported and done, inspections, leases that started or ended.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d := chiParam(r, "date")
	if !validDate(d) || d == "" {
		writeErr(w, http.StatusBadRequest, "the day is YYYY-MM-DD")
		return
	}
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	type item struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
		Sub   string `json:"subtitle"`
		URL   string `json:"url"`
	}
	items := []item{}
	for _, l := range b.leases {
		for _, p := range b.payments[l.ID] {
			if p.Date == d {
				items = append(items, item{"payment", "Rent in: " + fmtMoney(p.Amount, p.Currency), l.Title, s.link("/leases/" + itoa(l.ID))})
			}
		}
		if l.StartDate == d && l.Status != store.LeaseDraft {
			items = append(items, item{"lease", "Lease started", l.Title, s.link("/leases/" + itoa(l.ID))})
		}
		if l.EndedOn == d {
			items = append(items, item{"lease", "Lease ended", l.Title, s.link("/leases/" + itoa(l.ID))})
		}
	}
	for _, rq := range b.requests {
		if rq.ReportedOn == d {
			items = append(items, item{"repair", "Repair reported: " + rq.Title, place(rq.UnitName, rq.PropertyName), s.link("/repairs/" + itoa(rq.ID))})
		}
		if rq.DoneOn == d && rq.Status == "done" {
			items = append(items, item{"repair", "Repair done: " + rq.Title, place(rq.UnitName, rq.PropertyName), s.link("/repairs/" + itoa(rq.ID))})
		}
	}
	insp, _ := s.store.ListInspections(ctx, uid, 0, 0)
	for _, i := range insp {
		if i.Date == d {
			items = append(items, item{"inspection", inspectionWords[i.Kind], place(i.UnitName, i.PropertyName), s.link("/inspections/" + itoa(i.ID))})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": d, "items": items})
}
