package store

import (
	"context"
	"database/sql"
)

const entryCols = `id, user_id, lease_id, property_id, request_id, kind, title, body, at, app, url, source_app, created_at`

func scanEntry(sc interface{ Scan(...any) error }) (*Entry, error) {
	e := &Entry{}
	var lease, prop, req sql.NullInt64
	err := sc.Scan(&e.ID, &e.UserID, &lease, &prop, &req, &e.Kind, &e.Title, &e.Body, &e.At, &e.App, &e.URL, &e.SourceApp, &e.CreatedAt)
	e.LeaseID, e.PropertyID, e.RequestID = nullPtr(lease), nullPtr(prop), nullPtr(req)
	return e, err
}

// TimelineFilter picks whose timeline.
type TimelineFilter struct {
	LeaseID, PropertyID, RequestID int64
	From, To                       string // at, YYYY-MM-DD…
	Limit                          int
}

// ListTimeline lists what happened, newest first.
func (s *Store) ListTimeline(ctx context.Context, uid int64, f TimelineFilter) ([]*Entry, error) {
	q := `SELECT ` + entryCols + ` FROM timeline WHERE user_id = ?`
	args := []any{uid}
	if f.LeaseID > 0 {
		q += ` AND lease_id = ?`
		args = append(args, f.LeaseID)
	}
	if f.PropertyID > 0 {
		q += ` AND (property_id = ? OR lease_id IN (SELECT l.id FROM leases l JOIN units u ON u.id = l.unit_id WHERE u.property_id = ?))`
		args = append(args, f.PropertyID, f.PropertyID)
	}
	if f.RequestID > 0 {
		q += ` AND request_id = ?`
		args = append(args, f.RequestID)
	}
	if f.From != "" {
		q += ` AND at >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND at < ?`
		args = append(args, f.To)
	}
	q += ` ORDER BY at DESC, id DESC`
	if f.Limit <= 0 {
		f.Limit = 100
	}
	q += ` LIMIT ?`
	args = append(args, f.Limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddEntry writes a line on a timeline.
func (s *Store) AddEntry(ctx context.Context, e *Entry) error {
	if e.At == "" {
		e.At = Now()
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO timeline (user_id, lease_id, property_id, request_id, kind, title, body, at, app, url, source_app) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, nullI(e.LeaseID), nullI(e.PropertyID), nullI(e.RequestID), e.Kind, e.Title, e.Body, e.At, e.App, e.URL, e.SourceApp)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// GetEntry is one line.
func (s *Store) GetEntry(ctx context.Context, uid, id int64) (*Entry, error) {
	e, err := scanEntry(s.DB.QueryRowContext(ctx, `SELECT `+entryCols+` FROM timeline WHERE user_id = ? AND id = ?`, uid, id))
	return e, notFound(err)
}

// DeleteEntry removes a line.
func (s *Store) DeleteEntry(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM timeline WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// ---- made: things in the other apps ----

const madeCols = `id, user_id, owner, owner_id, target, app, ref, title, url, state, error, checked_at, created_at`

func scanMade(sc interface{ Scan(...any) error }) (*Made, error) {
	m := &Made{}
	err := sc.Scan(&m.ID, &m.UserID, &m.Owner, &m.OwnerID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.State, &m.Error, &m.CheckedAt, &m.CreatedAt)
	return m, err
}

func (s *Store) madeQuery(ctx context.Context, q string, args ...any) ([]*Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+madeCols+` FROM made WHERE `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Made{}
	for rows.Next() {
		m, err := scanMade(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMade lists what one thing has in the other apps.
func (s *Store) ListMade(ctx context.Context, uid int64, owner string, ownerID int64) ([]*Made, error) {
	return s.madeQuery(ctx, `user_id = ? AND owner = ? AND owner_id = ? ORDER BY id DESC`, uid, owner, ownerID)
}

// AddMade remembers something made elsewhere.
func (s *Store) AddMade(ctx context.Context, uid int64, m *Made) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO made (user_id, owner, owner_id, target, app, ref, title, url, state, error, checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, m.Owner, m.OwnerID, m.Target, m.App, m.Ref, m.Title, m.URL, m.State, m.Error, Now())
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	m.UserID, m.CreatedAt, m.CheckedAt = uid, Now(), Now()
	return nil
}

// GetMade is one of the person's made things.
func (s *Store) GetMade(ctx context.Context, uid, id int64) (*Made, error) {
	m, err := scanMade(s.DB.QueryRowContext(ctx, `SELECT `+madeCols+` FROM made WHERE user_id = ? AND id = ?`, uid, id))
	return m, notFound(err)
}

// DeleteMade forgets a made thing (it stays in the other app).
func (s *Store) DeleteMade(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// MadeToCheck lists made things whose state is worth a look (oldest look first).
func (s *Store) MadeToCheck(ctx context.Context, uid int64, limit int) ([]*Made, error) {
	return s.madeQuery(ctx, `user_id = ? AND ref <> '' AND target IN ('signing', 'invoice', 'task') AND state NOT IN ('completed', 'done', 'paid', 'declined', 'voided', 'expired', 'cancelled')
		ORDER BY checked_at LIMIT ?`, uid, limit)
}

// SetMadeState records what was seen and when.
func (s *Store) SetMadeState(ctx context.Context, uid, id int64, state, errText string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE made SET state = ?, error = ?, checked_at = ? WHERE user_id = ? AND id = ?`, state, errText, Now(), uid, id)
	return err
}
