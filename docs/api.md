# API

Everything the web app does goes through `/api/v1`, with a session cookie or `Authorization: Bearer prt_…` (Settings → API & shortcuts). Money is always in **cents** of the row's currency. Every row belongs to the person signed in; another person's ids answer 404.

## Places and units
| | |
|---|---|
| `GET /properties` (`?archived=1`) | Places with their units, each unit's current lease, let count, monthly rent roll, rent late |
| `POST /properties` | `{name, kind, address, city, currency, units:[{name, rent, deposit}]}` — without units, one is made for a house, flat, room, garage |
| `GET/PUT/DELETE /properties/{id}` | A place with leases, repairs, inspections, this year's money in and out |
| `POST /properties/{id}/units`, `PUT/DELETE /units/{id}`, `GET /units` | Units (a unit with leases is archived, not deleted) |

## Tenants and leases
| | |
|---|---|
| `GET/POST /tenants`, `GET/PUT/DELETE /tenants/{id}` | Tenants (a new one with an email already here is that one) |
| `GET /leases` (`?status=active\|draft\|ended&property=`) | Leases with tenants, `balance` (owed now), `overdue`, `next_due`, `days_left` |
| `POST /leases` | `{unit_id, start_date, end_date?, rent, frequency, due_day, deposit, deposit_state, charges_from?, tenants:[{id}\|{name,email,phone}]}` |
| `GET/PUT/DELETE /leases/{id}` | A lease with its charges (each with `paid` and `state`), payments, timeline and links |
| `POST /leases/{id}/end` `{on, reason}` · `/renew` `{end_date, rent}` · `/deposit` `{state, back, note, money}` | |
| `POST /leases/{id}/charges`, `PUT/DELETE /charges/{id}` | Extra charges; rent is made by the app (waive it with `{"waived": true}`) |
| `POST /leases/{id}/payments` | `{date, amount, method, note, money?, account_id?, money_tx_ref?}` — payments pay the oldest charges first |
| `PUT/DELETE /payments/{id}` (`?money=1` removes the Pocket Money transaction too), `POST /payments/{id}/money` | |
| `GET /rent?month=YYYY-MM` | Every charge due in a month and how far it is paid |
| `GET /money/suggestions`, `POST /money/match {tx_id, lease_id}` | Rent already in Pocket Money, and linking it |

## Repairs, costs, inspections, photos
| | |
|---|---|
| `GET/POST /requests`, `GET/PUT/DELETE /requests/{id}` | Repairs; `status: "done"` makes the cost an expense, charges the tenant when `tenant_pays`, ticks off its task, logs it in Pocket Home |
| `GET/POST /expenses`, `PUT/DELETE /expenses/{id}`, `POST /expenses/{id}/money` | Costs of a place |
| `GET/POST /inspections`, `GET/PUT/DELETE /inspections/{id}`, `PUT /inspections/{id}/items` | Inspections; `from_id` starts from another one's lines; moving out answers with `before` |
| `POST /files?owner=item\|request\|lease\|property&owner_id=` (multipart `file`), `GET/DELETE /files/{id}` | Photos and PDFs |

## Overviews and other apps
`GET /today`, `/summary`, `/reports?year=`, `/reports/year.csv?year=`, `/search?q=`, `/lookup?email=` (a tenant by email, for Pocket Contacts), `/day/YYYY-MM-DD` (for Pocket Day), `/entries`, `POST /entries {lease_id|property_id|request_id, kind, title, body}`, `POST /make {owner, owner_id, target}` (contact, task, note, email, signing, invoice, document), `GET /apps` (per app: does it work for you, and if not why), `GET /ics/{lease|request|inspection}/{id}`, `GET /form-answers`, `POST /form-answers/{id}/take`, `POST /ai {want: reminder|late|ending|inspection|repair}`.

## Pocket Core
Offers `rentals:read` (every GET above that is not settings, tokens or exports) and `rentals:write` (`POST /requests`, `PUT /requests/{id}` for repairs that app added, `POST /entries`). Events: `rentals.rent_due`, `rentals.rent_late`, `rentals.payment_recorded`, `rentals.lease_started`, `rentals.lease_ending`, `rentals.lease_ended`, `rentals.request_created`, `rentals.request_done` — each with `user_email` and a `url`.

## Export and import
`GET /export.json` (everything, with keys), `GET /export.csv` (one row per lease), `POST /import` (the JSON back, a CSV of leases, or a CSV of payments; twice is the same as once), `GET /backup` (admins: the whole database).
