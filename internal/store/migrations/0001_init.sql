CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);



-- ---------- Pocket Rentals: everything below is per person ----------
-- Money is always in minor units (cents) of the row's currency.

-- A place you rent out: a house, a flat, a building with flats, a garage.
-- home_ref points at the same place in Pocket Home (repairs go to its log);
-- value is what it is worth today (Pocket Wealth can read it).
CREATE TABLE properties (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  kind       TEXT NOT NULL DEFAULT 'apartment',     -- house | apartment | building | room | commercial | garage | other
  address    TEXT NOT NULL DEFAULT '',
  city       TEXT NOT NULL DEFAULT '',
  country    TEXT NOT NULL DEFAULT '',
  currency   TEXT NOT NULL DEFAULT 'EUR',
  value      INTEGER NOT NULL DEFAULT 0,
  bought_on  TEXT NOT NULL DEFAULT '',
  bought_for INTEGER NOT NULL DEFAULT 0,
  home_ref   TEXT NOT NULL DEFAULT '',              -- Pocket Home home id
  money_tag  TEXT NOT NULL DEFAULT '',              -- tag on its transactions in Pocket Money
  color      TEXT NOT NULL DEFAULT '',
  notes      TEXT NOT NULL DEFAULT '',
  archived   INTEGER NOT NULL DEFAULT 0,
  position   INTEGER NOT NULL DEFAULT 0,
  client_key TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX properties_user ON properties(user_id, archived, position);
CREATE UNIQUE INDEX properties_client ON properties(user_id, client_key) WHERE client_key <> '';

-- What is let on its own: a flat, a room, the whole house.
CREATE TABLE units (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
  name        TEXT NOT NULL DEFAULT '',
  bedrooms    INTEGER NOT NULL DEFAULT 0,
  bathrooms   INTEGER NOT NULL DEFAULT 0,
  area_m2     REAL NOT NULL DEFAULT 0,
  rent        INTEGER NOT NULL DEFAULT 0,           -- the asking rent
  deposit     INTEGER NOT NULL DEFAULT 0,           -- the asking deposit
  notes       TEXT NOT NULL DEFAULT '',
  position    INTEGER NOT NULL DEFAULT 0,
  archived    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX units_user ON units(user_id, property_id, position);

-- Someone who rents (or rented) from you. contact_ref = their card in Pocket Contacts.
CREATE TABLE tenants (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL DEFAULT '',
  email       TEXT NOT NULL DEFAULT '',
  phone       TEXT NOT NULL DEFAULT '',
  contact_ref TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  client_key  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX tenants_user ON tenants(user_id, name);
CREATE INDEX tenants_email ON tenants(user_id, email);
CREATE UNIQUE INDEX tenants_client ON tenants(user_id, client_key) WHERE client_key <> '';

-- A lease: a unit let to one or more tenants from a day, for a rent.
-- Rent is charged every period from charges_from (the start, or the day it
-- was brought in) on due_day; end_date '' = it runs until notice.
CREATE TABLE leases (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  unit_id       INTEGER NOT NULL REFERENCES units(id) ON DELETE CASCADE,
  status        TEXT NOT NULL DEFAULT 'active',     -- draft | active | ended
  start_date    TEXT NOT NULL DEFAULT '',
  end_date      TEXT NOT NULL DEFAULT '',
  charges_from  TEXT NOT NULL DEFAULT '',
  rent          INTEGER NOT NULL DEFAULT 0,
  currency      TEXT NOT NULL DEFAULT 'EUR',
  frequency     TEXT NOT NULL DEFAULT 'monthly',    -- monthly | weekly | quarterly | yearly
  due_day       INTEGER NOT NULL DEFAULT 1,         -- day of the month (1–28), weekday for weekly (1 = Monday)
  grace_days    INTEGER NOT NULL DEFAULT 5,
  late_fee      INTEGER NOT NULL DEFAULT 0,
  notice_days   INTEGER NOT NULL DEFAULT 30,
  deposit       INTEGER NOT NULL DEFAULT 0,
  deposit_state TEXT NOT NULL DEFAULT 'none',       -- none | due | held | returned | part_returned
  deposit_back  INTEGER NOT NULL DEFAULT 0,
  deposit_note  TEXT NOT NULL DEFAULT '',
  ended_on      TEXT NOT NULL DEFAULT '',
  end_reason    TEXT NOT NULL DEFAULT '',
  signing_ref   TEXT NOT NULL DEFAULT '',           -- the lease in Pocket Sign
  signing_state TEXT NOT NULL DEFAULT '',
  notes         TEXT NOT NULL DEFAULT '',
  source_app    TEXT NOT NULL DEFAULT '',
  client_key    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX leases_user ON leases(user_id, status, unit_id);
CREATE UNIQUE INDEX leases_client ON leases(user_id, client_key) WHERE client_key <> '';

CREATE TABLE lease_tenants (
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  lease_id  INTEGER NOT NULL REFERENCES leases(id) ON DELETE CASCADE,
  tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  position  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (lease_id, tenant_id)
);
CREATE INDEX lease_tenants_tenant ON lease_tenants(user_id, tenant_id);

-- What a lease owes: each period's rent (made by the app, never twice),
-- the deposit, a bill passed on, a late fee, a repair the tenant pays for.
CREATE TABLE charges (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  lease_id    INTEGER NOT NULL REFERENCES leases(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL DEFAULT 'rent',         -- rent | deposit | utilities | late_fee | repair | other
  title       TEXT NOT NULL DEFAULT '',
  period      TEXT NOT NULL DEFAULT '',             -- rent: the first day of the period it pays for
  due_date    TEXT NOT NULL DEFAULT '',
  amount      INTEGER NOT NULL DEFAULT 0,
  currency    TEXT NOT NULL DEFAULT 'EUR',
  waived      INTEGER NOT NULL DEFAULT 0,
  invoice_ref TEXT NOT NULL DEFAULT '',             -- a draft invoice in Pocket Invoicing
  note        TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX charges_lease ON charges(user_id, lease_id, due_date);
CREATE INDEX charges_due ON charges(user_id, due_date);
CREATE UNIQUE INDEX charges_period ON charges(lease_id, kind, period) WHERE period <> '';

-- Money in from a lease. Payments pay the oldest charges first.
-- money_tx_ref = the transaction in Pocket Money: booked from here, or
-- found there (money_found = 1, never booked twice).
CREATE TABLE payments (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  lease_id     INTEGER NOT NULL REFERENCES leases(id) ON DELETE CASCADE,
  date         TEXT NOT NULL DEFAULT '',
  amount       INTEGER NOT NULL DEFAULT 0,
  currency     TEXT NOT NULL DEFAULT 'EUR',
  method       TEXT NOT NULL DEFAULT 'bank',        -- bank | cash | card | other
  note         TEXT NOT NULL DEFAULT '',
  money_tx_ref TEXT NOT NULL DEFAULT '',
  money_found  INTEGER NOT NULL DEFAULT 0,
  source_app   TEXT NOT NULL DEFAULT '',
  client_key   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX payments_lease ON payments(user_id, lease_id, date);
CREATE INDEX payments_date ON payments(user_id, date);
CREATE UNIQUE INDEX payments_money ON payments(user_id, money_tx_ref) WHERE money_tx_ref <> '';
CREATE UNIQUE INDEX payments_client ON payments(user_id, client_key) WHERE client_key <> '';

-- A repair or something to fix, reported by a tenant or found by you.
CREATE TABLE requests (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  property_id     INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
  unit_id         INTEGER REFERENCES units(id) ON DELETE SET NULL,
  lease_id        INTEGER REFERENCES leases(id) ON DELETE SET NULL,
  title           TEXT NOT NULL DEFAULT '',
  details         TEXT NOT NULL DEFAULT '',
  priority        TEXT NOT NULL DEFAULT 'normal',   -- low | normal | urgent
  status          TEXT NOT NULL DEFAULT 'new',      -- new | planned | in_progress | waiting | done | cancelled
  reported_by     TEXT NOT NULL DEFAULT '',
  reported_on     TEXT NOT NULL DEFAULT '',
  planned_on      TEXT NOT NULL DEFAULT '',
  planned_time    TEXT NOT NULL DEFAULT '',
  done_on         TEXT NOT NULL DEFAULT '',
  cost            INTEGER NOT NULL DEFAULT 0,
  currency        TEXT NOT NULL DEFAULT 'EUR',
  pro_name        TEXT NOT NULL DEFAULT '',
  pro_phone       TEXT NOT NULL DEFAULT '',
  pro_ref         TEXT NOT NULL DEFAULT '',          -- the tradesperson in Pocket Home
  tenant_pays     INTEGER NOT NULL DEFAULT 0,
  task_ref        TEXT NOT NULL DEFAULT '',
  event_ref       TEXT NOT NULL DEFAULT '',
  home_record_ref TEXT NOT NULL DEFAULT '',
  source_app      TEXT NOT NULL DEFAULT '',
  source_ref      TEXT NOT NULL DEFAULT '',          -- e.g. forms:answer:12
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX requests_user ON requests(user_id, status, property_id);
CREATE UNIQUE INDEX requests_source ON requests(user_id, source_ref) WHERE source_ref <> '';

-- Money out for a place: insurance, tax, a repair, the building fee.
CREATE TABLE expenses (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  property_id  INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
  unit_id      INTEGER REFERENCES units(id) ON DELETE SET NULL,
  request_id   INTEGER REFERENCES requests(id) ON DELETE SET NULL,
  date         TEXT NOT NULL DEFAULT '',
  amount       INTEGER NOT NULL DEFAULT 0,
  currency     TEXT NOT NULL DEFAULT 'EUR',
  category     TEXT NOT NULL DEFAULT 'other',
  payee        TEXT NOT NULL DEFAULT '',
  note         TEXT NOT NULL DEFAULT '',
  money_tx_ref TEXT NOT NULL DEFAULT '',
  money_found  INTEGER NOT NULL DEFAULT 0,
  client_key   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX expenses_user ON expenses(user_id, property_id, date);
CREATE UNIQUE INDEX expenses_money ON expenses(user_id, money_tx_ref) WHERE money_tx_ref <> '';
CREATE UNIQUE INDEX expenses_client ON expenses(user_id, client_key) WHERE client_key <> '';

-- A walk through a unit: moving in, moving out, a routine look.
CREATE TABLE inspections (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
  unit_id     INTEGER REFERENCES units(id) ON DELETE SET NULL,
  lease_id    INTEGER REFERENCES leases(id) ON DELETE SET NULL,
  kind        TEXT NOT NULL DEFAULT 'routine',      -- move_in | move_out | routine
  date        TEXT NOT NULL DEFAULT '',
  time        TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'planned',      -- planned | done
  summary     TEXT NOT NULL DEFAULT '',
  event_ref   TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX inspections_user ON inspections(user_id, date);

-- One line of an inspection: a room, a thing in it, how it looks.
CREATE TABLE inspection_items (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  inspection_id INTEGER NOT NULL REFERENCES inspections(id) ON DELETE CASCADE,
  room          TEXT NOT NULL DEFAULT '',
  item          TEXT NOT NULL DEFAULT '',
  condition     TEXT NOT NULL DEFAULT '',            -- '' | good | fair | poor | damaged | missing
  note          TEXT NOT NULL DEFAULT '',
  position      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX inspection_items_insp ON inspection_items(inspection_id, position);

-- Photos and files (kept in DATA_DIR/files), on an inspection line, a repair, a lease, a place.
CREATE TABLE files (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  owner      TEXT NOT NULL,                          -- item | request | lease | property | inspection
  owner_id   INTEGER NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  mime       TEXT NOT NULL DEFAULT '',
  size       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX files_owner ON files(user_id, owner, owner_id);

-- What happened: notes, calls, emails, payments, changes, things done in other apps.
CREATE TABLE timeline (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  lease_id    INTEGER REFERENCES leases(id) ON DELETE CASCADE,
  property_id INTEGER REFERENCES properties(id) ON DELETE CASCADE,
  request_id  INTEGER REFERENCES requests(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL DEFAULT 'note',          -- note | call | email | payment | charge | lease | repair | inspection | made
  title       TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL DEFAULT '',
  at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  app         TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL DEFAULT '',
  source_app  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX timeline_lease ON timeline(user_id, lease_id, at);
CREATE INDEX timeline_property ON timeline(user_id, property_id, at);
CREATE INDEX timeline_request ON timeline(user_id, request_id, at);

-- What things here have in the other Pocket apps (a task, an event, a note,
-- a document to sign, an invoice, a document in the archive…), with a link
-- to it — or why making it failed.
CREATE TABLE made (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  owner      TEXT NOT NULL,                          -- lease | request | inspection | property | tenant | charge
  owner_id   INTEGER NOT NULL,
  target     TEXT NOT NULL,                          -- contact | task | event | note | email | signing | invoice | document | record
  app        TEXT NOT NULL DEFAULT '',
  ref        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL DEFAULT '',
  error      TEXT NOT NULL DEFAULT '',
  checked_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX made_owner ON made(user_id, owner, owner_id);
