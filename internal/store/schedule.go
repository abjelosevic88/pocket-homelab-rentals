package store

import (
	"time"
)

// ---- when rent is due, and what is paid ----
//
// Rent is paid ahead, for the period that starts on its due day: a monthly
// lease with due day 5 pays on the 5th for the 5th to the 4th. A lease that
// starts between two due days first pays the part up to the next due day,
// by the day; one that ends between two due days pays the last part the
// same way. Nothing here touches the database.

// Period is one stretch of rent.
type Period struct {
	Start, End string // End is the day after the last day
	Due        string
	Amount     int64
	Part       bool // a part period (paid by the day)
}

func day(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

// anchor is the k-th due day of a monthly/quarterly/yearly lease counted
// from the month of base (due days past 28 count as 28).
func anchor(base time.Time, step, k, dueDay int) time.Time {
	dd := min(max(dueDay, 1), 28)
	return time.Date(base.Year(), base.Month()+time.Month(step*k), dd, 0, 0, 0, 0, time.UTC)
}

func monthsPer(freq string) int {
	switch freq {
	case "quarterly":
		return 3
	case "yearly":
		return 12
	}
	return 1
}

func prorate(rent int64, part, whole time.Duration) int64 {
	if whole <= 0 {
		return rent
	}
	pd, wd := int64(part.Hours()/24+0.5), int64(whole.Hours()/24+0.5)
	if wd == 0 {
		return rent
	}
	return (rent*pd + wd/2) / wd
}

// Periods lists the rent periods of a lease whose due day is on or before
// upTo. charges_from (when later than the start) skips the time before it.
func Periods(l *Lease, upTo string) []Period {
	start, ok := day(l.StartDate)
	if !ok || l.Rent <= 0 {
		return nil
	}
	limit, ok := day(upTo)
	if !ok {
		return nil
	}
	var end time.Time // the day after the last day
	if e, ok := day(l.EndDate); ok {
		end = e.AddDate(0, 0, 1)
	}
	if e, ok := day(l.EndedOn); ok && (end.IsZero() || e.AddDate(0, 0, 1).Before(end)) {
		end = e.AddDate(0, 0, 1)
	}
	from := start
	if cf, ok := day(l.ChargesFrom); ok && cf.After(start) {
		from = cf
	}
	// the period boundaries, in order
	var bounds []time.Time
	if l.Frequency == "weekly" {
		wd := min(max(l.DueDay, 1), 7) % 7 // 1 = Monday … 7 = Sunday → time.Weekday
		first := start
		for int(first.Weekday()) != wd {
			first = first.AddDate(0, 0, 1)
		}
		// one boundary before the start too, for the part period's length
		for t := first.AddDate(0, 0, -7); !t.After(limit.AddDate(0, 0, 7)); t = t.AddDate(0, 0, 7) {
			bounds = append(bounds, t)
		}
	} else {
		step := monthsPer(l.Frequency)
		for k := -1; ; k++ {
			t := anchor(start, step, k, l.DueDay)
			if t.After(limit.AddDate(0, step, 0)) {
				break
			}
			bounds = append(bounds, t)
			if k > 1200 {
				break
			}
		}
	}
	var out []Period
	add := func(ps, pe time.Time, whole time.Duration) {
		if !end.IsZero() && !ps.Before(end) {
			return
		}
		if ps.Before(from) {
			if !pe.After(from) {
				return
			}
			if !from.Equal(start) {
				return // brought in later: whole periods only, from the first due day after
			}
		}
		if ps.After(limit) {
			return
		}
		p := Period{Start: ymd(ps), Due: ymd(ps), Amount: l.Rent}
		pend := pe
		if !end.IsZero() && end.Before(pe) {
			pend = end
		}
		p.End = ymd(pend)
		if pend.Sub(ps) < whole-12*time.Hour {
			p.Part = true
			p.Amount = prorate(l.Rent, pend.Sub(ps), whole)
		}
		if p.Amount > 0 {
			out = append(out, p)
		}
	}
	for i := 0; i+1 < len(bounds); i++ {
		a, b := bounds[i], bounds[i+1]
		whole := b.Sub(a)
		switch {
		case !b.After(start):
			continue
		case a.Before(start):
			// the part before the first due day
			if from.Equal(start) {
				add(start, b, whole)
			}
		default:
			add(a, b, whole)
		}
	}
	return out
}

// NextDue is the next rent day after today ("" when the lease ends first).
func NextDue(l *Lease, today string) string {
	t, ok := day(today)
	if !ok {
		return ""
	}
	horizon := ymd(t.AddDate(0, monthsPer(l.Frequency)+1, 7))
	for _, p := range Periods(l, horizon) {
		if p.Due > today {
			return p.Due
		}
	}
	return ""
}

// Ledger works out what each charge has been paid (oldest first; the
// deposit before rent on the same day) and what the lease owes: balance =
// charges due by today − payments; overdue = what is still unpaid of the
// charges past their grace days. Waived charges count for nothing.
func Ledger(charges []*Charge, payments []*Payment, today string, grace int) (balance, overdue int64) {
	var paid int64
	for _, p := range payments {
		paid += p.Amount
	}
	// a running pot of money, handed to charges in order
	pot := paid
	pays := payments
	spent := int64(0)
	pi := 0
	var payRun int64
	for _, c := range charges {
		c.Paid, c.PaidOn = 0, ""
		if c.Waived {
			c.State = "waived"
			continue
		}
		take := max(min(c.Amount, pot), 0)
		c.Paid = take
		pot -= take
		spent += take
		// the day the money that covers it came in
		for pi < len(pays) && payRun+pays[pi].Amount < spent {
			payRun += pays[pi].Amount
			pi++
		}
		if c.Paid >= c.Amount && pi < len(pays) {
			c.PaidOn = pays[pi].Date
		} else if c.Paid >= c.Amount && len(pays) > 0 {
			c.PaidOn = pays[len(pays)-1].Date
		}
		lateFrom := c.DueDate
		if d, ok := day(c.DueDate); ok {
			lateFrom = ymd(d.AddDate(0, 0, grace))
		}
		switch {
		case c.Paid >= c.Amount:
			c.State = "paid"
		case c.DueDate > today:
			c.State = "upcoming"
		case today > lateFrom:
			c.State = "late"
			overdue += c.Amount - c.Paid
		case c.Paid > 0:
			c.State = "part"
		default:
			c.State = "due"
		}
		if c.DueDate <= today {
			balance += c.Amount
		}
	}
	balance -= paid
	return balance, overdue
}
