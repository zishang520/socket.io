.DEFAULT_GOAL := help
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --no-print-directory

export GOPROXY ?= https://proxy.golang.org,direct
export TEST_TIMEOUT ?= 60s
export PROJECT_ROOT := $(CURDIR)
export VERSION_FILE := pkg/version/version.go
export CORE_DEP := github.com/zishang520/socket.io/
export VERSION_PATTERN := ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$$
export MODULE VERSION

# Keep this list and its order in sync with the root make.bat.
MODULES := parsers/engine \
           parsers/socket \
           servers/engine \
           servers/socket \
           adapters/adapter \
           adapters/mongo \
           adapters/postgres \
           adapters/redis \
           adapters/unix \
           adapters/valkey \
           clients/engine \
           clients/socket

# Validate the entire scope before any command can change module files.
# Compare literal names, not make filter patterns (which accept '%').
define VALIDATE_SCOPE
scope=". $(MODULES)"; \
if [ -n "$$MODULE" ]; then \
	valid=; \
	for dir in $$scope; do [ "$$dir" != "$$MODULE" ] || valid=1; done; \
	[ -n "$$valid" ] || { printf '[Error] Unknown module: %s\n' "$$MODULE" >&2; exit 1; }; \
	scope=$$MODULE; \
fi; \
for dir in $$scope; do \
	[ -f "$$dir/go.mod" ] || { printf '[Error] Module file not found: %s/go.mod\n' "$$dir" >&2; exit 1; }; \
done
endef

# $1: label, $2: command. Compound commands must explicitly chain failures.
define EXECUTE
	@$(VALIDATE_SCOPE); \
	for dir in $$scope; do \
		printf '[%s] Processing: %s\n' '$1' "$$dir"; \
		(cd "$$dir" && $2) || { \
			status=$$?; \
			printf '[Error] Failed in %s (exit code: %s)\n' "$$dir" "$$status" >&2; \
			exit "$$status"; \
		}; \
	done
endef

DEPS = go mod tidy && go mod vendor

.PHONY: all help env deps get update build fmt vet lint clean test version release
all: help

help:
	@printf '%s\n' \
		'Usage: make command [MODULE=path] [options]' \
		'' \
		'  deps       Run go mod tidy and go mod vendor' \
		'  get        Run go get ./...' \
		'  update     Update dependencies and refresh vendor' \
		'  build      Build packages' \
		'  fmt        Format Go code' \
		'  clean      Clean packages recursively' \
		'  vet        Refresh deps, then run go vet' \
		'  lint       Refresh deps, then run golangci-lint (FIX=1 to auto-fix)' \
		'  test       Refresh deps, then test with race detection and coverage' \
		'  env        Show go env' \
		'  version VERSION=vX.Y.Z     Update VERSION and sync all modules' \
		'  release [MODULE=path]      Create local tags (FORCE=1 to overwrite)' \
		'' \
		'Module: . for root, or one of: $(MODULES)' \
		'Omit MODULE to process root and all listed modules.' \
		'Overrides: GOPROXY, TEST_TIMEOUT (default: 60s).'

env:
	@go env

deps:
	$(call EXECUTE,Deps,$(DEPS))

get:
	$(call EXECUTE,Get,go get ./...)

update:
	$(call EXECUTE,Update,go get -u -v ./...)
	$(call EXECUTE,Deps,$(DEPS))

build:
	$(call EXECUTE,Build,go build ./...)

fmt:
	$(call EXECUTE,Fmt,go fmt ./...)

clean:
	$(call EXECUTE,Clean,go clean -v -r ./...)

vet: deps
	$(call EXECUTE,Vet,go vet ./...)

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { printf '%s\n' '[Error] golangci-lint is not installed. See https://golangci-lint.run/welcome/install/' >&2; exit 1; }
	$(call EXECUTE,Deps,$(DEPS))
	$(call EXECUTE,Lint,golangci-lint run --timeout=5m --config="$$PROJECT_ROOT/.golangci.yml" $(if $(filter 1,$(FIX)),--fix) ./... </dev/null)

test: deps
	$(call EXECUTE,Test,go test -count=1 -timeout="$$TEST_TIMEOUT" -race -cover -covermode=atomic ./... </dev/null)

# Version changes always cover the whole project, independent of MODULE.
version: override MODULE :=
version:
	@$(VALIDATE_SCOPE); \
	[[ "$$VERSION" =~ $$VERSION_PATTERN ]] || { printf '%s\n' '[Error] Expected VERSION=vX.Y.Z or vX.Y.Z-prerelease' >&2; exit 1; }; \
	[ "$$(grep -c '^const VERSION = "[^"]*"$$' "$$VERSION_FILE")" = 1 ] || { printf '%s\n' '[Error] Expected one VERSION constant' >&2; exit 1; }; \
	printf '[Version] Updating %s to %s\n' "$$VERSION_FILE" "$$VERSION"; \
	sed "s/^const VERSION = \"[^\"]*\"$$/const VERSION = \"$$VERSION\"/" "$$VERSION_FILE" > "$$VERSION_FILE.tmp"; \
	mv "$$VERSION_FILE.tmp" "$$VERSION_FILE"
	@for dir in $(MODULES); do \
		printf '[Version] Syncing %s\n' "$$dir"; \
		(cd "$$dir" && \
			go mod tidy && \
			dependencies=$$(go list -mod=mod -f '{{if not .Main}}{{.Path}}{{end}}' -m all) && \
			for dep in $$dependencies; do \
				case "$$dep" in "$$CORE_DEP"*) go get -v "$$dep@$$VERSION" || exit $$? ;; esac; \
			done && \
			go mod tidy) || exit $$?; \
	done
	$(call EXECUTE,Deps,$(DEPS))
	@printf '[Version] Updated to %s\n' "$$VERSION"

release:
	@$(VALIDATE_SCOPE); \
	version=$$(sed -n 's/^const VERSION = "\([^"]*\)"$$/\1/p' "$$VERSION_FILE"); \
	[[ "$$version" =~ $$VERSION_PATTERN ]] || { printf '%s\n' '[Error] Could not read a valid VERSION constant' >&2; exit 1; }; \
	for dir in $$scope; do \
		tag=$$version; \
		[ "$$dir" = . ] || tag="$$dir/$$version"; \
		printf '[Release] Tagging %s\n' "$$tag"; \
		git tag $(if $(filter 1,$(FORCE)),-f) "$$tag"; \
	done; \
	printf '%s\n' '[Release] Tags created locally. Push the intended tags when ready.'
