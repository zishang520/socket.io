.PHONY: default deps fmt clean test
# export GOPATH:=$(shell pwd)/vendor
# Set the GOPROXY environment variable
export GOPROXY=https://goproxy.io,direct
# export http_proxy=socks5://127.0.0.1:1080
# export https_proxy=%http_proxy%

default:

deps:
	go mod tidy
	go mod vendor

fmt:
	go fmt -mod=mod ./...

clean:
	go clean -mod=mod -v -r ./...

test:
	go clean -testcache
	go test -race -cover -covermode=atomic -mod=mod -v ./...
