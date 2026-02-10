@echo OFF
setlocal ENABLEDELAYEDEXPANSION
pushd "%~dp0"

:: ============================================================================
::  1. ENVIRONMENT & COLORS
:: ============================================================================
:: Generate ESC character safely for ANSI colors
for /F "tokens=1,2 delims=#" %%a in ('"prompt #$H#$E# & echo on & for %%b in (1) do rem"') do set "ESC=%%b"

:: Color Definitions
set "C_RESET=%ESC%[0m"
set "C_CYAN=%ESC%[36m"
set "C_GREEN=%ESC%[32m"
set "C_YELLOW=%ESC%[33m"
set "C_RED=%ESC%[31m"

:: Configuration
set "GOPROXY=https://goproxy.io,direct"
set "MODULES=parsers/engine parsers/socket servers/engine servers/socket adapters/adapter adapters/redis clients/engine clients/socket"
set "TEST_TIMEOUT=60s"
set "VERSION_FILE=pkg\version\version.go"
set "CORE_DEPENDENCY=github.com/zishang520/socket.io"

:: Check for Go installation
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo %C_RED%[Fatal] Go is not installed or not in PATH.%C_RESET%
    exit /b 1
)

:: ============================================================================
::  2. ROUTER (Command Switch)
:: ============================================================================
if "%~1"=="" goto :help

:: Simple mappings
if /I "%~1"=="help"    goto :help
if /I "%~1"=="env"     goto :cmd_env
if /I "%~1"=="version" goto :cmd_version
if /I "%~1"=="release" goto :cmd_release

:: Execution Wrappers
if /I "%~1"=="deps"    call :RunBatch "go mod tidy && go mod vendor" "%~2" "Deps"   & goto :finalize
if /I "%~1"=="get"     call :RunBatch "go get ./..."                 "%~2" "Get"    & goto :finalize
if /I "%~1"=="build"   call :RunBatch "go build ./..."               "%~2" "Build"  & goto :finalize
if /I "%~1"=="fmt"     call :RunBatch "go fmt ./..."                 "%~2" "Fmt"    & goto :finalize
if /I "%~1"=="clean"   call :RunBatch "go clean -v -r ./..."         "%~2" "Clean"  & goto :finalize

:: Composite Commands
if /I "%~1"=="update" (
    call :RunBatch "go get -u -v ./..." "%~2" "Update"
    if !ERRORLEVEL! EQU 0 call :RunBatch "go mod tidy && go mod vendor" "%~2" "Deps"
    goto :finalize
)

if /I "%~1"=="vet" (
    call :RunBatch "go mod tidy && go mod vendor" "%~2" "Deps"
    if !ERRORLEVEL! EQU 0 call :RunBatch "go vet ./..." "%~2" "Vet"
    goto :finalize
)

if /I "%~1"=="test" (
    call :RunBatch "go mod tidy && go mod vendor" "%~2" "Deps"
    echo %C_CYAN%[Test] Cleaning test cache...%C_RESET%
    go clean -testcache
    call :RunBatch "go test -timeout=%TEST_TIMEOUT% -race -cover -covermode=atomic ./..." "%~2" "Test"
    goto :finalize
)

:: Default Fallback
echo %C_RED%[Error] Unknown command: %~1%C_RESET%
goto :help

:finalize
if %ERRORLEVEL% NEQ 0 exit /b %ERRORLEVEL%
exit /b 0

:: ============================================================================
::  3. CORE FUNCTIONS (The Engine)
:: ============================================================================

:: :RunBatch [Command] [TargetModule?] [Label]
:: Logic: If Target is set, run only there. Otherwise run Root + All Modules.
:RunBatch
    set "CMD=%~1"
    set "TARGET=%~2"
    set "LABEL=%~3"

    :: Mode A: Single Target
    if not "%TARGET%"=="" (
        if not exist "%TARGET%" (
            echo %C_RED%[Error] Module path not found: %TARGET%%C_RESET%
            exit /b 1
        )
        call :RunInDir "%TARGET%" "%CMD%" "%LABEL%"
        exit /b !ERRORLEVEL!
    )

    :: Mode B: Root + Submodules
    echo %C_CYAN%[%LABEL%] Processing: root%C_RESET%
    call %CMD%
    if !ERRORLEVEL! NEQ 0 (
        echo %C_RED%[%LABEL%] Failed at root.%C_RESET%
        exit /b 1
    )

    for %%M in (%MODULES%) do (
        if exist "%%M" (
            call :RunInDir "%%M" "%CMD%" "%LABEL%"
            if !ERRORLEVEL! NEQ 0 exit /b 1
        ) else (
            echo %C_YELLOW%[Warn] Skipping missing module: %%M%C_RESET%
        )
    )
    exit /b 0

:: :RunInDir [Directory] [Command] [Label]
:RunInDir
    set "DIR=%~1"
    set "EXEC=%~2"
    set "TAG=%~3"

    echo %C_CYAN%[%TAG%] Processing: %DIR%%C_RESET%
    pushd "%DIR%"
    call %EXEC%
    set "EXIT_CODE=!ERRORLEVEL!"
    popd

    if !EXIT_CODE! NEQ 0 (
        echo %C_RED%[Error] Failed in %DIR% ^(Exit Code: !EXIT_CODE!^)%C_RESET%
        exit /b !EXIT_CODE!
    )
    exit /b 0

:: ============================================================================
::  4. SPECIALIZED COMMANDS
:: ============================================================================

:cmd_env
    go env
    exit /b 0

:cmd_version
    set "NEW_VER=%~2"
    if "%NEW_VER%"=="" (
        echo %C_RED%[Error] Usage: %0 version vX.Y.Z%C_RESET%
        exit /b 1
    )

    :: Validate Version Format using PowerShell
    powershell -NoProfile -Command "if ('%NEW_VER%' -notmatch '^v\d+\.\d+\.\d+(-[\w\.]+)?$') { exit 1 }"
    if %ERRORLEVEL% NEQ 0 (
        echo %C_RED%[Error] Invalid version format: %NEW_VER% ^(Expected vX.Y.Z^)%C_RESET%
        exit /b 1
    )

    if not exist "%VERSION_FILE%" (
        echo %C_RED%[Error] Version file not found: %VERSION_FILE%%C_RESET%
        exit /b 1
    )

    echo %C_CYAN%[Version] updating %VERSION_FILE% to %NEW_VER%...%C_RESET%

    :: Update version in Go file
    powershell -NoProfile -Command "(Get-Content '%VERSION_FILE%') -replace 'VERSION = \".*?\"', 'VERSION = \"%NEW_VER%\"' | Set-Content '%VERSION_FILE%'"
    if %ERRORLEVEL% NEQ 0 exit /b 1

    :: Update Dependencies in Modules
    for %%M in (%MODULES%) do (
        if exist "%%M" (
            echo %C_CYAN%[Version] Syncing deps in %%M...%C_RESET%
            pushd "%%M"

            :: 1. Tidy first to ensure clean state
            call go mod tidy

            :: 2. Find and update the core dependency specifically
            for /F "delims=" %%D in ('go list -mod=mod -f "{{if and (not .Main)}}{{.Path}}@%NEW_VER%{{end}}" -m all ^| findstr "^%CORE_DEPENDENCY%"') do (
                echo    Updating %%D
                call go get -v %%D
                if !ERRORLEVEL! NEQ 0 (
                    popd
                    echo %C_RED%[Error] Failed to update %%D in %%M%C_RESET%
                    exit /b 1
                )
            )

            :: 3. Final Tidy
            call go mod tidy
            popd
        )
    )

    :: Refresh root deps
    call :RunBatch "go mod tidy && go mod vendor" "" "Deps"
    echo %C_GREEN%[Version] Successfully updated to %NEW_VER%%C_RESET%
    exit /b 0

:cmd_release
    set "FORCE_FLAG="
    if /I "%~2"=="--force" set "FORCE_FLAG=-f"
    if /I "%~2"=="-f"      set "FORCE_FLAG=-f"

    :: Extract Version from file using PowerShell (More robust than findstr for quotes)
    for /F "usebackq tokens=*" %%V in (`powershell -NoProfile -Command "(Select-String 'VERSION =' '%VERSION_FILE%').Line.Split([char]34)[1]"`) do set "CURRENT_VER=%%V"

    if "%CURRENT_VER%"=="" (
        echo %C_RED%[Error] Could not parse version from %VERSION_FILE%%C_RESET%
        exit /b 1
    )

    echo %C_CYAN%[Release] Tagging version: %CURRENT_VER% (Force: %FORCE_FLAG%)%C_RESET%

    :: Tag Root
    git tag %FORCE_FLAG% "%CURRENT_VER%"
    if %ERRORLEVEL% NEQ 0 exit /b 1

    :: Tag Modules
    for %%M in (%MODULES%) do (
        if exist "%%M" (
            echo    Tagging module: %%M/%CURRENT_VER%
            git tag %FORCE_FLAG% "%%M/%CURRENT_VER%"
            if !ERRORLEVEL! NEQ 0 exit /b 1
        )
    )

    echo %C_GREEN%[Release] All tags created successfully.%C_RESET%
    echo    Don't forget to run: git push origin --tags
    exit /b 0

:help
    echo.
    echo %C_YELLOW%Usage: make.bat [command] [module_path] [options]%C_RESET%
    echo.
    echo %C_CYAN%Standard Commands:%C_RESET%
    echo   deps        Run 'go mod tidy ^& vendor'
    echo   get         Run 'go get ./...'
    echo   build       Run 'go build ./...'
    echo   fmt         Run 'go fmt ./...'
    echo   clean       Run 'go clean' (recursive)
    echo   test        Run tests with race detection and coverage
    echo.
    echo %C_CYAN%Composite Commands:%C_RESET%
    echo   update      Update all dependencies (-u) and vendor
    echo   vet         Run 'vet' after tidying modules
    echo.
    echo %C_CYAN%Release Management:%C_RESET%
    echo   version vX.Y.Z     Bump VERSION file and sync %CORE_DEPENDENCY% in all modules
    echo   release [--force]  Create git tags for root and all modules based on VERSION file
    echo.
    exit /b 0