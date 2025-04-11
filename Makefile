.PHONY: all default deps fmt goimports clean test help

export GOPROXY=https://goproxy.io,direct

MODULES = \
    adapters/adapter \
    adapters/redis \
    clients/engine \
    clients/socket \
    parsers/engine \
    parsers/socket \
    servers/engine \
    servers/socket

default: help

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  deps        Run go mod tidy for all modules"
	@echo "  fmt         Run go fmt and gofmt for all modules"
	@echo "  goimports   Run goimports -w for all modules"
	@echo "  clean       Clean all build/test cache"
	@echo "  test        Run tests with race and coverage"
	@echo ""

deps:
	go mod tidy
	@for dir in $(MODULES); do \
		cd $$dir && go mod tidy && cd - >/dev/null; \
	done

fmt:
	go fmt -mod=mod ./...
	@for dir in $(MODULES); do \
		gofmt -w $$dir; \
	done

goimports:
	goimports -w .
	@for dir in $(MODULES); do \
		goimports -w $$dir; \
	done

clean:
	go clean -mod=mod -v -r ./...
	@for dir in $(MODULES); do \
		cd $$dir && go clean -v -r ./... && cd - >/dev/null; \
	done

test:
	go clean -testcache
	go test -race -cover -covermode=atomic ./...
	@for dir in $(MODULES); do \
		cd $$dir && go test -race -cover -covermode=atomic ./... && cd - >/dev/null; \
	done
