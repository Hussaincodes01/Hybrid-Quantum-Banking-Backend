<#
.SYNOPSIS
  Runs the FINIX demo API and exposes it over HTTPS so an Android build on any
  network can reach it — no router port-forwarding, no static IP.

.DESCRIPTION
  Starts the backend bound to LOOPBACK ONLY and puts a Cloudflare Tunnel in
  front of it. Only the tunnel can reach the server: the port is never open to
  the LAN, let alone the internet, so there is no listening port for anyone to
  scan or hit directly.

  Secrets are generated fresh on each run unless already present in the
  environment. This matters because the moment the API is public, the throwaway
  development values ("local-dev-secret…", an all-zero encryption key) would be
  the only thing standing between a stranger and a signed session token.

  The tunnel gives a public https:// URL, which also removes the need for the
  app's cleartext-HTTP exemption — Android allows HTTPS by default.

.PARAMETER Port
  Local port for the backend. Default 8080.

.PARAMETER Cloudflared
  Path to cloudflared.exe. Default: resolved from PATH, else ~\bin\cloudflared.exe.

.EXAMPLE
  .\demo-remote.ps1
  Prints a https://<random>.trycloudflare.com URL to paste into the app.

.NOTES
  A quick tunnel is anonymous and its hostname changes on every run — fine for a
  demo. For a stable hostname use a named tunnel with a Cloudflare account.
#>
[CmdletBinding()]
param(
    [int]$Port = 8080,
    [string]$Cloudflared = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot   # ...\backend

function New-Secret([int]$Bytes) {
    $buf = New-Object byte[] $Bytes
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buf)
    ($buf | ForEach-Object { $_.ToString("x2") }) -join ""
}

function Resolve-Cloudflared {
    if ($Cloudflared -and (Test-Path $Cloudflared)) { return $Cloudflared }
    $cmd = Get-Command cloudflared -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $fallback = Join-Path $env:USERPROFILE "bin\cloudflared.exe"
    if (Test-Path $fallback) { return $fallback }
    throw "cloudflared not found. Install it (https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) or pass -Cloudflared <path>."
}

# ── Secrets ────────────────────────────────────────────────────────────
# Only generated when absent, so an operator can pin real values by exporting
# them beforehand.
if (-not $env:FINIX_JWT_SECRET)          { $env:FINIX_JWT_SECRET          = New-Secret 32 }
if (-not $env:FINIX_DATA_ENCRYPTION_KEY) { $env:FINIX_DATA_ENCRYPTION_KEY = New-Secret 32 }  # must be 64 hex chars
if (-not $env:FINIX_AADHAAR_HASH_KEY)    { $env:FINIX_AADHAAR_HASH_KEY    = New-Secret 32 }
# Guard the service-to-service and admin surfaces with strong random tokens.
if (-not $env:FINIX_INTERNAL_TOKEN)      { $env:FINIX_INTERNAL_TOKEN      = New-Secret 24 }
if (-not $env:FINIX_ADMIN_TOKEN)         { $env:FINIX_ADMIN_TOKEN         = New-Secret 24 }
if (-not $env:GROQ_API_KEY)              { $env:GROQ_API_KEY              = "disabled" }

# ── Exposure posture ───────────────────────────────────────────────────
# Loopback only: cloudflared connects locally, nothing else can.
$env:APP_ADDR = "127.0.0.1:$Port"
# Native mobile clients send no Origin header and are unaffected by CORS. Keep
# the browser policy tight rather than opening it to "*" for the tunnel.
if (-not $env:CORS_ALLOWED_ORIGINS) { $env:CORS_ALLOWED_ORIGINS = "http://localhost:*,http://127.0.0.1:*" }
# Demo pacing: a blocked transaction otherwise locks payments for 4h.
if (-not $env:FINIX_COOLOFF_BASE_MINUTES) { $env:FINIX_COOLOFF_BASE_MINUTES = "1" }

Write-Host ""
Write-Host "FINIX demo — secure remote access" -ForegroundColor Cyan
Write-Host "=================================" -ForegroundColor Cyan
Write-Host "  bind        : $env:APP_ADDR  (loopback only — not reachable from the LAN)"
Write-Host "  secrets     : generated for this run"
Write-Host "  admin token : $env:FINIX_ADMIN_TOKEN"
Write-Host "  cors        : $env:CORS_ALLOWED_ORIGINS"
Write-Host ""

# ── Backend ────────────────────────────────────────────────────────────
Push-Location $repoRoot
$serverLog = Join-Path $env:TEMP "finix-demo-server.log"
Write-Host "[1/3] starting backend (log: $serverLog)..." -ForegroundColor Yellow
$server = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/server" `
    -RedirectStandardOutput $serverLog -RedirectStandardError "$serverLog.err" `
    -NoNewWindow -PassThru
Pop-Location

$ready = $false
foreach ($i in 1..60) {
    Start-Sleep -Seconds 1
    try {
        $r = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/healthz" -TimeoutSec 2 -UseBasicParsing
        if ($r.StatusCode -eq 200) { $ready = $true; break }
    } catch { }
}
if (-not $ready) {
    Write-Host "backend did not become healthy — see $serverLog" -ForegroundColor Red
    if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force }
    exit 1
}
Write-Host "      backend healthy" -ForegroundColor Green

# Surface the demo logins the app signs in with.
Select-String -Path $serverLog -Pattern "CKYC" -ErrorAction SilentlyContinue |
    Select-Object -Last 11 | ForEach-Object { Write-Host "      $($_.Line.Substring($_.Line.IndexOf('[SEED]')))" }

# ── Tunnel ─────────────────────────────────────────────────────────────
$cf = Resolve-Cloudflared
$tunnelLog = Join-Path $env:TEMP "finix-demo-tunnel.log"
Write-Host ""
Write-Host "[2/3] opening HTTPS tunnel via $cf ..." -ForegroundColor Yellow
$tunnel = Start-Process -FilePath $cf `
    -ArgumentList "tunnel", "--no-autoupdate", "--url", "http://127.0.0.1:$Port" `
    -RedirectStandardOutput $tunnelLog -RedirectStandardError "$tunnelLog.err" `
    -NoNewWindow -PassThru

$publicUrl = $null
foreach ($i in 1..60) {
    Start-Sleep -Seconds 1
    # cloudflared prints the hostname on stderr.
    $text = (Get-Content "$tunnelLog.err", $tunnelLog -ErrorAction SilentlyContinue) -join "`n"
    $m = [regex]::Match($text, "https://[a-z0-9-]+\.trycloudflare\.com")
    if ($m.Success) { $publicUrl = $m.Value; break }
}

if (-not $publicUrl) {
    Write-Host "tunnel did not report a URL — see $tunnelLog.err" -ForegroundColor Red
} else {
    Write-Host "      $publicUrl" -ForegroundColor Green
    Write-Host ""
    Write-Host "[3/3] point the app at it" -ForegroundColor Yellow
    Write-Host "      In the app: Profile -> API address, or in code:"
    Write-Host "        ApiService.instance.setBaseUrl('$publicUrl');"
    Write-Host "      Verify from any network:"
    Write-Host "        curl $publicUrl/healthz"
    Write-Host ""
    Write-Host "      Sign in with CKYC 2000000001 / PIN 123456" -ForegroundColor Cyan
}

Write-Host ""
Write-Host "Anyone with this URL can use the demo API. Stop it when you are done." -ForegroundColor DarkYellow
Write-Host "Ctrl+C to shut down both processes."

try {
    while ($true) {
        Start-Sleep -Seconds 2
        if ($server.HasExited) { Write-Host "backend exited" -ForegroundColor Red; break }
        if ($tunnel.HasExited) { Write-Host "tunnel exited"  -ForegroundColor Red; break }
    }
} finally {
    foreach ($p in @($tunnel, $server)) {
        if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    }
    Write-Host "stopped." -ForegroundColor Yellow
}
