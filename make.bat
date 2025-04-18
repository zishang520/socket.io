@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: Set Go proxy
SET "GOPROXY=https://goproxy.io,direct"

:: List of all submodules
SET modules=adapters\adapter adapters\redis clients\engine clients\socket parsers\engine parsers\socket servers\engine servers\socket

:: Get command argument
SET "args=%~1"
IF /I "%args%"=="" GOTO :help
IF /I "%args%"=="deps" GOTO :deps
IF /I "%args%"=="get" GOTO :get
IF /I "%args%"=="build" GOTO :build
IF /I "%args%"=="fmt" GOTO :fmt
IF /I "%args%"=="vet" GOTO :vet
IF /I "%args%"=="clean" GOTO :clean
IF /I "%args%"=="test" GOTO :test

GOTO :help

:help
    echo.
    echo Usage: build.bat [deps^|get^|build^|fmt^|vet^|clean^|test] [module_path]
    echo If no module_path is given, the command applies to all modules.
    echo.
    GOTO :EOF

:run_for_all_modules
    SET "cmd=%~1"
    SET "label=%~2"
    echo [%label%] Running in: [.]
    CALL %cmd%
    FOR %%M IN (%modules%) DO (
        IF EXIST "%%M" (
            pushd "%%M"
            echo [%label%] Running in: %%M
            CALL %cmd%
            popd
        ) ELSE (
            echo [Warn] Skipped missing module: %%M
        )
    )
    GOTO :EOF

:deps
    SET "MODULE=%~2"
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
        CALL :run_for_all_modules "go mod tidy" "Deps"
    )
    GOTO :EOF

:fmt
    SET "MODULE=%~2"
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
        CALL :run_for_all_modules "go fmt ./..." "Fmt"
    )
    GOTO :EOF

:vet
    SET "MODULE=%~2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Vet] Checking module: %MODULE%
            pushd "%MODULE%"
            CALL go vet ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        CALL :run_for_all_modules "go vet ./..." "Vet"
    )
    GOTO :EOF

:get
    SET "MODULE=%~2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Get] Downloading module deps: %MODULE%
            pushd "%MODULE%"
            CALL go get ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        CALL :run_for_all_modules "go get ./..." "Get"
    )
    GOTO :EOF

:build
    SET "MODULE=%~2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Build] Building module: %MODULE%
            pushd "%MODULE%"
            CALL go build ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        CALL :run_for_all_modules "go build ./..." "Build"
    )
    GOTO :EOF

:clean
    SET "MODULE=%~2"
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
        CALL :run_for_all_modules "go clean -v -r ./..." "Clean"
    )
    GOTO :EOF

:test
    echo [Test] Cleaning test cache...
    CALL go clean -testcache

    SET "MODULE=%~2"
    IF NOT "%MODULE%"=="" (
        IF EXIST "%MODULE%" (
            echo [Test] Testing module: %MODULE%
            pushd "%MODULE%"
            CALL go test -race -cover -covermode=atomic ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        CALL :run_for_all_modules "go test -race -cover -covermode=atomic ./..." "Test"
    )
    GOTO :EOF
