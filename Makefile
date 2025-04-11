# Makefile
SHELL := bash
GOPROXY := https://goproxy.io,direct
MODULES := adapters/adapter adapters/redis clients/engine clients/socket parsers/engine parsers/socket servers/engine servers/socket

.DEFAULT_GOAL := help

export GOPROXY

.PHONY: help default deps fmt clean test

default:
	@echo "[Info] No command provided. Doing nothing."

help:
	@echo
	@echo "Usage: make [default|deps|fmt|clean|test] [MODULE=module_path]"
	@echo "If no MODULE is given, the command applies to all."
	@echo

deps:
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Deps] Tidy module: $(MODULE)"; \
		(cd "$(MODULE)" && go mod tidy); \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
		exit 1; \
	fi
else
	@echo "[Deps] Running go mod tidy for all modules..."
	go mod tidy
	@for MOD in $(MODULES); do \
		if [ -d "$$MOD" ]; then \
			(cd "$$MOD" && go mod tidy); \
		else \
			echo "[Warn] Skipped missing module: $$MOD"; \
		fi; \
	done
endif

fmt:
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Fmt] Formatting module: $(MODULE)"; \
		(cd "$(MODULE)" && go fmt ./...); \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
		exit 1; \
	fi
else
	@echo "[Fmt] Formatting all modules..."
	go fmt ./...
	@for MOD in $(MODULES); do \
		if [ -d "$$MOD" ]; then \
			(cd "$$MOD" && go fmt ./...); \
		else \
			echo "[Warn] Skipped missing module: $$MOD"; \
		fi; \
	done
endif

clean:
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Clean] Cleaning module: $(MODULE)"; \
		(cd "$(MODULE)" && go clean -v -r ./...); \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
		exit 1; \
	fi
else
	@echo "[Clean] Cleaning all modules..."
	go clean -v -r ./...
	@for MOD in $(MODULES); do \
		if [ -d "$$MOD" ]; then \
			(cd "$$MOD" && go clean -v -r ./...); \
		else \
			echo "[Warn] Skipped missing module: $$MOD"; \
		fi; \
	done
endif

test:
	@echo "[Test] Cleaning test cache..."
	go clean -testcache
ifdef MODULE
	@if [ -d "$(MODULE)" ]; then \
		echo "[Test] Running test in module: $(MODULE)"; \
		(cd "$(MODULE}" && go test -race -cover -covermode=atomic ./...); \
	else \
		echo "[Error] Module path not found: $(MODULE)"; \
		exit 1; \
	fi
else
	@echo "[Test] Running tests for all modules..."
	go test -race -cover -covermode=atomic ./...
	@for MOD in $(MODULES); do \
		if [ -d "$$MOD" ]; then \
			(cd "$$MOD" && go test -race -cover -covermode=atomic ./...); \
		else \
			echo "[Warn] Skipped missing module: $$MOD"; \
		fi; \
	done
endif