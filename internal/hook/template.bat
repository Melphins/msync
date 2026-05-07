@echo off
REM msync pre-commit hook for Windows
REM This script checks if your local database is in sync with the target before allowing commits.

setlocal ENABLEDELAYEDEXPANSION

REM Get the project root
for /f "delims=" %%i in ('git rev-parse --show-toplevel 2^>nul') do set "PROJECT_ROOT=%%i"
if not defined PROJECT_ROOT (
    echo Warning: Not in a git repository. Skipping msync hook.
    exit /b 0
)

set "CONFIG_FILE=%PROJECT_ROOT%\.msync.yml"

REM Check if config exists
if not exist "%CONFIG_FILE%" (
    echo [WARNING] msync: No configuration file found at .msync.yml
    echo    Run 'msync init' to create one, or disable the hook in config.
    exit /b 0
)

REM Default settings
set "HOOK_ENABLED=true"
set "EXCLUDE_BRANCHES=main master staging develop"
set "TRIGGER_PATHS=migrations/ models/ schema/"

REM Check if yq is available (optional)
where yq >nul 2>nul
if %ERRORLEVEL% equ 0 (
    REM Check if hook is enabled
    for /f "delims=" %%i in ('yq e ".hook.enabled // true" "%CONFIG_FILE%" 2^>nul') do set "HOOK_ENABLED=%%i"

    REM Read exclude branches
    for /f "usebackq tokens=*" %%i in (`yq e ".hook.exclude_branches[]?" "%CONFIG_FILE%" 2^>nul`) do (
        if not "%%i"=="" (
            set "EXCLUDE_BRANCHES=!EXCLUDE_BRANCHES! %%i"
        )
    )

    REM Read trigger paths
    set "TRIGGER_PATHS="
    for /f "usebackq tokens=*" %%i in (`yq e ".hook.trigger_paths[]?" "%CONFIG_FILE%" 2^>nul`) do (
        if not "%%i"=="" (
            if not defined TRIGGER_PATHS (
                set "TRIGGER_PATHS=%%i"
            ) else (
                set "TRIGGER_PATHS=!TRIGGER_PATHS! %%i"
            )
        )
    )

    REM Add defaults if none configured
    if not defined TRIGGER_PATHS (
        set "TRIGGER_PATHS=migrations/ models/ schema/"
    )
)

REM Check if hook is enabled
if /i "!HOOK_ENABLED!" neq "true" (
    exit /b 0
)

REM Check for excluded branches
for /f "delims=" %%i in ('git rev-parse --abbrev-ref HEAD 2^>nul') do set "CURRENT_BRANCH=%%i"

for %%e in (%EXCLUDE_BRANCHES%) do (
    if /i "!CURRENT_BRANCH!"=="%%e" (
        echo [SKIPPED] msync: Skipping on excluded branch '!CURRENT_BRANCH!'
        exit /b 0
    )
)

REM Check if any staged files match trigger paths
set "TRIGGERED=false"
for /f "delims=" %%f in ('git diff --cached --name-only 2^>nul') do (
    set "FILE=%%f"
    for %%p in (%TRIGGER_PATHS%) do (
        set "PATTERN=%%~p"
        REM Remove trailing slash
        if "!PATTERN:~-1!"=="\" set "PATTERN=!PATTERN:~0,-1!"
        if "!FILE:~0,%~zPATTERN%!"=="!PATTERN!\" (
            set "TRIGGERED=true"
            goto :triggered
        )
        REM Check if pattern is in filename
        echo !FILE! | findstr /c:"!PATTERN!" >nul
        if %ERRORLEVEL% equ 0 (
            set "TRIGGERED=true"
            goto :triggered
        )
    )
)

:triggered
if /i "!TRIGGERED!"=="false" (
    exit /b 0
)

echo [CHECK] msync: Checking database migration status...

REM Run msync verify
set "TARGET_NAME=production"
if not "%~1"=="" set "TARGET_NAME=%~1"

cd /d "%PROJECT_ROOT%"
msync verify --target "!TARGET_NAME!" 2>nul
if %ERRORLEVEL% neq 0 (
    echo.
    echo [FAILED] msync: Verification failed!
    echo.
    echo Your database is out of sync with the target environment.
    echo Please run 'msync up' to apply pending migrations before committing.
    echo.
    echo To skip this check (not recommended), use:
    echo   git commit --no-verify
    echo.
    exit /b 1
)

echo [OK] msync: Database is in sync
exit /b 0
