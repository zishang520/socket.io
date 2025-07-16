# Configuration
# Define Go proxy and project modules
export GOPROXY := https://goproxy.io,direct
MODULES := cmd/socket.io parsers/engine parsers/socket servers/engine servers/socket adapters/adapter adapters/redis clients/engine clients/socket
TARGET_MODULES = $(if $(MODULE),$(MODULE),$(MODULES))
FORCE ?= 0
VERSION_FILE := pkg/version/version.go
TEST_TIMEOUT := 60s

# Declare phony targets to avoid conflicts with files
.PHONY: all help env deps update build fmt vet clean test version release

# Default target: display help
all: help

# Display usage information
help:
	@echo ""
	@echo "Usage: make [command] [MODULE=path/to/module] [VERSION=vX.Y.Z[-alpha|beta|rc[.N]]]"
	@echo "Commands: env deps update build fmt vet clean test version release"
	@echo "If MODULE is not specified, command applies to all modules."
	@echo "Use FORCE=1 for release to overwrite existing tags."
	@echo ""

# Generic function to run a command on all modules or the root directory
define run_module_cmd
	@echo "[$1] Processing: [.]"
	@$2 || { echo "[Error] Failed in [.]"; exit 1; }
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[$1] Processing: $$mod"; \
			(cd "$$mod" && $2) || { echo "[Error] Failed in $$mod"; exit 1; }; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi; \
	done
endef

# Generic function to run a command on a single module
define run_single_module_cmd
	@if [ -d "$2" ]; then \
		echo "[$1] Processing: $2"; \
		(cd "$2" && $3) || { echo "[Error] Failed in $2"; exit 1; }; \
	else \
		echo "[Error] Module not found: $2"; \
		exit 1; \
	fi
endef

# Show env
env:
	@go env || { echo "[Error] Failed in [.]"; exit 1; }

# Manage dependencies: tidy and vendor
deps:
	$(if $(MODULE),$(call run_single_module_cmd,Deps,$(MODULE),go mod tidy && go mod vendor),$(call run_module_cmd,Deps,go mod tidy && go mod vendor))

# Update dependencies and tidy
update:
	$(if $(MODULE),$(call run_single_module_cmd,Update,$(MODULE),go get -u -v ./...),$(call run_module_cmd,Update,go get -u -v ./...))
	@$(MAKE) deps

# Build modules
build:
	$(if $(MODULE),$(call run_single_module_cmd,Build,$(MODULE),go build ./...),$(call run_module_cmd,Build,go build ./...))

# Format code
fmt:
	$(if $(MODULE),$(call run_single_module_cmd,Fmt,$(MODULE),go fmt ./...),$(call run_module_cmd,Fmt,go fmt ./...))

# Run static analysis
vet:
	$(if $(MODULE),$(call run_single_module_cmd,Vet,$(MODULE),go vet ./...),$(call run_module_cmd,Vet,go vet ./...))

# Clean build artifacts
clean:
	$(if $(MODULE),$(call run_single_module_cmd,Clean,$(MODULE),go clean -v -r ./...),$(call run_module_cmd,Clean,go clean -v -r ./...))

# Run tests with race detection and coverage
test: deps
	@echo "[Test] Cleaning test cache..."
	@go clean -testcache || { echo "[Error] Failed to clean test cache"; exit 1; }
	$(if $(MODULE),$(call run_single_module_cmd,Test,$(MODULE),go test -timeout=$(TEST_TIMEOUT) -race -cover -covermode=atomic ./...),$(call run_module_cmd,Test,go test -timeout=$(TEST_TIMEOUT) -race -cover -covermode=atomic ./...))

# Update version in version file and dependencies
version:
ifndef VERSION
	$(error [Error] VERSION is required, e.g., make version VERSION=vX.Y.Z[-alpha|beta|rc[.N]])
endif
	@echo "[Version] Validating format: $(VERSION)"
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$$' || \
		{ echo "[Error] Invalid version format: $(VERSION)"; echo "Expected: vX.Y.Z[-prerelease]"; exit 1; }
	@echo "[Version] Updating to $(VERSION)"
	@if [ ! -f "$(VERSION_FILE)" ]; then \
		echo "[Error] $(VERSION_FILE) not found"; exit 1; \
	fi
	@sed -i.bak 's/VERSION = ".*"/VERSION = "$(VERSION)"/' $(VERSION_FILE) && rm -f $(VERSION_FILE).bak || \
		{ echo "[Error] Failed to update $(VERSION_FILE)"; exit 1; }
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Version] Processing: $$mod"; \
			(cd "$$mod" && \
				go mod tidy && \
				go list -mod=mod -f '{{if and (not .Main)}}{{.Path}}@$(VERSION){{end}}' -m all | \
				grep '^github.com/zishang520/socket.io' | \
				xargs -I {} go get -v {} && \
				go mod tidy) || { echo "[Error] Failed in $$mod"; exit 1; }; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi; \
	done
	@echo "[Version] Completed successfully"
	@$(MAKE) deps

# Create and verify Git tags for release
release:
	@ \
	if [ ! -f "$(VERSION_FILE)" ]; then \
		echo "[Error] $(VERSION_FILE) not found"; \
		exit 1; \
	fi; \
	VERSION=$$(awk -F'"' '/const VERSION/ {print $$2}' "$(VERSION_FILE)"); \
	if [ -z "$$VERSION" ]; then \
		echo "[Error] Failed to read VERSION from $(VERSION_FILE)"; \
		exit 1; \
	fi; \
	echo "[Release] Processing version: $$VERSION"; \
	\
	if [ "$(FORCE)" = "1" ]; then \
		TAG_CMD="git tag -f"; \
		echo "[Release] FORCE mode enabled"; \
	else \
		TAG_CMD="git tag"; \
		echo "[Release] Creating tags (use FORCE=1 to overwrite)"; \
	fi; \
	\
	echo "[Release] Processing: [.]"; \
	$$TAG_CMD "$$VERSION" || { echo "[Error] Failed to create main tag $$VERSION"; exit 1; }; \
	\
	for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Release] Processing: $$mod"; \
			$$TAG_CMD "$$mod/$$VERSION" || { echo "[Error] Failed to create tag $$mod/$$VERSION"; exit 1; }; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi; \
	done; \
	\
	echo "[Release] Verifying tags..."; \
	git show "$$VERSION" >/dev/null 2>&1 || { echo "[Error] Failed to verify main tag $$VERSION"; exit 1; }; \
	\
	for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			git show "$$mod/$$VERSION" >/dev/null 2>&1 || { echo "[Error] Failed to verify tag $$mod/$$VERSION"; exit 1; }; \
		fi; \
	done; \
	\
	echo "[Release] All tags created and verified successfully"