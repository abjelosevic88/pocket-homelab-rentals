package server

import (
	"context"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// seedDemo fills the owner's account with a small landlord's year: a
// building with three flats, an apartment, a cottage between tenants and a
// garage; tenants who pay on time, one who pays late, a lease ending soon,
// repairs, costs and inspections. Dates follow today, so it always looks
// current. Only when the account is empty.
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if ps, _ := s.store.ListProperties(ctx, uid, true); len(ps) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	now := time.Now()
	m0 := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	d := func(t time.Time) string { return t.Format("2006-01-02") }
	month := func(n int) time.Time { return m0.AddDate(0, n, 0) }
	t := today()

	type unitSpec struct {
		name          string
		beds, baths   int
		area          float64
		rent, deposit int64
	}
	mkProp := func(p store.Property, units ...unitSpec) (*store.Property, []*store.Unit) {
		p.UserID = uid
		if p.Currency == "" {
			p.Currency = "EUR"
		}
		_ = s.store.CreateProperty(ctx, &p)
		var us []*store.Unit
		for _, u := range units {
			x := &store.Unit{UserID: uid, PropertyID: p.ID, Name: u.name, Bedrooms: u.beds, Bathrooms: u.baths, AreaM2: u.area, Rent: u.rent, Deposit: u.deposit}
			_ = s.store.CreateUnit(ctx, x)
			us = append(us, x)
		}
		pid := p.ID
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, PropertyID: &pid, Kind: "note", Title: "Place added", At: d(month(-30)) + "T10:00:00Z"})
		return &p, us
	}
	court, flats := mkProp(store.Property{Name: "Linden Court", Kind: "building", Address: "Linden Street 14", City: "Riverside", Value: 68000000, BoughtOn: "2019-04-12", BoughtFor: 51000000,
		MoneyTag: "linden", Color: "#1d6fa5", Notes: "Three flats over two floors. Stairwell cleaned every Monday. Boiler room key in the meter box."},
		unitSpec{"Flat 1", 2, 1, 64, 65000, 130000}, unitSpec{"Flat 2", 1, 1, 48, 52000, 104000}, unitSpec{"Flat 3", 0, 1, 31, 43000, 86000})
	harbour, hu := mkProp(store.Property{Name: "Harbour View", Kind: "apartment", Address: "Quay Road 3, 4th floor", City: "Riverside", Value: 31000000, BoughtOn: "2021-09-01", BoughtFor: 27500000,
		MoneyTag: "harbour", Color: "#0c8599", Notes: "Building manager: Marko, +385 91 555 0101. Fee paid by standing order."},
		unitSpec{"Apartment", 2, 1, 71, 78000, 156000})
	cottage, cu := mkProp(store.Property{Name: "Garden Cottage", Kind: "house", Address: "Old Mill Lane 7", City: "Brookfield", Value: 24500000, BoughtOn: "2017-06-20", BoughtFor: 16000000,
		MoneyTag: "cottage", Color: "#2f9e44", Notes: "Well water, septic tank emptied every two years (last: spring 2025)."},
		unitSpec{"Whole place", 3, 1, 96, 95000, 190000})
	garage, gu := mkProp(store.Property{Name: "Garage, Linden Street", Kind: "garage", Address: "Linden Street 14, yard", City: "Riverside", Value: 1800000, MoneyTag: "linden"},
		unitSpec{"Garage", 0, 0, 16, 6000, 0})

	tenant := func(name, email, phone, notes string) *store.Tenant {
		x := &store.Tenant{UserID: uid, Name: name, Email: email, Phone: phone, Notes: notes}
		_ = s.store.CreateTenant(ctx, x)
		return x
	}
	ana := tenant("Ana Marić", "ana.maric@example.com", "+385 91 234 5678", "Works from home; prefers messages after 5 pm.")
	luka := tenant("Luka Novak", "luka.novak@example.com", "+385 98 765 4321", "")
	mia := tenant("Mia Novak", "mia.novak@example.com", "", "")
	jonas := tenant("Jonas Weber", "jonas.weber@example.com", "+49 151 2345 6789", "Student; parents guarantee the rent (Klaus Weber, +49 170 555 0102).")
	sofia := tenant("Sofia Rossi", "sofia.rossi@example.com", "+39 333 123 4567", "")
	peter := tenant("Peter Hall", "peter.hall@example.com", "+44 7700 900123", "Keeps a classic car in the garage.")
	emma := tenant("Emma Clarke", "emma.clarke@example.com", "+44 7700 900456", "Moved to the city for work.")

	lease := func(u *store.Unit, l store.Lease, ts ...*store.Tenant) *store.Lease {
		l.UserID, l.UnitID = uid, u.ID
		if l.Currency == "" {
			l.Currency = "EUR"
		}
		if l.Frequency == "" {
			l.Frequency = "monthly"
		}
		if l.DueDay == 0 {
			l.DueDay = 1
		}
		if l.Status == "" {
			l.Status = store.LeaseActive
		}
		if l.GraceDays == 0 {
			l.GraceDays = 5
		}
		if l.NoticeDays == 0 {
			l.NoticeDays = 30
		}
		if l.DepositState == "" {
			l.DepositState = "held"
		}
		_ = s.store.CreateLease(ctx, &l)
		var ids []int64
		for _, x := range ts {
			ids = append(ids, x.ID)
		}
		_ = s.store.SetLeaseTenants(ctx, uid, l.ID, ids)
		lid := l.ID
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: "Lease made: " + fmtMoney(l.Rent, l.Currency) + " monthly from " + niceDate(l.StartDate), At: l.StartDate + "T09:00:00Z"})
		full, _ := s.store.GetLease(ctx, uid, l.ID)
		s.ensureLease(ctx, full)
		return full
	}
	lAna := lease(flats[0], store.Lease{StartDate: d(month(-20)), EndDate: d(month(4).AddDate(0, 0, -1)), Rent: 65000, Deposit: 130000, Notes: "Renewed once; rent went from 620 to 650."}, ana)
	lNovak := lease(flats[1], store.Lease{StartDate: d(month(-5).AddDate(0, 0, 14)), EndDate: d(month(7).AddDate(0, 0, 13)), Rent: 52000, Deposit: 104000}, luka, mia)
	lJonas := lease(flats[2], store.Lease{StartDate: d(month(-26)), Rent: 43000, Deposit: 86000, LateFee: 2000, Notes: "Runs until notice. Pays from a student account; often a few days late."}, jonas)
	lSofia := lease(hu[0], store.Lease{StartDate: d(month(-12)), EndDate: d(month(2).AddDate(0, 0, -1)), Rent: 78000, Deposit: 156000, DueDay: 5}, sofia)
	lPeter := lease(gu[0], store.Lease{StartDate: d(month(-40)), ChargesFrom: d(month(-10)), Rent: 6000, Deposit: 0, DepositState: "none", Frequency: "monthly"}, peter)
	lEmma := lease(cu[0], store.Lease{StartDate: d(month(-30)), EndDate: d(month(-1).AddDate(0, 0, -1)), Rent: 92000, Deposit: 184000}, emma)
	lEmma.Status, lEmma.EndedOn, lEmma.EndReason = store.LeaseEnded, d(month(-1).AddDate(0, 0, -1)), "Moved to the city for work."
	_ = s.store.UpdateLease(ctx, lEmma)
	_ = s.store.DropRentAfter(ctx, uid, lEmma.ID, lEmma.EndedOn)
	lid := lEmma.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: "Lease ended on " + niceDate(lEmma.EndedOn), Body: lEmma.EndReason, At: lEmma.EndedOn + "T17:00:00Z"})

	// rent paid: on time mostly, Jonas late and short lately
	pay := func(l *store.Lease, late func(c *store.Charge) (days int, part float64)) {
		cs, _ := s.store.ListCharges(ctx, uid, l.ID)
		for _, c := range cs {
			if c.DueDate > t || c.Kind == "late_fee" {
				continue
			}
			days, part := 0, 1.0
			if late != nil {
				days, part = late(c)
			} else {
				days = rng.Intn(4) - 1
			}
			if part <= 0 {
				continue
			}
			day := store.AddDays(c.DueDate, days)
			if day > t {
				continue
			}
			amt := int64(float64(c.Amount) * part)
			p := &store.Payment{UserID: uid, LeaseID: l.ID, Date: day, Amount: amt, Currency: c.Currency, Method: "bank"}
			if c.Kind == "deposit" {
				p.Note = "Deposit"
			}
			_ = s.store.AddPayment(ctx, p)
		}
	}
	for _, l := range []*store.Lease{lAna, lNovak, lSofia, lPeter, lEmma} {
		pay(l, nil)
	}
	prev := d(month(-1))
	pay(lJonas, func(c *store.Charge) (int, float64) {
		switch {
		case c.DueDate >= d(m0):
			return 0, 0 // this month: not yet
		case c.DueDate >= prev:
			return 9, 0.5 // last month: half, late
		}
		return 3 + rng.Intn(6), 1
	})
	jl := lJonas.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &jl, Kind: "call", Title: "Called Jonas about last month's rent", Body: "Said his student loan is late; he will pay the rest with this month's.", At: store.AddDays(prev, 12) + "T18:30:00Z"})
	al := lAna.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &al, Kind: "note", Title: "Ana asked about a longer lease", Body: "Would like two years from the next renewal. Fine by me if the rent follows inflation.", At: d(month(-1).AddDate(0, 0, 20)) + "T11:00:00Z"})
	// deposits were paid at the start (before Peter's records start)
	for _, l := range []*store.Lease{lAna, lNovak, lJonas, lSofia, lEmma} {
		l2, _ := s.store.GetLease(ctx, uid, l.ID)
		if l2 != nil && l2.DepositState == "due" {
			l2.DepositState = "held"
			_ = s.store.UpdateLease(ctx, l2)
		}
	}
	for _, l := range []*store.Lease{lAna, lNovak, lJonas, lSofia, lPeter} {
		full, _ := s.store.GetLease(ctx, uid, l.ID)
		s.ensureLease(ctx, full)
	}

	// repairs
	req := func(p *store.Property, u *store.Unit, l *store.Lease, r store.Request) *store.Request {
		r.UserID, r.PropertyID = uid, p.ID
		if u != nil {
			r.UnitID = &u.ID
		}
		if l != nil {
			r.LeaseID = &l.ID
			if r.ReportedBy == "" {
				r.ReportedBy = tenantNames(l.Tenants)
			}
		}
		if r.Currency == "" {
			r.Currency = "EUR"
		}
		if r.Priority == "" {
			r.Priority = "normal"
		}
		_ = s.store.CreateRequest(ctx, &r)
		rid, pid := r.ID, p.ID
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, RequestID: &rid, PropertyID: &pid, LeaseID: r.LeaseID, Kind: "repair", Title: "Repair reported: " + r.Title, At: r.ReportedOn + "T08:00:00Z"})
		if r.Status == "done" {
			_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, RequestID: &rid, PropertyID: &pid, LeaseID: r.LeaseID, Kind: "repair", Title: "Done on " + niceDate(r.DoneOn), At: r.DoneOn + "T16:00:00Z"})
			if r.Cost > 0 {
				_ = s.store.AddExpense(ctx, &store.Expense{UserID: uid, PropertyID: p.ID, UnitID: r.UnitID, RequestID: &rid, Date: r.DoneOn, Amount: r.Cost, Currency: r.Currency, Category: "repairs", Payee: r.ProName, Note: r.Title})
			}
		}
		return &r
	}
	req(court, flats[1], lNovak, store.Request{Title: "Dripping kitchen tap", Details: "The cold tap in the kitchen drips all the time, worse at night.", Status: "done",
		ReportedOn: d(month(-2).AddDate(0, 0, 3)), PlannedOn: d(month(-2).AddDate(0, 0, 6)), DoneOn: d(month(-2).AddDate(0, 0, 6)), Cost: 8500, ProName: "Ivo the plumber", ProPhone: "+385 91 555 0199"})
	req(court, flats[2], lJonas, store.Request{Title: "No heating in the bedroom", Details: "The radiator in the bedroom stays cold since the weekend; the living room one works.", Priority: "urgent",
		Status: "planned", ReportedOn: store.AddDays(t, -2), PlannedOn: store.AddDays(t, 1), PlannedTime: "09:00", ProName: "Warmline Heating", ProPhone: "+385 1 555 0142"})
	req(court, nil, nil, store.Request{Title: "Stairwell light on the 1st floor", Details: "The motion sensor light does not switch on any more.", Status: "new", ReportedOn: store.AddDays(t, -6), ReportedBy: "Ana Marić"})
	req(harbour, hu[0], lSofia, store.Request{Title: "Bedroom window does not close tight", Details: "Draught along the top of the window; the handle is loose.", Status: "waiting",
		ReportedOn: store.AddDays(t, -16), ProName: "Glass & Frame Co.", ProPhone: "+385 1 555 0177"})
	req(cottage, cu[0], nil, store.Request{Title: "Repaint the small bedroom", Details: "Marks on the walls after the last tenant; before the next one moves in.", Status: "planned",
		ReportedOn: store.AddDays(t, -20), PlannedOn: store.AddDays(t, 6), Cost: 0, ProName: "Myself"})
	req(cottage, cu[0], lEmma, store.Request{Title: "Septic tank emptied", Status: "done", ReportedOn: d(month(-8)), DoneOn: d(month(-8).AddDate(0, 0, 9)), Cost: 18000, ProName: "Brookfield Septic"})
	req(harbour, hu[0], lSofia, store.Request{Title: "Washing machine leaking", Status: "done", Priority: "urgent", ReportedOn: d(month(-6).AddDate(0, 0, 11)), DoneOn: d(month(-6).AddDate(0, 0, 12)), Cost: 39900,
		ProName: "Appliance Fix", Details: "Replaced the door seal and the drain hose."})

	// costs
	exp := func(p *store.Property, date, cat, payee, note string, amount int64) {
		_ = s.store.AddExpense(ctx, &store.Expense{UserID: uid, PropertyID: p.ID, Date: date, Amount: amount, Currency: "EUR", Category: cat, Payee: payee, Note: note})
	}
	for i := -10; i <= 0; i++ {
		m := month(i)
		if d(m) > t {
			break
		}
		exp(court, d(m.AddDate(0, 0, 2)), "cleaning", "Sparkle Cleaning", "Stairwell, every Monday", 6000)
		exp(harbour, d(m.AddDate(0, 0, 1)), "building_fee", "Quay Road 3 owners", "Building fee", 4500)
		if i%3 == 0 {
			exp(court, d(m.AddDate(0, 0, 14)), "utilities", "City Water", "Water for the common areas", 3200+int64(rng.Intn(900)))
		}
	}
	exp(court, d(month(-7).AddDate(0, 0, 9)), "insurance", "Riverside Mutual", "Building insurance for the year", 42000)
	exp(court, d(month(-4).AddDate(0, 0, 20)), "tax", "City of Riverside", "Property tax", 61000)
	exp(harbour, d(month(-4).AddDate(0, 0, 20)), "tax", "City of Riverside", "Property tax", 27000)
	exp(cottage, d(month(-5).AddDate(0, 0, 3)), "tax", "Brookfield council", "Property tax", 19000)
	exp(cottage, d(month(-1).AddDate(0, 0, 4)), "cleaning", "Fresh Start", "Deep clean after moving out", 22000)
	exp(harbour, d(month(-3).AddDate(0, 0, 8)), "furniture", "Home Store", "New sofa bed", 54900)
	exp(court, d(month(-2).AddDate(0, 0, 25)), "loan_interest", "Riverside Bank", "Mortgage interest, quarter", 118000)

	// inspections
	insp := func(p *store.Property, u *store.Unit, l *store.Lease, kind, date, status, summary string, conds map[string]string) {
		in := &store.Inspection{UserID: uid, PropertyID: p.ID, Kind: kind, Date: date, Status: status, Summary: summary}
		if u != nil {
			in.UnitID = &u.ID
		}
		if l != nil {
			in.LeaseID = &l.ID
		}
		if status == "planned" {
			in.Time = "10:00"
		}
		_ = s.store.CreateInspection(ctx, in)
		var items []*store.InspectionItem
		for _, r := range defaultRooms {
			for _, it := range r.items {
				c := ""
				if status == "done" {
					c = "good"
					if v, ok := conds[r.room+"|"+it]; ok {
						c = v
					} else if rng.Intn(6) == 0 {
						c = "fair"
					}
				}
				note := ""
				if v, ok := conds[r.room+"|"+it+"|note"]; ok {
					note = v
				}
				items = append(items, &store.InspectionItem{Room: r.room, Item: it, Condition: c, Note: note})
			}
		}
		_ = s.store.SetItems(ctx, uid, in.ID, items)
		pid := p.ID
		title := inspectionWords[kind] + " planned for " + niceDate(date)
		if status == "done" {
			title = inspectionWords[kind] + " done"
		}
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: in.LeaseID, PropertyID: &pid, Kind: "inspection", Title: title, URL: "/inspections/" + itoa(in.ID), At: date + "T10:30:00Z"})
	}
	insp(cottage, cu[0], lEmma, "move_in", lEmma.StartDate, "done", "Freshly painted, new kitchen fitted the year before.", map[string]string{})
	insp(cottage, cu[0], lEmma, "move_out", lEmma.EndedOn, "done", "Clean overall. Marks on the small bedroom walls; one kitchen cupboard door hangs loose; one key missing.",
		map[string]string{"Bedroom|Walls": "damaged", "Bedroom|Walls|note": "Marks and two holes from shelves", "Kitchen|Cupboards": "poor", "Kitchen|Cupboards|note": "Left door hinge broken",
			"Entrance|Keys": "missing", "Entrance|Keys|note": "Two of three keys returned", "Meters|Electricity reading|note": "48211 kWh", "Meters|Water reading|note": "1290 m³"})
	insp(court, flats[1], lNovak, "move_in", lNovak.StartDate, "done", "All good. Fridge a little noisy.", map[string]string{"Kitchen|Fridge": "fair", "Kitchen|Fridge|note": "Noisy compressor, works"})
	insp(harbour, hu[0], lSofia, "routine", store.AddDays(t, 9), "planned", "", nil)
	insp(court, flats[0], lAna, "routine", d(month(-3).AddDate(0, 0, 10)), "done", "Well kept. Bathroom silicone needs renewing next year.", map[string]string{"Bathroom|Shower or bath": "fair"})

	_ = harbour
	_ = garage
	s.forgetEnsured(uid)
	s.log.Info("demo data added", "user", uid)
	return nil
}
