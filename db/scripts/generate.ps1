# Regenerates Go code from db/queries into backend/internal/platform/db.
# Usage: pwsh -File db/scripts/generate.ps1
. "$PSScriptRoot\common.ps1"
Initialize-ToolPath
Require-Tool "sqlc" "Install it with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"

Push-Location (Get-DbDir)
try {
    & sqlc generate
    if ($LASTEXITCODE -ne 0) { throw "sqlc generate failed" }
} finally {
    Pop-Location
}
Write-Host "Generated backend/internal/platform/db"
