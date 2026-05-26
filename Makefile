SHELL := /bin/zsh

APP_NAME := merger
GO ?= go
CONFIG ?= config/merger.yaml
BUILD_DIR ?= .build
TEST_PKGS := $(shell find . -type f -name '*_test.go' -not -path './.build/*' | xargs -n1 dirname | sort -u)
GO_PACKAGES := ./...
TEST_TREE := ./tests/...
GO_FILES := $(shell find cmd internal pkg proto tests -type f -name '*.go' | sort)
GOCYCLO_VERSION ?= v0.6.0
GOVULNCHECK_VERSION ?= v1.0.4
MIN_INTERNAL_COVERAGE ?= 60.0
BASE_REF ?= origin/main
export GOCACHE := $(abspath $(BUILD_DIR)/go-cache)

.PHONY: fmt fmt-check test test-all vet gocyclo coverage smoke benchmark security proto ci-fast ci build verify run-ingest run-controlplane compose-up compose-down docker-build clean

$(BUILD_DIR):
	mkdir -p $(GOCACHE)

fmt: $(BUILD_DIR)
	@if [ -n "$(GO_FILES)" ]; then gofmt -w $(GO_FILES); fi

fmt-check: $(BUILD_DIR)
	@unformatted="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		echo "$$unformatted"; \
		exit 1; \
	fi

test: $(BUILD_DIR)
	@if [ -z "$(TEST_PKGS)" ]; then echo "no test packages found"; exit 1; fi
	$(GO) test $(TEST_PKGS)

test-all: $(BUILD_DIR)
	$(GO) test $(GO_PACKAGES)

vet: $(BUILD_DIR)
	$(GO) vet $(GO_PACKAGES)

gocyclo: $(BUILD_DIR)
	$(GO) install github.com/fzipp/gocyclo/cmd/gocyclo@$(GOCYCLO_VERSION)
	@files="$$(find cmd internal pkg -type f -name '*.go' ! -name '*_test.go' | sort)"; \
	if [ -z "$$files" ]; then \
		echo "No non-test Go files found."; \
		exit 0; \
	fi; \
	find cmd internal pkg -type f -name '*.go' ! -name '*_test.go' -print0 | xargs -0 "$$($(GO) env GOPATH)/bin/gocyclo" -over 15

coverage: $(BUILD_DIR)
	$(GO) test $(TEST_TREE) -coverpkg=./internal/... -coverprofile=$(BUILD_DIR)/coverage.internal.out
	@total="$$(go tool cover -func=$(BUILD_DIR)/coverage.internal.out | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}')"; \
	echo "Total internal coverage: $${total}%"; \
	awk -v total="$$total" -v min="$(MIN_INTERNAL_COVERAGE)" 'BEGIN { exit !(total + 0 >= min + 0) }'

smoke: $(BUILD_DIR)
	$(GO) test ./tests/controlplane ./tests/github ./tests/ingest

benchmark: $(BUILD_DIR)
	@if ! rg -n "^func Benchmark" cmd internal pkg tests >/dev/null 2>&1; then \
		echo "No benchmarks defined."; \
		exit 0; \
	fi
	$(GO) test $(GO_PACKAGES) -run '^$$' -bench . -benchmem

security: $(BUILD_DIR)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	"$$(go env GOPATH)/bin/govulncheck" $(GO_PACKAGES)

proto: $(BUILD_DIR)
	PATH="$$PATH:$$($(GO) env GOPATH)/bin" protoc --proto_path=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/merger/v1/controlplane.proto

ci-fast: fmt-check vet test coverage smoke build

ci: ci-fast gocyclo security test-all

build: $(BUILD_DIR)
	$(GO) build ./cmd/...

verify: test-all build

run-ingest: $(BUILD_DIR)
	MERGER_CONFIG_PATH=$(CONFIG) $(GO) run ./cmd/merger-ingest

run-controlplane: $(BUILD_DIR)
	MERGER_CONFIG_PATH=$(CONFIG) $(GO) run ./cmd/merger-controlplane

compose-up:
	docker compose -f deployments/local/docker-compose.yml up -d

compose-down:
	docker compose -f deployments/local/docker-compose.yml down

docker-build:
	docker build -f deployments/docker/Dockerfile -t $(APP_NAME):dev .

clean:
	rm -rf ./.build
