@echo off
setlocal EnableExtensions DisableDelayedExpansion
pushd "%~dp0" || exit /b 1
set "PROJECT_ROOT=%CD%"

:: Keep this list and its order in sync with the root Makefile.
set "MODULES=parsers/engine parsers/socket servers/engine servers/socket"
set "MODULES=%MODULES% adapters/adapter adapters/mongo adapters/postgres adapters/redis adapters/unix adapters/valkey"
set "MODULES=%MODULES% clients/engine clients/socket"
if not defined GOPROXY set "GOPROXY=https://proxy.golang.org,direct"
if not defined TEST_TIMEOUT set "TEST_TIMEOUT=60s"
set "VERSION_FILE=pkg\version\version.go"
set "CORE_DEP=github.com/zishang520/socket.io/"
set "VERSION_PATTERN=^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$"

set "MAKE_ARGS=%*"
call :Main %%MAKE_ARGS%%
set "RESULT=%ERRORLEVEL%"
popd
exit /b %RESULT%

:Main
if "%~1"=="" goto :Help
set "ACTION="
for %%A in (help env deps get update build fmt vet lint clean test version release) do if /I "%~1"=="%%A" set "ACTION=%%A"
if not defined ACTION (
    echo [Error] Unknown command: %~1
    exit /b 1
)
if "%ACTION%"=="help" goto :Help
if "%ACTION%"=="version" goto :Version
if "%ACTION%"=="env" (
    call go env
    exit /b
)

set "TARGET="
set "FIX="
set "FORCE="
shift
:ParseArgs
if "%~1"=="" goto :Run
if "%ACTION%"=="lint" if /I "%~1"=="--fix" (
    set "FIX=--fix"
    shift
    goto :ParseArgs
)
if "%ACTION%"=="release" if /I "%~1"=="--force" goto :ForceArg
if "%ACTION%"=="release" if /I "%~1"=="-f" goto :ForceArg
if defined TARGET (
    echo [Error] Unexpected argument: %~1
    exit /b 1
)
call :ValidateModule "%~1" || exit /b 1
set "TARGET=%~1"
shift
goto :ParseArgs

:ForceArg
set "FORCE=-f"
shift
goto :ParseArgs

:Run
set "SCOPE=. %MODULES%"
if defined TARGET set "SCOPE=%TARGET%"
call :ValidateScope || exit /b 1
if "%ACTION%"=="release" goto :Release
where go >nul 2>nul || (
    echo [Error] Go is not installed or not in PATH.
    exit /b 1
)
call :Run_%ACTION%
exit /b

:ValidateModule
for %%M in (. %MODULES%) do if "%%M"=="%~1" exit /b 0
echo [Error] Unknown module: %~1. Expected: . %MODULES%
exit /b 1

:ValidateScope
for %%M in (%SCOPE%) do if not exist "%%M/go.mod" (
    echo [Error] Module file not found: %%M/go.mod
    exit /b 1
)
exit /b 0

:: All module commands share directory handling and preserve the failing exit code.
:RunBatch
for %%M in (%SCOPE%) do (
    call :RunInDir "%%M" "%~1"
    if errorlevel 1 exit /b
)
exit /b 0

:RunInDir
echo [%~2] Processing: %~1
pushd "%~1" || exit /b 1
call :Exec_%~2
set "MODULE_RESULT=%ERRORLEVEL%"
popd
if not "%MODULE_RESULT%"=="0" echo [Error] Failed in %~1 ^(exit code: %MODULE_RESULT%^)
exit /b %MODULE_RESULT%

:Run_deps
:Run_get
:Run_build
:Run_fmt
:Run_clean
call :RunBatch "%ACTION%"
exit /b

:Run_update
call :RunBatch update || exit /b
call :RunBatch deps
exit /b

:Run_vet
:Run_test
call :RunBatch deps || exit /b
call :RunBatch "%ACTION%"
exit /b

:Run_lint
where golangci-lint >nul 2>nul || (
    echo [Error] golangci-lint is not installed. See https://golangci-lint.run/welcome/install/
    exit /b 1
)
call :RunBatch deps || exit /b
call :RunBatch lint
exit /b

:Exec_deps
call go mod tidy || exit /b
call go mod vendor
exit /b

:Exec_get
call go get ./...
exit /b

:Exec_update
call go get -u -v ./...
exit /b

:Exec_build
call go build ./...
exit /b

:Exec_fmt
call go fmt ./...
exit /b

:Exec_clean
call go clean -v -r ./...
exit /b

:Exec_vet
call go vet ./...
exit /b

:Exec_test
:: Bypass cached results for this run without clearing the global test cache.
call go test -count=1 -timeout=%TEST_TIMEOUT% -race -cover -covermode=atomic ./... <nul
exit /b

:Exec_lint
call golangci-lint run --timeout=5m --config="%PROJECT_ROOT%\.golangci.yml" %FIX% ./... <nul
exit /b

:Version
set "NEW_VER=%~2"
if not "%~3"=="" (
    echo [Error] Usage: make.bat version vX.Y.Z
    exit /b 1
)
powershell -NoProfile -Command "if ($env:NEW_VER -cnotmatch $env:VERSION_PATTERN) { Write-Error 'Expected version vX.Y.Z or vX.Y.Z-prerelease'; exit 1 }" <nul
if errorlevel 1 exit /b
set "SCOPE=. %MODULES%"
call :ValidateScope || exit /b 1
where go >nul 2>nul || exit /b 1
echo [Version] Updating %VERSION_FILE% to %NEW_VER%
powershell -NoProfile -Command "$ErrorActionPreference = 'Stop'; $path = Join-Path $env:PROJECT_ROOT $env:VERSION_FILE; $text = [IO.File]::ReadAllText($path); $pattern = 'const VERSION = \x22[^\x22]*\x22'; if ([regex]::Matches($text, $pattern).Count -ne 1) { throw 'Expected one VERSION constant' }; $text = [regex]::Replace($text, $pattern, ('const VERSION = ' + [char]34 + $env:NEW_VER + [char]34)); [IO.File]::WriteAllText($path, $text, (New-Object Text.UTF8Encoding $false))" <nul
if errorlevel 1 exit /b
set "SCOPE=%MODULES%"
call :RunBatch version || exit /b
set "SCOPE=. %MODULES%"
call :RunBatch deps || exit /b
echo [Version] Updated to %NEW_VER%
exit /b 0

:Exec_version
call go mod tidy || exit /b
:: Capture go list separately: FOR /F cannot propagate a producer's failure.
set "DEPS_FILE=%TEMP%\socketio-modules-%RANDOM%-%RANDOM%.txt"
call :UpdateDependencies "%DEPS_FILE%"
set "DEPS_RESULT=%ERRORLEVEL%"
del /q "%DEPS_FILE%" >nul 2>nul
if not "%DEPS_RESULT%"=="0" exit /b %DEPS_RESULT%
call go mod tidy
exit /b

:UpdateDependencies
call go list -mod=mod -f "{{if not .Main}}{{.Path}}{{end}}" -m all >"%~1"
if errorlevel 1 exit /b
for /F "usebackq delims=" %%D in (`findstr /B /L /C:"%CORE_DEP%" "%~1"`) do (
    call go get -v "%%D@%NEW_VER%"
    if errorlevel 1 exit /b
)
exit /b 0

:Release
set "CURRENT_VER="
for /F "delims=" %%V in ('powershell -NoProfile -Command "$ErrorActionPreference = 'Stop'; $text = [IO.File]::ReadAllText((Join-Path $env:PROJECT_ROOT $env:VERSION_FILE)); $m = [regex]::Matches($text, 'const VERSION = \x22([^\x22]*)\x22'); if ($m.Count -ne 1 -or $m[0].Groups[1].Value -cnotmatch $env:VERSION_PATTERN) { exit 1 }; $m[0].Groups[1].Value" ^<nul') do set "CURRENT_VER=%%V"
if not defined CURRENT_VER (
    echo [Error] Could not read a valid version from %VERSION_FILE%
    exit /b 1
)
for %%M in (%SCOPE%) do (
    call :TagModule "%%M"
    if errorlevel 1 exit /b
)
echo [Release] Tags created locally. Push the intended tags when ready.
exit /b 0

:TagModule
set "TAG=%~1/%CURRENT_VER%"
if "%~1"=="." set "TAG=%CURRENT_VER%"
echo [Release] Tagging %TAG%
call git tag %FORCE% "%TAG%"
exit /b

:Help
echo Usage: make.bat command [module] [options]
echo.
echo   deps       Run go mod tidy and go mod vendor
echo   get        Run go get ./...
echo   update     Update dependencies and refresh vendor
echo   build      Build packages
echo   fmt        Format Go code
echo   clean      Clean packages recursively
echo   vet        Refresh deps, then run go vet
echo   lint       Refresh deps, then run golangci-lint [--fix]
echo   test       Refresh deps, then test with race detection and coverage
echo   env        Show go env
echo   version vX.Y.Z             Update VERSION and sync all modules
echo   release [module] [--force] Create local tags; -f also accepted
echo.
echo Module: . for root, or one of: %MODULES%
echo Omit module to process root and all listed modules.
echo Environment overrides: GOPROXY, TEST_TIMEOUT ^(default: 60s^).
exit /b 0
