@echo off
REM ===================================================================
REM  FINIX demo launcher
REM
REM  Double-click to start EVERYTHING: the backend plus a public HTTPS
REM  URL (ngrok) on the reserved domain that the APK is built against.
REM  That is the combination phones on other networks need, so it is the
REM  default rather than something you have to remember to ask for.
REM
REM    start-finix.bat                 backend + public HTTPS URL
REM    start-finix.bat local           backend only, http://localhost:8080
REM    start-finix.bat rag             + the Python finix-rag service
REM    start-finix.bat all             + finix-rag as well
REM    start-finix.bat local rag       local only, with RAG
REM
REM  finix-rag needs Docker running (Qdrant, OPA, Vault, Redis). It is not
REM  in the default because a stopped Docker would take the whole launch
REM  down with it; add "rag" once Docker is up.
REM
REM  The Groq API key and any pinned secrets are read from .env.local
REM  (gitignored). Everything shuts down when you press Ctrl+C.
REM ===================================================================
setlocal
cd /d "%~dp0"

REM Tunnel is on unless "local" is passed.
set "TUNNEL=1"
set "RAG="

:parse
if "%~1"=="" goto run
if /i "%~1"=="tunnel" set "TUNNEL=1"
if /i "%~1"=="local"  set "TUNNEL="
if /i "%~1"=="rag"    set "RAG=1"
if /i "%~1"=="all"    set "TUNNEL=1" & set "RAG=1"
shift
goto parse

:run
set "ARGS="
if defined TUNNEL set "ARGS=%ARGS% -Tunnel"
if defined RAG    set "ARGS=%ARGS% -WithRag"

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
