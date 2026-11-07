package store

import (
	"context"
	"database/sql"
)

// ---- repairs ----

const requestCols = `r.id, r.user_id, r.property_id, r.unit_id, r.lease_id, r.title, r.details, r.priority, r.status, r.reported_by, r.reported_on, r.planned_on, r.planned_time, r.done_on,
	r.cost, r.currency, r.pro_name, r.pro_phone, r.pro_ref, r.tenant_pays, r.task_ref, r.event_ref, r.home_record_ref, r.source_app, r.source_ref, r.created_at, r.updated_at,
	p.name, COALESCE(u.name, ''), (SELECT COUNT(1) FROM files f WHERE f.user_id = r.user_id AND f.owner = 'request' AND f.owner_id = r.id)`

const requestFrom = ` FROM requests r JOIN properties p ON p.id = r.property_id LEFT JOIN units u ON u.id = r.unit_id`

func scanRequest(sc interface{ Scan(...any) error }) (*Request, error) {
	r := &Request{}
	var unit, lease sql.NullInt64
	var pays int
	err := sc.Scan(&r.ID, &r.UserID, &r.PropertyID, &unit, &lease, &r.Title, &r.Details, &r.Priority, &r.Status, &r.ReportedBy, &r.ReportedOn, &r.PlannedOn, &r.PlannedTime, &r.DoneOn,
		&r.Cost, &r.Currency, &r.ProName, &r.ProPhone, &r.ProRef, &pays, &r.TaskRef, &r.EventRef, &r.HomeRecordRef, &r.SourceApp, &r.SourceRef, &r.CreatedAt, &r.UpdatedAt,
		&r.PropertyName, &r.UnitName, &r.Photos)
	r.UnitID, r.LeaseID, r.TenantPays = nullPtr(unit), nullPtr(lease), pays == 1
	return r, err
}

func nullPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// RequestFilter narrows a list of repairs.
type RequestFilter struct {
	Open       bool // not done or cancelled
	Status     string
	PropertyID int64
	LeaseID    int64
	Limit      int
}

// ListRequests lists a person's repairs, the urgent and open first.
func (s *Store) ListRequests(ctx context.Context, uid int64, f RequestFilter) ([]*Request, error) {
	q := `SELECT ` + requestCols + requestFrom + ` WHERE r.user_id = ?`
	args := []any{uid}
	if f.Open {
		q += ` AND r.status NOT IN ('done', 'cancelled')`
	}
	if f.Status != "" {
		q += ` AND r.status = ?`
		args = append(args, f.Status)
	}
	if f.PropertyID > 0 {
		q += ` AND r.property_id = ?`
		args = append(args, f.PropertyID)
	}
	if f.LeaseID > 0 {
		q += ` AND r.lease_id = ?`
		args = append(args, f.LeaseID)
	}
	q += ` ORDER BY CASE WHEN r.status IN ('done', 'cancelled') THEN 1 ELSE 0 END, CASE r.priority WHEN 'urgent' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END,
		CASE WHEN r.status IN ('done', 'cancelled') THEN r.done_on END DESC, r.reported_on DESC, r.id DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Request{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRequest is one of the person's repairs.
func (s *Store) GetRequest(ctx context.Context, uid, id int64) (*Request, error) {
	r, err := scanRequest(s.DB.QueryRowContext(ctx, `SELECT `+requestCols+requestFrom+` WHERE r.user_id = ? AND r.id = ?`, uid, id))
	return r, notFound(err)
}

// RequestBySource is the repair made from something elsewhere (a form answer).
func (s *Store) RequestBySource(ctx context.Context, uid int64, ref string) (*Request, error) {
	r, err := scanRequest(s.DB.QueryRowContext(ctx, `SELECT `+requestCols+requestFrom+` WHERE r.user_id = ? AND r.source_ref = ? AND r.source_ref <> ''`, uid, ref))
	return r, notFound(err)
}

// CreateRequest adds a repair.
func (s *Store) CreateRequest(ctx context.Context, r *Request) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO requests (user_id, property_id, unit_id, lease_id, title, details, priority, status, reported_by, reported_on, planned_on, planned_time, done_on,
		cost, currency, pro_name, pro_phone, pro_ref, tenant_pays, task_ref, event_ref, home_record_ref, source_app, source_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.UserID, r.PropertyID, nullI(r.UnitID), nullI(r.LeaseID), r.Title, r.Details, r.Priority, r.Status, r.ReportedBy, r.ReportedOn, r.PlannedOn, r.PlannedTime, r.DoneOn,
		r.Cost, r.Currency, r.ProName, r.ProPhone, r.ProRef, boolInt(r.TenantPays), r.TaskRef, r.EventRef, r.HomeRecordRef, r.SourceApp, r.SourceRef)
	if err != nil {
		return conflict(err)
	}
	r.ID, _ = res.LastInsertId()
	r.CreatedAt, r.UpdatedAt = Now(), Now()
	return nil
}

// UpdateRequest saves a repair.
func (s *Store) UpdateRequest(ctx context.Context, r *Request) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE requests SET property_id = ?, unit_id = ?, lease_id = ?, title = ?, details = ?, priority = ?, status = ?, reported_by = ?, reported_on = ?,
		planned_on = ?, planned_time = ?, done_on = ?, cost = ?, currency = ?, pro_name = ?, pro_phone = ?, pro_ref = ?, tenant_pays = ?, task_ref = ?, event_ref = ?, home_record_ref = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		r.PropertyID, nullI(r.UnitID), nullI(r.LeaseID), r.Title, r.Details, r.Priority, r.Status, r.ReportedBy, r.ReportedOn, r.PlannedOn, r.PlannedTime, r.DoneOn,
		r.Cost, r.Currency, r.ProName, r.ProPhone, r.ProRef, boolInt(r.TenantPays), r.TaskRef, r.EventRef, r.HomeRecordRef, Now(), r.UserID, r.ID)
	return affected(res, err)
}

// DeleteRequest removes a repair.
func (s *Store) DeleteRequest(ctx context.Context, uid, id int64) error {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND owner = 'request' AND owner_id = ?`, uid, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM files WHERE user_id = ? AND owner = 'request' AND owner_id = ?`, uid, id)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM requests WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// ---- expenses ----

const expenseCols = `e.id, e.user_id, e.property_id, e.unit_id, e.request_id, e.date, e.amount, e.currency, e.category, e.payee, e.note, e.money_tx_ref, e.money_found, e.client_key, e.created_at, p.name`

func scanExpense(sc interface{ Scan(...any) error }) (*Expense, error) {
	e := &Expense{}
	var unit, req sql.NullInt64
	var found int
	err := sc.Scan(&e.ID, &e.UserID, &e.PropertyID, &unit, &req, &e.Date, &e.Amount, &e.Currency, &e.Category, &e.Payee, &e.Note, &e.MoneyTxRef, &found, &e.ClientKey, &e.CreatedAt, &e.PropertyName)
	e.UnitID, e.RequestID, e.MoneyFound = nullPtr(unit), nullPtr(req), found == 1
	return e, err
}

// ListExpenses lists a person's expenses, newest first.
func (s *Store) ListExpenses(ctx context.Context, uid, propertyID int64, from, to string) ([]*Expense, error) {
	q := `SELECT ` + expenseCols + ` FROM expenses e JOIN properties p ON p.id = e.property_id WHERE e.user_id = ?`
	args := []any{uid}
	if propertyID > 0 {
		q += ` AND e.property_id = ?`
		args = append(args, propertyID)
	}
	if from != "" {
		q += ` AND e.date >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND e.date <= ?`
		args = append(args, to)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY e.date DESC, e.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetExpense is one of the person's expenses.
func (s *Store) GetExpense(ctx context.Context, uid, id int64) (*Expense, error) {
	e, err := scanExpense(s.DB.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses e JOIN properties p ON p.id = e.property_id WHERE e.user_id = ? AND e.id = ?`, uid, id))
	return e, notFound(err)
}

// ExpenseForRequest is the expense booked for a repair.
func (s *Store) ExpenseForRequest(ctx context.Context, uid, requestID int64) (*Expense, error) {
	e, err := scanExpense(s.DB.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses e JOIN properties p ON p.id = e.property_id WHERE e.user_id = ? AND e.request_id = ? LIMIT 1`, uid, requestID))
	return e, notFound(err)
}

// AddExpense records money out.
func (s *Store) AddExpense(ctx context.Context, e *Expense) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO expenses (user_id, property_id, unit_id, request_id, date, amount, currency, category, payee, note, money_tx_ref, money_found, client_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.PropertyID, nullI(e.UnitID), nullI(e.RequestID), e.Date, e.Amount, e.Currency, e.Category, e.Payee, e.Note, e.MoneyTxRef, boolInt(e.MoneyFound), e.ClientKey)
	if err != nil {
		return conflict(err)
	}
	e.ID, _ = res.LastInsertId()
	e.CreatedAt = Now()
	return nil
}

// UpdateExpense saves an expense.
func (s *Store) UpdateExpense(ctx context.Context, e *Expense) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE expenses SET property_id = ?, unit_id = ?, request_id = ?, date = ?, amount = ?, currency = ?, category = ?, payee = ?, note = ?, money_tx_ref = ?, money_found = ?
		WHERE user_id = ? AND id = ?`,
		e.PropertyID, nullI(e.UnitID), nullI(e.RequestID), e.Date, e.Amount, e.Currency, e.Category, e.Payee, e.Note, e.MoneyTxRef, boolInt(e.MoneyFound), e.UserID, e.ID)
	return affected(res, err)
}

// DeleteExpense removes an expense.
func (s *Store) DeleteExpense(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM expenses WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}
