@echo off
REM Build script for Multi-Model Router (Windows)
REM Usage: build.bat [dev|release]

setlocal enabledelayedexpansion

set "MODE=%~1"
if "%MODE%"=="" set "MODE=release"
set "BINARY=MultiModelRouter.exe"
set "OUTPUT_DIR=build\bin"

echo ==^> Building Multi-Model Router (%MODE% mode)...

REM Check Go
where go >nul 2>&1
if errorlevel 1 (
    echo ERROR: Go is not installed
    exit /b 1
)

REM Check Wails
where wails >nul 2>&1
if errorlevel 1 (
    set "WAILS=%USERPROFILE%\go\bin\wails.exe"
    if not exist "!WAILS!" (
        echo ERROR: Wails CLI is not installed. Run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
        exit /b 1
    )
) else (
    set "WAILS=wails"
)

REM Install frontend dependencies
echo ==^> Installing frontend dependencies...
cd frontend && call npm install && cd ..

REM Build
echo ==^> Compiling...
if "%MODE%"=="dev" (
    "!WAILS!" build
) else (
    "!WAILS!" build -clean -ldflags "-s -w"
)

REM Verify
if exist "%OUTPUT_DIR%\%BINARY%" (
    for %%A in ("%OUTPUT_DIR%\%BINARY%") do set "SIZE=%%~zA"
    echo.
    echo ==^> Build successful!
    echo     Output: %OUTPUT_DIR%\%BINARY%
    echo.
    echo Usage:
    echo     %OUTPUT_DIR%\%BINARY%                   # GUI mode
    echo     %OUTPUT_DIR%\%BINARY% serve --port 9680  # Headless proxy
    echo     %OUTPUT_DIR%\%BINARY% tui                # Terminal UI
    echo     %OUTPUT_DIR%\%BINARY% version            # Print version
) else (
    echo ERROR: Build failed - binary not found
    exit /b 1
)

endlocal
