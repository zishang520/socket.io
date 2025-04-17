@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: Set Go proxy
SET "GOPROXY=https://goproxy.io,direct"

:: List of all submodules
SET modules=adapters\adapter adapters\redis clients\engine clients\socket parsers\engine parsers\socket servers\engine servers\socket

:: Get command argument
SET "args=%1"
IF /I "%args%"=="" GOTO :help
IF /I "%args%"=="default" GOTO :default
IF /I "%args%"=="deps" GOTO :deps
IF /I "%args%"=="fmt" GOTO :fmt
IF /I "%args%"=="clean" GOTO :clean
IF /I "%args%"=="test" GOTO :test

GOTO :help

:default
    echo [Info] No command provided. Doing nothing.
    GOTO :EOF

:help
    echo.
    echo Usage: build.bat [default^|deps^|fmt^|clean^|test] [module_path]
    echo If no module_path is given, the command applies to all.
    echo.
    GOTO :EOF

:deps
    SET "MODULE=%2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Deps] Tidy module: %MODULE%
            pushd "%MODULE%"
            CALL go mod tidy
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        echo [Deps] Running go mod tidy for all modules...
        CALL go mod tidy
        FOR %%M IN (%modules%) DO (
            IF EXIST "%%M" (
                pushd "%%M"
                CALL go mod tidy
                popd
            ) ELSE (
                echo [Warn] Skipped missing module: %%M
            )
        )
    )
    GOTO :EOF

:fmt
    SET "MODULE=%2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Fmt] Formatting module: %MODULE%
            pushd "%MODULE%"
            CALL go fmt ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        echo [Fmt] Formatting all modules...
        CALL go fmt ./...
        FOR %%M IN (%modules%) DO (
            IF EXIST "%%M" (
                pushd "%%M"
                CALL go fmt ./...
                popd
            ) ELSE (
                echo [Warn] Skipped missing module: %%M
            )
        )
    )
    GOTO :EOF

:clean
    SET "MODULE=%2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Clean] Cleaning module: %MODULE%
            pushd "%MODULE%"
            CALL go clean -v -r ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        echo [Clean] Cleaning all modules...
        CALL go clean -v -r ./...
        FOR %%M IN (%modules%) DO (
            IF EXIST "%%M" (
                pushd "%%M"
                CALL go clean -v -r ./...
                popd
            ) ELSE (
                echo [Warn] Skipped missing module: %%M
            )
        )
    )
    GOTO :EOF

:test
    echo [Test] Cleaning test cache...
    CALL go clean -testcache

    SET "MODULE=%2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Test] Running test in module: %MODULE%
            pushd "%MODULE%"
            CALL go test -race -cover -covermode=atomic ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        echo [Test] Running tests for all modules...
        CALL go test -race -cover -covermode=atomic ./...
        FOR %%M IN (%modules%) DO (
            IF EXIST "%%M" (
                pushd "%%M"
                CALL go test -race -cover -covermode=atomic ./...
                popd
            ) ELSE (
                echo [Warn] Skipped missing module: %%M
            )
        )
    )
    GOTO :EOF
