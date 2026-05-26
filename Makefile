SHELL := /bin/zsh

APP_NAME := merger
GO ?= go
CONFIG ?= config/merger.yaml
BUILD_DIR ?= .build
TEST_PKGS := $(shell find . -type f -name '*_test.go' -not -path './.build/*' | xargs -n1 dirname | sort -u)
GO_PACKAGES := ./...
TEST_TREE := ./tests/...
GOCYCLO_VERSION ?= v0.6.0
GOVULNCHECK_VERSION ?= v1.3.0
MIN_INTERNAL_COVERAGE ?= 55.0
BASE_REF ?= origin/main
export GOCACHE := $(abspath $(BUILD_DIR)/go-cache)
export GOMODCACHE := $(abspath $(BUILD_DIR)/go-mod-cache)

.PHONY: fmt fmt-check test test-all vet gocyclo coverage smoke benchmark security ci build verify run-ingest run-controlplane compose-up compose-down docker-build clean

$(BUILD_DIR):
	mkdir -p $(GOCACHE) $(GOMODCACHE)

fmt: $(BUILD_DIR)
	$(GO) fmt ./...

fmt-check: $(BUILD_DIR)
	$(GO) fmt ./...
	git diff --exit-code

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
	"$$($(GO) env GOPATH)/bin/gocyclo" -over 15 $$files

coverage: $(BUILD_DIR)
	$(GO) test $(TEST_TREE) -coverpkg=./internal/... -coverprofile=.coverage.internal.out
	@total="$$(go tool cover -func=.coverage.internal.out | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}')"; \
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

ci: fmt-check vet gocyclo test-all coverage smoke security build

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
