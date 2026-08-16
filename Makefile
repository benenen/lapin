.PHONY: dev build test test-go test-web web-install web-build clean

TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable

dev:
	docker compose up --build

build: web-build
	go build -o bin/lapin ./cmd/lapin

web-install:
	npm --prefix web ci

web-build: web-install
	npm --prefix web run build

test: test-web test-go

test-go: web-build
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -race -coverpkg=./... -cover ./...

test-web: web-install
	npm --prefix web test
	npm --prefix web run typecheck

clean:
	go clean
	npm --prefix web run build -- --emptyOutDir
