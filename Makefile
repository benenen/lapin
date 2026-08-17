.PHONY: dev watch air watch-admin-bootstrap build build-cli test test-go test-web test-watch test-browser web-install web-build clean

TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable
DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable
HASHID_SALT ?= lapin-development-salt
AIR_VERSION := v1.67.3
AIR_GOBIN := $(CURDIR)/bin/tools/air/$(AIR_VERSION)
AIR_BIN := $(AIR_GOBIN)/air
AIR_INSTALLER := scripts/install-air.sh
WATCH_RUNNER := scripts/watch.sh
WATCH_ADMIN_BOOTSTRAP_BIN := $(CURDIR)/bin/watch/lapin-admin-bootstrap
BIN_DIR := bin

dev:
	docker compose up --build

air: $(AIR_INSTALLER)
	sh '$(AIR_INSTALLER)' '$(AIR_VERSION)' '$(AIR_BIN)'

watch: export DATABASE_URL := $(DATABASE_URL)
watch: export HASHID_SALT := $(HASHID_SALT)
watch: | $(WATCH_RUNNER)
	@bash '$(WATCH_RUNNER)' '$(AIR_BIN)' '$(WATCH_ADMIN_BOOTSTRAP_BIN)'

watch-admin-bootstrap:
	mkdir -p '$(dir $(WATCH_ADMIN_BOOTSTRAP_BIN))'
	go build -o '$(WATCH_ADMIN_BOOTSTRAP_BIN)' ./cmd/lapin-admin-bootstrap

build: web-build build-cli | $(BIN_DIR)
	go build -o $(BIN_DIR)/lapin ./cmd/lapin

build-cli: | $(BIN_DIR)
	go build -o $(BIN_DIR)/lapin-cli ./cmd/lapin-cli

$(BIN_DIR):
	mkdir -p '$(BIN_DIR)'

web-install:
	npm --prefix web ci

web-build: web-install
	npm --prefix web run build

test: test-web test-watch test-go

test-go: web-build
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -race -coverpkg=./... -cover ./...

test-web: web-install
	npm --prefix web test
	npm --prefix web run typecheck

test-watch:
	bash scripts/watch_test.sh

test-browser:
	bash scripts/whiteboard_browser_test.sh

clean:
	go clean
	npm --prefix web run build -- --emptyOutDir
