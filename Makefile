# Set Go proxy
export GOPROXY := https://goproxy.io,direct

# List of submodules (relative paths)
MODULES = \
    adapters/adapter \
    adapters/redis \
    clients/engine \
    clients/socket \
    parsers/engine \
    parsers/socket \
    servers/engine \
    servers/socket

# Use the provided MODULE or all modules
TARGET_MODULES = $(if $(MODULE),$(MODULE),$(MODULES))

.PHONY: all help deps get build fmt vet clean test

all: help

help:
	@echo ""
	@echo "Usage: make [deps|get|build|fmt|clean|test] [MODULE=path/to/module]"
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
	@go test -race -cover -covermode=atomic ./...
	@for mod in $(TARGET_MODULES); do \
		if [ -d "$$mod" ]; then \
			echo "[Test] Testing $$mod"; \
			cd $$mod && go test -race -cover -covermode=atomic ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$mod"; \
		fi \
	done
