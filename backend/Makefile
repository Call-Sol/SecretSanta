# Run these targets from the backend directory.

COMPOSE = docker compose -p secret-santa --project-directory . -f deploy/docker-compose.yml
LOCAL_DATABASE_URL = postgres://secretsanta:secretsanta@localhost:54329/secretsanta?sslmode=disable

.PHONY: up down stack run-bot run-worker run-admin migrate sqlc test test-integration lint

up:
	$(COMPOSE) up -d --wait postgres

stack:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down

run-bot:
	go run ./cmd/bot

run-worker:
	go run ./cmd/worker

run-admin:
	@set -a; [ -f .env ] && . ./.env; set +a; \
		DATABASE_URL="$${DATABASE_URL:-$(LOCAL_DATABASE_URL)}"; \
		export DATABASE_URL; \
		go run ./cmd/admin

migrate: up
	@set -a; [ -f .env ] && . ./.env; set +a; \
		DATABASE_URL="$${DATABASE_URL:-$(LOCAL_DATABASE_URL)}"; \
		export DATABASE_URL; \
		go run ./cmd/migrate

sqlc:
	@if ls db/queries/*.sql >/dev/null 2>&1; then \
		sqlc generate; \
	else \
		echo "sqlc: no query files in db/queries yet"; \
	fi

test:
	go test ./...

test-integration: up
	@set -a; [ -f .env ] && . ./.env; set +a; \
		DATABASE_URL="$${DATABASE_URL:-$(LOCAL_DATABASE_URL)}"; \
		export DATABASE_URL; \
		go test -count=1 -tags=integration ./...

lint:
	go vet ./...
	go vet -tags=integration ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
