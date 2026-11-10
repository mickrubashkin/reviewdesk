.PHONY: start build test deploy logs public-review db-up db-down db-logs db-migrate db-migrate-status db-migrate-down db-seed db-seed-demo db-reset-demo db-sync-meta api-dev web-dev dev setup-dev test-api test-web lint smoke-check

# Everyday entry points.

start: dev

build:
	cd apps/api && go build ./...
	cd apps/web && npm run build

test: test-api test-web

# Builds the VibeCode package and deploys it to the VM (deploy/vibecode/README.md).
# MSG="…" also posts a changelog line to the app's channel in Bitrix24.
deploy:
	./scripts/vibecode-build.sh
	./scripts/vibecode-deploy.sh "$(MSG)"

# The last N (default 100) lines of the app's log on the VibeCode VM.
logs:
	./scripts/vibecode-logs.sh $(N)

# What the public copy would get from REF (default origin/main), left
# uncommitted in ../reviewdesk to look at.
public-review:
	./scripts/public/sync.sh --review $(REF)

db-up:
	docker compose up -d db

db-down:
	docker compose down

db-logs:
	docker compose logs -f db

db-migrate:
	set -a; . ./.env; set +a; goose -dir db/migrations postgres "$$DATABASE_URL" up

db-migrate-status:
	set -a; . ./.env; set +a; goose -dir db/migrations postgres "$$DATABASE_URL" status

db-migrate-down:
	set -a; . ./.env; set +a; goose -dir db/migrations postgres "$$DATABASE_URL" down

db-seed:
	cd apps/api && go run ./cmd/seed

db-seed-demo:
	cd apps/api && go run ./cmd/demoseed

db-reset-demo:
	cd apps/api && go run ./cmd/demoseed -reset

db-sync-meta:
	cd apps/api && go run ./cmd/syncmeta

api-dev:
	cd apps/api && go run ./cmd/server

web-dev:
	cd apps/web && npm run dev

setup-dev: db-up db-migrate
	@if [ -d db/seeds/emails ]; then $(MAKE) db-seed; else DEMO_SEED_ENABLED=true $(MAKE) db-seed-demo; fi

# Database tests run on email_review_tool_test next to DATABASE_URL (or
# TEST_DATABASE_URL), migrated first; without it they are skipped. .env stays
# out of go test: its AUTH_DEV_LOGIN_ENABLED fails an auth test.
test-api:
	@db="$${TEST_DATABASE_URL:-$$(set -a; . ./.env; printf '%s' "$${DATABASE_URL%/*}/email_review_tool_test")}"; \
	if goose -dir db/migrations postgres "$$db" up >/dev/null 2>&1; then \
		cd apps/api && TEST_DATABASE_URL="$$db" go test -p 1 ./...; \
	else \
		echo "No test database (make db-up?): database tests are skipped"; \
		cd apps/api && go test ./...; \
	fi

test-web:
	cd apps/web && npm run lint && npm test && npm run build

lint: test-api test-web

dev: db-up
	@echo "Starting API and web dev servers..."
	@trap 'kill $$api_pid $$web_pid 2>/dev/null || true' INT TERM EXIT; \
	(cd apps/api && go run ./cmd/server) & api_pid=$$!; \
	(cd apps/web && npm run dev) & web_pid=$$!; \
	wait

smoke-check:
	./scripts/smoke-check.sh "$$SMOKE_ORIGIN"
