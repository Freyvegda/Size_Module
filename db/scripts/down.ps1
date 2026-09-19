# Stops the local containers.
# Usage: pwsh -File db/scripts/down.ps1 [-Volumes]
param([switch]$Volumes)

. "$PSScriptRoot\common.ps1"

$compose = Get-ComposeFile
$dockerArgs = @("compose", "-f", $compose, "down")
if ($Volumes) { $dockerArgs += "-v" }
& docker @dockerArgs
if ($LASTEXITCODE -ne 0) { throw "docker compose down failed" }
Write-Host "Containers stopped."
