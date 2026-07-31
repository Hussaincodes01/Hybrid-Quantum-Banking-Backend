# setup_onnx_runtime.ps1 — download the ONNX Runtime shared library that matches
# onnxruntime_go v1.31.0 (ORT_API_VERSION 26 -> onnxruntime 1.26.0) and place it
# under backend/runtime/. Run once on the Windows host that will serve the models.
#
#   pwsh backend/scripts/setup_onnx_runtime.ps1
#
# It does NOT install a C compiler (see backend/models/ONNX_RUNTIME_SETUP.md §1).

$ErrorActionPreference = "Stop"
$Version = "1.26.0"
$Zip     = "onnxruntime-win-x64-$Version.zip"
$Url     = "https://github.com/microsoft/onnxruntime/releases/download/v$Version/$Zip"

# backend/ is the parent of this scripts/ dir.
$Backend  = Split-Path -Parent $PSScriptRoot
$Runtime  = Join-Path $Backend "runtime"
$TmpZip   = Join-Path $env:TEMP $Zip
$TmpDir   = Join-Path $env:TEMP "onnxruntime-$Version"

New-Item -ItemType Directory -Force -Path $Runtime | Out-Null

Write-Host "Downloading $Url ..."
Invoke-WebRequest -Uri $Url -OutFile $TmpZip

Write-Host "Extracting ..."
if (Test-Path $TmpDir) { Remove-Item -Recurse -Force $TmpDir }
Expand-Archive -Path $TmpZip -DestinationPath $TmpDir

$Dll = Get-ChildItem -Path $TmpDir -Recurse -Filter "onnxruntime.dll" | Select-Object -First 1
if (-not $Dll) { throw "onnxruntime.dll not found in the archive" }
Copy-Item $Dll.FullName (Join-Path $Runtime "onnxruntime.dll") -Force

# Providers DLLs, if present, must sit beside onnxruntime.dll.
Get-ChildItem -Path (Split-Path $Dll.FullName) -Filter "onnxruntime_providers_*.dll" -ErrorAction SilentlyContinue |
    ForEach-Object { Copy-Item $_.FullName $Runtime -Force }

$LibPath = Join-Path $Runtime "onnxruntime.dll"
Write-Host ""
Write-Host "ONNX Runtime $Version ready: $LibPath" -ForegroundColor Green
Write-Host ""
Write-Host "Next: set the env var and build with the onnx tag:" -ForegroundColor Cyan
Write-Host "  `$env:CGO_ENABLED = '1'"
Write-Host "  `$env:FINIX_ONNXRUNTIME_LIB = '$LibPath'"
Write-Host "  go build -tags onnx ./..."
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Host ""
    Write-Host "WARNING: gcc not found on PATH. Install a C compiler (see ONNX_RUNTIME_SETUP.md §1)." -ForegroundColor Yellow
}
