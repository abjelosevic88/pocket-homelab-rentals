package store

import "testing"

func TestPeriodsMonthly(t *testing.T) {
	l := &Lease{StartDate: "2026-01-01", Rent: 50000, Frequency: "monthly", DueDay: 1}
	ps := Periods(l, "2026-03-15")
	if len(ps) != 3 || ps[0].Due != "2026-01-01" || ps[2].Due != "2026-03-01" || ps[0].Amount != 50000 || ps[0].Part {
		t.Fatalf("got %+v", ps)
	}
	// starts mid-month, due on the 1st: a part month first
	l.StartDate = "2026-01-16"
	ps = Periods(l, "2026-02-10")
	if len(ps) != 2 || !ps[0].Part || ps[0].Due != "2026-01-16" || ps[0].End != "2026-02-01" || ps[1].Due != "2026-02-01" {
		t.Fatalf("got %+v", ps)
	}
	if ps[0].Amount != 25806 { // 16 of 31 days
		t.Fatalf("part amount %d", ps[0].Amount)
	}
	// due on the 16th, starting the 16th: whole months
	l.DueDay = 16
	ps = Periods(l, "2026-03-20")
	if len(ps) != 3 || ps[0].Part || ps[2].Due != "2026-03-16" {
		t.Fatalf("got %+v", ps)
	}
	// ends between two due days: the last part by the day
	l.EndDate = "2026-03-31"
	ps = Periods(l, "2026-06-01")
	if len(ps) != 3 || !ps[2].Part || ps[2].End != "2026-04-01" {
		t.Fatalf("got %+v", ps)
	}
	// brought in later: from the first due day on or after charges_from
	l = &Lease{StartDate: "2020-05-01", ChargesFrom: "2026-11-01", Rent: 40000, Frequency: "monthly", DueDay: 1}
	ps = Periods(l, "2026-12-05")
	if len(ps) != 2 || ps[0].Due != "2026-11-01" {
		t.Fatalf("got %+v", ps)
	}
}

func TestPeriodsWeeklyYearly(t *testing.T) {
	l := &Lease{StartDate: "2026-11-02", Rent: 10000, Frequency: "weekly", DueDay: 1} // a Monday
	ps := Periods(l, "2026-11-20")
	if len(ps) != 3 || ps[1].Due != "2026-11-09" {
		t.Fatalf("got %+v", ps)
	}
	l = &Lease{StartDate: "2026-02-01", Rent: 600000, Frequency: "yearly", DueDay: 1}
	ps = Periods(l, "2028-01-01")
	if len(ps) != 2 || ps[1].Due != "2027-02-01" {
		t.Fatalf("got %+v", ps)
	}
	l = &Lease{StartDate: "2026-01-01", Rent: 150000, Frequency: "quarterly", DueDay: 1}
	ps = Periods(l, "2026-12-31")
	if len(ps) != 4 || ps[3].Due != "2026-10-01" {
		t.Fatalf("got %+v", ps)
	}
}

func TestLedger(t *testing.T) {
	cs := []*Charge{
		{Kind: "deposit", DueDate: "2026-09-01", Amount: 100000},
		{Kind: "rent", DueDate: "2026-09-01", Amount: 50000},
		{Kind: "rent", DueDate: "2026-10-01", Amount: 50000},
		{Kind: "rent", DueDate: "2026-11-01", Amount: 50000},
		{Kind: "rent", DueDate: "2026-12-01", Amount: 50000},
	}
	ps := []*Payment{{Date: "2026-09-01", Amount: 150000}, {Date: "2026-10-03", Amount: 50000}, {Date: "2026-11-02", Amount: 20000}}
	bal, over := Ledger(cs, ps, "2026-11-10", 5)
	if bal != 30000 || over != 30000 {
		t.Fatalf("balance %d overdue %d", bal, over)
	}
	if cs[0].State != "paid" || cs[2].PaidOn != "2026-10-03" || cs[3].State != "late" || cs[3].Paid != 20000 || cs[4].State != "upcoming" {
		t.Fatalf("states %+v %+v %+v %+v", cs[0], cs[2], cs[3], cs[4])
	}
	bal, over = Ledger(cs, ps, "2026-11-04", 5)
	if bal != 30000 || over != 0 || cs[3].State != "part" {
		t.Fatalf("in grace: %d %d %s", bal, over, cs[3].State)
	}
	// paid ahead
	ps = append(ps, &Payment{Date: "2026-11-05", Amount: 80000})
	bal, _ = Ledger(cs, ps, "2026-11-10", 5)
	if bal != -50000 || cs[4].State != "paid" {
		t.Fatalf("ahead %d %s", bal, cs[4].State)
	}
}
