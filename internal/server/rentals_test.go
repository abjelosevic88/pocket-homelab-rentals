package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// alicePlace makes a place with one flat and an active lease for Alice.
func alicePlace(a *app) (prop, unit, lease string) {
	p := a.expect(201, a.alice, "POST", "/api/v1/properties", map[string]any{"name": "Linden Court", "kind": "building", "currency": "EUR",
		"units": []map[string]any{{"name": "Flat 1", "rent": 50000, "deposit": 100000}}})
	pr := p["property"].(map[string]any)
	units := pr["units"].([]any)
	u := units[0].(map[string]any)
	start := store.AddDays(today()[:8]+"01", 0)
	start = monthsAgo(start, 2)
	l := a.expect(201, a.alice, "POST", "/api/v1/leases", map[string]any{"unit_id": num(u, "id"), "start_date": start,
		"tenants": []map[string]any{{"name": "Ana Marić", "email": "ana@example.com"}}})
	return id(pr), id(u), id(l["lease"].(map[string]any))
}

func monthsAgo(d string, n int) string {
	t, _ := store.ParseDate(d)
	return t.AddDate(0, -n, 0).Format("2006-01-02")
}

func TestRentFlow(t *testing.T) {
	a := newApp(t, nil)
	_, _, lid := alicePlace(a)
	l := a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)
	lease := l["lease"].(map[string]any)
	// two months back + this month (+ maybe next) of rent, and the deposit
	charges := l["charges"].([]any)
	if len(charges) < 4 {
		t.Fatalf("charges = %d, want rent for 3+ months and the deposit", len(charges))
	}
	if num(lease, "rent") != 50000 || lease["deposit_state"] != "due" {
		t.Fatalf("lease took the unit's rent and deposit: %v", lease)
	}
	owed := int64(num(lease, "balance"))
	if owed != 100000+3*50000 {
		t.Fatalf("balance = %d, want deposit + 3 rents", owed)
	}
	// pay the deposit and two rents: one rent left
	a.expect(201, a.alice, "POST", "/api/v1/leases/"+lid+"/payments", map[string]any{"date": today(), "amount": 200000, "money": false})
	l = a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)
	lease = l["lease"].(map[string]any)
	if num(lease, "balance") != 50000 || lease["deposit_state"] != "held" {
		t.Fatalf("after paying: balance %v, deposit %v", lease["balance"], lease["deposit_state"])
	}
	// the rent month view shows this month's rent
	rent := a.ok(a.alice, "GET", "/api/v1/rent?month="+today()[:7], nil)
	if len(rent["items"].([]any)) == 0 {
		t.Fatal("this month's rent is missing")
	}
	// ending the lease today stops rent after today
	a.ok(a.alice, "POST", "/api/v1/leases/"+lid+"/end", map[string]any{"on": today(), "reason": "moved"})
	l = a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)
	if l["lease"].(map[string]any)["status"] != "ended" {
		t.Fatal("not ended")
	}
	for _, c := range l["charges"].([]any) {
		if cm := c.(map[string]any); cm["kind"] == "rent" && cm["period"].(string) > today() {
			t.Fatalf("rent after the end: %v", cm)
		}
	}
	// deleting rent is refused (it would come back): waive instead
	var rentID string
	for _, c := range l["charges"].([]any) {
		if cm := c.(map[string]any); cm["kind"] == "rent" {
			rentID = id(cm)
		}
	}
	a.expect(400, a.alice, "DELETE", "/api/v1/charges/"+rentID, nil)
	a.ok(a.alice, "PUT", "/api/v1/charges/"+rentID, map[string]any{"waived": true})
}

func TestLeaseClash(t *testing.T) {
	a := newApp(t, nil)
	_, uid, _ := alicePlace(a)
	a.expect(400, a.alice, "POST", "/api/v1/leases", map[string]any{"unit_id": n(uid), "start_date": today(), "rent": 1000})
	a.expect(201, a.alice, "POST", "/api/v1/leases", map[string]any{"unit_id": n(uid), "start_date": today(), "rent": 1000, "status": "draft"})
}

// Everything is per person: Bob sees none of Alice's rentals, and every id of hers is a 404 for him.
func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	pid, uid, lid := alicePlace(a)
	rq := a.expect(201, a.alice, "POST", "/api/v1/requests", map[string]any{"property_id": n(pid), "unit_id": n(uid), "title": "Tap drips"})["request"].(map[string]any)
	in := a.expect(201, a.alice, "POST", "/api/v1/inspections", map[string]any{"property_id": n(pid), "unit_id": n(uid), "kind": "move_in", "date": today()})["inspection"].(map[string]any)
	ex := a.expect(201, a.alice, "POST", "/api/v1/expenses", map[string]any{"property_id": n(pid), "amount": 4200, "category": "insurance", "money": false})["expense"].(map[string]any)
	tens := a.list(a.alice, "/api/v1/tenants")
	tid := id(tens[0].(map[string]any))
	fid := upload(a, a.alice, "lease", lid)

	for _, p := range []string{"/api/v1/properties", "/api/v1/leases", "/api/v1/tenants", "/api/v1/requests", "/api/v1/inspections", "/api/v1/expenses", "/api/v1/units"} {
		if l := a.list(a.bob, p); len(l) != 0 {
			t.Fatalf("Bob sees %d in %s", len(l), p)
		}
	}
	for _, p := range []string{"/api/v1/properties/" + pid, "/api/v1/leases/" + lid, "/api/v1/tenants/" + tid, "/api/v1/requests/" + id(rq), "/api/v1/inspections/" + id(in), "/api/v1/files/" + fid} {
		a.expect(404, a.bob, "GET", p, nil)
		a.expect(404, a.bob, "DELETE", p, nil)
	}
	a.expect(404, a.bob, "PUT", "/api/v1/expenses/"+id(ex), map[string]any{"amount": 1})
	a.expect(404, a.bob, "POST", "/api/v1/leases/"+lid+"/payments", map[string]any{"date": today(), "amount": 100})
	a.expect(404, a.bob, "POST", "/api/v1/properties/"+pid+"/units", map[string]any{"name": "Mine"})
	a.expect(400, a.bob, "POST", "/api/v1/leases", map[string]any{"unit_id": n(uid), "start_date": today(), "rent": 100})
	a.expect(400, a.bob, "POST", "/api/v1/requests", map[string]any{"property_id": n(pid), "title": "x"})
	a.expect(404, a.bob, "POST", "/api/v1/files?owner=lease&owner_id="+lid, nil)
	if l := a.ok(a.bob, "GET", "/api/v1/search?q=linden", nil)["results"].([]any); len(l) != 0 {
		t.Fatal("Bob finds Alice's place")
	}
	if a.ok(a.bob, "GET", "/api/v1/lookup?email=ana@example.com", nil)["tenant"] != nil {
		t.Fatal("Bob looks up Alice's tenant")
	}
	// Alice still has everything
	a.ok(a.alice, "GET", "/api/v1/files/"+fid, nil)
}

func upload(a *app, tok, owner, ownerID string) string {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="lease.pdf"`}
	h["Content-Type"] = []string{"application/pdf"}
	fw, _ := mw.CreatePart(h)
	_, _ = fw.Write([]byte("%PDF-1.4 test"))
	mw.Close()
	req, _ := http.NewRequest("POST", a.url+"/api/v1/files?owner="+owner+"&owner_id="+ownerID, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	if res.StatusCode != 201 {
		a.t.Fatalf("upload → %d %v", res.StatusCode, m)
	}
	return id(m)
}

func TestRepairDone(t *testing.T) {
	a := newApp(t, nil)
	pid, uid, lid := alicePlace(a)
	rq := a.expect(201, a.alice, "POST", "/api/v1/requests", map[string]any{"property_id": n(pid), "unit_id": n(uid), "title": "Broken window", "cost": 12000, "tenant_pays": true})["request"].(map[string]any)
	if num(rq, "lease_id") != float64(n(lid)) || rq["reported_by"] != "Ana Marić" {
		t.Fatalf("the repair found the lease and the tenant: %v", rq)
	}
	out := a.ok(a.alice, "PUT", "/api/v1/requests/"+id(rq), map[string]any{"status": "done"})
	if out["expense"] == nil {
		t.Fatal("no expense for the done repair")
	}
	l := a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)
	found := false
	for _, c := range l["charges"].([]any) {
		if c.(map[string]any)["kind"] == "repair" {
			found = true
		}
	}
	if !found {
		t.Fatal("the tenant was not charged for the repair")
	}
}

func TestInspectionCompare(t *testing.T) {
	a := newApp(t, nil)
	pid, uid, lid := alicePlace(a)
	in := a.expect(201, a.alice, "POST", "/api/v1/inspections", map[string]any{"property_id": n(pid), "lease_id": n(lid), "kind": "move_in", "date": monthsAgo(today(), 1)})["inspection"].(map[string]any)
	items := in["items"].([]any)
	if len(items) < 10 {
		t.Fatalf("checklist has %d lines", len(items))
	}
	first := items[0].(map[string]any)
	first["condition"] = "good"
	a.ok(a.alice, "PUT", "/api/v1/inspections/"+id(in)+"/items", items)
	a.ok(a.alice, "PUT", "/api/v1/inspections/"+id(in), map[string]any{"status": "done"})
	out := a.expect(201, a.alice, "POST", "/api/v1/inspections", map[string]any{"property_id": n(pid), "unit_id": n(uid), "lease_id": n(lid), "kind": "move_out", "date": today(), "from_id": num(in, "id")})
	if out["before"] == nil {
		t.Fatal("moving out is not compared with moving in")
	}
	_ = upload(a, a.alice, "item", id(out["inspection"].(map[string]any)["items"].([]any)[0].(map[string]any)))
}

func TestLinkScopes(t *testing.T) {
	r := func(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }
	read, write := []string{"rentals:read"}, []string{"rentals:write"}
	cases := []struct {
		scopes []string
		req    *http.Request
		want   bool
	}{
		{read, r("GET", "/api/v1/leases"), true},
		{read, r("GET", "/api/v1/leases/3"), true},
		{read, r("GET", "/api/v1/lookup"), true},
		{read, r("GET", "/api/v1/day/2026-11-07"), true},
		{read, r("POST", "/api/v1/requests"), false},
		{write, r("POST", "/api/v1/requests"), true},
		{write, r("PUT", "/api/v1/requests/4"), true},
		{write, r("POST", "/api/v1/entries"), true},
		{write, r("GET", "/api/v1/leases"), false},
		{read, r("POST", "/api/v1/leases/3/payments"), false},
		{append(read, write...), r("DELETE", "/api/v1/properties/1"), false},
		{nil, r("GET", "/api/v1/auth/me"), true},
		{read, r("GET", "/api/v1/settings"), false},
		{read, r("GET", "/api/v1/export.json"), false},
	}
	for _, c := range cases {
		if got := linkAllows(c.scopes, c.req); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.req.Method, c.req.URL.Path, got, c.want)
		}
	}
}

func TestExportImportTwice(t *testing.T) {
	a := newApp(t, nil)
	_, _, lid := alicePlace(a)
	a.expect(201, a.alice, "POST", "/api/v1/leases/"+lid+"/payments", map[string]any{"date": today(), "amount": 30000, "money": false})
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	res.Body.Close()
	if doc["app"] != "pocket-rentals" {
		t.Fatalf("export: %v", doc["app"])
	}
	// into Bob's empty account, twice: the same once
	for i := 0; i < 2; i++ {
		a.ok(a.bob, "POST", "/api/v1/import", doc)
	}
	if l := a.list(a.bob, "/api/v1/leases"); len(l) != 1 {
		t.Fatalf("Bob has %d leases after two imports", len(l))
	}
	if l := a.list(a.bob, "/api/v1/properties"); len(l) != 1 {
		t.Fatalf("Bob has %d places after two imports", len(l))
	}
	bl := a.list(a.bob, "/api/v1/leases")[0].(map[string]any)
	al := a.ok(a.alice, "GET", "/api/v1/leases/"+lid, nil)["lease"].(map[string]any)
	if num(bl, "balance") != num(al, "balance") {
		t.Fatalf("balance %v after import, %v before", bl["balance"], al["balance"])
	}
	// and Alice's own data is untouched
	if l := a.list(a.alice, "/api/v1/leases"); len(l) != 1 {
		t.Fatal("Alice's leases changed")
	}
}

func TestImportCSVLeases(t *testing.T) {
	a := newApp(t, nil)
	csv := "Property,Unit,Tenant,Email,Rent,Start,Deposit\nRiver House,Flat A,Ivan Horvat,ivan@example.com,\"450,00\",2024-03-01,900\nRiver House,Flat B,Eva Kos,eva@example.com,500,2025-01-15,\n"
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import", strings.NewReader(csv))
		req.Header.Set("Authorization", "Bearer "+a.alice)
		req.Header.Set("Content-Type", "text/csv")
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("import %d: %v %v", i, err, res.StatusCode)
		}
		res.Body.Close()
	}
	ls := a.list(a.alice, "/api/v1/leases")
	if len(ls) != 2 {
		t.Fatalf("%d leases, want 2", len(ls))
	}
	for _, x := range ls {
		l := x.(map[string]any)
		// brought in: no rent owed from before this month
		if num(l, "balance") > num(l, "rent") {
			t.Fatalf("an imported lease owes old rent: %v", l["balance"])
		}
	}
	if p := a.list(a.alice, "/api/v1/properties"); len(p) != 1 {
		t.Fatalf("%d places, want 1", len(p))
	}
}

func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "BAM", "due_day": 5, "money_book": true})
	if s := a.ok(a.bob, "GET", "/api/v1/settings", nil); s["currency"] != "EUR" || num(s, "due_day") != 1 || s["money_book"] != false {
		t.Fatalf("Bob got Alice's settings: %v %v %v", s["currency"], s["due_day"], s["money_book"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"due_day": 31})
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(t.Context(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	today := a.ok(a.alice, "GET", "/api/v1/today", nil)
	if len(today["late"].([]any)) == 0 || len(today["ending"].([]any)) == 0 || len(today["repairs"].([]any)) == 0 || len(today["vacant"].([]any)) == 0 {
		t.Fatalf("demo Today is missing something: late %d ending %d repairs %d vacant %d", len(today["late"].([]any)), len(today["ending"].([]any)), len(today["repairs"].([]any)), len(today["vacant"].([]any)))
	}
	rep := a.ok(a.alice, "GET", "/api/v1/reports", nil)
	if len(rep["properties"].([]any)) != 4 {
		t.Fatal("reports")
	}
}

func TestNoMadeUpLateFees(t *testing.T) {
	a := newApp(t, nil)
	p := a.expect(201, a.alice, "POST", "/api/v1/properties", map[string]any{"name": "Old flat", "kind": "apartment"})["property"].(map[string]any)
	u := p["units"].([]any)[0].(map[string]any)
	l := a.expect(201, a.alice, "POST", "/api/v1/leases", map[string]any{"unit_id": num(u, "id"), "start_date": monthsAgo(today(), 6), "rent": 40000, "late_fee": 2000})["lease"].(map[string]any)
	for _, c := range a.ok(a.alice, "GET", "/api/v1/leases/"+id(l), nil)["charges"].([]any) {
		if c.(map[string]any)["kind"] == "late_fee" {
			t.Fatal("a late fee for rent that was late before the lease was here")
		}
	}
}

func TestRepairRefOnce(t *testing.T) {
	a := newApp(t, nil)
	pid, _, _ := alicePlace(a)
	body := map[string]any{"property_id": n(pid), "title": "Leak", "ref": "pocket-flows:7:2"}
	first := a.expect(201, a.alice, "POST", "/api/v1/requests", body)["request"].(map[string]any)
	again := a.expect(200, a.alice, "POST", "/api/v1/requests", body)["request"].(map[string]any)
	if id(first) != id(again) || len(a.list(a.alice, "/api/v1/requests")) != 1 {
		t.Fatal("the same ref made two repairs")
	}
}
