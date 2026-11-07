# Backup

Everything lives in `DATA_DIR`: the SQLite file `pocket-rentals.db` (WAL mode) and the photos and PDFs in `files/<person>/<id>`. Take a consistent copy of the database with `pocket-rentals backup` (writes `DATA_DIR/backups/pocket-rentals-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin), and back up `files/` as it is. Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-rentals.db` (remove `-wal`/`-shm` files) and `files/`, start.

Every person can also export their own rentals as JSON (`/api/v1/export.json`, back in with Settings → Backup & data, twice is the same as once), their leases as CSV (`/api/v1/export.csv`), and each year's money in and out as CSV (`/api/v1/reports/year.csv?year=2026`).
