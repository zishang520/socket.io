.PHONY: default deps fmt goimports clean test
# export GOPATH:=$(shell pwd)/vendor
# Set the GOPROXY environment variable
export GOPROXY=https://goproxy.io,direct
# export http_proxy=socks5://127.0.0.1:1080
# export https_proxy=%http_proxy%

default:

deps:
	go mod tidy

fmt:
	go fmt -mod=mod ./...
	gofmt -w ./adapters/adapter
	gofmt -w ./adapters/redis
	gofmt -w ./clients/engine
	gofmt -w ./clients/socket
	gofmt -w ./parsers/engine
	gofmt -w ./parsers/socket
	gofmt -w ./servers/engine
	gofmt -w ./servers/socket

goimports:
	goimports -w .
	goimports -w ./adapters/adapter
	goimports -w ./adapters/redis
	goimports -w ./clients/engine
	goimports -w ./clients/socket
	goimports -w ./parsers/engine
	goimports -w ./parsers/socket
	goimports -w ./servers/engine
	goimports -w ./servers/socket

clean:
	go clean -mod=mod -v -r ./...
	cd ./adapters/adapter && go clean  -v -r ./... && cd ../../
	cd ./adapters/redis && go clean  -v -r ./... && cd ../../
	cd ./clients/engine && go clean  -v -r ./... && cd ../../
	cd ./clients/socket && go clean  -v -r ./... && cd ../../
	cd ./parsers/engine && go clean  -v -r ./... && cd ../../
	cd ./parsers/socket && go clean  -v -r ./... && cd ../../
	cd ./servers/engine && go clean  -v -r ./... && cd ../../
	cd ./servers/socket && go clean  -v -r ./... && cd ../../

test:
	go clean -testcache
	go test -race -cover -covermode=atomic ./...
	cd ./adapters/adapter && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./adapters/redis && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./clients/engine && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./clients/socket && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./parsers/engine && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./parsers/socket && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./servers/engine && go test -race -cover -covermode=atomic ./... && cd ../../
	cd ./servers/socket && go test -race -cover -covermode=atomic ./... && cd ../../
