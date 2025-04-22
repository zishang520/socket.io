# Set Go proxy
export GOPROXY := https://goproxy.io,direct

# List of submodules (relative paths)
MODULES = \
    cmd/socket.io \
    parsers/engine \
    parsers/socket \
    servers/engine \
    servers/socket \
    adapters/adapter \
    adapters/redis \
    clients/engine \
    clients/socket

# Use the provided MODULE or all modules
TARGET_MODULES = $(if $(MODULE),$(MODULE),$(MODULES))
FORCE ?= 0

.PHONY: all help deps get build fmt vet clean test version release

all: help

help:
	@echo ""
	@echo "Usage: make [deps|get|build|fmt|clean|test|version|release] [MODULE=path/to/module|VERSION=v3.0.0[-alpha|beta|rc[.x]]]"
	@echo "If MODULE is not specified, the command applies to all modules."
	@echo ""

deps:
	@echo "[Deps] Running go mod tidy..."
	@echo "[Deps] Tidying [.]"
	@go mod tidy
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Deps] Tidying $$mod"; \
			cd $$mod && go mod tidy && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

get:
	@echo "[Get] Running go get..."
	@echo "[Get] Getting deps for [.]"
	@go get ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Get] Getting deps for $$mod"; \
			cd $$mod && go get ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

build:
	@echo "[Build] Building..."
	@echo "[Build] Building [.]"
	@go build ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Build] Building $$mod"; \
			cd $$mod && go build ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

fmt:
	@echo "[Fmt] Formatting..."
	@echo "[Fmt] Formatting [.]"
	@go fmt ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Fmt] Formatting $$mod"; \
			cd $$mod && go fmt ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

vet:
	@echo "[Vet] Checking..."
	@echo "[Vet] Checking [.]"
	@go vet ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Vet] Checking $$mod"; \
			cd $$mod && go vet ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

clean:
	@echo "[Clean] Cleaning..."
	@echo "[Clean] Cleaning [.]"
	@go clean -v -r ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Clean] Cleaning $$mod"; \
			cd $$mod && go clean -v -r ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

test:
	@echo "[Test] Cleaning test cache..."
	@go clean -testcache
	@echo "[Test] Running tests..."
	@echo "[Test] Testing [.]"
	@go test -timeout=30s -race -cover -covermode=atomic ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Test] Testing $$mod"; \
			cd $$mod && go test -timeout=30s -race -cover -covermode=atomic ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done

version:
ifndef VERSION
	$(error VERSION is required, e.g. make version VERSION=v3.0.0[-alpha|beta|rc[.x]])
endif
	@echo "[Version] Updating version to $(VERSION)"

	@echo "[Version] Updating version.go"
	@sed -i.bak 's/VERSION = ".*"/VERSION = "$(VERSION)"/' pkg/version/version.go && rm -f pkg/version/version.go.bak
	@sed -i.bak -E 's|(github\.com/zishang520/socket\.io/cmd/socket\.io/v3 )[^[:space:]]*( // indirect)|\1$(VERSION)\2|' go.mod && rm -f go.mod.bak

	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Version] Updating dependencies in $$mod..."; \
			cd $$mod && \
			go mod tidy && \
			go list -f '{{if and (not .Indirect) (not .Main)}}{{.Path}}@$(VERSION){{end}}' -m all | \
			grep '^github.com/zishang520/socket.io' | \
			xargs -L1 go get -v && \
			go mod tidy && \
			cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi; \
	done

	@echo "[Version] Done."

release:
	@if ! [ -f "pkg/version/version.go" ]; then \
		echo "[Error] File pkg/version/version.go not found"; \
		exit 1; \
	fi
	@VERSION=$$(grep 'const VERSION' pkg/version/version.go | sed -E 's/.*const VERSION[[:space:]]*=[[:space:]]*"([^"]*)".*/\1/'); \
	if [ -z "$$VERSION" ]; then \
		echo "[Error] Failed to read VERSION from pkg/version/version.go"; \
		exit 1; \
	else \
		echo "[Debug] VERSION extracted: $$VERSION"; \
	fi; \
	echo "[Release] Running in: [.]"; \
	if [ "$(FORCE)" = "1" ]; then \
		echo "[Release] FORCE mode enabled (will overwrite existing tags)"; \
		echo "[Release] Creating/overwriting tags..."; \
		git tag -f "$$VERSION" || true; \
		for mod in $(TARGET_MODULES); do \
			if [ -d "$$mod" ]; then \
				echo "[Release] Forcing tag in: $$mod"; \
				git tag -f "$$mod/$$VERSION" || true; \
			else \
				echo "[Warn] Skipped missing module: $$mod"; \
			fi; \
		done \
	else \
		echo "[Release] Creating tags (use FORCE=1 to overwrite existing tags)..."; \
		git tag "$$VERSION" || true; \
		for mod in $(TARGET_MODULES); do \
			if [ -d "$$mod" ]; then \
				echo "[Release] Tagging: $$mod"; \
				git tag "$$mod/$$VERSION" || true; \
			else \
				echo "[Warn] Skipped missing module: $$mod"; \
			fi; \
		done \
	fi; \
	echo "[Release] Tagged as $$VERSION"