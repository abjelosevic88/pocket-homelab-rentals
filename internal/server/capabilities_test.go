package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/config"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// fakeCapCore is a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write> from Pocket Hub
// and remembers the manifest Rentals registers with.
type fakeCapCore struct {
	*httptest.Server
	mu       sync.Mutex
	manifest map[string]any
}

func newCapApp(t *testing.T) (*app, *fakeCapCore) {
	t.Helper()
	fc := &fakeCapCore{}
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "rentals:read", "write": "rentals:write"}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			fc.mu.Lock()
			fc.manifest = m
			fc.mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "rentals", "scope": scopes[parts[2]],
				"user": map[string]any{"id": 1, "sub": parts[1] + "@example.com", "email": parts[1] + "@example.com", "name": parts[1], "role": "member"},
				"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(fc.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = fc.URL, "key" }), fc
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

// item finds the row with that title.
func item(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

// repair adds a repair the way the app's own page does.
func (a *app) repair(tok, pid, uid, title string, more map[string]any) map[string]any {
	a.t.Helper()
	body := map[string]any{"property_id": n(pid), "title": title}
	if uid != "" {
		body["unit_id"] = n(uid)
	}
	for k, v := range more {
		body[k] = v
	}
	return a.expect(201, tok, "POST", "/api/v1/requests", body)["request"].(map[string]any)
}

// ledger counts a person's charges and payments: what the routes here must never add to.
func (a *app) ledger(who string) (charges, payments int) {
	a.t.Helper()
	uid := a.ids[who]
	if err := a.st.DB.QueryRow(`SELECT COUNT(1) FROM charges WHERE user_id = ?`, uid).Scan(&charges); err != nil {
		a.t.Fatal(err)
	}
	if err := a.st.DB.QueryRow(`SELECT COUNT(1) FROM payments WHERE user_id = ?`, uid).Scan(&payments); err != nil {
		a.t.Fatal(err)
	}
	return charges, payments
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	pid, uid, lid := alicePlace(a)
	flat2 := a.expect(201, a.alice, "POST", "/api/v1/properties/"+pid+"/units", map[string]any{"name": "Flat 2", "notes": "top floor, north side"})
	u2 := id(flat2)
	if u, ok := flat2["unit"].(map[string]any); ok {
		u2 = id(u)
	}
	rq := a.repair(a.alice, pid, uid, "Tap drips", nil)
	tid := id(a.list(a.alice, "/api/v1/tenants")[0].(map[string]any))
	find := func(tok, q string) []map[string]any {
		t.Helper()
		return items(t, a.ok(tok, "GET", "/api/v1/pocket/search?q="+q, nil))
	}

	// the place leads what its name finds; its lease and the repair there follow
	got := find(a.alice, "linden")
	if len(got) != 3 || got[0]["kind"] != "place" || got[0]["id"] != "place:"+pid || got[0]["title"] != "Linden Court" || got[0]["url"] != "/properties/"+pid ||
		got[0]["snippet"] != "1 of 2 let" || got[1]["kind"] != "lease" || got[1]["id"] != "lease:"+lid || got[1]["url"] != "/leases/"+lid ||
		got[1]["title"] != "Flat 1, Linden Court — Ana Marić" || !strings.HasPrefix(got[1]["snippet"].(string), "active · 500 EUR monthly") ||
		got[2]["kind"] != "repair" || got[2]["id"] != "repair:"+id(rq) {
		t.Fatalf("linden: %v", got)
	}
	// a unit by its own name or notes, opened on its place
	for _, q := range []string{"flat+2", "north+side"} {
		if got := find(a.alice, q); len(got) != 1 || got[0]["kind"] != "unit" || got[0]["id"] != "unit:"+u2 || got[0]["title"] != "Flat 2, Linden Court" ||
			got[0]["snippet"] != "Empty" || got[0]["url"] != "/properties/"+pid {
			t.Fatalf("%s: %v", q, got)
		}
	}
	got = find(a.alice, "flat")
	if len(got) != 4 || got[0]["id"] != "unit:"+uid || got[0]["snippet"] != "Ana Marić · 500 EUR monthly" || got[1]["id"] != "unit:"+u2 || got[2]["kind"] != "lease" || got[3]["kind"] != "repair" {
		t.Fatalf("flat: %v", got)
	}
	// a tenant before the lease that carries the name; contact details as /search shows them
	got = find(a.alice, "ana")
	if len(got) != 2 || got[0]["kind"] != "tenant" || got[0]["id"] != "tenant:"+tid || got[0]["title"] != "Ana Marić" || got[0]["snippet"] != "ana@example.com" ||
		got[0]["url"] != "/tenants/"+tid || got[1]["kind"] != "lease" {
		t.Fatalf("ana: %v", got)
	}
	if strings.Contains(got[1]["snippet"].(string), "ana@") {
		t.Fatalf("the lease shows the tenant's address: %v", got[1])
	}
	got = find(a.alice, "tap")
	if len(got) != 1 || got[0]["kind"] != "repair" || got[0]["title"] != "Tap drips" || got[0]["snippet"] != "Flat 1, Linden Court · new" || got[0]["date"] != today() ||
		got[0]["url"] != "/repairs/"+id(rq) {
		t.Fatalf("tap: %v", got)
	}
	// a place with one unit is one match, not two; its unit is still found by its own name
	hv := a.expect(201, a.alice, "POST", "/api/v1/properties", map[string]any{"name": "Apartment on the quay", "kind": "apartment"})["property"].(map[string]any)
	if got := find(a.alice, "apartment"); len(got) != 1 || got[0]["id"] != "place:"+id(hv) {
		t.Fatalf("a place and its only unit: %v", got)
	}
	a.ok(a.alice, "PUT", "/api/v1/properties/"+id(hv), map[string]any{"name": "Harbour View"})
	if got := find(a.alice, "apartment"); len(got) != 1 || got[0]["kind"] != "unit" || got[0]["title"] != "Apartment, Harbour View" || got[0]["url"] != "/properties/"+id(hv) {
		t.Fatalf("the only unit by its name: %v", got)
	}
	if got := find(a.alice, "flat&limit=1"); len(got) != 1 || got[0]["kind"] != "unit" {
		t.Fatalf("limit 1: %v", got)
	}
	// no words, or nothing that is a word
	for _, q := range []string{"", "%3F%3F", "*", "+"} {
		if got := find(a.alice, q); len(got) != 0 {
			t.Fatalf("q=%q: %v", q, got)
		}
	}
	// another person's places, units, tenants, leases and repairs never appear
	for _, q := range []string{"linden", "flat", "ana", "tap", "north"} {
		if got := find(a.bob, q); len(got) != 0 {
			t.Fatalf("bob finds alice's %q: %v", q, got)
		}
	}
	a.expect(401, "", "GET", "/api/v1/pocket/search?q=linden", nil)
	// the old route still answers as before: its own kinds, no units
	old := a.ok(a.alice, "GET", "/api/v1/search?q=linden", nil)["results"].([]any)
	if first := old[0].(map[string]any); len(old) != 3 || first["kind"] != "property" || first["path"] != "/properties/"+pid || first["url"] != "http://rentals.test/properties/"+pid ||
		first["subtitle"] != "1 of 2 let" || num(first, "id") != float64(n(pid)) {
		t.Fatalf("/search changed: %v", old)
	}
	if old := a.ok(a.alice, "GET", "/api/v1/search?q=flat+2", nil)["results"].([]any); len(old) != 0 {
		t.Fatalf("/search finds units now: %v", old)
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	pid, uid, lid := alicePlace(a)
	now := today()
	lease := a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)
	start := lease["lease"].(map[string]any)["start_date"].(string)
	dueToday := 0
	for _, c := range lease["charges"].([]any) {
		if c.(map[string]any)["due_date"] == now {
			dueToday++
		}
	}

	// the day the lease started: the deposit and the first rent were due, and nothing is paid
	day := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+start, nil)
	if day["date"] != start || day["summary"] != "2 payments due, 1 lease started" || len(items(t, day)) != 3 || day["metrics"] != nil {
		t.Fatalf("the first day: %v", day)
	}
	dep := item(t, day, "Deposit due")
	if dep["kind"] != "due" || dep["sub"] != "Flat 1, Linden Court · Ana Marić · 1,000 EUR · late" || dep["done"] != nil || dep["amount"] != nil || dep["url"] != "/leases/"+lid ||
		!strings.HasPrefix(dep["id"].(string), "charge:") {
		t.Fatalf("deposit: %v", dep)
	}
	if it := item(t, day, "Lease started"); it["kind"] != "lease" || it["id"] != "lease:"+lid || it["sub"] != "Flat 1, Linden Court · Ana Marić" || it["url"] != "/leases/"+lid {
		t.Fatalf("lease started: %v", it)
	}

	// today: rent comes in, one repair is reported, one reported and done, an inspection at half past ten
	a.expect(201, a.alice, "POST", "/api/v1/leases/"+lid+"/payments", map[string]any{"date": now, "amount": 130000, "note": "by bank", "money": false})
	tap := a.repair(a.alice, pid, uid, "Tap drips", nil)
	win := a.repair(a.alice, pid, "", "Broken window", map[string]any{"cost": 12000})
	a.ok(a.alice, "PUT", "/api/v1/requests/"+id(win), map[string]any{"status": "done"})
	insp := a.expect(201, a.alice, "POST", "/api/v1/inspections", map[string]any{"property_id": n(pid), "unit_id": n(uid), "kind": "routine", "date": now, "time": "10:30",
		"blank": true})["inspection"].(map[string]any)

	day = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+now, nil)
	want := "Rent received from Ana Marić"
	if dueToday > 0 {
		want += ", " + count(dueToday, "payment due", "payments due")
	}
	want += ", 1 repair reported, 1 repair done, 1 inspection"
	if day["summary"] != want || len(items(t, day)) != 4+dueToday {
		t.Fatalf("today: %v, want %q", day, want)
	}
	if ms := day["metrics"].([]any); len(ms) != 1 || ms[0].(map[string]any)["label"] != "Rent in" || ms[0].(map[string]any)["value"] != "1,300 EUR" {
		t.Fatalf("metrics: %v", ms)
	}
	pay := item(t, day, "Rent from Ana Marić")
	if pay["kind"] != "payment" || num(pay, "amount") != 130000 || pay["currency"] != "EUR" || pay["sub"] != "Flat 1, Linden Court" || pay["note"] != "by bank" ||
		pay["url"] != "/leases/"+lid || !strings.HasPrefix(pay["id"].(string), "payment:") {
		t.Fatalf("payment: %v", pay)
	}
	if it := item(t, day, "Repair reported: Tap drips"); it["kind"] != "repair" || it["id"] != "repair:"+id(tap) || it["sub"] != "Flat 1, Linden Court · by Ana Marić" ||
		it["done"] != nil || it["url"] != "/repairs/"+id(tap) {
		t.Fatalf("repair reported: %v", it)
	}
	if it := item(t, day, "Repair done: Broken window"); it["id"] != "repair:"+id(win) || it["sub"] != "Linden Court · 120 EUR · reported the same day" || it["done"] != true ||
		it["amount"] != nil {
		t.Fatalf("repair done: %v", it)
	}
	if it := item(t, day, "Inspection"); it["kind"] != "inspection" || it["id"] != "inspection:"+id(insp) || it["time"] != "10:30" || it["sub"] != "Flat 1, Linden Court" ||
		it["url"] != "/inspections/"+id(insp) {
		t.Fatalf("inspection: %v", it)
	}
	// the first day again: the deposit is paid now, the rent of that day in part
	day = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+start, nil)
	if dep := item(t, day, "Deposit due"); dep["sub"] != "Flat 1, Linden Court · Ana Marić · 1,000 EUR · paid" || dep["done"] != true {
		t.Fatalf("deposit paid: %v", dep)
	}
	part := 0
	for _, it := range items(t, day) {
		if it["kind"] == "due" && strings.HasSuffix(it["sub"].(string), " · 500 EUR · late") {
			part++
		}
	}
	if part != 1 {
		t.Fatalf("the first rent is still late: %v", day)
	}

	// a day with nothing, somebody else's day, a day that is not one
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2019-03-04", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("an empty day: %v", d)
	}
	for _, d := range []string{now, start} {
		if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/"+d+"?tz=Europe/Sarajevo", nil); d["summary"] != nil || len(items(t, d)) != 0 {
			t.Fatalf("bob sees alice's day: %v", d)
		}
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/yesterday", nil)
	a.expect(401, "", "GET", "/api/v1/pocket/day/"+now, nil)

	// ending the lease today shows as ended
	a.ok(a.alice, "POST", "/api/v1/leases/"+lid+"/end", map[string]any{"on": now, "reason": "moved"})
	day = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+now, nil)
	if it := item(t, day, "Lease ended"); it["kind"] != "lease" || it["done"] != true || !strings.HasSuffix(day["summary"].(string), ", 1 lease ended") {
		t.Fatalf("lease ended: %v", day)
	}
	// the old route still answers as before: both lines of the repair done the same day, no rent due
	old := a.ok(a.alice, "GET", "/api/v1/day/"+now, nil)["items"].([]any)
	if first := old[0].(map[string]any); len(old) != 6 || first["kind"] != "payment" || first["title"] != "Rent in: 1,300 EUR" ||
		first["subtitle"] != "Flat 1, Linden Court — Ana Marić" || first["url"] != "http://rentals.test/leases/"+lid {
		t.Fatalf("/day changed: %v", old)
	}
}

// A lease that runs out shows on its last day while that day is still to
// come, not as something that happened on a day gone by.
func TestPocketDayLeaseEnds(t *testing.T) {
	a := newApp(t, nil)
	_, _, lid := alicePlace(a)
	now := today()
	soon := store.AddDays(now, 10)
	if strings.HasSuffix(soon, "-01") { // not a rent day, to keep the day's line short
		soon = store.AddDays(now, 11)
	}
	a.ok(a.alice, "PUT", "/api/v1/leases/"+lid, map[string]any{"end_date": soon})
	day := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+soon, nil)
	if it := item(t, day, "Lease ends"); day["summary"] != "1 lease ends" || len(items(t, day)) != 1 || it["kind"] != "lease" || it["id"] != "lease:"+lid ||
		it["sub"] != "Flat 1, Linden Court · Ana Marić" || it["done"] != nil {
		t.Fatalf("lease ends: %v", day)
	}
	// a lease whose end day went by and that still runs
	p := a.expect(201, a.bob, "POST", "/api/v1/properties", map[string]any{"name": "Old flat", "kind": "apartment"})["property"].(map[string]any)
	u := p["units"].([]any)[0].(map[string]any)
	gone := store.AddDays(now, -40)
	if strings.HasSuffix(gone, "-01") {
		gone = store.AddDays(now, -41)
	}
	a.expect(201, a.bob, "POST", "/api/v1/leases", map[string]any{"unit_id": num(u, "id"), "start_date": monthsAgo(now, 6), "end_date": gone, "rent": 40000})
	for _, it := range items(t, a.ok(a.bob, "GET", "/api/v1/pocket/day/"+gone, nil)) {
		if it["kind"] == "lease" {
			t.Fatalf("a lease 'ends' on a day gone by: %v", it)
		}
	}
}

// The three reading routes read: rent that is not in the book yet is not
// made by them (the app's own pages and its morning run make it).
func TestPocketRoutesAddNoRent(t *testing.T) {
	a := newApp(t, nil)
	_, _, lid := alicePlace(a)
	if _, err := a.st.DB.Exec(`DELETE FROM charges WHERE lease_id = ? AND kind = 'rent' AND period = ?`, n(lid), today()[:8]+"01"); err != nil {
		t.Fatal(err)
	}
	a.s.forgetEnsured(a.ids["Alice"])
	charges, payments := a.ledger("Alice")
	for _, p := range []string{"/api/v1/pocket/search?q=linden", "/api/v1/pocket/day/" + today(), "/api/v1/pocket/day/" + today()[:8] + "01", "/api/v1/pocket/widget"} {
		a.ok(a.alice, "GET", p, nil)
	}
	if c, p := a.ledger("Alice"); c != charges || p != payments {
		t.Fatalf("reading made rent: %d charges (were %d), %d payments (were %d)", c, charges, p, payments)
	}
	// the app's own page makes it, as before
	a.ok(a.alice, "GET", "/api/v1/today", nil)
	if c, _ := a.ledger("Alice"); c != charges+1 {
		t.Fatalf("/today no longer makes the rent that is due: %d charges, were %d", c, charges)
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	// no place to look after: nothing to show
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || w["metrics"] != nil || w["url"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("without places: %v", w)
	}
	pid, uid, lid := alicePlace(a)
	now := today()
	a.repair(a.alice, pid, uid, "Tap drips", nil)
	heat := a.repair(a.alice, pid, "", "No heating", map[string]any{"priority": "urgent"})
	a.repair(a.alice, pid, "", "Paint the hall", map[string]any{"status": "done"})
	end := store.AddDays(now, 20)
	a.ok(a.alice, "PUT", "/api/v1/leases/"+lid, map[string]any{"end_date": end})

	// what the app's own Today page has
	page := a.ok(a.alice, "GET", "/api/v1/today", nil)
	nDue := len(page["due"].([]any))
	if len(page["late"].([]any)) == 0 || len(page["ending"].([]any)) != 1 || len(page["repairs"].([]any)) != 2 {
		t.Fatalf("today: %v", page)
	}
	lease := a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)["lease"].(map[string]any)
	overdue, start := int64(num(lease, "overdue")), lease["start_date"].(string)

	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	line := "1 rent late"
	if nDue > 0 {
		line += ", " + count(nDue, "rent due soon", "rents due soon")
	}
	line += ", 2 repairs open, 1 lease ending soon"
	if w["line"] != line || w["url"] != "/" {
		t.Fatalf("widget: %v, want %q", w, line)
	}
	ms := w["metrics"].([]any)
	for i, want := range [][2]string{{"Rent late", "1"}, {"Due soon", strconv.Itoa(nDue)}, {"Repairs open", "2"}, {"Let", "1 of 1"}} {
		if m := ms[i].(map[string]any); len(ms) != 4 || m["label"] != want[0] || m["value"] != want[1] {
			t.Fatalf("metrics: %v", ms)
		}
	}
	rows := items(t, w)
	if len(rows) != 4+nDue {
		t.Fatalf("rows: %v", rows)
	}
	// late rent first: one row for the lease, with all it owes late
	if r := rows[0]; r["title"] != "Flat 1, Linden Court" || r["detail"] != "Ana Marić · "+fmtMoney(overdue, "EUR")+" late · since "+shortDay(start, now) ||
		r["url"] != "/leases/"+lid || !strings.HasPrefix(r["id"].(string), "charge:") {
		t.Fatalf("late: %v", rows[0])
	}
	for _, r := range rows[1 : 1+nDue] {
		// the amount may be for part of a month: the lease ends in it
		if d := r["detail"].(string); r["title"] != "Flat 1, Linden Court" || !strings.HasPrefix(d, "Ana Marić · ") || !strings.Contains(d, " EUR · due ") || r["url"] != "/leases/"+lid {
			t.Fatalf("due soon: %v", r)
		}
	}
	rest := rows[1+nDue:]
	if rest[0]["title"] != "No heating" || rest[0]["detail"] != "Linden Court · urgent" || rest[0]["id"] != "repair:"+id(heat) || rest[0]["url"] != "/repairs/"+id(heat) ||
		rest[1]["title"] != "Tap drips" || rest[1]["detail"] != "Flat 1, Linden Court" {
		t.Fatalf("repairs, urgent first: %v", rest)
	}
	if r := rest[2]; r["title"] != "Flat 1, Linden Court" || r["detail"] != "Ana Marić · lease ends "+shortDay(end, now) || r["id"] != "lease:"+lid || r["url"] != "/leases/"+lid {
		t.Fatalf("ending: %v", r)
	}

	// more than fits: eight rows, and the lease that ends keeps its row
	for i := range 9 {
		a.repair(a.alice, pid, "", "Small job "+itoa(int64(i)), nil)
	}
	rows = items(t, a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil))
	if len(rows) != 8 || rows[0]["id"] == nil || rows[7]["id"] != "lease:"+lid || !strings.HasPrefix(rows[6]["id"].(string), "repair:") {
		t.Fatalf("eight rows: %v", rows)
	}

	// all paid: nothing is late any more
	a.expect(201, a.alice, "POST", "/api/v1/leases/"+lid+"/payments", map[string]any{"date": now, "amount": num(lease, "balance"), "money": false})
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if strings.Contains(w["line"].(string), "late") || w["metrics"].([]any)[0].(map[string]any)["value"] != "0" {
		t.Fatalf("after paying: %v", w)
	}
	// a place and nothing that needs you: the numbers, no line, no rows; nothing of alice's
	a.expect(201, a.bob, "POST", "/api/v1/properties", map[string]any{"name": "Bob's flat", "kind": "apartment"})
	w = a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil)
	if w["line"] != nil || len(items(t, w)) != 0 || len(w["metrics"].([]any)) != 4 || w["metrics"].([]any)[3].(map[string]any)["value"] != "0 of 1" ||
		w["metrics"].([]any)[2].(map[string]any)["value"] != "0" {
		t.Fatalf("bob's widget: %v", w)
	}
	a.expect(401, "", "GET", "/api/v1/pocket/widget", nil)
}

// capture posts a quick line and returns the answer.
func capture(a *app, tok, requestID, text string, more map[string]any) (int, map[string]any) {
	body := map[string]any{"request_id": requestID, "kind": "repair", "text": text}
	for k, v := range more {
		body[k] = v
	}
	code, m, _ := a.call(tok, "POST", "/api/v1/pocket/capture", body)
	return code, m
}

// repairsOf is every repair a person has.
func (a *app) repairsOf(tok string) []map[string]any {
	a.t.Helper()
	out := []map[string]any{}
	for _, r := range a.list(tok, "/api/v1/requests") {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestPocketCapture(t *testing.T) {
	a := newApp(t, nil)
	keep := func(tok, requestID, text string) map[string]any {
		t.Helper()
		code, m := capture(a, tok, requestID, text, nil)
		if code != 201 {
			t.Fatalf("capture %q → %d %v", text, code, m)
		}
		return a.ok(tok, "GET", "/api/v1/requests/"+strings.TrimPrefix(m["id"].(string), "repair:"), nil)["request"].(map[string]any)
	}
	refuse := func(tok, requestID, text, want string) {
		t.Helper()
		if code, m := capture(a, tok, requestID, text, nil); code != 400 || m["error"] != want {
			t.Fatalf("capture %q → %d %v, want 400 %q", text, code, m, want)
		}
	}
	refuse(a.alice, "req-0000000000000000", "Tap drips", "add a place in Pocket Rentals first")

	// one place: the line is a repair there, for the whole place
	pid, uid, lid := alicePlace(a)
	charges, payments := a.ledger("Alice")
	code, first := capture(a, a.alice, "req-0000000000000001", "tap drips in the kitchen", map[string]any{"due": "2026-01-05", "date": "2026-01-05"})
	rid := strings.TrimPrefix(first["id"].(string), "repair:")
	if code != 201 || first["title"] != "Linden Court: tap drips in the kitchen" || first["url"] != "/repairs/"+rid || first["id"] != "repair:"+rid {
		t.Fatalf("capture: %d %v", code, first)
	}
	view := a.ok(a.alice, "GET", "/api/v1/requests/"+rid, nil)
	got := view["request"].(map[string]any)
	if num(got, "property_id") != float64(n(pid)) || got["unit_id"] != nil || got["lease_id"] != nil || got["title"] != "tap drips in the kitchen" || got["status"] != "new" ||
		got["priority"] != "normal" || got["reported_on"] != today() || got["reported_by"] != "" || got["planned_on"] != "" || got["source_app"] != "" ||
		got["source_ref"] != "capture:req-0000000000000001" || num(got, "cost") != 0 || got["task_ref"] != "" {
		t.Fatalf("the repair: %v", got)
	}
	if tl := view["timeline"].([]any); len(tl) != 1 || tl[0].(map[string]any)["title"] != "Repair reported: tap drips in the kitchen" {
		t.Fatalf("its timeline: %v", view["timeline"])
	}
	// the same request again: the same answer, nothing added
	if code, again := capture(a, a.alice, "req-0000000000000001", "tap drips in the kitchen", nil); code != 201 || again["id"] != first["id"] || again["title"] != first["title"] ||
		again["url"] != first["url"] || len(a.repairsOf(a.alice)) != 1 {
		t.Fatalf("a retry: %d %v, repairs %v", code, again, a.repairsOf(a.alice))
	}

	// a place or a unit in front says where; a unit brings its lease along, as in the app
	for i, c := range []struct {
		text, title string
		unit        bool
	}{
		{"flat 1: boiler makes a noise", "Boiler makes a noise", true},
		{"Flat 1, Linden Court: door sticks", "Door sticks", true},
		{"LINDEN COURT,  flat 1 : window is stuck", "Window is stuck", true},
		{"Linden Court: plumber comes at 10:30", "Plumber comes at 10:30", false},
		{"Kitchen: tap drips", "Kitchen: tap drips", false},
		{"Flat 1:", "Flat 1:", false},
	} {
		r := keep(a.alice, "req-10000000000000"+itoa(int64(10+i)), c.text)
		if r["title"] != c.title || num(r, "property_id") != float64(n(pid)) || (r["unit_id"] != nil) != c.unit {
			t.Fatalf("%q → %q unit %v, want %q unit %v", c.text, r["title"], r["unit_id"], c.title, c.unit)
		}
		if c.unit && (num(r, "unit_id") != float64(n(uid)) || num(r, "lease_id") != float64(n(lid)) || r["reported_by"] != "Ana Marić") {
			t.Fatalf("%q did not find the unit's lease: %v", c.text, r)
		}
	}
	if r := keep(a.alice, "req-0000000000000003", "Flat 1: no hot water\nsince Monday,\nboiler shows E4"); r["title"] != "No hot water" || r["details"] != "since Monday,\nboiler shows E4" {
		t.Fatalf("more lines: %v", r)
	}
	long := strings.Repeat("ž", 250)
	if r := keep(a.alice, "req-0000000000000004", "Flat 1: "+long); len([]rune(r["title"].(string))) != 200 || !strings.HasSuffix(r["title"].(string), "…") || r["details"] != "Ž"+long[2:] {
		t.Fatalf("a long line: %v", r)
	}
	if code, m := capture(a, a.alice, "req-0000000000000005", "the notes", map[string]any{"title": "Flat 1: light is out"}); code != 201 || m["title"] != "Flat 1, Linden Court: Light is out" {
		t.Fatalf("a title and a text: %d %v", code, m)
	}

	// several places: the line has to say where
	river := a.expect(201, a.alice, "POST", "/api/v1/properties", map[string]any{"name": "River House", "kind": "building",
		"units": []map[string]any{{"name": "Flat 1"}, {"name": "Flat A"}}})["property"].(map[string]any)
	before := len(a.repairsOf(a.alice))
	refuse(a.alice, "req-0000000000000006", "Tap drips", `start with the place or unit, e.g. "Linden Court: tap drips"`)
	refuse(a.alice, "req-0000000000000007", "Kitchen: tap drips", `start with the place or unit, e.g. "Linden Court: tap drips"`)
	refuse(a.alice, "req-0000000000000008", "Flat 1: tap drips", `more than one of your units is called Flat 1; say the place too, e.g. "Flat 1, Linden Court: tap drips"`)
	if len(a.repairsOf(a.alice)) != before {
		t.Fatal("a refused line was kept")
	}
	if r := keep(a.alice, "req-0000000000000009", "Flat A: door sticks"); num(r, "property_id") != num(river, "id") || r["unit_name"] != "Flat A" || r["title"] != "Door sticks" {
		t.Fatalf("a unit only one place has: %v", r)
	}
	if r := keep(a.alice, "req-0000000000000010", "flat 1, river house: bell is broken"); num(r, "property_id") != num(river, "id") || r["unit_name"] != "Flat 1" || r["lease_id"] != nil ||
		r["reported_by"] != "" {
		t.Fatalf("unit and place: %v", r)
	}
	if r := keep(a.alice, "req-0000000000000011", "River House: roof leaks"); num(r, "property_id") != num(river, "id") || r["unit_id"] != nil || r["title"] != "Roof leaks" {
		t.Fatalf("a place: %v", r)
	}
	// a place put away no longer counts: the line goes to the one left
	a.ok(a.alice, "PUT", "/api/v1/properties/"+id(river), map[string]any{"archived": true})
	if r := keep(a.alice, "req-0000000000000012", "Gutter is full"); num(r, "property_id") != float64(n(pid)) {
		t.Fatalf("with one place left: %v", r)
	}

	// what the route checks itself
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "short", "text": "x"})
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000013", "kind": "payment", "text": "500"})
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000013", "text": " "})
	a.expect(401, "", "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000013", "text": "x"})

	// a repair is all a line becomes: no charge, no payment
	if c, p := a.ledger("Alice"); c != charges || p != payments {
		t.Fatalf("capture touched the rent: %d charges (were %d), %d payments (were %d)", c, charges, p, payments)
	}

	// per person: bob's line never reaches alice's places, a request id is his own
	refuse(a.bob, "req-0000000000000001", "Linden Court: tap drips", "add a place in Pocket Rentals first")
	a.expect(201, a.bob, "POST", "/api/v1/properties", map[string]any{"name": "Bob's flat", "kind": "apartment"})
	mine := len(a.repairsOf(a.alice))
	if r := keep(a.bob, "req-0000000000000001", "Linden Court: tap drips"); r["property_name"] != "Bob's flat" || r["title"] != "Linden Court: tap drips" {
		t.Fatalf("bob's line: %v", r)
	}
	if len(a.repairsOf(a.bob)) != 1 || len(a.repairsOf(a.alice)) != mine {
		t.Fatalf("bob's repairs %d, alice's %d (were %d)", len(a.repairsOf(a.bob)), len(a.repairsOf(a.alice)), mine)
	}
}

// A kept line tells the Core bus once, a retry not again.
func TestPocketCaptureEvent(t *testing.T) {
	cfg := testConfig(t.TempDir())
	cfg.CoreURL = "http://core.invalid"
	st := openTestStore(t, cfg)
	s := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	ctx := context.Background()
	if err := s.core.Init(ctx); err != nil {
		t.Fatal(err)
	}
	u := &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	tok, _, _ := st.CreateAPIToken(ctx, u.ID, "t")
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	a := &app{t: t, url: ts.URL, s: s, st: st}
	a.expect(201, tok, "POST", "/api/v1/properties", map[string]any{"name": "Old flat", "kind": "apartment"})
	for range 2 {
		if code, m := capture(a, tok, "req-0000000000000001", "Tap drips", nil); code != 201 {
			t.Fatalf("capture → %d %v", code, m)
		}
	}
	rows, err := st.DB.QueryContext(ctx, `SELECT body FROM pocket_outbox ORDER BY created_at, rowid`)
	if err != nil {
		t.Fatal(err)
	}
	var bodies [][]byte
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, b)
	}
	rows.Close()
	created := 0
	for _, b := range bodies {
		var ev struct {
			Topic   string
			Payload map[string]any
		}
		if err := json.Unmarshal(b, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Topic != "rentals.request_created" {
			continue
		}
		created++
		if ev.Payload["title"] != "Tap drips" || ev.Payload["place"] != "Old flat" || ev.Payload["status"] != "new" || ev.Payload["priority"] != "normal" ||
			ev.Payload["user_email"] != "ana@example.com" {
			t.Errorf("payload = %v", ev.Payload)
		}
	}
	if created != 1 {
		t.Fatalf("request_created events = %d, want 1", created)
	}
}

// A link token with one pocket:* scope reaches its one route and nothing
// else: above all, no rent, payment or charge.
func TestPocketLinkTokens(t *testing.T) {
	a, _ := newCapApp(t)
	pid, uid, lid := alicePlace(a)
	rq := a.repair(a.alice, pid, uid, "Tap drips", nil)
	now := today()
	line := func(requestID string) map[string]any {
		return map[string]any{"request_id": requestID, "kind": "repair", "text": "Door sticks"}
	}
	money := map[string]any{"date": now, "amount": 100, "money": false}
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=linden", nil, 200},
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/" + now, nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", line("req-0000000000000001"), 201},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/day/" + now, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/pocket/capture", line("req-0000000000000002"), 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=linden", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=linden", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/day/" + now, nil, 403},
		{"pcx_alice_widget", "POST", "/api/v1/pocket/capture", line("req-0000000000000003"), 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/search?q=linden", nil, 403},
		// the rest of the app
		{"pcx_alice_widget", "GET", "/api/v1/today", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/rent", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/search?q=linden", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/tenants", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/lookup?email=ana@example.com", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/day/" + now, nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/leases/" + lid, nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/requests", map[string]any{"property_id": n(pid), "title": "x"}, 403},
		{"pcx_alice_capture", "PUT", "/api/v1/requests/" + id(rq), map[string]any{"status": "done"}, 403},
		{"pcx_alice_capture", "DELETE", "/api/v1/requests/" + id(rq), nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/entries", map[string]any{"lease_id": n(lid), "title": "x"}, 403},
		{"pcx_alice_capture", "POST", "/api/v1/leases/" + lid + "/payments", money, 403},
		{"pcx_alice_capture", "POST", "/api/v1/leases/" + lid + "/charges", map[string]any{"title": "x", "amount": 100}, 403},
		{"pcx_alice_capture", "POST", "/api/v1/leases/" + lid + "/end", map[string]any{"on": now}, 403},
		{"pcx_alice_capture", "POST", "/api/v1/expenses", map[string]any{"property_id": n(pid), "amount": 100}, 403},
		{"pcx_alice_capture", "GET", "/api/v1/settings", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/export.json", nil, 403},
		// the app's own scopes do not open the new routes, and still open their own
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=linden", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/day/" + now, nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "POST", "/api/v1/pocket/capture", line("req-0000000000000004"), 403},
		{"pcx_alice_read", "GET", "/api/v1/search?q=linden", nil, 200},
		{"pcx_alice_read", "GET", "/api/v1/day/" + now, nil, 200},
		{"pcx_alice_write", "POST", "/api/v1/leases/" + lid + "/payments", money, 403},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	// the token acts for its person only
	if got := items(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=linden", nil)); len(got) == 0 || got[0]["title"] != "Linden Court" {
		t.Fatalf("alice through the link: %v", got)
	}
	if got := items(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=linden", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's place: %v", got)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); len(items(t, w)) != 0 || w["line"] != nil {
		t.Fatalf("bob's widget through the link: %v", w)
	}
	if d := a.ok("pcx_bob_day", "GET", "/api/v1/pocket/day/"+now, nil); len(items(t, d)) != 0 {
		t.Fatalf("bob's day through the link: %v", d)
	}
	if code, m := capture(a, "pcx_bob_capture", "req-0000000000000001", "Linden Court: door sticks", nil); code != 400 {
		t.Fatalf("bob has no place to put a repair: %d %v", code, m)
	}
	// only the one allowed capture landed, marked with the app that sent it; a retry adds nothing
	if code, m := capture(a, "pcx_alice_capture", "req-0000000000000001", "Door sticks", nil); code != 201 {
		t.Fatalf("retry through the link: %d %v", code, m)
	}
	var mine []map[string]any
	for _, r := range a.repairsOf(a.alice) {
		if r["title"] == "Door sticks" {
			mine = append(mine, r)
		}
	}
	if len(a.repairsOf(a.alice)) != 2 || len(mine) != 1 || mine[0]["source_app"] != "hub" || mine[0]["source_ref"] != "capture:req-0000000000000001" || mine[0]["status"] != "new" {
		t.Fatalf("alice's repairs: %v", a.repairsOf(a.alice))
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeWidget, req("GET", "/api/v1/today"), false},
		{core.ScopeCapture, req("GET", "/api/v1/pocket/capture"), false},
		{core.ScopeCapture, req("POST", "/api/v1/requests"), false},
		{core.ScopeCapture, req("POST", "/api/v1/leases/3/payments"), false},
		{core.ScopeCapture, req("POST", "/api/v1/leases/3/charges"), false},
		{"rentals:read", req("GET", "/api/v1/pocket/widget"), false},
		{"rentals:read", req("GET", "/api/v1/pocket/search"), false},
		{"rentals:write", req("POST", "/api/v1/pocket/capture"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Rentals tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, fc := newCapApp(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	m := fc.manifest
	fc.mu.Unlock()
	if m["slug"] != "rentals" || m["category"] != "Money" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	cap, _ := m["capture"].(map[string]any)
	if search == nil || day == nil || day["label"] != "Rentals" || day["one"] != "entry" || day["many"] != "entries" || widget == nil || widget["title"] != "Rentals" || cap == nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, cap)
	}
	if bangs := search["bangs"].([]any); len(bangs) != 2 || bangs[0] != "rentals" || bangs[1] != "rent" || len(search["kinds"].([]any)) != 5 {
		t.Fatalf("search: %v", search)
	}
	if k := cap["kinds"].([]any)[0].(map[string]any); len(cap["kinds"].([]any)) != 1 || k["kind"] != "repair" || k["label"] != "Repair" || k["fields"] != nil {
		t.Fatalf("capture kinds: %v", cap)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"rentals:read", "rentals:write", core.ScopeMoveExport, core.ScopeMoveImport, core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
}
