package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- places and units ----

type unitIn struct {
	Name      *string  `json:"name"`
	Bedrooms  *int     `json:"bedrooms"`
	Bathrooms *int     `json:"bathrooms"`
	AreaM2    *float64 `json:"area_m2"`
	Rent      *int64   `json:"rent"`
	Deposit   *int64   `json:"deposit"`
	Notes     *string  `json:"notes"`
	Archived  *bool    `json:"archived"`
}

func (in unitIn) apply(u *store.Unit) error {
	trimP(&u.Name, in.Name)
	trimP(&u.Notes, in.Notes)
	if in.Bedrooms != nil {
		u.Bedrooms = max(*in.Bedrooms, 0)
	}
	if in.Bathrooms != nil {
		u.Bathrooms = max(*in.Bathrooms, 0)
	}
	if in.AreaM2 != nil {
		u.AreaM2 = max(*in.AreaM2, 0)
	}
	if in.Rent != nil {
		u.Rent = max(*in.Rent, 0)
	}
	if in.Deposit != nil {
		u.Deposit = max(*in.Deposit, 0)
	}
	if in.Archived != nil {
		u.Archived = *in.Archived
	}
	u.Name = clip(u.Name, 120)
	if u.Name == "" {
		return &userErr{"give the unit a name, like “Flat 2” or “Room 1”"}
	}
	return nil
}

type propertyIn struct {
	Name      *string  `json:"name"`
	Kind      *string  `json:"kind"`
	Address   *string  `json:"address"`
	City      *string  `json:"city"`
	Country   *string  `json:"country"`
	Currency  *string  `json:"currency"`
	Value     *int64   `json:"value"`
	BoughtOn  *string  `json:"bought_on"`
	BoughtFor *int64   `json:"bought_for"`
	HomeRef   *string  `json:"home_ref"`
	MoneyTag  *string  `json:"money_tag"`
	Color     *string  `json:"color"`
	Notes     *string  `json:"notes"`
	Archived  *bool    `json:"archived"`
	Units     []unitIn `json:"units"`
}

func (in propertyIn) apply(p *store.Property) error {
	trimP(&p.Name, in.Name)
	trimP(&p.Kind, in.Kind)
	trimP(&p.Address, in.Address)
	trimP(&p.City, in.City)
	trimP(&p.Country, in.Country)
	trimP(&p.BoughtOn, in.BoughtOn)
	trimP(&p.HomeRef, in.HomeRef)
	trimP(&p.MoneyTag, in.MoneyTag)
	trimP(&p.Color, in.Color)
	trimP(&p.Notes, in.Notes)
	if in.Currency != nil {
		p.Currency = cleanCurrency(*in.Currency)
	}
	if in.Value != nil {
		p.Value = max(*in.Value, 0)
	}
	if in.BoughtFor != nil {
		p.BoughtFor = max(*in.BoughtFor, 0)
	}
	if in.Archived != nil {
		p.Archived = *in.Archived
	}
	if p.Name == "" {
		p.Name = p.Address
	}
	p.Name, p.Address, p.MoneyTag = clip(p.Name, 120), clip(p.Address, 300), strings.ToLower(clip(strings.TrimPrefix(p.MoneyTag, "#"), 40))
	switch {
	case p.Name == "":
		return &userErr{"give the place a name or an address"}
	case !oneOf(p.Kind, store.PropertyKinds):
		return &userErr{"a place is a house, an apartment, a building, a room, commercial, a garage or something else"}
	case p.Currency == "":
		return &userErr{"the currency is a three-letter code like EUR"}
	case !validDate(p.BoughtOn):
		return &userErr{"the day you bought it is YYYY-MM-DD"}
	case p.Color != "" && !reHex.MatchString(p.Color):
		return &userErr{"the colour is like #1d6fa5"}
	}
	return nil
}

// defaultUnit is what a new place is let as when no units are given.
func defaultUnit(kind string) string {
	switch kind {
	case "building":
		return ""
	case "apartment":
		return "Apartment"
	case "room":
		return "Room"
	case "garage":
		return "Garage"
	case "commercial":
		return "Premises"
	}
	return "Whole place"
}

func (s *Server) handleListProperties(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, err := s.loadBook(ctx, actorID(ctx), queryBool(r, "archived"))
	if err != nil {
		s.fail(w, err, "places")
		return
	}
	writeJSON(w, http.StatusOK, b.props)
}

func (s *Server) handleCreateProperty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in propertyIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := &store.Property{UserID: uid, Kind: "apartment", Currency: s.settingsOf(ctx, uid).Currency}
	if err := in.apply(p); err != nil {
		s.userFail(w, err, "place")
		return
	}
	if err := s.store.CreateProperty(ctx, p); err != nil {
		s.fail(w, err, "place")
		return
	}
	units := in.Units
	if len(units) == 0 {
		if n := defaultUnit(p.Kind); n != "" {
			units = []unitIn{{Name: &n}}
		}
	}
	for _, ui := range units {
		u := &store.Unit{UserID: uid, PropertyID: p.ID}
		if err := ui.apply(u); err != nil {
			continue
		}
		_ = s.store.CreateUnit(ctx, u)
	}
	pid := p.ID
	_ = s.store.AddEntry(ctx, &store.Entry{UserID: uid, PropertyID: &pid, Kind: "note", Title: "Place added"})
	s.respondProperty(w, r, p.ID, http.StatusCreated)
}

// respondProperty answers with a place as its page shows it.
func (s *Server) respondProperty(w http.ResponseWriter, r *http.Request, id int64, status int) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.loadBook(ctx, uid, true)
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	p := b.propByID[id]
	if p == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var leases []*store.Lease
	for _, l := range b.leases {
		if l.PropertyID == id {
			leases = append(leases, l)
		}
	}
	if leases == nil {
		leases = []*store.Lease{}
	}
	var reqs []*store.Request
	for _, rq := range b.requests {
		if rq.PropertyID == id {
			reqs = append(reqs, rq)
		}
	}
	if reqs == nil {
		reqs = []*store.Request{}
	}
	year := today()[:4]
	exps, _ := s.store.ListExpenses(ctx, uid, id, year+"-01-01", year+"-12-31")
	var income, costs int64
	for _, l := range leases {
		for _, pm := range b.payments[l.ID] {
			if strings.HasPrefix(pm.Date, year) && pm.Currency == p.Currency {
				income += pm.Amount
			}
		}
	}
	for _, e := range exps {
		if e.Currency == p.Currency {
			costs += e.Amount
		}
	}
	insp, _ := s.store.ListInspections(ctx, uid, id, 0)
	made, _ := s.store.ListMade(ctx, uid, "property", id)
	files, _ := s.store.ListFiles(ctx, uid, "property", id)
	home := ""
	if p.HomeRef != "" {
		home = s.siblingLink(ctx, uid, appHome, "/homes/"+p.HomeRef)
	}
	writeJSON(w, status, map[string]any{"property": p, "leases": leases, "requests": reqs, "inspections": insp, "made": made, "files": files,
		"year": map[string]any{"year": year, "income": income, "expenses": costs, "net": income - costs, "currency": p.Currency}, "home_url": home})
}

func (s *Server) handleGetProperty(w http.ResponseWriter, r *http.Request) {
	s.respondProperty(w, r, idParam(r, "id"), http.StatusOK)
}

func (s *Server) handleUpdateProperty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetProperty(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	var in propertyIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(p); err != nil {
		s.userFail(w, err, "place")
		return
	}
	if err := s.store.UpdateProperty(ctx, p); err != nil {
		s.fail(w, err, "place")
		return
	}
	s.respondProperty(w, r, p.ID, http.StatusOK)
}

func (s *Server) handleDeleteProperty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	files, _ := s.store.ListFiles(ctx, uid, "property", id)
	if err := s.store.DeleteProperty(ctx, uid, id); err != nil {
		s.fail(w, err, "place")
		return
	}
	for _, f := range files {
		s.removeFile(uid, f.ID)
	}
	s.cleanFiles(ctx)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListUnits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, err := s.loadBook(ctx, actorID(ctx), false)
	if err != nil {
		s.fail(w, err, "units")
		return
	}
	out := []*store.Unit{}
	for _, u := range b.units {
		if p := b.propByID[u.PropertyID]; p != nil && !u.Archived {
			out = append(out, u)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUnit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetProperty(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	var in unitIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u := &store.Unit{UserID: uid, PropertyID: p.ID}
	if err := in.apply(u); err != nil {
		s.userFail(w, err, "unit")
		return
	}
	if err := s.store.CreateUnit(ctx, u); err != nil {
		s.fail(w, err, "unit")
		return
	}
	u.PropertyName = p.Name
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleUpdateUnit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	u, err := s.store.GetUnit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "unit")
		return
	}
	var in unitIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(u); err != nil {
		s.userFail(w, err, "unit")
		return
	}
	if err := s.store.UpdateUnit(ctx, u); err != nil {
		s.fail(w, err, "unit")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleDeleteUnit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetUnit(ctx, uid, id); err != nil {
		s.fail(w, err, "unit")
		return
	}
	if ls, _ := s.store.ListLeases(ctx, uid, store.LeaseFilter{UnitID: id}); len(ls) > 0 {
		writeErr(w, http.StatusConflict, "this unit has leases: archive it instead (its history stays)")
		return
	}
	if err := s.store.DeleteUnit(ctx, uid, id); err != nil {
		s.fail(w, err, "unit")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
