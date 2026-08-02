@echo off
REM ===================================================================
REM  FINIX demo launcher
REM
REM  Double-click to start everything locally, or run from a terminal:
REM
REM    start-finix.bat                 backend only, http://localhost:8080
REM    start-finix.bat tunnel          + public HTTPS URL for phones anywhere
REM    start-finix.bat tunnel rag      + the Python finix-rag service
REM    start-finix.bat rag             local, with RAG
REM
REM  The Groq API key and any pinned secrets are read from .env.local
REM  (gitignored). Everything shuts down when you press Ctrl+C.
REM ===================================================================
setlocal
cd /d "%~dp0"

set "ARGS="
:parse
if "%~1"=="" goto run
if /i "%~1"=="tunnel" set "ARGS=%ARGS% -Tunnel"
if /i "%~1"=="rag"    set "ARGS=%ARGS% -WithRag"
if /i "%~1"=="local"  rem no flag: local-only is the default
shift
goto parse

:run
echo.
echo Starting FINIX%ARGS%
echo.

REM -ExecutionPolicy Bypass so a fresh Windows install does not refuse to run
REM the script; -NoProfile keeps a user profile from changing the environment.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-finix.ps1"%ARGS%

set "RC=%ERRORLEVEL%"
if not "%RC%"=="0" (
  echo.
  echo Launcher exited with code %RC%.
)
echo.
REM Keep the window open when double-clicked so errors stay readable.
pause
endlocal
