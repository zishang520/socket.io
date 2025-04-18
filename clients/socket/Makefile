# Set Go proxy (optional override)
export GOPROXY ?= https://goproxy.io,direct

.PHONY: all deps fmt clean test

all:
	@echo "Usage: make [deps | fmt | clean | test]"

deps:
	@echo "[deps] Tidying Go module dependencies..."
	go mod tidy

fmt:
	@echo "[fmt] Formatting Go code..."
	go fmt ./...

clean:
	@echo "[clean] Cleaning build artifacts..."
	go clean -v -r ./...

test:
	@echo "[test] Running Go tests with race and coverage checks..."
	go clean -testcache
	go test -race -cover -covermode=atomic ./...
