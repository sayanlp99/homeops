# Development

Everything runs locally without Docker. Requires **Go 1.27+** (also looked for in
`~/.local/go`) and **Node 24+**.

## Run it

**One command, closest to production.** This builds the web app and server, then
starts an embedded Postgres plus the app on http://localhost:8080:

```sh
./run-local.sh            # or: make local          (--reset wipes local data)
```

Data lives in `backend/.devdata` (database on :5433, uploads in `files/`). The
script prints a LAN address for phones. On plain http, browsers disable passkeys,
push and offline mode, so test those on localhost or through the tunnel.

**With hot reload** (three terminals):

```sh
make dev-db     # embedded Postgres on :5433
make dev-api    # API on :8080 (uploads to backend/data when no S3 is set)
make dev-web    # Vite on http://localhost:5173, proxying /api to :8080
```

`go run ./cmd/devdb` honours `DEVDB_PORT` and `DEVDB_DIR`, so a second, throwaway
database can run next to the main one (handy for end-to-end tests).

## Tests

```sh
make test       # backend tests + frontend type-check
cd backend && go test ./...            # just the backend
cd backend && go test ./internal/api -run TestHouseholdsAndRoles -v
```

- API tests (`internal/api/*_test.go`) are integration tests. Each starts a fresh
  embedded Postgres on its own port, runs every migration (twice, to prove they're
  idempotent) and drives the HTTP API with real cookies.
  `HOMEOPS_TEST_S3_ENDPOINT` / `_ACCESS_KEY` / `_SECRET_KEY` run them against real
  object storage.
- Parser tests use **anonymised** fixtures in `internal/statement/testdata` and
  `internal/invoice/testdata`. Never commit real statements or invoices. For a new
  bank or shop format, anonymise names, account numbers, UPI references and
  addresses first.
- Email in tests goes to a capture mailer (`households_test.go`), so invite and
  reset links can be followed.

## Conventions

- **Money** is integer paise everywhere; format it only in the UI.
- **Every query is household-scoped.** Take `currentUser(r).HouseholdID`, and for
  money also apply the owner (privacy) condition. A handler that forgets the scope is
  a data leak between households.
- **Roles**: put role checks on routes (`requireRole`, `platformAdminOnly`) rather
  than inside handlers. Viewers are blocked centrally in `requireHousehold`.
- **Notifications** must set `HouseholdID`. Dedupe keys make scheduled jobs safe to
  re-run.
- **Realtime**: after a write, `s.hub.Publish(householdID, eventX.from(r))` so other
  devices refresh. The frontend maps topics to query keys in `lib/realtime.ts`.
- **Migrations** are append-only: add `NNNN_name.sql` and convert existing data in
  the same file. Rehearse big ones against a copy of production
  ([operations.md](operations.md#rehearsing-a-migration)).
- **Frontend** mutations that should work offline are registered with defaults in
  `lib/tasks.ts`, `lib/meals.ts` and `lib/money.ts`. Optimistic updates, then
  invalidate on settle.
- Keep the style of the surrounding code; comments explain *why*, not *what*.

## Adding a recipe site

Most sites work through their schema.org JSON-LD (`parseStructuredRecipe`). For a
site that leaves it empty, as BongEats does:

1. Add a parser to `siteParsers` in `backend/internal/api/recipe_sites.go`. It
   receives the parsed HTML document and refines the draft.
2. Reuse `parseIngredientLine` for amounts and units, and `classifyAisle` / `guessDiet`.
3. Add a test with a small **synthetic** page in the site's layout (see
   `TestBongEatsImport`). Don't commit copies of the site's pages.

Regional ingredient names (Bengali, Hindi…) belong in the aisle rules in `units.go`.

## Building for the server

```sh
make bundle     # static linux/amd64 binaries + web build + infra + pinned downloads
```

See [deployment.md](deployment.md).
