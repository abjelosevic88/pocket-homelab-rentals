package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Rentals (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read what /search, /day and /today read, as the request's person,
// from the book as it stands (readBook: rent is not made here). Only the
// capture adds something: one repair, never a payment or a charge.

// dots joins the parts of a line that are there: "Flat 1, Linden Court · Ana Marić · late".
func dots(parts ...string) string { return joinNonEmpty(" · ", parts...) }

// upperFirst starts a line with a capital letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// count is "1 repair" or "3 repairs".
func count(n int, one, many string) string {
	return strconv.Itoa(n) + " " + plural(n, one, many)
}

// shortDay is a day next to today: "today", "tomorrow", "3 Dec", "3 Jan 2027".
func shortDay(d, t string) string {
	at, err := time.Parse("2006-01-02", d)
	switch {
	case err != nil:
		return d
	case d == t:
		return "today"
	case d == store.AddDays(t, 1):
		return "tomorrow"
	case strings.HasPrefix(t, d[:4]):
		return at.Format("2 Jan")
	}
	return at.Format("2 Jan 2006")
}

// ---------- search ----------

// pocketSearch answers Pocket Search with what /search finds (places,
// tenants, leases, repairs, with the same lines under them) and the units
// by their own name and notes. A name or title that has the words comes
// first; contact details show only where /search shows them: on the tenant.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if strings.IndexFunc(q, func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }) < 0 {
		return nil, nil // "??" or "*" is not a word to look for
	}
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.readBook(ctx, uid, true)
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	score := func(title string) float64 {
		switch t := strings.ToLower(title); {
		case strings.HasPrefix(t, lq):
			return 3
		case strings.Contains(t, lq):
			return 2
		}
		return 1
	}
	kinds := map[string]string{"property": "place", "lease": "lease", "tenant": "tenant", "repair": "repair"}
	by := map[string][]core.SearchItem{}
	found := map[int64]bool{} // the places among the matches
	for _, h := range s.find(ctx, uid, b, lq, 0) {
		k := kinds[h.Kind]
		by[k] = append(by[k], core.SearchItem{ID: k + ":" + itoa(h.ID), Title: h.Title, Snippet: h.Sub, Kind: k, Date: h.date, URL: h.Path, Score: score(h.Title)})
		if k == "place" {
			found[h.ID] = true
		}
	}
	for _, u := range b.units {
		p := b.propByID[u.PropertyID]
		// a unit that is the whole place is found as the place, and so is a
		// place's only unit when the place is a match already
		if p == nil || u.Archived || place(u.Name, p.Name) == p.Name || (p.UnitCount == 1 && found[p.ID]) ||
			!strings.Contains(strings.ToLower(u.Name+" "+u.Notes), lq) {
			continue
		}
		sub := "Empty"
		if l := u.Lease; l != nil && l.Status == store.LeaseActive {
			sub = dots(firstNonEmpty(tenantNames(l.Tenants), "Let"), fmtMoney(l.Rent, l.Currency)+" "+l.Frequency)
		}
		// a unit has no page of its own: its place shows it
		by["unit"] = append(by["unit"], core.SearchItem{ID: "unit:" + itoa(u.ID), Title: place(u.Name, p.Name), Snippet: sub, Kind: "unit",
			URL: "/properties/" + itoa(p.ID), Score: score(u.Name)})
	}
	out := []core.SearchItem{}
	for _, k := range capabilities.Search.Kinds {
		out = append(out, by[k]...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// pocketDay answers Pocket Day with what /day reads for the journal (rent
// that came in, repairs reported and done, inspections, leases that started
// or ended), and what was due that day: each rent or other charge and how
// far it is paid now, and a lease that runs out today or later. Money that
// came in carries its amount; a charge only says it in words, so nothing is
// counted twice. Days here are plain dates; the time zone only says what
// "today" is.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.readBook(ctx, uid, true)
	if err != nil {
		return nil, err
	}
	now := today()
	if tz != nil {
		now = time.Now().In(tz).Format("2006-01-02")
	}
	day := &core.Day{Date: date}
	add := func(it core.DayItem) { day.Items = append(day.Items, it) }
	in := money{}
	var paid []*store.Lease
	var due, started, ended, ends int
	for _, l := range b.leases {
		where, who, url := place(l.UnitName, l.PropertyName), tenantNames(l.Tenants), "/leases/"+itoa(l.ID)
		got := false
		for _, p := range b.payments[l.ID] {
			if p.Date != date || p.Amount <= 0 {
				continue
			}
			title := "Rent in"
			if who != "" {
				title = "Rent from " + who
			}
			add(core.DayItem{ID: "payment:" + itoa(p.ID), Title: title, Sub: where, Kind: "payment", Amount: p.Amount, Currency: p.Currency, Note: p.Note, URL: url})
			in.add(p.Currency, p.Amount)
			got = true
		}
		if got {
			paid = append(paid, l)
		}
		if l.Status == store.LeaseDraft {
			continue
		}
		for _, c := range b.charges[l.ID] {
			if c.DueDate != date || c.Waived {
				continue
			}
			state := map[string]string{"paid": "paid", "part": "part paid", "late": "late"}[c.State]
			add(core.DayItem{ID: "charge:" + itoa(c.ID), Title: c.Title + " due", Sub: dots(where, who, fmtMoney(c.Amount, c.Currency), state), Kind: "due",
				Done: c.State == "paid", URL: url})
			due++
		}
		switch {
		case l.EndedOn == date:
			add(core.DayItem{ID: "lease:" + itoa(l.ID), Title: "Lease ended", Sub: dots(where, who), Kind: "lease", Done: true, URL: url})
			ended++
		case l.StartDate == date:
			add(core.DayItem{ID: "lease:" + itoa(l.ID), Title: "Lease started", Sub: dots(where, who), Kind: "lease", URL: url})
			started++
		case l.Status == store.LeaseActive && l.EndDate == date && date >= now:
			add(core.DayItem{ID: "lease:" + itoa(l.ID), Title: "Lease ends", Sub: dots(where, who), Kind: "lease", URL: url})
			ends++
		}
	}
	var reported, done int
	for _, rq := range b.requests {
		where, url := place(rq.UnitName, rq.PropertyName), "/repairs/"+itoa(rq.ID)
		switch {
		case rq.DoneOn == date && rq.Status == "done":
			cost, same := "", ""
			if rq.Cost > 0 {
				cost = fmtMoney(rq.Cost, rq.Currency)
			}
			if rq.ReportedOn == date {
				same = "reported the same day"
			}
			add(core.DayItem{ID: "repair:" + itoa(rq.ID), Title: "Repair done: " + rq.Title, Sub: dots(where, cost, same), Kind: "repair", Done: true, URL: url})
			done++
		case rq.ReportedOn == date:
			by := ""
			if rq.ReportedBy != "" {
				by = "by " + rq.ReportedBy
			}
			add(core.DayItem{ID: "repair:" + itoa(rq.ID), Title: "Repair reported: " + rq.Title, Sub: dots(where, by), Kind: "repair", URL: url})
			reported++
		}
	}
	insp, err := s.store.ListInspections(ctx, uid, 0, 0)
	if err != nil {
		return nil, err
	}
	inspections := 0
	for _, i := range insp {
		if i.Date != date {
			continue
		}
		it := core.DayItem{ID: "inspection:" + itoa(i.ID), Title: inspectionWords[i.Kind], Sub: place(i.UnitName, i.PropertyName), Kind: "inspection",
			Done: i.Status == "done", URL: "/inspections/" + itoa(i.ID)}
		if reTime.MatchString(i.Time) {
			it.Time = i.Time
		}
		add(it)
		inspections++
	}
	if len(day.Items) == 0 {
		return nil, nil
	}

	var parts []string
	switch len(paid) {
	case 0:
	case 1:
		l := paid[0]
		parts = append(parts, "rent received from "+firstNonEmpty(tenantNames(l.Tenants), place(l.UnitName, l.PropertyName)))
	default:
		parts = append(parts, fmt.Sprintf("rent received on %d leases", len(paid)))
	}
	for _, p := range []struct {
		n         int
		one, many string
	}{{due, "payment due", "payments due"}, {reported, "repair reported", "repairs reported"}, {done, "repair done", "repairs done"},
		{inspections, "inspection", "inspections"}, {started, "lease started", "leases started"}, {ended, "lease ended", "leases ended"},
		{ends, "lease ends", "leases end"}} {
		if p.n > 0 {
			parts = append(parts, count(p.n, p.one, p.many))
		}
	}
	day.Summary = upperFirst(strings.Join(parts, ", "))
	curs := make([]string, 0, len(in))
	for c := range in {
		curs = append(curs, c)
	}
	sort.Strings(curs)
	for _, c := range curs {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Rent in", Value: fmtMoney(in[c], c)})
	}
	return day, nil
}

// ---------- widget ----------

// widgetRows is how many rows a widget shows.
const widgetRows = 8

// pocketWidget answers Pocket Hub with what needs the landlord, as the
// Today page has it: a lease with rent late (the longest wait on top), rent
// due soon, open repairs (urgent first), leases ending soon. When there is
// more than fits, the four share the rows. Someone without a place gets
// nothing to show; when all is fine the line stays empty.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.readBook(ctx, uid, false)
	if err != nil {
		return nil, err
	}
	if len(b.props) == 0 {
		return nil, nil
	}
	st := s.settingsOf(ctx, uid)
	t := today()
	horizon := store.AddDays(t, max(st.RemindDays, 0)+7)
	type row struct {
		key string // what the rows of a group are sorted by
		core.WidgetItem
	}
	var late, due, ending []row
	for _, l := range b.leases {
		if l.Status == store.LeaseDraft {
			continue
		}
		where, who, url := place(l.UnitName, l.PropertyName), tenantNames(l.Tenants), "/leases/"+itoa(l.ID)
		lateRow := false
		for _, c := range b.charges[l.ID] {
			switch {
			case c.State == "late" && !lateRow:
				// one row for the lease: all it owes late, since its oldest late day
				lateRow = true
				late = append(late, row{c.DueDate, core.WidgetItem{ID: "charge:" + itoa(c.ID), Title: where,
					Detail: dots(who, fmtMoney(l.Overdue, l.Currency)+" late", "since "+shortDay(c.DueDate, t)), URL: url}})
			case l.Status == store.LeaseActive && (c.State == "due" || c.State == "part" || c.State == "upcoming") && c.DueDate <= horizon:
				what, when := "", "due "+shortDay(c.DueDate, t)
				if c.Kind != "rent" {
					what = c.Title
				}
				if c.DueDate < t {
					when = "due since " + shortDay(c.DueDate, t)
				}
				due = append(due, row{c.DueDate, core.WidgetItem{ID: "charge:" + itoa(c.ID), Title: where,
					Detail: dots(who, what, fmtMoney(c.Amount-c.Paid, c.Currency), when), URL: url}})
			}
		}
		if l.Status == store.LeaseActive && l.DaysLeft != nil && *l.DaysLeft <= st.EndingDays {
			when := "lease ends " + shortDay(l.EndDate, t)
			if *l.DaysLeft < 0 {
				when = "lease ran out " + shortDay(l.EndDate, t)
			}
			ending = append(ending, row{l.EndDate, core.WidgetItem{ID: "lease:" + itoa(l.ID), Title: where, Detail: dots(who, when), URL: url}})
		}
	}
	var repairs []row // as listed: urgent first, then the newest
	for _, rq := range b.requests {
		if !rq.Open() {
			continue
		}
		state := map[string]string{"in_progress": "work started", "waiting": "waiting"}[rq.Status]
		if rq.Status == "planned" && rq.PlannedOn != "" {
			state = "planned " + shortDay(rq.PlannedOn, t)
		}
		urgent := ""
		if rq.Priority == "urgent" {
			urgent = "urgent"
		}
		repairs = append(repairs, row{"", core.WidgetItem{ID: "repair:" + itoa(rq.ID), Title: rq.Title,
			Detail: dots(place(rq.UnitName, rq.PropertyName), urgent, state), URL: "/repairs/" + itoa(rq.ID)}})
	}

	w := &core.WidgetData{URL: "/"}
	groups := [][]row{late, due, repairs, ending}
	// the rows are handed out one at a time, late rent first, so a long list
	// of one kind does not push the others out
	take := make([]int, len(groups))
	for room, gave := widgetRows, true; room > 0 && gave; {
		gave = false
		for i, g := range groups {
			if room > 0 && take[i] < len(g) {
				take[i]++
				room--
				gave = true
			}
		}
	}
	for i, g := range groups {
		sort.SliceStable(g, func(a, b int) bool { return g[a].key < g[b].key })
		for _, x := range g[:take[i]] {
			w.Items = append(w.Items, x.WidgetItem)
		}
	}
	var parts []string
	for _, p := range []struct {
		n         int
		one, many string
	}{{len(late), "rent late", "rents late"}, {len(due), "rent due soon", "rents due soon"}, {len(repairs), "repair open", "repairs open"},
		{len(ending), "lease ending soon", "leases ending soon"}} {
		if p.n > 0 {
			parts = append(parts, count(p.n, p.one, p.many))
		}
	}
	w.Line = strings.Join(parts, ", ")
	units, let := 0, 0
	for _, p := range b.props {
		units += p.UnitCount
		let += p.Let
	}
	w.Metrics = []core.Metric{{Label: "Rent late", Value: strconv.Itoa(len(late))}, {Label: "Due soon", Value: strconv.Itoa(len(due))},
		{Label: "Repairs open", Value: strconv.Itoa(len(repairs))}, {Label: "Let", Value: fmt.Sprintf("%d of %d", let, units)}}
	return w, nil
}

// ---------- capture ----------

// spot is where a repair goes: a place, and a unit when the line names one.
type spot struct {
	prop *store.Property
	unit *store.Unit
	many bool // more than one place or unit goes by that name
}

// placeKey folds a name for comparing: capitals, commas and extra spaces do not matter.
func placeKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, ",", " "))), " ")
}

// captureSpot finds where a quick line belongs and what is left to write
// down. "Flat 2: tap drips" starts with one of the person's own places or
// units and a colon: a place's name, a unit's name when only one unit has
// it, or both ("Flat 2, Linden Court: …", either way round). The name is a
// label, so it is dropped and the repair reads "Tap drips". Without one the
// line goes to the only place, when there is exactly one. A place alone
// means the whole place: no unit, so no tenant is put down as having
// reported it. A nil spot = it cannot be placed; many = the name is not
// enough to tell.
func captureSpot(props []*store.Property, units []*store.Unit, line string) (*spot, string) {
	names := map[string]*spot{}
	byID := map[int64]*store.Property{}
	for _, p := range props {
		byID[p.ID] = p
		if k := placeKey(p.Name); names[k] == nil {
			names[k] = &spot{prop: p}
		} else {
			names[k].many = true
		}
	}
	for _, u := range units {
		p := byID[u.PropertyID]
		// a unit that is the whole place goes by the place's name
		if p == nil || u.Archived || place(u.Name, p.Name) == p.Name {
			continue
		}
		for _, k := range []string{placeKey(u.Name + " " + p.Name), placeKey(p.Name + " " + u.Name), placeKey(u.Name)} {
			switch old := names[k]; {
			case old == nil:
				names[k] = &spot{prop: p, unit: u}
			case old.unit != nil && old.unit.ID != u.ID:
				old.many = true
			}
		}
	}
	// the longest name in front of a colon wins: "Block A: north: tap" may be a place called "Block A: north"
	for i := len(line) - 1; i > 0; i-- {
		if line[i] != ':' {
			continue
		}
		rest := strings.TrimSpace(line[i+1:])
		if sp := names[placeKey(line[:i])]; sp != nil && rest != "" {
			return sp, upperFirst(rest)
		}
	}
	if len(props) == 1 {
		return &spot{prop: props[0]}, line
	}
	return nil, line
}

// captured is the answer for a kept line: the repair with its place in
// front, so the person sees where it went.
func captured(rq *store.Request) *core.CaptureResult {
	return &core.CaptureResult{ID: "repair:" + itoa(rq.ID), Title: place(rq.UnitName, rq.PropertyName) + ": " + rq.Title, URL: "/repairs/" + itoa(rq.ID)}
}

// pocketCapture keeps a quick line from Pocket Hub as a new repair, the way
// POST /requests with only a place and a title does: status new, normal
// priority, reported today, on the place's timeline, and a task in Pocket
// Tasks when the person's settings make one for every repair. No tenant is
// told and no money is touched. Like a ref on POST /requests, the same
// request_id again answers with the repair made the first time and adds
// nothing.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	ref := "capture:" + in.RequestID
	old, err := s.store.RequestBySource(ctx, uid, ref)
	if err == nil {
		return captured(old), nil // an identical retry
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	// the first line is the repair, anything after it the details
	line, details := in.Title, in.Text
	if line == "" {
		line, details, _ = strings.Cut(in.Text, "\n")
	}
	line, details = strings.Join(strings.Fields(line), " "), strings.TrimSpace(details)
	props, err := s.store.ListProperties(ctx, uid, false)
	if err != nil {
		return nil, err
	}
	if len(props) == 0 {
		return nil, fmt.Errorf("%w: add a place in Pocket Rentals first", core.ErrCapture)
	}
	units, err := s.store.ListUnits(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	sp, title := captureSpot(props, units, line)
	switch {
	case sp == nil:
		return nil, fmt.Errorf("%w: start with the place or unit, e.g. \"%s: tap drips\"", core.ErrCapture, props[0].Name)
	case sp.many && sp.unit != nil:
		return nil, fmt.Errorf("%w: more than one of your units is called %s; say the place too, e.g. \"%s, %s: tap drips\"", core.ErrCapture, sp.unit.Name, sp.unit.Name, sp.prop.Name)
	case sp.many:
		return nil, fmt.Errorf("%w: more than one of your places is called %s; add this repair in Pocket Rentals", core.ErrCapture, sp.prop.Name)
	}
	if utf8.RuneCountInString(title) > 200 { // a long line: the start is the title, all of it is in the details
		title, details = clip(title, 200), strings.TrimSpace(title+"\n"+details)
	}
	body := requestIn{PropertyID: &sp.prop.ID, Title: &title, Details: &details}
	if sp.unit != nil {
		body.UnitID = &sp.unit.ID
	}
	rq, _, err := s.createRequest(ctx, uid, body, linkApp(ctx), ref)
	var ue *userErr
	switch {
	case errors.Is(err, store.ErrConflict): // the same request, at the same moment
		if old, err := s.store.RequestBySource(ctx, uid, ref); err == nil {
			return captured(old), nil
		}
		return nil, err
	case errors.As(err, &ue):
		return nil, fmt.Errorf("%w: %s", core.ErrCapture, ue.msg)
	case err != nil:
		return nil, err
	}
	return captured(rq), nil
}
