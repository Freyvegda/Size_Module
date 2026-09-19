# Starts the local PostgreSQL (and Adminer) containers.
# Usage: pwsh -File db/scripts/up.ps1
. "$PSScriptRoot\common.ps1"

$compose = Get-ComposeFile
Write-Host "Starting containers from $compose ..."
& docker compose -f $compose up -d --wait
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }

Write-Host ""
Write-Host "PostgreSQL is ready:"
Write-Host "  host      localhost:5432"
Write-Host "  user      cutoptics"
Write-Host "  password  cutoptics"
Write-Host "  database  cutoptics"
Write-Host "  adminer   http://localhost:8081"
Write-Host ""
Write-Host "Next: db/scripts/migrate.ps1 then db/scripts/seed.ps1"
