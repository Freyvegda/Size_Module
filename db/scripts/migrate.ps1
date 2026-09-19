# Applies goose migrations.
# Usage:
#   pwsh -File db/scripts/migrate.ps1            # apply all pending migrations
#   pwsh -File db/scripts/migrate.ps1 -Status    # show migration status
#   pwsh -File db/scripts/migrate.ps1 -Down      # roll back one migration
#   pwsh -File db/scripts/migrate.ps1 -To 1      # migrate up to version 1
param(
    [string]$To = "",
    [switch]$Down,
    [switch]$Status
)

. "$PSScriptRoot\common.ps1"
Initialize-ToolPath
Require-Tool "goose" "Install it with: go install github.com/pressly/goose/v3/cmd/goose@latest"

$dir = Get-MigrationsDir
$dbUrl = Get-DbUrl

if ($Status) {
    & goose -dir $dir postgres $dbUrl status
    exit $LASTEXITCODE
}
if ($Down) {
    & goose -dir $dir postgres $dbUrl down
    exit $LASTEXITCODE
}
if ($To) {
    & goose -dir $dir postgres $dbUrl up-to $To
    exit $LASTEXITCODE
}
& goose -dir $dir postgres $dbUrl up
if ($LASTEXITCODE -ne 0) { throw "migration failed" }
