$ErrorActionPreference = "Stop"

# Download the ONNX Runtime for Windows x64
$onnxVersion = "1.17.1"
$url = "https://github.com/microsoft/onnxruntime/releases/download/v$onnxVersion/onnxruntime-win-x64-$onnxVersion.zip"
$zipFile = "onnxruntime.zip"
$extractPath = "onnxruntime-win-x64-$onnxVersion"

Write-Host "Downloading ONNX Runtime v$onnxVersion..."
Invoke-WebRequest -Uri $url -OutFile $zipFile

Write-Host "Extracting archive..."
Expand-Archive -Path $zipFile -DestinationPath . -Force

Write-Host "Copying onnxruntime.dll to the Go backend root folder..."
Copy-Item -Path ".\$extractPath\lib\onnxruntime.dll" -Destination ".\onnxruntime.dll" -Force
Copy-Item -Path ".\$extractPath\lib\onnxruntime_providers_shared.dll" -Destination ".\onnxruntime_providers_shared.dll" -Force

Write-Host "Cleaning up..."
Remove-Item -Path $zipFile -Force
Remove-Item -Path $extractPath -Recurse -Force

Write-Host "Done! You can now run the Go server with ONNX ML algorithms enabled."
