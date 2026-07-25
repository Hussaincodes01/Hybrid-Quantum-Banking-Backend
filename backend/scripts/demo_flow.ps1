param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [switch]$OpenDashboard
)

$ErrorActionPreference = "Stop"

Write-Host "FINIX backend demo flow" -ForegroundColor Cyan
Write-Host "Base URL: $BaseUrl"

$health = Invoke-RestMethod -Uri "$BaseUrl/healthz"
if ($health.status -ne "ok") {
    throw "Health check failed"
}

$ts = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$mobile = "9" + (($ts % 1000000000).ToString().PadLeft(9, "0"))

$registerBody = @{
    name = "Demo User"
    mobile = $mobile
    email = "demo.$ts@finix.local"
    deviceIdFingerprint = "demo-device-$ts"
    deviceType = "flutter"
    appVersion = "0.1.0"
    ipAddress = "127.0.0.1"
} | ConvertTo-Json

$reg = Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/auth/register" -ContentType "application/json" -Body $registerBody
$uid = $reg.userId

Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/auth/ekyc/verify" -ContentType "application/json" -Body (@{
    userId = $uid
    panLast4 = "1234"
    aadhaarLast4 = "5678"
} | ConvertTo-Json) | Out-Null

Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/auth/biometric/register" -ContentType "application/json" -Body (@{
    userId = $uid
    publicKeyB64 = "demo-public-key"
} | ConvertTo-Json) | Out-Null

$challenge = Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/auth/login/challenge" -ContentType "application/json" -Body (@{
    userId = $uid
} | ConvertTo-Json)

$verify = Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/auth/login/verify" -ContentType "application/json" -Body (@{
    userId = $uid
    challenge = $challenge.challenge
    signature = "sig-demo"
} | ConvertTo-Json)

$token = $verify.accessToken
$headers = @{ Authorization = "Bearer $token" }

$dashboard = Invoke-RestMethod -Uri "$BaseUrl/v1/dashboard" -Headers $headers
$flow = Invoke-RestMethod -Uri "$BaseUrl/v1/system/flow-dashboard"

Write-Host ""
Write-Host "Demo complete" -ForegroundColor Green
Write-Host "  user id      : $uid"
Write-Host "  risk band    : $($dashboard.riskBand)"
Write-Host "  flow requests: $($flow.totals.requests)"
Write-Host "  flow errors  : $($flow.totals.errors)"
Write-Host ""
Write-Host "Top flow groups:"
$flow.groups | Select-Object -First 6 group, requests, errors | Format-Table -AutoSize

Write-Host "JSON dashboard: $BaseUrl/v1/system/flow-dashboard"
Write-Host "HTML dashboard: $BaseUrl/ops/flow-dashboard"

if ($OpenDashboard) {
    Start-Process "$BaseUrl/ops/flow-dashboard"
}
