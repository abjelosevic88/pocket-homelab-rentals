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
| `GET/POST /requests` (`ref`: the same one twice gives back the same repair), `GET/PUT/DELETE /requests/{id}` | Repairs; `status: "done"` makes the cost an expense, charges the tenant when `tenant_pays`, ticks off its task, logs it in Pocket Home |
| `GET/POST /expenses`, `PUT/DELETE /expenses/{id}`, `POST /expenses/{id}/money` | Costs of a place |
| `GET/POST /inspections`, `GET/PUT/DELETE /inspections/{id}`, `PUT /inspections/{id}/items` | Inspections; `from_id` starts from another one's lines; moving out answers with `before` |
| `POST /files?owner=item\|request\|lease\|property&owner_id=` (multipart `file`), `GET/DELETE /files/{id}` | Photos and PDFs |

## Overviews and other apps
`GET /today`, `/summary`, `/reports?year=`, `/reports/year.csv?year=`, `/search?q=`, `/lookup?email=` (a tenant by email, for Pocket Contacts), `/day/YYYY-MM-DD` (for Pocket Day), `/entries`, `POST /entries {lease_id|property_id|request_id, kind, title, body}`, `POST /make {owner, owner_id, target}` (contact, task, note, email, signing, invoice, document), `GET /apps` (per app: does it work for you, and if not why), `GET /ics/{lease|request|inspection}/{id}`, `GET /form-answers`, `POST /form-answers/{id}/take`, `POST /ai {want: reminder|late|ending|inspection|repair}`.

## Pocket Core
Offers `rentals:read` (every GET above that is not settings, tokens or exports) and `rentals:write` (`POST /requests`, `PUT /requests/{id}` for repairs that app added, `POST /entries`). Events: `rentals.rent_due`, `rentals.rent_late`, `rentals.payment_recorded`, `rentals.lease_started`, `rentals.lease_ending`, `rentals.lease_ended`, `rentals.request_created`, `rentals.request_done` — each with `user_email` and a `url`.

## Pocket Search, Pocket Day and Pocket Hub

Rentals answers the four routes every Pocket app can offer (`docs/capabilities.md` in Pocket Core), so Pocket Search, Pocket Day and Pocket Hub find it on their own. Each has its own link scope that opens that one route and nothing else (`rentals:read` and `rentals:write` do not open them); a signed-in person or a personal token can call them too. Addresses (`url`) are paths in the app. The three `GET`s only read: rent that is due is made by the app's own pages and its morning run, not here.

| Route | Scope | What |
|---|---|---|
| `GET /pocket/search?q&limit` | `pocket:search` | `{items}`: what `/search` finds, plus units by their own name and notes; a name that has the words first. Each has `id` (`place:3`, `unit:7`, `tenant:2`, `lease:5`, `repair:9`), `title`, `snippet` (the line `/search` shows under it; a unit: who rents it, or "Empty"), `kind` (place, unit, tenant, lease, repair), `date` (a repair: the day it was reported) and `url` (a unit opens its place) |
| `GET /pocket/day/{date}` | `pocket:day` | `{date, summary, items, metrics}`: rent that came in (`kind` payment, with `amount` in cents and `currency`), what was due that day and how far it is paid now (`kind` due, the amount in `sub`, `done` when paid), repairs reported and done, inspections (with `time`), leases that started or ended, and a lease that runs out on that day when it is today or later. `summary` is "Rent received from Ana Marić, 1 repair reported"; `metrics` is the rent in, per currency |
| `GET /pocket/widget` | `pocket:widget` | `{line, metrics, items, url}`: what needs you, as on Today. `line` is "1 rent late, 2 repairs open", empty when nothing does; the items are leases with rent late first, then rent due soon, open repairs (urgent first) and leases ending soon, with `detail` "Ana Marić · 500 EUR late · since 1 Oct"; `metrics` are rent late, due soon, repairs open and units let |
| `POST /pocket/capture` | `pocket:capture` | `{request_id, kind: "repair", text}` adds a repair (new, normal, reported today) and answers `201 {id, title, url}`. With one place it goes there. With more, start with the place or unit and a colon: "Flat 2: tap drips" is kept for Flat 2 as "Tap drips"; "Flat 2, Linden Court: …" when two places have a Flat 2; a place alone means the whole place. Without one the answer is `400` with what to write. More lines become the details. It is added like `POST /requests` with only a place and a title: on the timeline, as a task when your settings make one for every repair; nobody is told and no money is touched. The same `request_id` again answers the same and adds nothing |

## Export and import
`GET /export.json` (everything, with keys), `GET /export.csv` (one row per lease), `POST /import` (the JSON back, a CSV of leases, or a CSV of payments; twice is the same as once), `GET /backup` (admins: the whole database).

Pocket Move can copy your places, leases and rent ledger out and bring them in (`GET /pocket/portable`; link scopes `move:export` / `move:import`). The import answers its counts under `added` and at the top level.
