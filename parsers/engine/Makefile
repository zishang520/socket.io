# Reuse the root commands, restricted to this module.
.DEFAULT_GOAL := help
.PHONY: all help env deps get update build fmt vet lint clean test
all: help

help:
	@echo 'Usage: make [env | deps | get | update | build | fmt | vet | lint | clean | test]'
	@echo 'Runs only parsers/engine; options: FIX=1, GOPROXY, TEST_TIMEOUT.'

env:
	@go env

deps get update build fmt vet lint clean test:
	@$(MAKE) --no-print-directory -C ../.. $@ MODULE=parsers/engine
