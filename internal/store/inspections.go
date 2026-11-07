package store

import (
	"context"
	"database/sql"
)

const inspectionCols = `i.id, i.user_id, i.property_id, i.unit_id, i.lease_id, i.kind, i.date, i.time, i.status, i.summary, i.event_ref, i.created_at, i.updated_at, p.name, COALESCE(u.name, ''),
	(SELECT COUNT(1) FROM inspection_items x WHERE x.inspection_id = i.id),
	(SELECT COUNT(1) FROM inspection_items x WHERE x.inspection_id = i.id AND x.condition IN ('poor', 'damaged', 'missing'))`

const inspectionFrom = ` FROM inspections i JOIN properties p ON p.id = i.property_id LEFT JOIN units u ON u.id = i.unit_id`

func scanInspection(sc interface{ Scan(...any) error }) (*Inspection, error) {
	in := &Inspection{}
	var unit, lease sql.NullInt64
	err := sc.Scan(&in.ID, &in.UserID, &in.PropertyID, &unit, &lease, &in.Kind, &in.Date, &in.Time, &in.Status, &in.Summary, &in.EventRef, &in.CreatedAt, &in.UpdatedAt,
		&in.PropertyName, &in.UnitName, &in.ItemCount, &in.Issues)
	in.UnitID, in.LeaseID = nullPtr(unit), nullPtr(lease)
	return in, err
}

// ListInspections lists a person's inspections, planned ones first.
func (s *Store) ListInspections(ctx context.Context, uid, propertyID, leaseID int64) ([]*Inspection, error) {
	q := `SELECT ` + inspectionCols + inspectionFrom + ` WHERE i.user_id = ?`
	args := []any{uid}
	if propertyID > 0 {
		q += ` AND i.property_id = ?`
		args = append(args, propertyID)
	}
	if leaseID > 0 {
		q += ` AND i.lease_id = ?`
		args = append(args, leaseID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY CASE i.status WHEN 'planned' THEN 0 ELSE 1 END, CASE WHEN i.status = 'planned' THEN i.date END, i.date DESC, i.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Inspection{}
	for rows.Next() {
		in, err := scanInspection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// GetInspection is one inspection with its lines and their photos.
func (s *Store) GetInspection(ctx context.Context, uid, id int64) (*Inspection, error) {
	in, err := scanInspection(s.DB.QueryRowContext(ctx, `SELECT `+inspectionCols+inspectionFrom+` WHERE i.user_id = ? AND i.id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, inspection_id, room, item, condition, note, position FROM inspection_items WHERE user_id = ? AND inspection_id = ? ORDER BY position, id`, uid, id)
	if err != nil {
		return nil, err
	}
	in.Items = []*InspectionItem{}
	byID := map[int64]*InspectionItem{}
	for rows.Next() {
		it := &InspectionItem{Photos: []*File{}}
		if err := rows.Scan(&it.ID, &it.UserID, &it.InspectionID, &it.Room, &it.Item, &it.Condition, &it.Note, &it.Position); err != nil {
			rows.Close()
			return nil, err
		}
		in.Items = append(in.Items, it)
		byID[it.ID] = it
	}
	rows.Close()
	files, err := s.ListFiles(ctx, uid, "item", 0)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if it := byID[f.OwnerID]; it != nil {
			it.Photos = append(it.Photos, f)
		}
	}
	return in, nil
}

// CreateInspection adds an inspection.
func (s *Store) CreateInspection(ctx context.Context, in *Inspection) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO inspections (user_id, property_id, unit_id, lease_id, kind, date, time, status, summary, event_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.UserID, in.PropertyID, nullI(in.UnitID), nullI(in.LeaseID), in.Kind, in.Date, in.Time, in.Status, in.Summary, in.EventRef)
	if err != nil {
		return err
	}
	in.ID, _ = res.LastInsertId()
	in.CreatedAt, in.UpdatedAt = Now(), Now()
	return nil
}

// UpdateInspection saves an inspection.
func (s *Store) UpdateInspection(ctx context.Context, in *Inspection) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE inspections SET property_id = ?, unit_id = ?, lease_id = ?, kind = ?, date = ?, time = ?, status = ?, summary = ?, event_ref = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		in.PropertyID, nullI(in.UnitID), nullI(in.LeaseID), in.Kind, in.Date, in.Time, in.Status, in.Summary, in.EventRef, Now(), in.UserID, in.ID)
	return affected(res, err)
}

// DeleteInspection removes an inspection with its lines and photos (the file ids are returned to remove).
func (s *Store) DeleteInspection(ctx context.Context, uid, id int64) ([]int64, error) {
	files, _ := s.filesOfInspection(ctx, uid, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM files WHERE user_id = ? AND owner = 'item' AND owner_id IN (SELECT id FROM inspection_items WHERE inspection_id = ?)`, uid, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND owner = 'inspection' AND owner_id = ?`, uid, id)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM inspections WHERE user_id = ? AND id = ?`, uid, id)
	return files, affected(res, err)
}

func (s *Store) filesOfInspection(ctx context.Context, uid, id int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT f.id FROM files f JOIN inspection_items ii ON ii.id = f.owner_id AND f.owner = 'item' WHERE f.user_id = ? AND ii.inspection_id = ?`, uid, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var fid int64
		if err := rows.Scan(&fid); err != nil {
			return nil, err
		}
		out = append(out, fid)
	}
	return out, rows.Err()
}

// SetItems replaces an inspection's lines; lines with an id keep it (and their photos).
func (s *Store) SetItems(ctx context.Context, uid, inspectionID int64, items []*InspectionItem) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	keep := map[int64]bool{}
	for i, it := range items {
		if it.ID > 0 {
			res, err := tx.ExecContext(ctx, `UPDATE inspection_items SET room = ?, item = ?, condition = ?, note = ?, position = ? WHERE user_id = ? AND inspection_id = ? AND id = ?`,
				it.Room, it.Item, it.Condition, it.Note, i, uid, inspectionID, it.ID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				keep[it.ID] = true
				continue
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO inspection_items (user_id, inspection_id, room, item, condition, note, position) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uid, inspectionID, it.Room, it.Item, it.Condition, it.Note, i)
		if err != nil {
			return err
		}
		it.ID, _ = res.LastInsertId()
		keep[it.ID] = true
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM inspection_items WHERE user_id = ? AND inspection_id = ?`, uid, inspectionID)
	if err != nil {
		return err
	}
	var drop []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !keep[id] {
			drop = append(drop, id)
		}
	}
	rows.Close()
	for _, id := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM inspection_items WHERE user_id = ? AND id = ?`, uid, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE user_id = ? AND owner = 'item' AND owner_id = ?`, uid, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inspections SET updated_at = ? WHERE user_id = ? AND id = ?`, Now(), uid, inspectionID); err != nil {
		return err
	}
	return tx.Commit()
}

// ItemOwner is the inspection a line belongs to (the person's own, else ErrNotFound).
func (s *Store) ItemOwner(ctx context.Context, uid, itemID int64) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT inspection_id FROM inspection_items WHERE user_id = ? AND id = ?`, uid, itemID).Scan(&id)
	return id, notFound(err)
}

// ---- files ----

// ListFiles lists files of an owner kind (ownerID 0 = all of that kind).
func (s *Store) ListFiles(ctx context.Context, uid int64, owner string, ownerID int64) ([]*File, error) {
	q := `SELECT id, user_id, owner, owner_id, name, mime, size, created_at FROM files WHERE user_id = ? AND owner = ?`
	args := []any{uid, owner}
	if ownerID > 0 {
		q += ` AND owner_id = ?`
		args = append(args, ownerID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*File{}
	for rows.Next() {
		f := &File{}
		if err := rows.Scan(&f.ID, &f.UserID, &f.Owner, &f.OwnerID, &f.Name, &f.Mime, &f.Size, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetFile is one of the person's files.
func (s *Store) GetFile(ctx context.Context, uid, id int64) (*File, error) {
	f := &File{}
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, owner, owner_id, name, mime, size, created_at FROM files WHERE user_id = ? AND id = ?`, uid, id).
		Scan(&f.ID, &f.UserID, &f.Owner, &f.OwnerID, &f.Name, &f.Mime, &f.Size, &f.CreatedAt)
	return f, notFound(err)
}

// AddFile records a stored file.
func (s *Store) AddFile(ctx context.Context, f *File) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO files (user_id, owner, owner_id, name, mime, size) VALUES (?, ?, ?, ?, ?, ?)`, f.UserID, f.Owner, f.OwnerID, f.Name, f.Mime, f.Size)
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	f.CreatedAt = Now()
	return nil
}

// DeleteFile forgets a file.
func (s *Store) DeleteFile(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM files WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// AllFileIDs is every file id (to clean the files folder).
func (s *Store) AllFileIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
