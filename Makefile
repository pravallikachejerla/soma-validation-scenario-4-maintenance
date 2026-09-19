.PHONY: build test test-public test-private up down seed migrate migrate-down fmt vet lint clean

GO ?= go
NPM ?= npm
DC ?= docker compose

build:
	$(GO) build ./...

test: test-public test-private

test-public:
	$(GO) test ./tests/public/...

test-private:
	$(GO) test ./private/...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

up:
	$(DC) up --build -d

down:
	$(DC) down

migrate:
	$(GO) run ./cmd/migrate -direction up

migrate-down:
	$(GO) run ./cmd/migrate -direction down

seed:
	$(GO) run ./cmd/seed

clean:
	rm -rf frontend/dist
