@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: ============================================================================
::  ANSI COLOR GENERATION (The "Dark Magic" Logic)
::  We generate the ESC character (ASCII 27) dynamically because
::  it cannot be reliably copy-pasted.
:: ============================================================================
FOR /F "tokens=1,2 delims=#" %%a IN ('"prompt #$H#$E# & echo on & for %%b in (1) do rem"') DO (
  set "ESC=%%b"
)

:: Define Colors using the dynamic ESC variable
SET "COL_RESET=!ESC![0m"
SET "COL_INFO=!ESC![36m"
SET "COL_OK=!ESC![32m"
SET "COL_WARN=!ESC![33m"
SET "COL_ERR=!ESC![31m"

:: ============================================================================
::  CONFIGURATION
:: ============================================================================
SET "GOPROXY=https://goproxy.io,direct"
SET "MODULES=parsers/engine parsers/socket servers/engine servers/socket adapters/adapter adapters/redis clients/engine clients/socket"
SET "TEST_TIMEOUT=60s"
SET "VERSION_FILE=pkg\version\version.go"
SET "CORE_DEPENDENCY=github.com/zishang520/socket.io"

:: ============================================================================
::  ROUTER
:: ============================================================================
IF "%~1"=="" GOTO :help

REM Map commands to internal logic
IF /I "%~1"=="help"    GOTO :help
IF /I "%~1"=="env"     GOTO :env
IF /I "%~1"=="version" GOTO :version
IF /I "%~1"=="release" GOTO :release

REM Standard Go commands using the Executor pattern
REM Usage: call :executor [CommandString] [ModuleFilter] [Label]
IF /I "%~1"=="deps"    CALL :executor "go mod tidy && go mod vendor" "%~2" "Deps"    & GOTO :end_check
IF /I "%~1"=="get"     CALL :executor "go get ./..."                 "%~2" "Get"     & GOTO :end_check
IF /I "%~1"=="build"   CALL :executor "go build ./..."               "%~2" "Build"   & GOTO :end_check
IF /I "%~1"=="fmt"     CALL :executor "go fmt ./..."                 "%~2" "Fmt"     & GOTO :end_check
IF /I "%~1"=="clean"   CALL :executor "go clean -v -r ./..."         "%~2" "Clean"   & GOTO :end_check

REM Composite commands
IF /I "%~1"=="update" (
    CALL :executor "go get -u -v ./..." "%~2" "Update"
    IF !ERRORLEVEL! EQU 0 CALL :executor "go mod tidy && go mod vendor" "%~2" "Deps"
    GOTO :end_check
)

IF /I "%~1"=="vet" (
    CALL :executor "go mod tidy && go mod vendor" "%~2" "Deps"
    IF !ERRORLEVEL! EQU 0 CALL :executor "go vet ./..." "%~2" "Vet"
    GOTO :end_check
)

IF /I "%~1"=="test" (
    CALL :executor "go mod tidy && go mod vendor" "%~2" "Deps"
    echo %COL_INFO%[Test] Cleaning test cache...%COL_RESET%
    go clean -testcache
    CALL :executor "go test -timeout=%TEST_TIMEOUT% -race -cover -covermode=atomic ./..." "%~2" "Test"
    GOTO :end_check
)

REM Fallback
echo %COL_ERR%[Error] Unknown command: %~1%COL_RESET%
GOTO :help

:end_check
IF %ERRORLEVEL% NEQ 0 EXIT /B %ERRORLEVEL%
EXIT /B 0

REM ============================================================================
REM  THE EXECUTOR (The Engine)
REM  Abstracts the loop vs single module logic.
REM  %1: Command to run
REM  %2: Module filter (optional)
REM  %3: Display Label
REM ============================================================================
:executor
    SET "CMD=%~1"
    SET "TARGET=%~2"
    SET "LABEL=%~3"

    REM Case 1: Target specified (Single Module)
    IF NOT "%TARGET%"=="" (
        IF NOT EXIST "%TARGET%" (
            echo %COL_ERR%[Error] Module not found: %TARGET%%COL_RESET%
            EXIT /B 1
        )
        CALL :run_in_dir "%TARGET%" "%CMD%" "%LABEL%"
        EXIT /B !ERRORLEVEL!
    )

    REM Case 2: No target (Root + All Modules)
    REM Run on Root first
    echo %COL_INFO%[%LABEL%] Processing root directory...%COL_RESET%
    CALL %CMD% || (echo %COL_ERR%[Error] Failed in root%COL_RESET% & EXIT /B 1)

    REM Run on Submodules
    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            CALL :run_in_dir "%%M" "%CMD%" "%LABEL%"
            IF !ERRORLEVEL! NEQ 0 EXIT /B 1
        ) ELSE (
            echo %COL_WARN%[Warn] Skipping missing module: %%M%COL_RESET%
        )
    )
    EXIT /B 0

:run_in_dir
    SET "DIR=%~1"
    SET "EXEC=%~2"
    SET "TAG=%~3"
    echo %COL_INFO%[%TAG%] Processing: %DIR%%COL_RESET%
    pushd "%DIR%"
    CALL %EXEC%
    SET "EXIT_CODE=!ERRORLEVEL!"
    popd
    IF !EXIT_CODE! NEQ 0 (
        echo %COL_ERR%[Error] Failed in %DIR%%COL_RESET%
        EXIT /B !EXIT_CODE!
    )
    EXIT /B 0

REM ============================================================================
REM  SPECIAL COMMANDS
REM ============================================================================
:env
    go env
    EXIT /B 0

:version
    SET "NEW_VER=%~2"
    IF "%NEW_VER%"=="" (
        echo %COL_ERR%[Error] Usage: make.bat version vX.Y.Z%COL_RESET%
        EXIT /B 1
    )

    REM Validate Regex (Requires PowerShell)
    powershell -Command "$v='%NEW_VER%'; if ($v -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z\-\.]+)?$') { exit 1 }"
    IF ERRORLEVEL 1 (
        echo %COL_ERR%[Error] Invalid version format: %NEW_VER%%COL_RESET%
        EXIT /B 1
    )

    echo %COL_INFO%[Version] Updating %VERSION_FILE% to %NEW_VER%%COL_RESET%
    IF NOT EXIST "%VERSION_FILE%" (
        echo %COL_ERR%[Error] File not found: %VERSION_FILE%%COL_RESET%
        EXIT /B 1
    )

    REM Update Version File
    powershell -Command "(Get-Content '%VERSION_FILE%') -replace 'VERSION = \"(.*?)\"', 'VERSION = \"%NEW_VER%\"' | Set-Content '%VERSION_FILE%'" || EXIT /B 1

    REM Update Dependencies in Modules
    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            echo %COL_INFO%[Version] Syncing dependencies in %%M%COL_RESET%
            pushd "%%M"
            CALL go mod tidy
            REM Find dependency dynamically and update
            FOR /F "delims=" %%D IN ('go list -mod=mod -f "{{if and (not .Main)}}{{.Path}}@%NEW_VER%{{end}}" -m all ^| findstr "^%CORE_DEPENDENCY%"') DO (
                CALL go get -v %%D || (popd & EXIT /B 1)
            )
            CALL go mod tidy
            popd
        )
    )

    REM Refresh deps
    CALL :executor "go mod tidy && go mod vendor" "" "Deps"
    echo %COL_OK%[Version] Completed successfully.%COL_RESET%
    EXIT /B 0

:release
    SET "FORCE_FLAG="
    IF /I "%~2"=="--force" SET "FORCE_FLAG=-f"
    IF /I "%~2"=="-f"      SET "FORCE_FLAG=-f"

    REM Extract Version
    FOR /F "tokens=2 delims==" %%i IN ('findstr /C:"const VERSION" %VERSION_FILE%') DO SET "CURRENT_VER=%%i"
    SET "CURRENT_VER=%CURRENT_VER:"=%"
    SET "CURRENT_VER=%CURRENT_VER: =%"

    IF "%CURRENT_VER%"=="" (
        echo %COL_ERR%[Error] Could not read version from file.%COL_RESET%
        EXIT /B 1
    )

    echo %COL_INFO%[Release] Tagging version: %CURRENT_VER% (Force: %FORCE_FLAG%)%COL_RESET%

    REM Tag Root
    git tag %FORCE_FLAG% "%CURRENT_VER%" || EXIT /B 1

    REM Tag Modules
    FOR %%M IN (%MODULES%) DO (
        IF EXIST "%%M" (
            echo [Release] Tagging %%M/%CURRENT_VER%
            git tag %FORCE_FLAG% "%%M/%CURRENT_VER%" || EXIT /B 1
        )
    )

    echo %COL_OK%[Release] All tags created.%COL_RESET%
    EXIT /B 0

:help
    echo.
    echo %COL_WARN%Usage: make.bat [command] [module_path] [options]%COL_RESET%
    echo Commands:
    echo   deps, get, build, fmt, clean, test  (Standard Go commands)
    echo   update, vet                         (Composite commands)
    echo   version [vX.Y.Z]                    (Bump version and sync deps)
    echo   release [--force]                   (Git tag based on version file)
    echo.
    EXIT /B 0