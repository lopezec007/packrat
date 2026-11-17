.PHONY: build test e2e run demo clean dist install

# Git description baked into the binary as the build id (see `packrat -version`);
# the semantic version itself lives in main.go.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -X main.build=$(VERSION)

# A scratch database for `make run` / `make demo` (its name must end in _dev).
DEV_DB ?= postgres:///packrat_dev?host=/var/run/postgresql

build:
	go build -ldflags "$(LDFLAGS)" -o packrat .

# Database tests need PACKRAT_TEST_DATABASE_URL (see internal/testdb); they skip without it.
test:
	go vet ./... && go test ./...

# Needs PACKRAT_E2E_DATABASE_URL (see e2e/playwright.config.js).
e2e:
	cd e2e && (test -d node_modules || npm install) && npx playwright test

# Run against the dev database, serving the UI straight from static/ so edits show on reload.
run: build
	./packrat -addr 127.0.0.1:8082 -db "$(DEV_DB)" -static static

# Wipe the dev database and fill it with a made-up library and a few lists.
demo:
	go run ./cmd/devtool reset "$(DEV_DB)" && go run ./cmd/devtool demo "$(DEV_DB)"

# Static linux/amd64 binary for deployment (pure Go, no cgo: runs on any x86-64 Linux, including Debian 13).
dist:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/packrat .
	@echo "built dist/packrat ($(VERSION))"

# Install as a systemd service on this machine (needs root). Builds the binary
# if Go is available; otherwise pass a prebuilt one: make install BIN=dist/packrat
install:
	./deploy/install.sh $(BIN)

clean:
	rm -f packrat
	rm -rf dist
