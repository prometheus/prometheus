@echo off
REM Claude-Flow local wrapper
REM This script ensures claude-flow runs from your project directory

set PROJECT_DIR=%CD%
set PWD=%PROJECT_DIR%
set CLAUDE_WORKING_DIR=%PROJECT_DIR%

REM Try to find claude-flow binary
REM Check common locations for npm/npx installations

REM 1. Local node_modules (npm install claude-flow)
if exist "%PROJECT_DIR%\node_modules\.bin\claude-flow.cmd" (
  cd /d "%PROJECT_DIR%"
  "%PROJECT_DIR%\node_modules\.bin\claude-flow.cmd" %*
  exit /b %ERRORLEVEL%
)

REM 2. Parent directory node_modules (monorepo setup)
if exist "%PROJECT_DIR%\..\node_modules\.bin\claude-flow.cmd" (
  cd /d "%PROJECT_DIR%"
  "%PROJECT_DIR%\..\node_modules\.bin\claude-flow.cmd" %*
  exit /b %ERRORLEVEL%
)

REM 3. Global installation (npm install -g claude-flow)
where claude-flow >nul 2>nul
if %ERRORLEVEL% EQU 0 (
  cd /d "%PROJECT_DIR%"
  claude-flow %*
  exit /b %ERRORLEVEL%
)

REM 4. Fallback to npx (will download if needed)
cd /d "%PROJECT_DIR%"
npx claude-flow@latest %*
