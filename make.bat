@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: Configuration
SET "GOPROXY=https://goproxy.io,direct"
SET "MODULES=cmd/socket.io parsers/engine parsers/socket servers/engine servers/socket adapters/adapter adapters/redis clients/engine clients/socket"
SET "TEST_TIMEOUT=60s"
SET "VERSION_FILE=pkg\version\version.go"

:: Command parsing
SET "COMMAND=%~1"
SET "MODULE=%~2"
SET "FORCE=%~2"

:: Command dispatch
CALL :handle_command %COMMAND%
IF ERRORLEVEL 1 GOTO :EOF
GOTO :EOF

:handle_command
    IF /I "%~1"=="" CALL :help & EXIT /B 1
    IF /I "%~1"=="env" CALL :env & EXIT /B 0
    IF /I "%~1"=="deps" CALL :deps %MODULE% & EXIT /B 0
    IF /I "%~1"=="get" CALL :get %MODULE% & EXIT /B 0
    IF /I "%~1"=="update" CALL :update %MODULE% & EXIT /B 0
    IF /I "%~1"=="build" CALL :build %MODULE% & EXIT /B 0
    IF /I "%~1"=="fmt" CALL :fmt %MODULE% & EXIT /B 0
    IF /I "%~1"=="vet" CALL :vet %MODULE% & EXIT /B 0
    IF /I "%~1"=="clean" CALL :clean %MODULE% & EXIT /B 0
    IF /I "%~1"=="test" CALL :test %MODULE% & EXIT /B 0
    IF /I "%~1"=="version" CALL :version %MODULE% & EXIT /B 0
    IF /I "%~1"=="release" CALL :release %FORCE% & EXIT /B 0
    echo [Error] Unknown command: %~1
    CALL :help
    EXIT /B 1

:help
    echo.
    echo Usage: make.bat [command] [module_path] [options]
    echo Commands: deps, get, update, build, fmt, vet, clean, test, version, release
    echo If no module_path is given, command applies to all modules
    echo version requires VERSION (e.g., v3.0.0[-alpha^|beta^|rc[.x]])
    echo release supports --force or -f option
    echo.
    EXIT /B 0

:process_modules
    SET "cmd=%~1"
    SET "label=%~2"
    echo [%label%] Processing root directory
    CALL %cmd% || (echo [Error] Failed in root directory & EXIT /B 1)

    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            echo [%label%] Processing: %%M
            pushd "%%M"
            CALL %cmd% || (echo [Error] Failed in %%M & popd & EXIT /B 1)
            popd
        ) ELSE (
            echo [Warn] Skipping missing module: %%M
        )
    )
    EXIT /B 0

:process_single_module
    SET "cmd=%~1"
    SET "label=%~2"
    SET "module=%~3"
    IF NOT EXIST "%module%" (
        echo [Error] Module not found: %module%
        EXIT /B 1
    )
    echo [%label%] Processing: %module%
    pushd "%module%"
    CALL %cmd% || (echo [Error] Failed in %module% & popd & EXIT /B 1)
    popd
    EXIT /B 0

:env
    CALL go env || (echo [Error] Failed in root directory & EXIT /B 1)
    EXIT /B 0

:deps
    IF NOT "%~1"=="" (
        CALL :process_single_module "go mod tidy && go mod vendor" "Deps" "%~1"
    ) ELSE (
        CALL :process_modules "go mod tidy && go mod vendor" "Deps"
    )
    EXIT /B 0

:get
    IF NOT "%~1"=="" (
        CALL :process_single_module "go get ./..." "Get" "%~1"
    ) ELSE (
        CALL :process_modules "go get ./..." "Get"
    )
    EXIT /B 0

:update
    IF NOT "%~1"=="" (
        CALL :process_single_module "go get -u -v ./..." "Update" "%~1"
    ) ELSE (
        CALL :process_modules "go get -u -v ./..." "Update"
    )

    CALL :deps
    EXIT /B 0

:build
    IF NOT "%~1"=="" (
        CALL :process_single_module "go build ./..." "Build" "%~1"
    ) ELSE (
        CALL :process_modules "go build ./..." "Build"
    )
    EXIT /B 0

:fmt
    IF NOT "%~1"=="" (
        CALL :process_single_module "go fmt ./..." "Fmt" "%~1"
    ) ELSE (
        CALL :process_modules "go fmt ./..." "Fmt"
    )
    EXIT /B 0

:vet
    CALL :deps

    IF NOT "%~1"=="" (
        CALL :process_single_module "go vet ./..." "Vet" "%~1"
    ) ELSE (
        CALL :process_modules "go vet ./..." "Vet"
    )
    EXIT /B 0

:clean
    IF NOT "%~1"=="" (
        CALL :process_single_module "go clean -v -r ./..." "Clean" "%~1"
    ) ELSE (
        CALL :process_modules "go clean -v -r ./..." "Clean"
    )
    EXIT /B 0

:test
    CALL :deps

    echo [Test] Cleaning test cache...
    CALL go clean -testcache || (echo [Error] Failed to clean test cache & EXIT /B 1)

    IF NOT "%~1"=="" (
        CALL :process_single_module "go test -timeout=%TEST_TIMEOUT% -race -cover -covermode=atomic ./..." "Test" "%~1"
    ) ELSE (
        CALL :process_modules "go test -timeout=%TEST_TIMEOUT% -race -cover -covermode=atomic ./..." "Test"
    )
    EXIT /B 0

:version
    SET "VERSION=%~1"
    IF "%VERSION%"=="" (
        echo [Error] VERSION required. Usage: make.bat version v3.0.0[-alpha^|beta^|rc[.x]]
        EXIT /B 1
    )

    powershell -Command "$v='%VERSION%'; if ($v -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z\-\.]+)?$') { Write-Error '[Error] Invalid version format: %VERSION%'; exit 1 }"
    IF ERRORLEVEL 1 EXIT /B 1

    echo [Version] Updating to %VERSION%
    IF NOT EXIST "%VERSION_FILE%" (
        echo [Error] Version file not found: %VERSION_FILE%
        EXIT /B 1
    )

    powershell -Command "(Get-Content '%VERSION_FILE%') -replace 'VERSION = \"(.*?)\"', 'VERSION = \"%VERSION%\"' | Set-Content '%VERSION_FILE%'" || (
        echo [Error] Failed to update version file
        EXIT /B 1
    )

    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            echo [Version] Updating dependencies in %%M
            pushd "%%M"
            CALL go mod tidy || (echo [Error] Failed to tidy module %%M & popd & EXIT /B 1)
            FOR /F "delims=" %%D IN ('go list -mod=mod -f "{{if and (not .Main)}}{{.Path}}@%VERSION%{{end}}" -m all ^| findstr "^github.com/zishang520/socket.io"') DO (
                CALL go get -v %%D || (echo [Error] Failed to get dependency %%D & popd & EXIT /B 1)
            )
            CALL go mod tidy || (echo [Error] Failed to tidy module %%M & popd & EXIT /B 1)
            popd
        ) ELSE (
            echo [Warn] Skipping missing module: %%M
        )
    )
    echo [Version] Completed successfully

    CALL :deps
    EXIT /B 0

:release
    SET "FORCE_FLAG=0"
    IF /I "%~1"=="--force" SET "FORCE_FLAG=1"
    IF /I "%~1"=="-f" SET "FORCE_FLAG=1"

    IF NOT EXIST "%VERSION_FILE%" (
        echo [Error] Version file not found: %VERSION_FILE%
        EXIT /B 1
    )

    FOR /F "tokens=2 delims==" %%i IN ('findstr /C:"const VERSION" %VERSION_FILE%') DO SET "VERSION=%%i"
    SET "VERSION=%VERSION:"=%"
    SET "VERSION=%VERSION: =%"

    IF "%VERSION%"=="" (
        echo [Error] Failed to extract version from %VERSION_FILE%
        EXIT /B 1
    )

    echo [Release] Processing version: %VERSION%

    SET "TAG_CMD=git tag"
    IF %FORCE_FLAG%==1 SET "TAG_CMD=git tag -f"

    echo [Release] Creating tags...
    CALL %TAG_CMD% "%VERSION%" || (echo [Error] Failed to create main tag %VERSION% & EXIT /B 1)

    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            echo [Release] Tagging: %%M
            CALL %TAG_CMD% "%%M/%VERSION%" || (echo [Error] Failed to create tag %%M/%VERSION% & EXIT /B 1)
        )
    )

    echo [Release] Verifying tags...
    CALL git show "%VERSION%" >nul 2>&1 || (echo [Error] Failed to verify main tag %VERSION% & EXIT /B 1)

    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            CALL git show "%%M/%VERSION%" >nul 2>&1 || (echo [Error] Failed to verify tag %%M/%VERSION% & EXIT /B 1)
        )
    )

    echo [Release] All tags created and verified successfully
    EXIT /B 0