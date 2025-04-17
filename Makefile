# Makefile

# Go proxy
export GOPROXY := https://goproxy.io,direct

# List of all submodules
MODULES := \
	adapters/adapter \
	adapters/redis \
	clients/engine \
	clients/socket \
	parsers/engine \
	parsers/socket \
	servers/engine \
	servers/socket

# Default target
.PHONY: default
default:
	@echo "[Info] No command provided. Doing nothing."

.PHONY: help
help:
	@echo ""
	@echo "Usage: make [default|deps|fmt|clean|test] [MODULE=module_path]"
	@echo "If MODULE is not provided, the command applies to all."
	@echo ""

.PHONY: deps
deps:
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Deps] Tidy module: $(MODULE)"; \
		cd "$(MODULE)" && go mod tidy && cd - >/dev/null; \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
	fi
else
	@echo "[Deps] Running go mod tidy for all modules..."
	go mod tidy
	@for m in $(MODULES); do \
		if [ -d "$$m" ]; then \
			cd "$$m" && go mod tidy && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$m"; \
		fi \
	done
endif

.PHONY: fmt
fmt:
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Fmt] Formatting module: $(MODULE)"; \
		cd "$(MODULE)" && go fmt ./... && cd - >/dev/null; \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
	fi
else
	@echo "[Fmt] Formatting all modules..."
	go fmt ./...
	@for m in $(MODULES); do \
		if [ -d "$$m" ]; then \
			cd "$$m" && go fmt ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$m"; \
		fi \
	done
endif

.PHONY: clean
clean:
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Clean] Cleaning module: $(MODULE)"; \
		cd "$(MODULE)" && go clean -v -r ./... && cd - >/dev/null; \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
	fi
else
	@echo "[Clean] Cleaning all modules..."
	go clean -v -r ./...
	@for m in $(MODULES); do \
		if [ -d "$$m" ]; then \
			cd "$$m" && go clean -v -r ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$m"; \
		fi \
	done
endif

.PHONY: test
test:
	@echo "[Test] Cleaning test cache..."
	go clean -testcache
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Test] Running test in module: $(MODULE)"; \
		cd "$(MODULE)" && go test -race -cover -covermode=atomic ./... && cd - >/dev/null; \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
	fi
else
	@echo "[Test] Running tests for all modules..."
	go test -race -cover -covermode=atomic ./...
	@for m in $(MODULES); do \
		if [ -d "$$m" ]; then \
			cd "$$m" && go test -race -cover -covermode=atomic ./... && cd - >/dev/null; \
		else \
			echo "[Warn] Skipped missing module: $$m"; \
		fi \
	done
endif
