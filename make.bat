@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

Set "GOPROXY=https://goproxy.io,direct"

set modules=adapters\adapter adapters\redis clients\engine clients\socket parsers\engine parsers\socket servers\engine servers\socket

set "args=%~1"
if /i "%args%"=="" goto help
if /i "%args%"=="default" goto :default
if /i "%args%"=="deps" goto :deps
if /i "%args%"=="fmt" goto :fmt
if /i "%args%"=="imports" goto :imports
if /i "%args%"=="clean" goto :clean
if /i "%args%"=="test" goto :test

goto :help

:default
    GOTO :EOF

:help
    echo.
    echo Usage: build.bat [default^|deps^|fmt^|imports^|clean^|test]
    echo.
    GOTO :EOF

:deps
    CALL go mod tidy
    for %%M in (%modules%) do (
        pushd %%M
        CALL go mod tidy
        popd
    )
    GOTO :EOF

:fmt
    CALL go fmt ./...
    for %%M in (%modules%) do (
        pushd %%M
        CALL go fmt ./...
        popd
    )
    GOTO :EOF

:imports
    CALL goimports -w .
    for %%M in (%modules%) do (
        CALL goimports -w %%M
    )
    GOTO :EOF

:clean
    CALL go clean -v -r ./...
    for %%M in (%modules%) do (
        pushd %%M
        CALL go clean -v -r ./...
        popd
    )
    GOTO :EOF

:test
    CALL go clean -testcache
    CALL go test -race -cover -covermode=atomic ./...
    for %%M in (%modules%) do (
        pushd %%M
        CALL go test -race -cover -covermode=atomic ./...
        popd
    )
    GOTO :EOF
