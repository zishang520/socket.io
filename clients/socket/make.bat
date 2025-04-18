@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: Set Go proxy
set "GOPROXY=https://goproxy.io,direct"

:: Normalize and simplify the argument
set "args=%~1"
set "args=!args:"=!"
set "args=!args:~0,4!"

:: Dispatch to command
if /i "!args!"=="deps"  goto :deps
if /i "!args!"=="fmt"   goto :fmt
if /i "!args!"=="test"  goto :test
if /i "!args!"=="clea"  goto :clean

goto :default

:default
    echo Usage: %~n0 [deps ^| fmt ^| clean ^| test]
    GOTO :EOF

:deps
    echo [deps] Tidying Go module dependencies...
    go mod tidy
    GOTO :EOF

:fmt
    echo [fmt] Formatting Go code...
    go fmt ./...
    GOTO :EOF

:clean
    echo [clean] Cleaning build artifacts...
    go clean -v -r ./...
    GOTO :EOF

:test
    echo [test] Running Go tests with race and coverage checks...
    go clean -testcache
    go test -race -cover -covermode=atomic ./...
    GOTO :EOF
