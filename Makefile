BINARY=bin/server
PKG=./...

.PHONY: build run test test-integration coverage fmt vet lint tidy migrate seed clean compose-up compose-down

build:
	go build -o $(BINARY) ./cmd/server

run: build
	./$(BINARY) --config configs/config.yaml

test:
	go test $(PKG) -v -race -cover

# Runs the MySQL integration test. It needs a reachable MySQL; the connection
# is taken from the TEST_MYSQL_* variables.
test-integration:
	TEST_MYSQL_HOST=$${TEST_MYSQL_HOST:-127.0.0.1} \
	TEST_MYSQL_PORT=$${TEST_MYSQL_PORT:-3306} \
	TEST_MYSQL_USER=$${TEST_MYSQL_USER:-root} \
	TEST_MYSQL_PASSWORD=$${DATABASE_PASSWORD:-} \
	TEST_MYSQL_DATABASE=$${TEST_MYSQL_DATABASE:-microservice_it} \
	go test ./internal/store -run Integration -v -race

coverage:
	go test $(PKG) -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out | tail -1

fmt:
	go fmt $(PKG)

vet:
	go vet $(PKG)

lint:
	golangci-lint run

tidy:
	go mod tidy

migrate:
	./scripts/migrate.sh

seed:
	./scripts/seed_users.sh

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down -v

clean:
	rm -rf bin coverage.out
