BINARY       := send.to
CLIENT       := send
PKG          := github.com/sooua/send.to
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS      := -s -w -X main.version=$(VERSION)
GOFILES      := $(shell find . -type f -name '*.go' -not -path './web/*')
WEB_DIR      := web

.PHONY: all build build-web build-server build-client clean dev dev-server dev-web fmt help \
        lint lint-go lint-web test test-race coverage vet vuln tidy docker \
        docker-run pre-commit

all: build

help:
	@echo "Common targets:"
	@echo "  make build        - build server + client binaries and web bundle"
	@echo "  make build-client - build the 'send' CLI client only"
	@echo "  make test         - run Go tests"
	@echo "  make test-race    - run Go tests with -race"
	@echo "  make coverage     - produce coverage.out + HTML"
	@echo "  make lint         - run Go + web linters"
	@echo "  make vuln         - run govulncheck"
	@echo "  make dev          - run server + web dev concurrently"
	@echo "  make docker       - build docker image"
	@echo "  make pre-commit   - fmt + vet + lint + test"

build: build-web build-server build-client

build-server:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

build-client:
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(CLIENT) ./cmd/send

build-web:
	cd $(WEB_DIR) && npm ci && npm run build

clean:
	rm -f $(BINARY) $(CLIENT) coverage.out coverage.html
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules/.cache

fmt:
	gofmt -s -w $(GOFILES)

GO_PKGS := ./client/... ./cmd/... ./internal/... ./server/... .

vet:
	go vet $(GO_PKGS)

tidy:
	go mod tidy

lint: lint-go lint-web

lint-go:
	golangci-lint run --config .golangci.yml

lint-web:
	cd $(WEB_DIR) && npm run lint && npm run format:check

test:
	go test $(GO_PKGS)

test-race:
	go test -race -count=1 $(GO_PKGS)

coverage:
	go test -coverprofile=coverage.out $(GO_PKGS)
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

vuln:
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck $(GO_PKGS)

dev:
	@echo "Starting server + web in parallel. Ctrl-C to stop."
	@$(MAKE) -j 2 dev-server dev-web

dev-server:
	go run . --listener :18080 --web-path $(WEB_DIR)/dist

dev-web:
	cd $(WEB_DIR) && npm run dev

docker:
	docker build -t $(BINARY):$(VERSION) .

docker-run:
	docker run --rm -p 18080:18080 $(BINARY):$(VERSION)

pre-commit: fmt vet lint test

# Windows-only end-to-end smoke test. Builds the server + supervisor and
# drives the full public API including graceful shutdown.
smoke:
	go build -o send.to.exe .
	go build -o runserver.exe ./test/runserver
	BIN=./send.to.exe RUNSERVER=./runserver.exe bash ./test/smoke/smoke.sh

.PHONY: smoke
