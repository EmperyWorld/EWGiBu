# iacpack developer targets. Behavior must not depend on ambient locale/timezone.
ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BIN := $(ROOT)/bin
GO ?= go
export GOTOOLCHAIN ?= go1.27.1
export GOFLAGS ?=
export CGO_ENABLED ?= 0

# Pinned analysis tools (exact versions).
STATICCHECK_VERSION ?= v0.8.1
GOVULNCHECK_VERSION ?= v1.8.0

.PHONY: help fmt fmt-check vet staticcheck test test-race test-security \
	test-integration test-e2e test-ui test-release test-all \
	test-fuzz-smoke coverage-core reproduce-build shellcheck rc-evidence \
	docker-e2e build ui ui-sync check tools tidy

help:
	@printf '%s\n' \
		'fmt              format Go sources' \
		'fmt-check        fail if gofmt needed' \
		'vet              go vet' \
		'staticcheck      run pinned staticcheck' \
		'test             go test ./...' \
		'test-race        go test -race ./...' \
		'test-security    govulncheck + security-oriented tests' \
		'test-integration compile/bundle/plan integration (no Docker required)' \
		'test-e2e         apply/destroy/resume scenario suite + runtime tests' \
		'test-ui          Web UI unit/build + Playwright' \
		'test-release     release rehearsal artifacts (no publish)' \
		'test-fuzz-smoke  short fuzz campaign' \
		'coverage-core    core package coverage floors' \
		'reproduce-build  double-build digest compare' \
		'shellcheck       lint shell scripts' \
		'docker-e2e       Docker compose-config (+ optional apply) gate' \
		'rc-evidence      aggregate local RC evidence pack' \
		'test-all         check + integration + e2e + ui + release + p15 smokes' \
		'build            build ./cmd/iacpack into ./bin/iacpack (local only)' \
		'check            fmt-check vet staticcheck test test-race test-security' \
		'tools            install pinned tools into ./bin'

tools: $(BIN)/staticcheck $(BIN)/govulncheck

$(BIN)/staticcheck:
	@mkdir -p $(BIN)
	GOBIN=$(BIN) $(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

$(BIN)/govulncheck:
	@mkdir -p $(BIN)
	GOBIN=$(BIN) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

fmt:
	gofmt -w $(shell find $(ROOT) -name '*.go' -not -path '$(ROOT)/.gocache/*')

fmt-check:
	@unformatted=$$(gofmt -l $$(find $(ROOT) -name '*.go' -not -path '$(ROOT)/.gocache/*')); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	cd $(ROOT) && $(GO) vet ./...

staticcheck: $(BIN)/staticcheck
	cd $(ROOT) && $(BIN)/staticcheck ./...

test:
	cd $(ROOT) && $(GO) test ./...

test-race:
	cd $(ROOT) && CGO_ENABLED=1 $(GO) test -race ./...

test-security: $(BIN)/govulncheck
	cd $(ROOT) && $(BIN)/govulncheck ./...
	cd $(ROOT) && $(GO) test ./internal/archrule ./internal/adapters/cli ./internal/bundle ./internal/domain/path ./internal/inputs

test-integration:
	cd $(ROOT) && $(GO) test ./internal/compile ./internal/bundle ./internal/lockfile \
		./internal/adapters/cli -run 'TestBuild|TestPlan|TestLock|TestValidate|TestYAML|TestPack|TestVerify' -count=1

test-e2e:
	cd $(ROOT) && $(GO) test ./internal/app/apply ./internal/app/destroy ./internal/runtime/state \
		./internal/adapters/cli -run 'TestApply|TestDestroy|TestResume|TestSecretCanary|TestProcess' -count=1

ui:
	cd $(ROOT)/web && pnpm install --frozen-lockfile
	cd $(ROOT)/web && pnpm check:all
	$(ROOT)/scripts/sync-ui-assets.sh

ui-sync:
	$(ROOT)/scripts/sync-ui-assets.sh

test-ui: ui
	cd $(ROOT)/web && pnpm exec playwright install chromium
	cd $(ROOT) && $(MAKE) build
	cd $(ROOT)/web && pnpm test:e2e

test-release: build
	cd $(ROOT) && sh scripts/release-rehearsal.sh
	@test -f $(ROOT)/dist/rehearsal/SHA256SUMS
	@test -f $(ROOT)/dist/rehearsal/sbom/iacpack.spdx.json
	@test -f $(ROOT)/dist/rehearsal/provenance/iacpack.provenance.json
	cd $(ROOT) && $(GO) test ./internal/supplychain ./internal/oci/layout ./internal/app/images

test-fuzz-smoke:
	cd $(ROOT) && sh scripts/p15/fuzz-smoke.sh

coverage-core:
	cd $(ROOT) && sh scripts/p15/coverage-core.sh

reproduce-build:
	cd $(ROOT) && sh scripts/p15/reproduce-build.sh

shellcheck:
	cd $(ROOT) && sh scripts/p15/shellcheck.sh

docker-e2e:
	cd $(ROOT) && sh scripts/p15/docker-e2e.sh

rc-evidence: check shellcheck reproduce-build coverage-core test-fuzz-smoke test-release docker-e2e
	cd $(ROOT) && FULL=1 sh scripts/p15/perf-bench.sh
	@mkdir -p $(ROOT)/dist/rc-evidence
	@cp -a $(ROOT)/dist/rehearsal/. $(ROOT)/dist/rc-evidence/rehearsal/ 2>/dev/null || true
	@printf 'RC evidence ready under dist/rc-evidence (see docs/release/PUBLISH.md)\n'

test-all: check test-integration test-e2e test-ui test-release shellcheck reproduce-build coverage-core

build:
	@mkdir -p $(BIN)
	cd $(ROOT) && $(GO) build -trimpath -ldflags "-s -w" -o $(BIN)/iacpack ./cmd/iacpack

check: fmt-check vet staticcheck test test-race test-security

tidy:
	cd $(ROOT) && $(GO) mod tidy
