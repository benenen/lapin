.PHONY: dev build test test-go test-web clean

TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable

dev:
	docker compose up --build

build:
	npm --prefix web ci
	npm --prefix web run build
	go build -o bin/lapin ./cmd/lapin

test: test-go test-web

test-go:
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -race -coverpkg=./... -cover ./...

test-web:
	npm --prefix web test
	npm --prefix web run typecheck

clean:
	go clean
	npm --prefix web run build -- --emptyOutDir
