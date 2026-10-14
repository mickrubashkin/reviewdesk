.PHONY: db-up db-down db-logs db-migrate db-migrate-status db-migrate-down db-seed db-seed-demo db-reset-demo api-dev web-dev dev setup-dev test-api test-web lint smoke-check

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

api-dev:
	cd apps/api && go run ./cmd/server

web-dev:
	cd apps/web && npm run dev

setup-dev: db-up db-migrate
	DEMO_SEED_ENABLED=true $(MAKE) db-seed-demo

test-api:
	cd apps/api && go test ./...

test-web:
	cd apps/web && npm run lint && npm run build

lint: test-api test-web

dev: db-up
	@echo "Starting API and web dev servers..."
	@trap 'kill $$api_pid $$web_pid 2>/dev/null || true' INT TERM EXIT; \
	(cd apps/api && go run ./cmd/server) & api_pid=$$!; \
	(cd apps/web && npm run dev) & web_pid=$$!; \
	wait

smoke-check:
	./scripts/smoke-check.sh "$$SMOKE_ORIGIN"
