<#
.SYNOPSIS
  One command to bring the FINIX demo up: backend, optional public tunnel,
  optional Python RAG service.

.DESCRIPTION
  Handles the setup that is easy to get wrong:

    * mandatory secrets — the server refuses to boot without them, and the
      values published in the docs must never be used on a public URL, so
      strong ones are generated per run unless you supply your own;
    * JAVA_HOME repair — the machine default points at a JDK that no longer
      exists, which breaks any Gradle/APK work started from the same shell;
    * bind address — loopback-only when tunnelling, so nothing is exposed to
      the LAN or the internet except through the tunnel;
    * a real readiness check instead of a fixed sleep.

  Everything it starts is torn down on Ctrl+C.

.PARAMETER Port
  Backend port. Default 8080.

.PARAMETER Tunnel
  Publish over HTTPS so phones on other networks can reach it.

.PARAMETER Subdomain
  Tunnel hostname to request. Keep it stable so a URL compiled into an APK
  (--dart-define=FINIX_BASE_URL) survives a restart. Default finix-psb-demo.

.PARAMETER GroqApiKey
  Enables real LLM chatbot replies. Without it the chatbot answers from
  keyword templates (still backed by the user's real data).

.PARAMETER WithRag
  Also start the Python finix-rag service and point the backend at it. Requires
  its dependencies and Qdrant/Vault/OPA/Redis; the script checks and tells you
  what is missing rather than failing obscurely.

.PARAMETER CoolOffMinutes
  Cooling-off window after a blocked transaction. Default 1 so a demo recovers
  quickly; production uses the 240-minute spec value.

.EXAMPLE
  .\start-finix.ps1
  Local only, on http://localhost:8080.

.EXAMPLE
  .\start-finix.ps1 -Tunnel -GroqApiKey $env:GROQ_API_KEY
  Public HTTPS URL with real LLM replies.
#>
[CmdletBinding()]
param(
    [int]$Port = 8080,
    [switch]$Tunnel,
    [string]$Subdomain = "finix-psb-demo",
    [string]$GroqApiKey = "",
    [switch]$WithRag,
    [int]$CoolOffMinutes = 1
)

$ErrorActionPreference = "Stop"
$root       = $PSScriptRoot
$backendDir = Join-Path $root "backend"
$ragDir     = Join-Path $root "finix-rag"
$procs      = @()

function Info($m) { Write-Host "  $m" }
function Good($m) { Write-Host "  $m" -ForegroundColor Green }
function Warn($m) { Write-Host "  $m" -ForegroundColor Yellow }
function Bad ($m) { Write-Host "  $m" -ForegroundColor Red }
function Step($m) { Write-Host ""; Write-Host $m -ForegroundColor Cyan }

function New-Secret([int]$bytes) {
    $b = New-Object byte[] $bytes
    # RandomNumberGenerator::Create()+GetBytes works on .NET Framework (Windows
    # PowerShell 5.1) as well as .NET Core; ::Fill is Core-only and throws here.
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($b) } finally { $rng.Dispose() }
    ($b | ForEach-Object { $_.ToString("x2") }) -join ""
}

function Wait-Healthy([string]$url, [int]$seconds) {
    foreach ($i in 1..$seconds) {
        Start-Sleep -Seconds 1
        try {
            if ((Invoke-WebRequest -Uri $url -TimeoutSec 3 -UseBasicParsing).StatusCode -eq 200) { return $true }
        } catch { }
    }
    return $false
}

Write-Host ""
Write-Host "FINIX demo launcher" -ForegroundColor Cyan
Write-Host "===================" -ForegroundColor Cyan

# ── Toolchain ──────────────────────────────────────────────────────────
Step "[1/5] toolchain"
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Bad "go not found on PATH"; exit 1 }
Good "go $((go version) -replace '^go version ','')"

# JAVA_HOME on this machine points at a JDK that was removed; repair it so an
# APK build launched from this shell works.
if ($env:JAVA_HOME -and -not (Test-Path (Join-Path $env:JAVA_HOME "bin\java.exe"))) {
    $found = Get-ChildItem "C:\Program Files\Eclipse Adoptium","C:\Program Files\Java" -Filter "java.exe" `
                -Recurse -Depth 3 -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) {
        $env:JAVA_HOME = Split-Path (Split-Path $found.FullName)
        Warn "JAVA_HOME was stale; using $env:JAVA_HOME"
    } else {
        Warn "JAVA_HOME points at a missing JDK and no replacement was found (only affects Android builds)"
    }
}

# ── Secrets and configuration ──────────────────────────────────────────
Step "[2/5] configuration"
if (-not $env:FINIX_JWT_SECRET)          { $env:FINIX_JWT_SECRET          = New-Secret 32 }
if (-not $env:FINIX_DATA_ENCRYPTION_KEY) { $env:FINIX_DATA_ENCRYPTION_KEY = New-Secret 32 }  # 64 hex chars
if (-not $env:FINIX_AADHAAR_HASH_KEY)    { $env:FINIX_AADHAAR_HASH_KEY    = New-Secret 32 }
if (-not $env:FINIX_INTERNAL_TOKEN)      { $env:FINIX_INTERNAL_TOKEN      = New-Secret 24 }
if (-not $env:FINIX_ADMIN_TOKEN)         { $env:FINIX_ADMIN_TOKEN         = New-Secret 24 }
Good "secrets generated for this run"

# GROQ_API_KEY is mandatory for the config to validate; "disabled" is the
# sentinel that turns the live LLM off and falls back to template replies.
if ($GroqApiKey)          { $env:GROQ_API_KEY = $GroqApiKey }
elseif (-not $env:GROQ_API_KEY) { $env:GROQ_API_KEY = "disabled" }
if ($env:GROQ_API_KEY -eq "disabled") {
    Warn "chatbot: template replies (pass -GroqApiKey for real LLM answers)"
} else {
    Good "chatbot: Groq LLM enabled"
}

$env:FINIX_COOLOFF_BASE_MINUTES = "$CoolOffMinutes"
if (-not $env:CORS_ALLOWED_ORIGINS) { $env:CORS_ALLOWED_ORIGINS = "http://localhost:*,http://127.0.0.1:*" }

# Loopback-only when tunnelling: the tunnel reaches in, nothing else can.
$env:APP_ADDR = if ($Tunnel) { "127.0.0.1:$Port" } else { "0.0.0.0:$Port" }
Info "bind $env:APP_ADDR"

# ── Optional RAG service ───────────────────────────────────────────────
Step "[3/5] finix-rag"
if ($WithRag) {
    $ragOk = $true
    if (-not (Test-Path $ragDir)) { Bad "finix-rag directory not found"; $ragOk = $false }
    if ($ragOk) {
        # Import-check the heavy dependency chain before promising anything.
        $probe = & python -c "
import importlib, sys
missing = [m for m in ('fastapi','uvicorn','llama_index','qdrant_client') if not importlib.util.find_spec(m)]
print(','.join(missing))
" 2>$null
        if ($probe) {
            Bad "missing Python packages: $probe"
            Info "install with:  pip install -r finix-rag/requirements.txt"
            $ragOk = $false
        }
    }
    if ($ragOk) {
        $ragLog = Join-Path $env:TEMP "finix-rag.log"
        $env:PYTHONPATH = Join-Path $ragDir "src"
        $procs += Start-Process -FilePath "python" `
            -ArgumentList "-m","uvicorn","finix_rag.api:app","--host","127.0.0.1","--port","8000" `
            -RedirectStandardOutput $ragLog -RedirectStandardError "$ragLog.err" -NoNewWindow -PassThru
        if (Wait-Healthy "http://127.0.0.1:8000/health" 40) {
            Good "finix-rag healthy on :8000"
            $env:AI_PROVIDER = "remote"
            $env:AIML_UPSTREAM_URL = "http://127.0.0.1:8000"
        } else {
            Warn "finix-rag did not become healthy - see $ragLog.err; continuing without it"
        }
    } else {
        Warn "continuing without finix-rag (chatbot uses the local path)"
    }
} else {
    Info "not requested (-WithRag to enable)"
}

# ── Backend ────────────────────────────────────────────────────────────
Step "[4/5] backend"
$log = Join-Path $env:TEMP "finix-backend.log"
Push-Location $backendDir

# Compile first and run the binary directly rather than `go run`. `go run`
# builds to a temp file and spawns it as a CHILD, so killing the `go` process
# leaves that child holding the port — the next start then fails with "address
# already in use". Running the binary ourselves means the PID we track is the
# server, and shutdown actually stops it.
$exe = Join-Path $env:TEMP "finix-server.exe"
Info "compiling..."
& go build -o $exe ./cmd/server
if ($LASTEXITCODE -ne 0) { Bad "go build failed"; Pop-Location; exit 1 }

$backend = Start-Process -FilePath $exe `
    -RedirectStandardOutput $log -RedirectStandardError "$log.err" -NoNewWindow -PassThru
$procs += $backend
Pop-Location

if (-not (Wait-Healthy "http://127.0.0.1:$Port/healthz" 90)) {
    Bad "backend did not become healthy - see $log.err"
    foreach ($p in $procs) { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } }
    exit 1
}
Good "backend healthy on :$Port"

# ── Public URL ─────────────────────────────────────────────────────────
Step "[5/5] public access"
$publicUrl = "http://localhost:$Port"
if ($Tunnel) {
    if (-not (Get-Command npx -ErrorAction SilentlyContinue)) {
        Bad "npx not found - cannot start localtunnel; running local-only"
    } else {
        $tl = Join-Path $env:TEMP "finix-tunnel.log"
        Remove-Item $tl,"$tl.err" -ErrorAction SilentlyContinue
        $procs += Start-Process -FilePath "cmd.exe" `
            -ArgumentList "/c","npx --yes localtunnel --port $Port --subdomain $Subdomain > `"$tl`" 2> `"$tl.err`"" `
            -WindowStyle Hidden -PassThru
        $found = $null
        foreach ($i in 1..60) {
            Start-Sleep -Seconds 1
            $txt = (Get-Content $tl,"$tl.err" -ErrorAction SilentlyContinue) -join "`n"
            $m = [regex]::Match($txt, "https://[a-z0-9-]+\.loca\.lt")
            if ($m.Success) { $found = $m.Value; break }
        }
        if ($found) {
            $publicUrl = $found
            Good "public URL: $publicUrl"
            if ($found -notmatch [regex]::Escape($Subdomain)) {
                Warn "requested subdomain was taken - a build with the old URL compiled in will not reach this run"
            }
        } else {
            Warn "tunnel produced no URL - see $tl.err (see REMOTE_ACCESS.md for alternatives)"
        }
    }
}

# ── Ready ──────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "READY" -ForegroundColor Green
Write-Host "  API        $publicUrl"
Write-Host "  health     $publicUrl/healthz"
Write-Host "  sign in    cKYC 2000000001-2000000010, PIN 123456"
Write-Host "  admin tok  $env:FINIX_ADMIN_TOKEN"
Write-Host "  logs       $log"
if ($Tunnel) {
    Write-Host ""
    Write-Host "  Build an APK that reaches this from any network:" -ForegroundColor Cyan
    Write-Host "    flutter build apk --release --dart-define=FINIX_BASE_URL=$publicUrl"
    Write-Host ""
    Write-Host "  This URL is public. Anyone with it can sign in as the demo users." -ForegroundColor DarkYellow
}
Write-Host ""
Write-Host "Ctrl+C to stop everything." -ForegroundColor DarkGray

try {
    # Only the backend exiting is fatal. The tunnel is launched through a cmd
    # wrapper that exits as soon as npx takes over, so treating ANY tracked
    # process exit as fatal tore the whole stack down seconds after it came up.
    # The tunnel is watched by re-checking the URL instead of by PID.
    $tunnelWarned = $false
    while ($true) {
        Start-Sleep -Seconds 5
        if ($backend.HasExited) { Warn "backend exited; shutting down"; break }

        if ($Tunnel -and -not $tunnelWarned -and $publicUrl -like "https://*") {
            try {
                Invoke-WebRequest -Uri "$publicUrl/healthz" -TimeoutSec 20 -UseBasicParsing | Out-Null
            } catch {
                Warn "tunnel is not responding - restart it with:"
                Info "  npx localtunnel --port $Port --subdomain $Subdomain"
                $tunnelWarned = $true   # say it once, keep the backend running
            }
        }
    }
} catch {
} finally {
    Write-Host ""
    Write-Host "stopping..." -ForegroundColor Yellow
    # Reverse order so the tunnel drops before the backend it fronts.
    [array]::Reverse($procs)
    foreach ($p in $procs) {
        if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    }
    Get-Process node -ErrorAction SilentlyContinue | Where-Object { $_.Path -like "*node*" } | ForEach-Object {
        # localtunnel runs under a cmd wrapper; clean up the node child too.
        try { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue } catch { }
    }
    Write-Host "stopped." -ForegroundColor Yellow
}
