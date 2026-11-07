package store

import (
	"context"
	"strings"
)

// ---- tenants ----

const tenantCols = `id, user_id, name, email, phone, contact_ref, notes, client_key, created_at, updated_at`

func scanTenant(sc interface{ Scan(...any) error }) (*Tenant, error) {
	t := &Tenant{}
	err := sc.Scan(&t.ID, &t.UserID, &t.Name, &t.Email, &t.Phone, &t.ContactRef, &t.Notes, &t.ClientKey, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// ListTenants lists everyone who rents or rented from the person.
func (s *Store) ListTenants(ctx context.Context, uid int64) ([]*Tenant, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+tenantCols+` FROM tenants WHERE user_id = ? ORDER BY name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Tenant{}
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTenant is one of the person's tenants.
func (s *Store) GetTenant(ctx context.Context, uid, id int64) (*Tenant, error) {
	t, err := scanTenant(s.DB.QueryRowContext(ctx, `SELECT `+tenantCols+` FROM tenants WHERE user_id = ? AND id = ?`, uid, id))
	return t, notFound(err)
}

// TenantByEmail finds a tenant by email.
func (s *Store) TenantByEmail(ctx context.Context, uid int64, email string) (*Tenant, error) {
	t, err := scanTenant(s.DB.QueryRowContext(ctx, `SELECT `+tenantCols+` FROM tenants WHERE user_id = ? AND email = ? COLLATE NOCASE AND email <> '' LIMIT 1`, uid, strings.TrimSpace(email)))
	return t, notFound(err)
}

// TenantByName finds a tenant by name.
func (s *Store) TenantByName(ctx context.Context, uid int64, name string) (*Tenant, error) {
	t, err := scanTenant(s.DB.QueryRowContext(ctx, `SELECT `+tenantCols+` FROM tenants WHERE user_id = ? AND name = ? COLLATE NOCASE LIMIT 1`, uid, strings.TrimSpace(name)))
	return t, notFound(err)
}

// CreateTenant adds a tenant.
func (s *Store) CreateTenant(ctx context.Context, t *Tenant) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO tenants (user_id, name, email, phone, contact_ref, notes, client_key) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Name, strings.ToLower(t.Email), t.Phone, t.ContactRef, t.Notes, t.ClientKey)
	if err != nil {
		return conflict(err)
	}
	t.ID, _ = res.LastInsertId()
	t.CreatedAt, t.UpdatedAt = Now(), Now()
	return nil
}

// UpdateTenant saves a tenant.
func (s *Store) UpdateTenant(ctx context.Context, t *Tenant) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE tenants SET name = ?, email = ?, phone = ?, contact_ref = ?, notes = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		t.Name, strings.ToLower(t.Email), t.Phone, t.ContactRef, t.Notes, Now(), t.UserID, t.ID)
	return affected(res, err)
}

// DeleteTenant removes a tenant (their leases stay, without them).
func (s *Store) DeleteTenant(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM tenants WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// ---- leases ----

const leaseCols = `l.id, l.user_id, l.unit_id, l.status, l.start_date, l.end_date, l.charges_from, l.rent, l.currency, l.frequency, l.due_day, l.grace_days, l.late_fee, l.notice_days,
	l.deposit, l.deposit_state, l.deposit_back, l.deposit_note, l.ended_on, l.end_reason, l.signing_ref, l.signing_state, l.notes, l.source_app, l.client_key, l.created_at, l.updated_at,
	u.name, p.id, p.name`

const leaseFrom = ` FROM leases l JOIN units u ON u.id = l.unit_id JOIN properties p ON p.id = u.property_id`

func scanLease(sc interface{ Scan(...any) error }) (*Lease, error) {
	l := &Lease{}
	err := sc.Scan(&l.ID, &l.UserID, &l.UnitID, &l.Status, &l.StartDate, &l.EndDate, &l.ChargesFrom, &l.Rent, &l.Currency, &l.Frequency, &l.DueDay, &l.GraceDays, &l.LateFee, &l.NoticeDays,
		&l.Deposit, &l.DepositState, &l.DepositBack, &l.DepositNote, &l.EndedOn, &l.EndReason, &l.SigningRef, &l.SigningState, &l.Notes, &l.SourceApp, &l.ClientKey, &l.CreatedAt, &l.UpdatedAt,
		&l.UnitName, &l.PropertyID, &l.PropertyName)
	l.Tenants = []*Tenant{}
	return l, err
}

// LeaseFilter narrows a list of leases.
type LeaseFilter struct {
	Status     string // "" = all
	PropertyID int64
	UnitID     int64
	TenantID   int64
}

// ListLeases lists a person's leases with their tenants.
func (s *Store) ListLeases(ctx context.Context, uid int64, f LeaseFilter) ([]*Lease, error) {
	q := `SELECT ` + leaseCols + leaseFrom + ` WHERE l.user_id = ?`
	args := []any{uid}
	if f.Status != "" {
		q += ` AND l.status = ?`
		args = append(args, f.Status)
	}
	if f.PropertyID > 0 {
		q += ` AND p.id = ?`
		args = append(args, f.PropertyID)
	}
	if f.UnitID > 0 {
		q += ` AND l.unit_id = ?`
		args = append(args, f.UnitID)
	}
	if f.TenantID > 0 {
		q += ` AND l.id IN (SELECT lease_id FROM lease_tenants WHERE tenant_id = ?)`
		args = append(args, f.TenantID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY CASE l.status WHEN 'active' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END, p.position, u.position, l.start_date DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := []*Lease{}
	for rows.Next() {
		l, err := scanLease(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.fillTenants(ctx, uid, out)
}

// fillTenants puts each lease's tenants on it (one query).
func (s *Store) fillTenants(ctx context.Context, uid int64, ls []*Lease) error {
	if len(ls) == 0 {
		return nil
	}
	by := map[int64]*Lease{}
	for _, l := range ls {
		by[l.ID] = l
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT lt.lease_id, `+prefixCols("t.", tenantCols)+` FROM lease_tenants lt JOIN tenants t ON t.id = lt.tenant_id WHERE lt.user_id = ? ORDER BY lt.position, t.name`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var lid int64
		t := &Tenant{}
		if err := rows.Scan(&lid, &t.ID, &t.UserID, &t.Name, &t.Email, &t.Phone, &t.ContactRef, &t.Notes, &t.ClientKey, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return err
		}
		if l := by[lid]; l != nil {
			l.Tenants = append(l.Tenants, t)
		}
	}
	return rows.Err()
}

func prefixCols(p, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = p + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

// GetLease is one of the person's leases with its tenants.
func (s *Store) GetLease(ctx context.Context, uid, id int64) (*Lease, error) {
	l, err := scanLease(s.DB.QueryRowContext(ctx, `SELECT `+leaseCols+leaseFrom+` WHERE l.user_id = ? AND l.id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	ls := []*Lease{l}
	if err := s.fillTenantsFor(ctx, uid, ls); err != nil {
		return nil, err
	}
	return l, nil
}

// LeaseByKey is the lease brought in under an import key.
func (s *Store) LeaseByKey(ctx context.Context, uid int64, key string) (*Lease, error) {
	l, err := scanLease(s.DB.QueryRowContext(ctx, `SELECT `+leaseCols+leaseFrom+` WHERE l.user_id = ? AND l.client_key = ?`, uid, key))
	return l, notFound(err)
}

func (s *Store) fillTenantsFor(ctx context.Context, uid int64, ls []*Lease) error {
	for _, l := range ls {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+prefixCols("t.", tenantCols)+` FROM lease_tenants lt JOIN tenants t ON t.id = lt.tenant_id WHERE lt.user_id = ? AND lt.lease_id = ? ORDER BY lt.position, t.name`, uid, l.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			t, err := scanTenant(rows)
			if err != nil {
				rows.Close()
				return err
			}
			l.Tenants = append(l.Tenants, t)
		}
		rows.Close()
	}
	return nil
}

// CreateLease adds a lease.
func (s *Store) CreateLease(ctx context.Context, l *Lease) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO leases (user_id, unit_id, status, start_date, end_date, charges_from, rent, currency, frequency, due_day, grace_days, late_fee, notice_days,
		deposit, deposit_state, deposit_back, deposit_note, ended_on, end_reason, signing_ref, signing_state, notes, source_app, client_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.UserID, l.UnitID, l.Status, l.StartDate, l.EndDate, l.ChargesFrom, l.Rent, l.Currency, l.Frequency, l.DueDay, l.GraceDays, l.LateFee, l.NoticeDays,
		l.Deposit, l.DepositState, l.DepositBack, l.DepositNote, l.EndedOn, l.EndReason, l.SigningRef, l.SigningState, l.Notes, l.SourceApp, l.ClientKey)
	if err != nil {
		return conflict(err)
	}
	l.ID, _ = res.LastInsertId()
	l.CreatedAt, l.UpdatedAt = Now(), Now()
	return nil
}

// UpdateLease saves a lease.
func (s *Store) UpdateLease(ctx context.Context, l *Lease) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE leases SET unit_id = ?, status = ?, start_date = ?, end_date = ?, charges_from = ?, rent = ?, currency = ?, frequency = ?, due_day = ?, grace_days = ?,
		late_fee = ?, notice_days = ?, deposit = ?, deposit_state = ?, deposit_back = ?, deposit_note = ?, ended_on = ?, end_reason = ?, signing_ref = ?, signing_state = ?, notes = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		l.UnitID, l.Status, l.StartDate, l.EndDate, l.ChargesFrom, l.Rent, l.Currency, l.Frequency, l.DueDay, l.GraceDays, l.LateFee, l.NoticeDays,
		l.Deposit, l.DepositState, l.DepositBack, l.DepositNote, l.EndedOn, l.EndReason, l.SigningRef, l.SigningState, l.Notes, Now(), l.UserID, l.ID)
	return affected(res, err)
}

// DeleteLease removes a lease with its charges and payments.
func (s *Store) DeleteLease(ctx context.Context, uid, id int64) error {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND owner = 'lease' AND owner_id = ?`, uid, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM files WHERE user_id = ? AND owner = 'lease' AND owner_id = ?`, uid, id)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM leases WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// SetLeaseTenants replaces who rents under a lease (ids must be the person's own).
func (s *Store) SetLeaseTenants(ctx context.Context, uid, leaseID int64, tenantIDs []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `DELETE FROM lease_tenants WHERE user_id = ? AND lease_id = ?`, uid, leaseID); err != nil {
		return err
	}
	seen := map[int64]bool{}
	for i, tid := range tenantIDs {
		if seen[tid] {
			continue
		}
		seen[tid] = true
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM tenants WHERE user_id = ? AND id = ?`, uid, tid).Scan(&n); err != nil || n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO lease_tenants (user_id, lease_id, tenant_id, position) VALUES (?, ?, ?, ?)`, uid, leaseID, tid, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetSigning remembers the lease's document in Pocket Sign and how far it is.
func (s *Store) SetSigning(ctx context.Context, uid, id int64, ref, state string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE leases SET signing_ref = ?, signing_state = ?, updated_at = ? WHERE user_id = ? AND id = ?`, ref, state, Now(), uid, id)
	return affected(res, err)
}

// ---- charges ----

const chargeCols = `id, user_id, lease_id, kind, title, period, due_date, amount, currency, waived, invoice_ref, note, created_at`

func scanCharge(sc interface{ Scan(...any) error }) (*Charge, error) {
	c := &Charge{}
	var waived int
	err := sc.Scan(&c.ID, &c.UserID, &c.LeaseID, &c.Kind, &c.Title, &c.Period, &c.DueDate, &c.Amount, &c.Currency, &waived, &c.InvoiceRef, &c.Note, &c.CreatedAt)
	c.Waived = waived == 1
	return c, err
}

// ListCharges lists a lease's charges, oldest first (leaseID 0 = all the person's).
func (s *Store) ListCharges(ctx context.Context, uid, leaseID int64) ([]*Charge, error) {
	q := `SELECT ` + chargeCols + ` FROM charges WHERE user_id = ?`
	args := []any{uid}
	if leaseID > 0 {
		q += ` AND lease_id = ?`
		args = append(args, leaseID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY lease_id, due_date, CASE kind WHEN 'deposit' THEN 0 WHEN 'rent' THEN 1 ELSE 2 END, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Charge{}
	for rows.Next() {
		c, err := scanCharge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCharge is one of the person's charges.
func (s *Store) GetCharge(ctx context.Context, uid, id int64) (*Charge, error) {
	c, err := scanCharge(s.DB.QueryRowContext(ctx, `SELECT `+chargeCols+` FROM charges WHERE user_id = ? AND id = ?`, uid, id))
	return c, notFound(err)
}

// AddCharge adds a charge; a period charge that is there already is left as it is (false).
func (s *Store) AddCharge(ctx context.Context, c *Charge) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO charges (user_id, lease_id, kind, title, period, due_date, amount, currency, waived, invoice_ref, note) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, c.UserID, c.LeaseID, c.Kind, c.Title, c.Period, c.DueDate, c.Amount, c.Currency, boolInt(c.Waived), c.InvoiceRef, c.Note)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	c.ID, _ = res.LastInsertId()
	c.CreatedAt = Now()
	return true, nil
}

// UpdateCharge saves a charge.
func (s *Store) UpdateCharge(ctx context.Context, c *Charge) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE charges SET kind = ?, title = ?, due_date = ?, amount = ?, currency = ?, waived = ?, invoice_ref = ?, note = ? WHERE user_id = ? AND id = ?`,
		c.Kind, c.Title, c.DueDate, c.Amount, c.Currency, boolInt(c.Waived), c.InvoiceRef, c.Note, c.UserID, c.ID)
	return affected(res, err)
}

// DeleteCharge removes a charge.
func (s *Store) DeleteCharge(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM charges WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// DropFutureRent removes rent not due yet (the terms changed: it is made again).
func (s *Store) DropFutureRent(ctx context.Context, uid, leaseID int64, after string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM charges WHERE user_id = ? AND lease_id = ? AND kind = 'rent' AND due_date > ? AND invoice_ref = ''`, uid, leaseID, after)
	return err
}

// DropRentAfter removes rent for periods after a lease ended.
func (s *Store) DropRentAfter(ctx context.Context, uid, leaseID int64, day string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM charges WHERE user_id = ? AND lease_id = ? AND kind = 'rent' AND period > ? AND invoice_ref = ''`, uid, leaseID, day)
	return err
}

// ---- payments ----

const paymentCols = `id, user_id, lease_id, date, amount, currency, method, note, money_tx_ref, money_found, source_app, client_key, created_at`

func scanPayment(sc interface{ Scan(...any) error }) (*Payment, error) {
	p := &Payment{}
	var found int
	err := sc.Scan(&p.ID, &p.UserID, &p.LeaseID, &p.Date, &p.Amount, &p.Currency, &p.Method, &p.Note, &p.MoneyTxRef, &found, &p.SourceApp, &p.ClientKey, &p.CreatedAt)
	p.MoneyFound = found == 1
	return p, err
}

// ListPayments lists a lease's payments, oldest first (leaseID 0 = all, from–to optional).
func (s *Store) ListPayments(ctx context.Context, uid, leaseID int64, from, to string) ([]*Payment, error) {
	q := `SELECT ` + paymentCols + ` FROM payments WHERE user_id = ?`
	args := []any{uid}
	if leaseID > 0 {
		q += ` AND lease_id = ?`
		args = append(args, leaseID)
	}
	if from != "" {
		q += ` AND date >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND date <= ?`
		args = append(args, to)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY date, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPayment is one of the person's payments.
func (s *Store) GetPayment(ctx context.Context, uid, id int64) (*Payment, error) {
	p, err := scanPayment(s.DB.QueryRowContext(ctx, `SELECT `+paymentCols+` FROM payments WHERE user_id = ? AND id = ?`, uid, id))
	return p, notFound(err)
}

// PaymentByMoney is the payment linked to a Pocket Money transaction.
func (s *Store) PaymentByMoney(ctx context.Context, uid int64, ref string) (*Payment, error) {
	p, err := scanPayment(s.DB.QueryRowContext(ctx, `SELECT `+paymentCols+` FROM payments WHERE user_id = ? AND money_tx_ref = ? AND money_tx_ref <> ''`, uid, ref))
	return p, notFound(err)
}

// AddPayment records money in.
func (s *Store) AddPayment(ctx context.Context, p *Payment) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO payments (user_id, lease_id, date, amount, currency, method, note, money_tx_ref, money_found, source_app, client_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, p.LeaseID, p.Date, p.Amount, p.Currency, p.Method, p.Note, p.MoneyTxRef, boolInt(p.MoneyFound), p.SourceApp, p.ClientKey)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	p.CreatedAt = Now()
	return nil
}

// UpdatePayment saves a payment.
func (s *Store) UpdatePayment(ctx context.Context, p *Payment) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE payments SET date = ?, amount = ?, currency = ?, method = ?, note = ?, money_tx_ref = ?, money_found = ? WHERE user_id = ? AND id = ?`,
		p.Date, p.Amount, p.Currency, p.Method, p.Note, p.MoneyTxRef, boolInt(p.MoneyFound), p.UserID, p.ID)
	return affected(res, err)
}

// DeletePayment removes a payment.
func (s *Store) DeletePayment(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM payments WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// MoneyRefs are the Pocket Money transactions already linked to something here.
func (s *Store) MoneyRefs(ctx context.Context, uid int64) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT money_tx_ref FROM payments WHERE user_id = ? AND money_tx_ref <> '' UNION SELECT money_tx_ref FROM expenses WHERE user_id = ? AND money_tx_ref <> ''`, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out[r] = true
	}
	return out, rows.Err()
}
