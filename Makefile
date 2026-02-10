# ==============================================================================
#  GLOBAL CONFIGURATION
# ==============================================================================
.DEFAULT_GOAL := help
SHELL := /bin/bash

# Silence make output for a clean CLI experience
MAKEFLAGS += --no-print-directory

# Environment
export GOPROXY := https://goproxy.io,direct
TEST_TIMEOUT   := 60s

# Project Metadata
VERSION_FILE   := pkg/version/version.go
CORE_DEP       := github.com/zishang520/socket.io

# Modules Definition (The Domain)
MODULES := parsers/engine \
           parsers/socket \
           servers/engine \
           servers/socket \
           adapters/adapter \
           adapters/redis \
           clients/engine \
           clients/socket

# Scope Logic: If MODULE=... is passed, use it; otherwise Root (.) + All Modules
SCOPE := $(if $(MODULE),$(MODULE),. $(MODULES))

# ANSI Color Codes
C_RESET  := \033[0m
C_CYAN   := \033[36m
C_GREEN  := \033[32m
C_RED    := \033[31m
C_YELLOW := \033[33m

# ==============================================================================
#  MACROS (The Abstract Machines)
# ==============================================================================

# Macro: EXECUTE
# Safe iteration with Fail-Fast logic.
# $1: Label (Context)
# $2: Command (Action)
define EXECUTE
	@for dir in $(SCOPE); do \
		if [ -d "$$dir" ]; then \
			printf "$(C_CYAN)[%s] Processing: $$dir$(C_RESET)\n" "$1"; \
			(cd "$$dir" && $2) || { \
				printf "$(C_RED)[Error] Failed in $$dir (Exit Code: $$?)$(C_RESET)\n"; \
				exit 1; \
			}; \
		else \
			printf "$(C_YELLOW)[Warn] Skipped missing module: $$dir$(C_RESET)\n"; \
		fi; \
	done
endef

# ==============================================================================
#  TARGETS (The Interfaces)
# ==============================================================================

.PHONY: all help env deps get update build fmt vet clean test version release

all: help

help:
	@printf "\n"
	@printf "$(C_GREEN)Project Makefile Interface$(C_RESET)\n"
	@printf "\n"
	@printf "$(C_YELLOW)Usage:$(C_RESET) make [command] [options]\n"
	@printf "\n"
	@printf "$(C_CYAN)Options:$(C_RESET)\n"
	@printf "  MODULE=path/to/dir   Run command on specific module only\n"
	@printf "  VERSION=vX.Y.Z       Required for 'version' command\n"
	@printf "  FORCE=1              Force overwrite tags in 'release' command\n"
	@printf "\n"
	@printf "$(C_CYAN)Commands:$(C_RESET)\n"
	@printf "  deps       Run 'go mod tidy' & 'go mod vendor'\n"
	@printf "  get        Run 'go get ./...'\n"
	@printf "  update     Run 'go get -u' and refresh deps\n"
	@printf "  build      Build all modules\n"
	@printf "  fmt        Format code (go fmt)\n"
	@printf "  vet        Run go vet\n"
	@printf "  clean      Clean build cache\n"
	@printf "  test       Run tests with race detection\n"
	@printf "  version    Update version file and sync submodules\n"
	@printf "  release    Create git tags for Root and Modules\n"
	@printf "\n"

env:
	@go env

deps:
	$(call EXECUTE,Deps,go mod tidy && go mod vendor)

get:
	$(call EXECUTE,Get,go get ./...)

update:
	$(call EXECUTE,Update,go get -u -v ./...)
	@$(MAKE) deps

build:
	$(call EXECUTE,Build,go build ./...)

fmt:
	$(call EXECUTE,Fmt,go fmt ./...)

vet: deps
	$(call EXECUTE,Vet,go vet ./...)

clean:
	$(call EXECUTE,Clean,go clean -v -r ./...)

test: deps
	@printf "$(C_CYAN)[Test] Cleaning test cache...$(C_RESET)\n"
	@go clean -testcache
	$(call EXECUTE,Test,go test -timeout=$(TEST_TIMEOUT) -race -cover -covermode=atomic ./...)

# ==============================================================================
#  SPECIAL OPERATIONS (High-Risk)
# ==============================================================================

version:
ifndef VERSION
	$(error $(C_RED)[Error] VERSION is required (e.g., make version VERSION=v1.0.0)$(C_RESET))
endif
	@# 1. Validation (Strict Regex)
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z\-\.]+)?$$' || \
		{ printf "$(C_RED)[Error] Invalid version format: $(VERSION)$(C_RESET)\n"; exit 1; }

	@# 2. Update Version File (Portable atomic write)
	@printf "$(C_CYAN)[Version] Updating $(VERSION_FILE) to $(VERSION)$(C_RESET)\n"
	@[ -f "$(VERSION_FILE)" ] || { printf "$(C_RED)[Error] File not found: $(VERSION_FILE)$(C_RESET)\n"; exit 1; }
	@sed 's/VERSION = ".*"/VERSION = "$(VERSION)"/' "$(VERSION_FILE)" > "$(VERSION_FILE).tmp" && \
		mv "$(VERSION_FILE).tmp" "$(VERSION_FILE)"

	@# 3. Update Dependencies in Submodules
	@# Note: Replaced xargs -r with shell logic for macOS compatibility
	@for mod in $(MODULES); do \
		if [ -d "$$mod" ]; then \
			printf "$(C_CYAN)[Version] Syncing $$mod$(C_RESET)\n"; \
			(cd "$$mod" && \
				go mod tidy && \
				TARGETS=$$(go list -mod=mod -f '{{if and (not .Main)}}{{.Path}}@$(VERSION){{end}}' -m all | grep "^$(CORE_DEP)"); \
				if [ -n "$$TARGETS" ]; then \
					echo "$$TARGETS" | xargs go get -v; \
				fi; \
				go mod tidy) || exit 1; \
		fi; \
	done
	@printf "$(C_GREEN)[Version] Completed successfully. Submodules synced.$(C_RESET)\n"
	@$(MAKE) deps

release:
	@[ -f "$(VERSION_FILE)" ] || { printf "$(C_RED)[Error] Version file missing$(C_RESET)\n"; exit 1; }
	$(eval CUR_VER := $(shell awk -F'"' '/const VERSION/ {print $$2}' "$(VERSION_FILE)"))
	@[ -n "$(CUR_VER)" ] || { printf "$(C_RED)[Error] Could not read version from $(VERSION_FILE)$(C_RESET)\n"; exit 1; }

	$(eval TAG_OPTS := $(if $(filter 1,$(FORCE)),-f,))
	@printf "$(C_CYAN)[Release] Tagging version: $(CUR_VER) (Force: $(FORCE))$(C_RESET)\n"

	@# Tag Root
	@git tag $(TAG_OPTS) "$(CUR_VER)" || exit 1

	@# Tag Modules
	@for mod in $(MODULES); do \
		if [ -d "$$mod" ]; then \
			printf "  Tagging $$mod/$(CUR_VER)\n"; \
			git tag $(TAG_OPTS) "$$mod/$(CUR_VER)" || exit 1; \
		fi; \
	done

	@# Verification
	@printf "$(C_GREEN)[Release] Verifying tags...$(C_RESET)\n"
	@git show "$(CUR_VER)" >/dev/null 2>&1 || exit 1
	@for mod in $(MODULES); do \
		[ -d "$$mod" ] && git show "$$mod/$(CUR_VER)" >/dev/null 2>&1 || exit 1; \
	done
	@printf "$(C_GREEN)[Release] All tags verified.$(C_RESET)\n"