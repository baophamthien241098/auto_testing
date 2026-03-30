$ErrorActionPreference = "Stop"

Write-Host "Building Go binary for Linux..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o main_linux .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Creating dist folder..."
if (Test-Path dist) { Remove-Item dist -Recurse -Force }
New-Item -ItemType Directory -Force -Path dist | Out-Null

Write-Host "Copying files..."
Copy-Item main_linux -Destination dist\
Copy-Item Dockerfile.release -Destination dist\Dockerfile
Copy-Item docker-compose.yml -Destination dist\
Copy-Item profiles.json -Destination dist\
Copy-Item config.json -Destination dist\
Copy-Item README_DEPLOY.txt -Destination dist\
Copy-Item comments.csv -Destination dist\
Copy-Item -Recurse public -Destination dist\

# Create data directory (for xlsx file)
New-Item -ItemType Directory -Force -Path dist\data | Out-Null
New-Item -ItemType File -Path dist\data\.gitkeep -Force | Out-Null

Write-Host "Zipping..."
$zipFile = "\\DESKTOP-8QPKBLG\Users\Administrator\Desktop\n8n\gpm-release.zip"
if (Test-Path $zipFile) { Remove-Item $zipFile -Force }
Compress-Archive -Path dist\* -DestinationPath $zipFile

Write-Host "Done! Package created at $zipFile"
