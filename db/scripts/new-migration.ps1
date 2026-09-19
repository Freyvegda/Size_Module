# Creates a new timestamped migration pair (up + down).
# Usage: pwsh -File db/scripts/new-migration.ps1 -Name add_remnants
param([Parameter(Mandatory = $true)][string]$Name)

. "$PSScriptRoot\common.ps1"
Initialize-ToolPath
Require-Tool "goose" "Install it with: go install github.com/pressly/goose/v3/cmd/goose@latest"

$dir = Get-MigrationsDir
& goose -dir $dir create $Name sql
if ($LASTEXITCODE -ne 0) { throw "could not create migration" }

Get-ChildItem $dir | Sort-Object Name | Select-Object -Last 2 | ForEach-Object {
    Write-Host "created $($_.FullName)"
}
Write-Host "Write the forward change in the .up.sql file, then run db/scripts/migrate.ps1"
