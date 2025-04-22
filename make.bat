@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: Set Go proxy
SET "GOPROXY=https://goproxy.io,direct"

:: List of all submodules
SET modules=cmd/socket.io parsers/engine parsers/socket servers/engine servers/socket adapters/adapter adapters/redis clients/engine clients/socket

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
IF /I "%args%"=="version" GOTO :version
IF /I "%args%"=="release" GOTO :release

GOTO :help

:help
    echo.
    echo Usage: make.bat [deps^|get^|build^|fmt^|vet^|clean^|test^|version^|release] [MODULE_PATH^|VERSION]
    echo If no module_path is given, the command applies to all modules.
    echo VERSION is required for version/release, e.g. make.bat version v3.0.0[-alpha^|beta^|rc[.x]]
    echo.
    GOTO :EOF

:: (keep existing targets: :deps :get :build :fmt :vet :clean :test)

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
            CALL go test -timeout=30s -race -cover -covermode=atomic ./...
            popd
        ) ELSE (
            echo [Error] Module path not found: %MODULE%
        )
    ) ELSE (
        CALL :run_for_all_modules "go test -timeout=30s -race -cover -covermode=atomic ./..." "Test"
    )
    GOTO :EOF

:version
    SET "VERSION=%~2"
    IF "%VERSION%"=="" (
        echo [Error] VERSION is required. Usage: make.bat version v3.0.0[-alpha^|beta^|rc[.x]
        GOTO :EOF
    )

    echo [Version] Updating version to %VERSION%
    powershell -Command "(Get-Content pkg/version/version.go) -replace 'VERSION = \"(.*?)\"', 'VERSION = \"%VERSION%\"' | Set-Content pkg/version/version.go"
    powershell -Command "(Get-Content go.mod) -replace '(github\.com/zishang520/socket\.io/cmd/socket\.io/v3 )(.*?)( // indirect)', '${1}%VERSION%${3}' | Set-Content go.mod"

    FOR %%M IN (%modules%) DO (
        IF EXIST "%%M" (
            echo [Version] Updating dependencies in %%M...
            pushd "%%M"
            CALL go mod tidy
            FOR /F "delims=" %%D IN ('go list -f "{{if and (not .Indirect) (not .Main)}}{{.Path}}@%VERSION%{{end}}" -m all ^| findstr "^github.com/zishang520/socket.io"') DO (
                CALL go get -v %%D
            )
            CALL go mod tidy
            popd
        ) ELSE (
            echo [Warn] Skipped missing module: %%M
        )
    )
    echo [Version] Done.
    GOTO :EOF

:release
    SET "FORCE=0"
    IF "%~21"=="--force" SET "FORCE=1" & SHIFT
    IF "%~2"=="-f" SET "FORCE=1" & SHIFT

    IF NOT EXIST "pkg\version\version.go" (
        echo [Error] File pkg/version/version.go not found
        GOTO :EOF
    )

    SET "VERSION="
    FOR /F "tokens=2 delims==" %%i IN ('findstr /C:"const VERSION" pkg\version\version.go') DO (
        SET "VERSION=%%i"
    )

    IF "!VERSION!"=="" (
        echo [Error] Failed to read VERSION from pkg/version/version.go
        GOTO :EOF
    )

    SET "VERSION=%VERSION:"=%"
    SET "VERSION=%VERSION: =%"

    IF "!VERSION!"=="" (
        echo [Error] VERSION is empty after cleanup
        GOTO :EOF
    )

    echo [Debug] VERSION extracted: !VERSION!

    IF !FORCE! EQU 1 (
        echo [Release] Creating FORCED tags...
        CALL git tag -f "!VERSION!"
        FOR %%M IN (%modules%) DO (
            IF EXIST "%%M" (
                echo [Release] Forcing tag in: %%M
                CALL git tag -f "%%M/!VERSION!"
            )
        )
    ) ELSE (
        echo [Release] Creating tags...
        CALL git tag "!VERSION!"
        FOR %%M IN (%modules%) DO (
            IF EXIST "%%M" (
                echo [Release] Tagging: %%M
                CALL git tag "%%M/!VERSION!"
            )
        )
    )

    echo [Release] Verifying tags...
    CALL git show "!VERSION!" >nul 2>&1
    IF ERRORLEVEL 1 (
        echo [Error] Failed to verify main tag !VERSION!
        GOTO :EOF
    )

    FOR %%M IN (%modules%) DO (
        IF EXIST "%%M" (
            CALL git show "%%M/!VERSION!" >nul 2>&1
            IF ERRORLEVEL 1 (
                echo [Error] Failed to verify module tag %%M/!VERSION!
            )
        )
    )

    echo [Release] All tags verified successfully
    GOTO :EOF