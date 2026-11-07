package store

import (
	"context"
	"database/sql"
	"strings"
)

const propertyCols = `id, user_id, name, kind, address, city, country, currency, value, bought_on, bought_for, home_ref, money_tag, color, notes, archived, position, client_key, created_at, updated_at`

func scanProperty(sc interface{ Scan(...any) error }) (*Property, error) {
	p := &Property{}
	var archived int
	err := sc.Scan(&p.ID, &p.UserID, &p.Name, &p.Kind, &p.Address, &p.City, &p.Country, &p.Currency, &p.Value, &p.BoughtOn, &p.BoughtFor,
		&p.HomeRef, &p.MoneyTag, &p.Color, &p.Notes, &archived, &p.Position, &p.ClientKey, &p.CreatedAt, &p.UpdatedAt)
	p.Archived = archived == 1
	return p, err
}

// ListProperties lists one person's places (archived ones too when asked).
func (s *Store) ListProperties(ctx context.Context, uid int64, archived bool) ([]*Property, error) {
	q := `SELECT ` + propertyCols + ` FROM properties WHERE user_id = ?`
	if !archived {
		q += ` AND archived = 0`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY archived, position, name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Property{}
	for rows.Next() {
		p, err := scanProperty(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProperty is one of the person's places.
func (s *Store) GetProperty(ctx context.Context, uid, id int64) (*Property, error) {
	p, err := scanProperty(s.DB.QueryRowContext(ctx, `SELECT `+propertyCols+` FROM properties WHERE user_id = ? AND id = ?`, uid, id))
	return p, notFound(err)
}

// PropertyByKey is the place brought in under an import key.
func (s *Store) PropertyByKey(ctx context.Context, uid int64, key string) (*Property, error) {
	p, err := scanProperty(s.DB.QueryRowContext(ctx, `SELECT `+propertyCols+` FROM properties WHERE user_id = ? AND client_key = ?`, uid, key))
	return p, notFound(err)
}

// PropertyByName finds a place by its name (case does not matter).
func (s *Store) PropertyByName(ctx context.Context, uid int64, name string) (*Property, error) {
	p, err := scanProperty(s.DB.QueryRowContext(ctx, `SELECT `+propertyCols+` FROM properties WHERE user_id = ? AND name = ? COLLATE NOCASE ORDER BY archived LIMIT 1`, uid, strings.TrimSpace(name)))
	return p, notFound(err)
}

// CreateProperty adds a place.
func (s *Store) CreateProperty(ctx context.Context, p *Property) error {
	var pos int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM properties WHERE user_id = ?`, p.UserID).Scan(&pos)
	p.Position = pos
	res, err := s.DB.ExecContext(ctx, `INSERT INTO properties (user_id, name, kind, address, city, country, currency, value, bought_on, bought_for, home_ref, money_tag, color, notes, archived, position, client_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, p.Name, p.Kind, p.Address, p.City, p.Country, p.Currency, p.Value, p.BoughtOn, p.BoughtFor, p.HomeRef, p.MoneyTag, p.Color, p.Notes, boolInt(p.Archived), p.Position, p.ClientKey)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	p.CreatedAt, p.UpdatedAt = Now(), Now()
	return nil
}

// UpdateProperty saves a place.
func (s *Store) UpdateProperty(ctx context.Context, p *Property) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE properties SET name = ?, kind = ?, address = ?, city = ?, country = ?, currency = ?, value = ?, bought_on = ?, bought_for = ?,
		home_ref = ?, money_tag = ?, color = ?, notes = ?, archived = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		p.Name, p.Kind, p.Address, p.City, p.Country, p.Currency, p.Value, p.BoughtOn, p.BoughtFor, p.HomeRef, p.MoneyTag, p.Color, p.Notes, boolInt(p.Archived), Now(), p.UserID, p.ID)
	return affected(res, err)
}

// DeleteProperty removes a place with everything in it.
func (s *Store) DeleteProperty(ctx context.Context, uid, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, q := range []string{
		`DELETE FROM files WHERE user_id = ? AND owner = 'item' AND owner_id IN (SELECT ii.id FROM inspection_items ii JOIN inspections i ON i.id = ii.inspection_id WHERE i.property_id = ?)`,
		`DELETE FROM files WHERE user_id = ? AND owner = 'inspection' AND owner_id IN (SELECT id FROM inspections WHERE property_id = ?)`,
		`DELETE FROM files WHERE user_id = ? AND owner = 'request' AND owner_id IN (SELECT id FROM requests WHERE property_id = ?)`,
		`DELETE FROM files WHERE user_id = ? AND owner = 'lease' AND owner_id IN (SELECT l.id FROM leases l JOIN units u ON u.id = l.unit_id WHERE u.property_id = ?)`,
		`DELETE FROM files WHERE user_id = ? AND owner = 'property' AND owner_id = ?`,
		`DELETE FROM made WHERE user_id = ?1 AND ((owner = 'property' AND owner_id = ?2) OR (owner = 'lease' AND owner_id IN (SELECT l.id FROM leases l JOIN units u ON u.id = l.unit_id WHERE u.property_id = ?2))
			OR (owner = 'request' AND owner_id IN (SELECT id FROM requests WHERE property_id = ?2)) OR (owner = 'inspection' AND owner_id IN (SELECT id FROM inspections WHERE property_id = ?2)))`,
	} {
		if _, err := tx.ExecContext(ctx, q, uid, id); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM properties WHERE user_id = ? AND id = ?`, uid, id)
	if err := affected(res, err); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- units ----

const unitCols = `u.id, u.user_id, u.property_id, u.name, u.bedrooms, u.bathrooms, u.area_m2, u.rent, u.deposit, u.notes, u.position, u.archived, u.created_at, p.name`

func scanUnit(sc interface{ Scan(...any) error }) (*Unit, error) {
	u := &Unit{}
	var archived int
	err := sc.Scan(&u.ID, &u.UserID, &u.PropertyID, &u.Name, &u.Bedrooms, &u.Bathrooms, &u.AreaM2, &u.Rent, &u.Deposit, &u.Notes, &u.Position, &archived, &u.CreatedAt, &u.PropertyName)
	u.Archived = archived == 1
	return u, err
}

// ListUnits lists a person's units (of one place, or all with propertyID 0).
func (s *Store) ListUnits(ctx context.Context, uid, propertyID int64) ([]*Unit, error) {
	q := `SELECT ` + unitCols + ` FROM units u JOIN properties p ON p.id = u.property_id WHERE u.user_id = ?`
	args := []any{uid}
	if propertyID > 0 {
		q += ` AND u.property_id = ?`
		args = append(args, propertyID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY p.position, u.position, u.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Unit{}
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetUnit is one of the person's units.
func (s *Store) GetUnit(ctx context.Context, uid, id int64) (*Unit, error) {
	u, err := scanUnit(s.DB.QueryRowContext(ctx, `SELECT `+unitCols+` FROM units u JOIN properties p ON p.id = u.property_id WHERE u.user_id = ? AND u.id = ?`, uid, id))
	return u, notFound(err)
}

// UnitByName finds a unit of a place by name.
func (s *Store) UnitByName(ctx context.Context, uid, propertyID int64, name string) (*Unit, error) {
	u, err := scanUnit(s.DB.QueryRowContext(ctx, `SELECT `+unitCols+` FROM units u JOIN properties p ON p.id = u.property_id WHERE u.user_id = ? AND u.property_id = ? AND u.name = ? COLLATE NOCASE LIMIT 1`, uid, propertyID, strings.TrimSpace(name)))
	return u, notFound(err)
}

// CreateUnit adds a unit to a place.
func (s *Store) CreateUnit(ctx context.Context, u *Unit) error {
	var pos int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM units WHERE user_id = ? AND property_id = ?`, u.UserID, u.PropertyID).Scan(&pos)
	u.Position = pos
	res, err := s.DB.ExecContext(ctx, `INSERT INTO units (user_id, property_id, name, bedrooms, bathrooms, area_m2, rent, deposit, notes, position, archived) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.UserID, u.PropertyID, u.Name, u.Bedrooms, u.Bathrooms, u.AreaM2, u.Rent, u.Deposit, u.Notes, u.Position, boolInt(u.Archived))
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	u.CreatedAt = Now()
	return nil
}

// UpdateUnit saves a unit.
func (s *Store) UpdateUnit(ctx context.Context, u *Unit) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE units SET name = ?, bedrooms = ?, bathrooms = ?, area_m2 = ?, rent = ?, deposit = ?, notes = ?, archived = ? WHERE user_id = ? AND id = ?`,
		u.Name, u.Bedrooms, u.Bathrooms, u.AreaM2, u.Rent, u.Deposit, u.Notes, boolInt(u.Archived), u.UserID, u.ID)
	return affected(res, err)
}

// DeleteUnit removes a unit (and its leases).
func (s *Store) DeleteUnit(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM units WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
