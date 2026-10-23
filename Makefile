# Production (on the home server)
up:            ## build and start everything
	docker compose up -d --build
logs:          ## follow app logs
	docker compose logs -f app
backup-now:    ## take a backup immediately
	docker compose exec backup /bin/sh /backup.sh now
update:        ## pull latest code and redeploy
	git pull && docker compose up -d --build

# Native deploy (systemd, no Docker): see infra/native/
bundle:        ## build a deploy bundle for a server without Go/Node
	./infra/native/bundle.sh

# Local testing on this machine, no Docker or Cloudflare: http://localhost:8080
local:         ## build and run everything locally (data in backend/.devdata)
	./run-local.sh

# Development (no Docker needed)
dev-db:        ## local Postgres on :5433 (data in backend/.devdata)
	cd backend && go run ./cmd/devdb
dev-api:       ## API on :8080, talking to dev-db
	cd backend && DATABASE_URL="postgres://homeops:homeops@localhost:5433/homeops?sslmode=disable" \
		HOMEOPS_PUBLIC_URL=http://localhost:5173 HOMEOPS_DEV=1 go run ./cmd/homeops
dev-web:       ## Vite dev server on :5173 (proxies /api to :8080)
	cd frontend && npm run dev
test:          ## backend tests (uses a throwaway embedded Postgres) + type-check
	cd backend && go test -race ./...
	cd frontend && npx tsc -b

.PHONY: bundle local up logs backup-now update dev-db dev-api dev-web test
