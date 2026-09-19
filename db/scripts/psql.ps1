# Opens a psql shell, or runs one SQL file against the database.
# Usage:
#   pwsh -File db/scripts/psql.ps1                       # interactive shell
#   pwsh -File db/scripts/psql.ps1 -File db/tests/smoke.sql
param([string]$File = "")

. "$PSScriptRoot\common.ps1"

if ($File) {
    if (-not (Test-Path $File)) { throw "SQL file not found: $File" }
    Invoke-PsqlFile -Path (Resolve-Path $File).Path
    exit 0
}

Assert-DockerUp
$compose = Get-ComposeFile
& docker compose -f $compose exec postgres psql -U (Get-DbUser) -d (Get-DbName)
