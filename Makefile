.PHONY: dev watch air build test test-go test-web web-install web-build clean

TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable
DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable
HASHID_SALT ?= lapin-development-salt
AIR_VERSION := v1.67.3
AIR_GOBIN := $(CURDIR)/bin/tools/air/$(AIR_VERSION)
AIR_BIN := $(AIR_GOBIN)/air
AIR_INSTALLER := scripts/install-air.sh
WATCH_RUNNER := scripts/watch.sh

dev:
	docker compose up --build

air: $(AIR_INSTALLER)
	sh '$(AIR_INSTALLER)' '$(AIR_VERSION)' '$(AIR_BIN)'

watch: export DATABASE_URL := $(DATABASE_URL)
watch: export HASHID_SALT := $(HASHID_SALT)
watch: web-build air | $(WATCH_RUNNER)
	@bash '$(WATCH_RUNNER)' '$(AIR_BIN)'

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
