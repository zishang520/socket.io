@echo OFF

set "args=%*"
pushd "%~dp0"
setlocal ENABLEDELAYEDEXPANSION
rem set GOPATH="%~dp0vendor"
rem Set the GOPROXY environment variable
Set GOPROXY=https://goproxy.io,direct
rem set http_proxy=socks5://127.0.0.1:1080
rem set https_proxy=%http_proxy%

if /i "%args%"=="default" goto %args%
if /i "%args%"=="deps" goto %args%
if /i "%args%"=="fmt" goto %args%
if /i "%args%"=="clean" goto %args%
if /i "%args%"=="test" goto %args%

goto default

:default
    GOTO :EOF

:deps
    CALL go mod tidy
    GOTO :EOF

:fmt
    CALL go fmt ./...
    CALL cd ./adapters/adapter && go fmt ./... && cd ../../
    CALL cd ./adapters/redis && go fmt ./... && cd ../../
    CALL cd ./clients/engine && go fmt ./... && cd ../../
    CALL cd ./clients/socket && go fmt ./... && cd ../../
    CALL cd ./parsers/engine && go fmt ./... && cd ../../
    CALL cd ./parsers/socket && go fmt ./... && cd ../../
    CALL cd ./servers/engine && go fmt ./... && cd ../../
    CALL cd ./servers/socket && go fmt ./... && cd ../../
    GOTO :EOF

:imports
    CALL goimports -w .
    CALL goimports -w ./adapters/adapter
    CALL goimports -w ./adapters/redis
    CALL goimports -w ./clients/engine
    CALL goimports -w ./clients/socket
    CALL goimports -w ./parsers/engine
    CALL goimports -w ./parsers/socket
    CALL goimports -w ./servers/engine
    CALL goimports -w ./servers/socket
    GOTO :EOF

:clean
    CALL go clean -v -r ./...
    CALL cd ./adapters/adapter && go clean  -v -r ./... && cd ../../
    CALL cd ./adapters/redis && go clean  -v -r ./... && cd ../../
    CALL cd ./clients/engine && go clean  -v -r ./... && cd ../../
    CALL cd ./clients/socket && go clean  -v -r ./... && cd ../../
    CALL cd ./parsers/engine && go clean  -v -r ./... && cd ../../
    CALL cd ./parsers/socket && go clean  -v -r ./... && cd ../../
    CALL cd ./servers/engine && go clean  -v -r ./... && cd ../../
    CALL cd ./servers/socket && go clean  -v -r ./... && cd ../../
    GOTO :EOF

:test
    CALL go clean -testcache
    CALL go test -race -cover -covermode=atomic ./...
    CALL cd ./adapters/adapter && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./adapters/redis && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./clients/engine && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./clients/socket && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./parsers/engine && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./parsers/socket && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./servers/engine && go test -race -cover -covermode=atomic ./... && cd ../../
    CALL cd ./servers/socket && go test -race -cover -covermode=atomic ./... && cd ../../
    GOTO :EOF
