.PHONY: default deps fmt clean test
export GOPROXY=https://goproxy.io,direct

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
