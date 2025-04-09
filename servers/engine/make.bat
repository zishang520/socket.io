@echo OFF

set "args=%*"
pushd "%~dp0"
setlocal ENABLEDELAYEDEXPANSION
rem Set the GOPROXY environment variable
Set GOPROXY=https://goproxy.io,direct

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
    CALL go mod vendor
    GOTO :EOF

:fmt
    CALL go fmt -mod=mod ./...
    GOTO :EOF

:clean
    CALL go clean -mod=mod -v -r ./...
    GOTO :EOF

:test
    CALL go clean -testcache
    CALL go test -race -cover -covermode=atomic -mod=mod ./...
    GOTO :EOF
