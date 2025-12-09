# ==============================================================================
#  GLOBAL CONFIGURATION
# ==============================================================================
# Silence recursive make output for a cleaner CLI experience
MAKEFLAGS += --no-print-directory

export GOPROXY := https://goproxy.io,direct
TEST_TIMEOUT   := 60s
VERSION_FILE   := pkg/version/version.go
CORE_DEP       := github.com/zishang520/socket.io

# Modules Set (The Domain)
MODULES := parsers/engine \
           parsers/socket \
           servers/engine \
           servers/socket \
           adapters/adapter \
           adapters/redis \
           clients/engine \
           clients/socket

# Scope Calculation: If MODULE is set, S = {MODULE}, else S = {Root} U {MODULES}
# We include '.' (Root) explicitly when running on all.
SCOPE := $(if $(MODULE),$(MODULE),. $(MODULES))

# ANSI Color Codes (Visual Entropy Reduction)
C_RESET  := \033[0m
C_CYAN   := \033[36m
C_GREEN  := \033[32m
C_RED    := \033[31m
C_YELLOW := \033[33m

# ==============================================================================
#  MACROS (The Abstract Machines)
# ==============================================================================

# Macro: EXECUTE
# Iterates through the Scope Set and applies the Command.
# $1: Label (Context)
# $2: Command (Action)
define EXECUTE
	@for dir in $(SCOPE); do \
		if [ -d "$$dir" ]; then \
			printf "$(C_CYAN)[%s] Processing: $$dir$(C_RESET)\n" "$1"; \
			(cd "$$dir" && $2) || { printf "$(C_RED)[Error] Failed in $$dir$(C_RESET)\n"; exit 1; }; \
		else \
			printf "$(C_YELLOW)[Warn] Skipped missing module: $$dir$(C_RESET)\n"; \
		fi; \
	done
endef

# ==============================================================================
#  TARGETS (The Interfaces)
# ==============================================================================
.PHONY: all help env deps update build fmt vet clean test version release

all: help

help:
	@echo ""
	@echo "Usage: make [command] [options]"
	@echo ""
	@echo "Options:"
	@echo "  MODULE=path/to/module   Apply command to specific module only"
	@echo "  VERSION=vX.Y.Z          Required for 'version' command"
	@echo "  FORCE=1                 Force overwrite tags in 'release' command"
	@echo ""
	@echo "Commands:"
	@echo "  deps, get, update       Manage dependencies"
	@echo "  build, fmt, vet, clean  Development workflow"
	@echo "  test                    Run tests with race detection"
	@echo "  version                 Bump version and sync dependencies"
	@echo "  release                 Git tag version (Atomic operation)"
	@echo ""

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
#  SPECIAL TARGETS (High-Entropy Operations)
# ==============================================================================

version:
ifndef VERSION
	$(error $(C_RED)[Error] VERSION is required (e.g., make version VERSION=v1.0.0)$(C_RESET))
endif
	@# 1. Validation
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z\-\.]+)?$$' || \
		{ printf "$(C_RED)[Error] Invalid version format: $(VERSION)$(C_RESET)\n"; exit 1; }

	@# 2. Update Version File (Portable sed: use temp file to avoid BSD/GNU sed differences)
	@printf "$(C_CYAN)[Version] Updating $(VERSION_FILE) to $(VERSION)$(C_RESET)\n"
	@[ -f "$(VERSION_FILE)" ] || { printf "$(C_RED)[Error] File not found: $(VERSION_FILE)$(C_RESET)\n"; exit 1; }
	@sed 's/VERSION = ".*"/VERSION = "$(VERSION)"/' "$(VERSION_FILE)" > "$(VERSION_FILE).tmp" && \
		mv "$(VERSION_FILE).tmp" "$(VERSION_FILE)"

	@# 3. Update Dependencies in Submodules
	@for mod in $(MODULES); do \
		if [ -d "$$mod" ]; then \
			printf "$(C_CYAN)[Version] Syncing $$mod$(C_RESET)\n"; \
			(cd "$$mod" && \
				go mod tidy && \
				go list -mod=mod -f '{{if and (not .Main)}}{{.Path}}@$(VERSION){{end}}' -m all | \
				grep "^$(CORE_DEP)" | \
				xargs -r -I {} go get -v {} && \
				go mod tidy) || exit 1; \
		fi; \
	done
	@printf "$(C_GREEN)[Version] Completed successfully$(C_RESET)\n"
	@$(MAKE) deps

release:
	@[ -f "$(VERSION_FILE)" ] || { printf "$(C_RED)[Error] Version file missing$(C_RESET)\n"; exit 1; }
	$(eval CUR_VER := $(shell awk -F'"' '/const VERSION/ {print $$2}' "$(VERSION_FILE)"))
	@[ -n "$(CUR_VER)" ] || { printf "$(C_RED)[Error] Could not read version$(C_RESET)\n"; exit 1; }

	$(eval TAG_OPTS := $(if $(filter 1,$(FORCE)),-f,))
	@printf "$(C_CYAN)[Release] Tagging version: $(CUR_VER) (Force: $(FORCE))$(C_RESET)\n"

	@# Tag Root
	@git tag $(TAG_OPTS) "$(CUR_VER)" || exit 1

	@# Tag Modules
	@for mod in $(MODULES); do \
		if [ -d "$$mod" ]; then \
			printf "[Release] Tagging $$mod/$(CUR_VER)\n"; \
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