# HTTP API

The web app is the only client, so the API is internal and may change between
releases. This page is a map for development and debugging.

- Base path `/api`, JSON in and out, dates in ISO 8601, money in integer **paise**.
- Every non-GET request needs the header `X-Requested-With: homeops` (CSRF guard).
- Auth is the `ho_session` cookie. Requests act on the session's **current household**.
- Errors look like `{"code": "...", "message": "..."}` with these codes:

| Status | Codes |
| --- | --- |
| 400 | `bad_request` |
| 401 | `unauthorized` |
| 403 | `forbidden`, `csrf`, `read_only` (viewer), `no_household` |
| 404 | `not_found` |
| 409 | `conflict` |
| 429 | `rate_limited` |

Access column: **public** = no sign-in, **signed in** = any account (even without a
household), **member** = any role in the current household (viewers may only GET),
**admin** / **owner** = at least that role, **platform** = platform administrator.

## Session and account

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | `/health` | public | `{"status":"ok"}` |
| GET | `/bootstrap` | public | Setup state; when signed in: user, household, members, labels, households, VAPID key |
| POST | `/setup` | public | First-run: creates the platform admin and first household. Closed once any user exists |
| POST | `/auth/login` | public | `{username, password}`. Username or email |
| POST | `/auth/logout` | public | |
| POST | `/auth/passkey/login/begin`, `/finish` | public | WebAuthn sign-in |
| POST | `/auth/password/forgot` | public | `{email}`. Always 204; emails a reset link if the account exists |
| GET | `/auth/password/reset/{token}` | public | Validates a reset link |
| POST | `/auth/password/reset` | public | `{token, password}`. Signs out everywhere, then signs in |
| GET | `/auth/invite/{code}` | public | `{householdName, invitedBy, email, role, accountExists}` |
| POST | `/auth/invite/accept` | public | New account from an invite: `{code, username, displayName, password}` |
| POST | `/invites/{code}/join` | signed in | Existing account joins; the invite email must match |
| PATCH | `/me` | signed in | Name, colour, notification prefs, quiet hours, `email` (needs `password`) |
| POST | `/me/password` | signed in | `{current, new}`. Signs out other devices |
| POST | `/me/household` | signed in | `{householdId}`. Switch the current household |
| GET/DELETE | `/me/sessions`, `/me/sessions/{id}` | signed in | Signed-in devices |
| GET/DELETE/POST | `/me/passkeys…` | signed in | Manage passkeys |
| POST | `/push/subscribe`, `/push/unsubscribe`, `/push/test` | signed in | Web Push |
| GET | `/push/devices` | signed in | Registered devices with delivery diagnostics |

## Household and members

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | `/events` | member | Server-Sent Events stream for the household |
| PATCH | `/household` | admin | Name, time zone, sprint cadence, planning time, meal times |
| DELETE | `/household` | owner | `{confirmName}`. Deletes everything |
| GET/POST | `/invites` | admin | List pending / `{email, role}` send (can't exceed your own role) |
| DELETE | `/invites/{code}` | admin | Revoke |
| PATCH | `/members/{id}` | admin | `{role}`. Owners only change owners; at least one owner remains |
| DELETE | `/members/{id}` | admin, or yourself | Remove, or leave |
| GET/POST/PATCH/DELETE | `/labels…` | member | Task labels |

## Platform administration

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | `/admin/households` | platform | All households with owners, member and invite counts |
| POST | `/admin/households` | platform | `{name, timezone, ownerEmail?}`. Owner is you, an existing account, or invited |
| DELETE | `/admin/households/{id}` | platform | `{confirmName}` |
| POST | `/admin/households/{id}/invites` | platform | Invite into any household |
| GET | `/admin/users` | platform | All accounts with their households |
| PATCH | `/admin/users/{id}` | platform | `{email?, isPlatformAdmin?}` |

## Tasks and sprints (member)

`/tasks` (GET, POST, PATCH/DELETE `/{id}`); `/sprints` (GET, PATCH `/{id}`,
POST `/{id}/start`, `/{id}/complete`); `/recurring` (CRUD); `/retros`, `/retro`,
`/retros/{id}/cards|reveal|close`, `/retro-cards/{id}` (PATCH/DELETE, POST `/vote`);
`/metrics/burndown`, `/metrics/team`; POST `/kudos`.

## Kitchen (member)

`/recipes` (CRUD, POST `/import` `{url}`); `/ingredients` (GET, PATCH, POST `/parse`);
`/meals` (CRUD, GET `/suggest`, POST `/autofill`); `/grocery` (CRUD, POST `/generate`,
`/complete`, GET `/trips`); `/pantry` (CRUD, POST `/restock`); `/files` (POST upload,
GET `/{id}`).

Recipes carry `nutrition` (`perServing`, `coverage`, `missing`, `contains`) and
`months` (in season; empty = all year). `PATCH /ingredients/{id}` takes `foodKey`
(`""` = match by name, `"-"` = leave out). `GET /recipes/packs` lists the built-in
recipe packs; `POST /recipes/packs/{name}` adds one, skipping titles you already have.
Meal items carry `portions` (`{userId: servings}`, `null` = default), `leftover` and
the resolved `eaters`.

## Nutrition (member; viewers can't see or change it)

`GET /health` lists each member's profile with computed and final daily `targets`;
`PUT /health/{userId}` saves one (`sex`, `birthYear`, `heightCm`, `weightKg`,
`activity`, `goal`, `avoid`, `overrides`) for **any** member of the household;
`DELETE /health/{userId}`. `GET /nutrition?from&to` returns per-person daily totals,
averages and gaps with recipe ideas. `GET /nutrition/foods` is the food table.

## Money (member; personal rows visible to their owner only)

Under `/money`: `/categories` (+ `/{id}/months/{month}` budgets), `/accounts`,
`/transactions`, `/summary`, `/classify`, `/rules`, `/import/preview` and
`/import/commit` (bank statements), `/imports` (list, DELETE to undo),
`/invoice/preview` and `/invoice/commit` (Zepto and Blinkit PDFs), `/bills`
(CRUD, POST `/{id}/pay`).

## Notifications (member)

GET `/notifications` (the current household's inbox and unread count);
POST `/notifications/read` `{ids?, all?}`, which viewers may also call.
