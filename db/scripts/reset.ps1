# Drops the whole schema, re-applies migrations and seeds.
# Usage: pwsh -File db/scripts/reset.ps1 [-Force]
param([switch]$Force)

. "$PSScriptRoot\common.ps1"

if (-not $Force) {
    $answer = Read-Host "This deletes all data in the database. Type 'yes' to continue"
    if ($answer -ne "yes") {
        Write-Host "Cancelled."
        exit 0
    }
}

Invoke-PsqlCommand -Sql "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
Write-Host "Schema dropped. Re-applying migrations..."
& "$PSScriptRoot\migrate.ps1"
Write-Host "Seeding..."
& "$PSScriptRoot\seed.ps1"
Write-Host "Database reset complete."
