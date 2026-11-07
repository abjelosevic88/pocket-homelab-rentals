package server

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- your rentals, out and in ----

// exportDoc is one person's whole book as JSON. Bringing it in again adds
// nothing twice: every row carries a key.
type exportDoc struct {
	App         string             `json:"app"`
	Version     string             `json:"version"`
	Exported    string             `json:"exported_at"`
	Properties  []exportProperty   `json:"properties"`
	Tenants     []exportTenant     `json:"tenants"`
	Leases      []exportLease      `json:"leases"`
	Requests    []exportRequest    `json:"requests"`
	Expenses    []exportExpense    `json:"expenses"`
	Inspections []exportInspection `json:"inspections"`
}

type exportProperty struct {
	store.Property
	Key   string        `json:"key"`
	Units []*store.Unit `json:"units"`
}

type exportTenant struct {
	store.Tenant
	Key string `json:"key"`
}

type exportLease struct {
	store.Lease
	Key         string           `json:"key"`
	PropertyKey string           `json:"property_key"`
	Unit        string           `json:"unit"`
	TenantKeys  []string         `json:"tenant_keys"`
	Charges     []*store.Charge  `json:"charges"`
	Payments    []*store.Payment `json:"payments"`
	Timeline    []*store.Entry   `json:"timeline"`
}

type exportRequest struct {
	store.Request
	Key         string `json:"key"`
	PropertyKey string `json:"property_key"`
	Unit        string `json:"unit"`
	LeaseKey    string `json:"lease_key"`
}

type exportExpense struct {
	store.Expense
	Key         string `json:"key"`
	PropertyKey string `json:"property_key"`
	Unit        string `json:"unit"`
}

type exportInspection struct {
	store.Inspection
	PropertyKey string `json:"property_key"`
	Unit        string `json:"unit"`
	LeaseKey    string `json:"lease_key"`
}

func keyOf(own string, kind string, id int64) string {
	return firstNonEmpty(own, fmt.Sprintf("rentals:%s:%d", kind, id))
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-rentals", Version: Version, Exported: store.Now(), Properties: []exportProperty{}, Tenants: []exportTenant{}, Leases: []exportLease{},
		Requests: []exportRequest{}, Expenses: []exportExpense{}, Inspections: []exportInspection{}}
	props, _ := s.store.ListProperties(ctx, uid, true)
	pkey := map[int64]string{}
	for _, p := range props {
		k := keyOf(p.ClientKey, "property", p.ID)
		pkey[p.ID] = k
		units, _ := s.store.ListUnits(ctx, uid, p.ID)
		doc.Properties = append(doc.Properties, exportProperty{Property: *p, Key: k, Units: units})
	}
	units, _ := s.store.ListUnits(ctx, uid, 0)
	uname := map[int64]string{}
	for _, u := range units {
		uname[u.ID] = u.Name
	}
	ts, _ := s.store.ListTenants(ctx, uid)
	tkey := map[int64]string{}
	for _, t := range ts {
		k := keyOf(t.ClientKey, "tenant", t.ID)
		tkey[t.ID] = k
		doc.Tenants = append(doc.Tenants, exportTenant{Tenant: *t, Key: k})
	}
	ls, _ := s.store.ListLeases(ctx, uid, store.LeaseFilter{})
	lkey := map[int64]string{}
	for _, l := range ls {
		k := keyOf(l.ClientKey, "lease", l.ID)
		lkey[l.ID] = k
		el := exportLease{Lease: *l, Key: k, PropertyKey: pkey[l.PropertyID], Unit: l.UnitName, TenantKeys: []string{}}
		for _, t := range l.Tenants {
			el.TenantKeys = append(el.TenantKeys, tkey[t.ID])
		}
		el.Tenants = nil
		el.Charges, _ = s.store.ListCharges(ctx, uid, l.ID)
		el.Payments, _ = s.store.ListPayments(ctx, uid, l.ID, "", "")
		el.Timeline, _ = s.store.ListTimeline(ctx, uid, store.TimelineFilter{LeaseID: l.ID, Limit: 5000})
		doc.Leases = append(doc.Leases, el)
	}
	rs, _ := s.store.ListRequests(ctx, uid, store.RequestFilter{})
	for _, r := range rs {
		er := exportRequest{Request: *r, Key: firstNonEmpty(r.SourceRef, fmt.Sprintf("rentals:request:%d", r.ID)), PropertyKey: pkey[r.PropertyID], Unit: r.UnitName}
		if r.LeaseID != nil {
			er.LeaseKey = lkey[*r.LeaseID]
		}
		doc.Requests = append(doc.Requests, er)
	}
	es, _ := s.store.ListExpenses(ctx, uid, 0, "", "")
	for _, e := range es {
		ee := exportExpense{Expense: *e, Key: keyOf(e.ClientKey, "expense", e.ID), PropertyKey: pkey[e.PropertyID]}
		if e.UnitID != nil {
			ee.Unit = uname[*e.UnitID]
		}
		doc.Expenses = append(doc.Expenses, ee)
	}
	is, _ := s.store.ListInspections(ctx, uid, 0, 0)
	for _, i := range is {
		full, err := s.store.GetInspection(ctx, uid, i.ID)
		if err != nil {
			continue
		}
		ei := exportInspection{Inspection: *full, PropertyKey: pkey[i.PropertyID], Unit: i.UnitName}
		if i.LeaseID != nil {
			ei.LeaseKey = lkey[*i.LeaseID]
		}
		doc.Inspections = append(doc.Inspections, ei)
	}
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-rentals-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

var csvHead = []string{"Property", "Address", "Unit", "Tenant", "Email", "Phone", "Rent", "Currency", "Frequency", "Due day", "Start", "End", "Deposit", "Status", "Owed now"}

// handleExportCSV is every lease in one table (a spreadsheet opens it, and it comes back in the same way).
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, err := s.loadBook(ctx, actorID(ctx), true)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-rentals-leases.csv"`)
	_, _ = w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHead)
	for _, l := range b.leases {
		p := b.propByID[l.PropertyID]
		addr := ""
		if p != nil {
			addr = joinNonEmpty(", ", p.Address, p.City)
		}
		var names, emails, phones []string
		for _, t := range l.Tenants {
			names, emails, phones = append(names, t.Name), append(emails, t.Email), append(phones, t.Phone)
		}
		_ = cw.Write([]string{l.PropertyName, addr, l.UnitName, strings.Join(names, "; "), strings.Join(emails, "; "), strings.Join(phones, "; "),
			fmt.Sprintf("%.2f", float64(l.Rent)/100), l.Currency, l.Frequency, fmt.Sprint(l.DueDay), l.StartDate, firstNonEmpty(l.EndedOn, l.EndDate),
			fmt.Sprintf("%.2f", float64(l.Deposit)/100), l.Status, fmt.Sprintf("%.2f", float64(max(l.Balance, 0))/100)})
	}
	cw.Flush()
}

// handleImport takes a Pocket Rentals export (again and again: nothing is
// doubled), or a spreadsheet of leases or of rent payments (CSV).
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		if data, err = io.ReadAll(f); err != nil {
			writeErr(w, http.StatusBadRequest, "could not read the file")
			return
		}
	} else {
		b, err := s.readBody(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		data = b
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		var doc exportDoc
		if err := json.Unmarshal(data, &doc); err != nil || doc.App != "pocket-rentals" {
			writeErr(w, http.StatusBadRequest, "that is not a Pocket Rentals export")
			return
		}
		writeJSON(w, http.StatusOK, s.importPocket(ctx, uid, doc))
		return
	}
	out, err := s.importCSV(ctx, uid, data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.forgetEnsured(uid)
	writeJSON(w, http.StatusOK, out)
}

// importPocket brings back a Pocket Rentals export.
func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) map[string]any {
	n := map[string]int{"properties": 0, "units": 0, "tenants": 0, "leases": 0, "payments": 0, "requests": 0, "expenses": 0, "inspections": 0, "skipped": 0}
	props := map[string]*store.Property{}
	for _, ep := range doc.Properties {
		p, err := s.store.PropertyByKey(ctx, uid, ep.Key)
		if err != nil {
			np := ep.Property
			np.ID, np.UserID, np.ClientKey = 0, uid, ep.Key
			if np.Currency == "" {
				np.Currency = "EUR"
			}
			if !oneOf(np.Kind, store.PropertyKinds) {
				np.Kind = "other"
			}
			if err := s.store.CreateProperty(ctx, &np); err != nil {
				n["skipped"]++
				continue
			}
			p = &np
			n["properties"]++
		}
		props[ep.Key] = p
		for _, u := range ep.Units {
			if _, err := s.store.UnitByName(ctx, uid, p.ID, u.Name); err == nil {
				continue
			}
			nu := *u
			nu.ID, nu.UserID, nu.PropertyID = 0, uid, p.ID
			if s.store.CreateUnit(ctx, &nu) == nil {
				n["units"]++
			}
		}
	}
	unitOf := func(pk, name string) *store.Unit {
		p := props[pk]
		if p == nil {
			return nil
		}
		u, err := s.store.UnitByName(ctx, uid, p.ID, name)
		if err != nil {
			return nil
		}
		return u
	}
	tenants := map[string]int64{}
	for _, et := range doc.Tenants {
		var t *store.Tenant
		if row := s.store.DB.QueryRowContext(ctx, `SELECT id FROM tenants WHERE user_id = ? AND client_key = ?`, uid, et.Key); row != nil {
			var id int64
			if row.Scan(&id) == nil {
				t, _ = s.store.GetTenant(ctx, uid, id)
			}
		}
		if t == nil && et.Email != "" {
			t, _ = s.store.TenantByEmail(ctx, uid, et.Email)
		}
		if t == nil {
			nt := et.Tenant
			nt.ID, nt.UserID, nt.ClientKey = 0, uid, et.Key
			if err := s.store.CreateTenant(ctx, &nt); err != nil {
				n["skipped"]++
				continue
			}
			t = &nt
			n["tenants"]++
		}
		tenants[et.Key] = t.ID
	}
	leases := map[string]int64{}
	for _, el := range doc.Leases {
		if l, err := s.store.LeaseByKey(ctx, uid, el.Key); err == nil {
			leases[el.Key] = l.ID
			continue
		}
		u := unitOf(el.PropertyKey, el.Unit)
		if u == nil {
			n["skipped"]++
			continue
		}
		nl := el.Lease
		nl.ID, nl.UserID, nl.UnitID, nl.ClientKey = 0, uid, u.ID, el.Key
		if err := s.store.CreateLease(ctx, &nl); err != nil {
			n["skipped"]++
			continue
		}
		n["leases"]++
		leases[el.Key] = nl.ID
		var ids []int64
		for _, k := range el.TenantKeys {
			if id := tenants[k]; id > 0 {
				ids = append(ids, id)
			}
		}
		_ = s.store.SetLeaseTenants(ctx, uid, nl.ID, ids)
		for _, c := range el.Charges {
			nc := *c
			nc.ID, nc.UserID, nc.LeaseID = 0, uid, nl.ID
			if nc.Period == "" {
				nc.Period = "import:" + itoa(c.ID)
			}
			_, _ = s.store.AddCharge(ctx, &nc)
		}
		for _, p := range el.Payments {
			np := *p
			np.ID, np.UserID, np.LeaseID = 0, uid, nl.ID
			np.ClientKey = firstNonEmpty(p.ClientKey, fmt.Sprintf("%s:payment:%d", el.Key, p.ID))
			if s.store.AddPayment(ctx, &np) == nil {
				n["payments"]++
			}
		}
		for i := len(el.Timeline) - 1; i >= 0; i-- {
			e := *el.Timeline[i]
			e.ID, e.UserID, e.LeaseID, e.PropertyID, e.RequestID = 0, uid, &nl.ID, nil, nil
			_ = s.store.AddEntry(ctx, &e)
		}
	}
	for _, er := range doc.Requests {
		if _, err := s.store.RequestBySource(ctx, uid, er.Key); err == nil {
			continue
		}
		p := props[er.PropertyKey]
		if p == nil {
			n["skipped"]++
			continue
		}
		nr := er.Request
		nr.ID, nr.UserID, nr.PropertyID, nr.SourceRef, nr.UnitID, nr.LeaseID = 0, uid, p.ID, er.Key, nil, nil
		nr.TaskRef, nr.EventRef, nr.HomeRecordRef = "", "", ""
		if u := unitOf(er.PropertyKey, er.Unit); u != nil {
			nr.UnitID = &u.ID
		}
		if id := leases[er.LeaseKey]; id > 0 {
			nr.LeaseID = &id
		}
		if s.store.CreateRequest(ctx, &nr) == nil {
			n["requests"]++
		}
	}
	for _, ee := range doc.Expenses {
		p := props[ee.PropertyKey]
		if p == nil {
			n["skipped"]++
			continue
		}
		ne := ee.Expense
		ne.ID, ne.UserID, ne.PropertyID, ne.ClientKey, ne.UnitID, ne.RequestID = 0, uid, p.ID, ee.Key, nil, nil
		if u := unitOf(ee.PropertyKey, ee.Unit); u != nil {
			ne.UnitID = &u.ID
		}
		if s.store.AddExpense(ctx, &ne) == nil {
			n["expenses"]++
		}
	}
	for _, ei := range doc.Inspections {
		p := props[ei.PropertyKey]
		if p == nil {
			continue
		}
		var exists int
		_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM inspections WHERE user_id = ? AND property_id = ? AND date = ? AND kind = ?`, uid, p.ID, ei.Date, ei.Kind).Scan(&exists)
		if exists > 0 {
			continue
		}
		ni := ei.Inspection
		ni.ID, ni.UserID, ni.PropertyID, ni.UnitID, ni.LeaseID, ni.EventRef = 0, uid, p.ID, nil, nil, ""
		if u := unitOf(ei.PropertyKey, ei.Unit); u != nil {
			ni.UnitID = &u.ID
		}
		if id := leases[ei.LeaseKey]; id > 0 {
			ni.LeaseID = &id
		}
		if s.store.CreateInspection(ctx, &ni) == nil {
			items := make([]*store.InspectionItem, 0, len(ei.Items))
			for _, it := range ei.Items {
				items = append(items, &store.InspectionItem{Room: it.Room, Item: it.Item, Condition: it.Condition, Note: it.Note})
			}
			_ = s.store.SetItems(ctx, uid, ni.ID, items)
			n["inspections"]++
		}
	}
	s.forgetEnsured(uid)
	return map[string]any{"added": n}
}

// ---- spreadsheets ----

var csvColumns = map[string][]string{
	"property":  {"property", "place", "building", "house", "name of property", "property name"},
	"address":   {"address", "street", "location"},
	"city":      {"city", "town"},
	"unit":      {"unit", "flat", "apartment", "apt", "room", "unit name"},
	"tenant":    {"tenant", "tenants", "tenant name", "name", "renter", "who"},
	"email":     {"email", "e-mail", "tenant email", "email address"},
	"phone":     {"phone", "mobile", "telephone", "tenant phone"},
	"rent":      {"rent", "monthly rent", "rent amount", "rent (monthly)"},
	"currency":  {"currency", "cur"},
	"frequency": {"frequency", "period", "paid", "rent period"},
	"due":       {"due day", "due", "rent due", "day due", "pay day"},
	"start":     {"start", "start date", "lease start", "from", "move in", "move-in"},
	"end":       {"end", "end date", "lease end", "to", "until", "move out", "move-out"},
	"deposit":   {"deposit", "security deposit", "bond"},
	"status":    {"status", "state"},
	"from":      {"charge from", "charges from", "rent from"},
	"date":      {"date", "paid on", "payment date", "day"},
	"amount":    {"amount", "paid", "sum", "payment"},
	"note":      {"note", "notes", "memo", "description"},
}

func csvDate(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) >= 10 {
		if _, err := time.Parse("2006-01-02", v[:10]); err == nil {
			return v[:10]
		}
	}
	for _, f := range []string{"02.01.2006", "2.1.2006", "02/01/2006", "1/2/2006", "01/02/2006", "Jan 2, 2006", "2 Jan 2006"} {
		part := v
		if i := strings.IndexAny(v, " T"); i > 0 && !strings.Contains(f, " ") {
			part = v[:i]
		}
		if t, err := time.Parse(f, part); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// parseAmount reads "1.200,50 €", "about 300", "€2,500" as cents (0 = none).
func parseAmount(v string) int64 {
	var digits strings.Builder
	seenDigit := false
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
			seenDigit = true
		case (r == '.' || r == ',') && seenDigit:
			digits.WriteRune(r)
		case seenDigit && r != ' ' && r != '\u00a0':
			goto done
		}
	}
done:
	s := digits.String()
	if s == "" {
		return 0
	}
	last := strings.LastIndexAny(s, ".,")
	if last >= 0 && len(s)-last-1 == 3 {
		s = strings.NewReplacer(".", "", ",", "").Replace(s)
	} else if last >= 0 {
		s = strings.NewReplacer(".", "", ",", "").Replace(s[:last]) + "." + s[last+1:]
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	return int64(f*100 + 0.5)
}

func (s *Server) importCSV(ctx context.Context, uid int64, data []byte) (map[string]any, error) {
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	head := data[:min(len(data), 4096)]
	if bytes.Count(head, []byte(";")) > bytes.Count(head, []byte(",")) {
		rd.Comma = ';'
	} else if bytes.Count(head, []byte("\t")) > bytes.Count(head, []byte(",")) {
		rd.Comma = '\t'
	}
	recs, err := rd.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil, errors.New("this does not look like a spreadsheet (a header row and at least one line)")
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		lh := strings.ToLower(strings.TrimSpace(strings.Trim(h, "\ufeff\"")))
		for k, names := range csvColumns {
			if _, ok := idx[k]; !ok && oneOf(lh, names) {
				idx[k] = i
			}
		}
	}
	_, hasRent := idx["rent"]
	_, hasAmount := idx["amount"]
	_, hasDate := idx["date"]
	if !hasRent && hasAmount && hasDate {
		return s.importPayments(ctx, uid, recs[1:], idx)
	}
	_, hasProp := idx["property"]
	_, hasAddr := idx["address"]
	if !hasRent || (!hasProp && !hasAddr) {
		return nil, errors.New("the first row needs columns for the place (Property or Address) and the Rent — or Date and Amount for payments")
	}
	st := s.settingsOf(ctx, uid)
	n := map[string]int{"properties": 0, "units": 0, "tenants": 0, "leases": 0, "skipped": 0}
	firstOfMonth := today()[:8] + "01"
	for _, rec := range recs[1:] {
		get := func(k string) string {
			if i, ok := idx[k]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		pname := firstNonEmpty(get("property"), get("address"))
		rent := parseAmount(get("rent"))
		if pname == "" || rent <= 0 {
			n["skipped"]++
			continue
		}
		cur := st.Currency
		if c := strings.ToUpper(get("currency")); reCur.MatchString(c) {
			cur = c
		}
		p, err := s.store.PropertyByName(ctx, uid, pname)
		if err != nil {
			p = &store.Property{UserID: uid, Name: clip(pname, 120), Kind: "apartment", Address: clip(get("address"), 300), City: clip(get("city"), 100), Currency: cur}
			if get("unit") == "" {
				p.Kind = "house"
			}
			if err := s.store.CreateProperty(ctx, p); err != nil {
				n["skipped"]++
				continue
			}
			n["properties"]++
		}
		uname := firstNonEmpty(get("unit"), defaultUnit(p.Kind), "Whole place")
		u, err := s.store.UnitByName(ctx, uid, p.ID, uname)
		if err != nil {
			u = &store.Unit{UserID: uid, PropertyID: p.ID, Name: clip(uname, 120), Rent: rent}
			if err := s.store.CreateUnit(ctx, u); err != nil {
				n["skipped"]++
				continue
			}
			n["units"]++
		}
		start := csvDate(get("start"))
		if start == "" {
			start = firstOfMonth
		}
		sum := sha1.Sum([]byte(strings.ToLower(strings.Join([]string{pname, uname, get("tenant"), get("email"), start}, "\x1f"))))
		key := "csv:" + hex.EncodeToString(sum[:])[:16]
		if _, err := s.store.LeaseByKey(ctx, uid, key); err == nil {
			n["skipped"]++
			continue
		}
		var tids []int64
		names, emails, phones := splitList(get("tenant")), splitList(get("email")), splitList(get("phone"))
		for i := 0; i < max(len(names), len(emails)); i++ {
			ti := tenantIn{}
			if i < len(names) {
				ti.Name = &names[i]
			}
			if i < len(emails) && reEmail.MatchString(emails[i]) {
				ti.Email = &emails[i]
			}
			if i < len(phones) {
				ti.Phone = &phones[i]
			}
			before, _ := s.store.ListTenants(ctx, uid)
			t, err := s.makeTenant(ctx, uid, ti)
			if err != nil {
				continue
			}
			if after, _ := s.store.ListTenants(ctx, uid); len(after) > len(before) {
				n["tenants"]++
			}
			tids = append(tids, t.ID)
		}
		freq := strings.ToLower(get("frequency"))
		switch {
		case strings.HasPrefix(freq, "week"):
			freq = "weekly"
		case strings.HasPrefix(freq, "quart"):
			freq = "quarterly"
		case strings.HasPrefix(freq, "year"), strings.HasPrefix(freq, "annual"):
			freq = "yearly"
		default:
			freq = "monthly"
		}
		due := st.DueDay
		if d := parseAmount(get("due")) / 100; d >= 1 && d <= 28 {
			due = int(d)
		}
		end := csvDate(get("end"))
		status := store.LeaseActive
		if ls := strings.ToLower(get("status")); strings.Contains(ls, "end") || strings.Contains(ls, "past") || strings.Contains(ls, "former") || (end != "" && end < today()) {
			status = store.LeaseEnded
		} else if strings.Contains(ls, "draft") {
			status = store.LeaseDraft
		}
		// rent before this month is not owed again: charges start now, unless the sheet says otherwise
		from := csvDate(get("from"))
		if from == "" && start < firstOfMonth {
			from = firstOfMonth
		}
		dep := parseAmount(get("deposit"))
		l := &store.Lease{UserID: uid, UnitID: u.ID, Status: status, StartDate: start, EndDate: end, ChargesFrom: from, Rent: rent, Currency: cur, Frequency: freq, DueDay: due,
			GraceDays: st.GraceDays, NoticeDays: st.NoticeDays, Deposit: dep, DepositState: "none", ClientKey: key}
		if dep > 0 {
			l.DepositState = "held"
		}
		if status == store.LeaseEnded {
			l.EndedOn = firstNonEmpty(end, today())
		}
		if msg := s.clash(ctx, l); msg != "" {
			l.Status = store.LeaseDraft
		}
		if err := s.store.CreateLease(ctx, l); err != nil {
			n["skipped"]++
			continue
		}
		_ = s.store.SetLeaseTenants(ctx, uid, l.ID, tids)
		lid := l.ID
		_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, LeaseID: &lid, Kind: "lease", Title: "Brought in from a spreadsheet"})
		n["leases"]++
	}
	return map[string]any{"added": n}, nil
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == '|' || r == '/' || r == '&' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// importPayments takes rent payments from a spreadsheet: each line finds its
// lease by the tenant's email or name, or the place.
func (s *Server) importPayments(ctx context.Context, uid int64, recs [][]string, idx map[string]int) (map[string]any, error) {
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		return nil, err
	}
	n := map[string]int{"payments": 0, "skipped": 0, "unmatched": 0}
	for _, rec := range recs {
		get := func(k string) string {
			if i, ok := idx[k]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		date, amount := csvDate(get("date")), parseAmount(get("amount"))
		if date == "" || amount <= 0 {
			n["skipped"]++
			continue
		}
		who, email, where := strings.ToLower(get("tenant")), strings.ToLower(get("email")), strings.ToLower(firstNonEmpty(get("unit"), get("property"), get("address")))
		var pick *store.Lease
		for _, l := range b.leases {
			if l.Status == store.LeaseDraft || l.StartDate > date || (l.EndedOn != "" && l.EndedOn < store.AddDays(date, -60)) {
				continue
			}
			hit := false
			for _, t := range l.Tenants {
				if (email != "" && strings.EqualFold(t.Email, email)) || (who != "" && strings.EqualFold(t.Name, who)) {
					hit = true
				}
			}
			if !hit && where != "" && strings.Contains(strings.ToLower(l.Title), where) {
				hit = true
			}
			if hit {
				pick = l
				break
			}
		}
		if pick == nil {
			n["unmatched"]++
			continue
		}
		sum := sha1.Sum([]byte(strings.Join(rec, "\x1f")))
		p := &store.Payment{UserID: uid, LeaseID: pick.ID, Date: date, Amount: amount, Currency: pick.Currency, Method: "bank", Note: clip(get("note"), 500),
			ClientKey: "csv:" + hex.EncodeToString(sum[:])[:16]}
		if err := s.store.AddPayment(ctx, p); err != nil {
			n["skipped"]++
			continue
		}
		n["payments"]++
	}
	return map[string]any{"added": n}, nil
}
