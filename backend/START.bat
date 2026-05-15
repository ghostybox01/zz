@echo off
REM RAVEN X 2.0 - Windows Launcher
REM Double-click this file to start everything automatically

title RAVEN X 2.0 - Auto Launcher

echo.
echo ============================================================
echo           RAVEN X 2.0 - ONE-CLICK STARTER
echo ============================================================
echo.

REM Check if Python is installed
python --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Python not found!
    echo Please install Python 3.7+ from https://python.org
    echo.
    pause
    exit /b 1
)

REM Change to script directory
cd /d "%~dp0"

REM Check if virtual environment exists
if not exist "venv\" (
    echo [1/2] Creating Python virtual environment...
    python -m venv venv
    if errorlevel 1 (
        echo [ERROR] Failed to create virtual environment
        pause
        exit /b 1
    )
)

REM Activate virtual environment
echo [2/2] Activating virtual environment...
call venv\Scripts\activate.bat

REM Install/upgrade dependencies
echo.
echo Installing Python dependencies...
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

REM Run the auto starter
echo.
echo ============================================================
echo Starting RAVEN X 2.0...
echo ============================================================
echo.

python start.py

REM If we reach here, the app has stopped
echo.
echo ============================================================
echo RAVEN X 2.0 stopped
echo ============================================================
echo.
pause
