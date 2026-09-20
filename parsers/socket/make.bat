@echo off
setlocal EnableExtensions DisableDelayedExpansion
if "%~1"=="" goto :Help
if /I "%~1"=="help" goto :Help
set "ACTION="
for %%A in (env deps get update build fmt vet lint clean test) do if /I "%~1"=="%%A" set "ACTION=%%A"
if not defined ACTION (
    echo [Error] Unknown command: %~1
    exit /b 1
)
set "FIX="
if "%ACTION%"=="lint" if /I "%~2"=="--fix" set "FIX=--fix"
if not "%~2"=="" if not defined FIX goto :InvalidArgs
if not "%~3"=="" goto :InvalidArgs
if "%ACTION%"=="env" goto :Env
call "%~dp0..\..\make.bat" %ACTION% parsers/socket %FIX%
exit /b %errorlevel%

:Env
pushd "%~dp0" || exit /b 1
call go env
set "RESULT=%errorlevel%"
popd
exit /b %RESULT%

:InvalidArgs
echo [Error] Only lint accepts an option: --fix.
exit /b 1

:Help
echo Usage: make.bat [env ^| deps ^| get ^| update ^| build ^| fmt ^| vet ^| lint ^| clean ^| test]
echo Runs only parsers/socket using the root build script.
echo Options: lint --fix; environment: GOPROXY, TEST_TIMEOUT.
exit /b 0
