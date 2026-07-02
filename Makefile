TEST_DB_ENV = TEST_DB_HOST=127.0.0.1 TEST_DB_PORT=55432 TEST_DB_USER=postgres TEST_DB_PASSWORD=admin TEST_DB_NAME=postgres

.PHONY: build test test-unit test-db-up test-db-down swagger lint

build:
	go build ./...

# Architecture fitness functions (hexagonal layer guards) + vet. No database required.
# See internal/architecture/guards_test.go and docs/adr/0001-phase3-hexagonal-fitness.md.
lint:
	go vet ./...
	go test ./internal/architecture/...

# Unit packages only — no database required.
test-unit:
	go test ./internal/...

# Start the throwaway test database (postgres on :55432) and wait for health.
test-db-up:
	docker compose -f docker-compose.test.yml up -d --wait

test-db-down:
	docker compose -f docker-compose.test.yml down -v

# Full suite, including the integration package (emplacc-api/test).
test: test-db-up
	$(TEST_DB_ENV) go test ./...

# Regenerate Swagger docs (requires swag: go install github.com/swaggo/swag/cmd/swag@latest).
swagger:
	swag init -g cmd/server/main.go --parseDependency --parseInternal -o docs
