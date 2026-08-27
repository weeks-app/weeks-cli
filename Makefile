# Conventions: github.com/basecamp/cli/MAKEFILE-CONVENTION.md. Targets that
# the toolkit defines but this repo cannot yet honour — release, skills sync,
# PGO — are deliberately absent rather than present and broken; they arrive
# with the release pipeline.

BINARY_NAME := weeks
BUILD_DIR := ./bin

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

MODULE := github.com/weeks-app/weeks-cli
LDFLAGS := -X $(MODULE)/internal/version.Version=$(VERSION) \
           -X $(MODULE)/internal/version.Commit=$(COMMIT) \
           -X $(MODULE)/internal/version.Date=$(DATE)

.DEFAULT_GOAL := check

.PHONY: check check-all check-toolchain build test test-race test-e2e vet lint fmt fmt-check \
	tidy tidy-check replace-check check-surface check-surface-diff check-surface-compat \
	vuln secrets security coverage clean

# Fast checks for the inner loop and the pre-commit hook.
check: fmt-check vet test tidy-check

# Everything CI runs. Slower: lint, the race detector, the CLI surface.
check-all: fmt-check vet lint test-race test-e2e check-surface

# mise puts Go on PATH per directory. A shell that half-activated it builds
# against one toolchain and links against another, which fails confusingly.
check-toolchain:
	@GOV=$$(go version | awk '{print $$3}'); \
	ROOT=$$(go env GOROOT); \
	ROOTV=$$($$ROOT/bin/go version | awk '{print $$3}'); \
	if [ "$$GOV" != "$$ROOTV" ]; then \
		echo "ERROR: Go toolchain mismatch"; \
		echo "  PATH go:   $$GOV ($$(which go))"; \
		echo "  GOROOT go: $$ROOTV ($$ROOT/bin/go)"; \
		echo "Fix: eval \"\$$(mise hook-env)\" && make <target>"; \
		exit 1; \
	fi

build: check-toolchain
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

test: check-toolchain
	go test ./...

test-race: check-toolchain
	go test -race ./...

# bats drives the built binary the way a caller does, which is the only way to
# test the envelope and the exit codes together.
test-e2e: build
	@command -v bats >/dev/null 2>&1 || { echo "bats not installed; skipping e2e (brew install bats-core)"; exit 0; }
	bats e2e/

vet:
	go vet ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt' to fix formatting:" && gofmt -l . && exit 1)

coverage: check-toolchain
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

tidy:
	go mod tidy

# Verify go.mod/go.sum are tidy without leaving them modified on failure.
tidy-check:
	@set -e; cp go.mod go.mod.tidycheck; cp go.sum go.sum.tidycheck; \
	restore() { mv go.mod.tidycheck go.mod; mv go.sum.tidycheck go.sum; }; \
	if ! go mod tidy; then \
		restore; \
		echo "'go mod tidy' failed. Restored original go.mod/go.sum."; \
		exit 1; \
	fi; \
	if ! git diff --quiet -- go.mod go.sum; then \
		restore; \
		echo "go.mod/go.sum are not tidy. Run 'make tidy' and commit the result."; \
		exit 1; \
	fi; \
	rm -f go.mod.tidycheck go.sum.tidycheck

replace-check:
	@if grep -q '^[[:space:]]*replace[[:space:]]' go.mod; then \
		echo "ERROR: go.mod contains replace directives"; \
		grep '^[[:space:]]*replace[[:space:]]' go.mod; \
		exit 1; \
	fi
	@echo "Replace check passed (no local replace directives)"

# The CLI surface is a contract with agents: a removed flag or subcommand
# breaks a plan someone already made.
check-surface: build
	@command -v jq >/dev/null 2>&1 || { \
		echo "ERROR: jq is required for check-surface but was not found."; \
		echo "Install with: brew install jq (macOS), apt-get install jq (Debian/Ubuntu)"; \
		exit 1; \
	}
	scripts/check-cli-surface.sh $(BUILD_DIR)/$(BINARY_NAME) /tmp/weeks-cli-surface.txt
	@echo "CLI surface snapshot generated ($$(wc -l < /tmp/weeks-cli-surface.txt) entries)"

check-surface-diff:
	scripts/check-cli-surface-diff.sh $(BASELINE) $(CURRENT)

check-surface-compat: build
	@scripts/check-cli-surface.sh $(BUILD_DIR)/$(BINARY_NAME) /tmp/weeks-current-surface.txt
	@PREV_TAG=$$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo ""); \
	if [ -n "$$PREV_TAG" ]; then \
		SCRIPT_DIR="$$(pwd)/scripts"; \
		BASELINE_DIR=$$(mktemp -d); \
		cleanup() { git worktree remove "$$BASELINE_DIR" --force 2>/dev/null || true; rm -rf "$$BASELINE_DIR" 2>/dev/null || true; }; \
		trap cleanup EXIT; \
		git worktree add "$$BASELINE_DIR" "$$PREV_TAG" || { echo "Failed to create worktree for $$PREV_TAG"; exit 1; }; \
		cd "$$BASELINE_DIR" && make build && \
		"$$SCRIPT_DIR/check-cli-surface.sh" "./bin/$(BINARY_NAME)" /tmp/weeks-baseline-surface.txt; \
		cd - >/dev/null; \
		cleanup; trap - EXIT; \
		scripts/check-cli-surface-diff.sh /tmp/weeks-baseline-surface.txt /tmp/weeks-current-surface.txt; \
	else \
		echo "First release — no baseline to compare against"; \
	fi

vuln:
	govulncheck ./...

secrets:
	@command -v gitleaks >/dev/null || (echo "Install gitleaks: brew install gitleaks" && exit 1)
	gitleaks detect --source . --verbose

security: lint vuln secrets

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
