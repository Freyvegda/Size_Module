# Applies every file in db/seeds in name order.
# Usage: pwsh -File db/scripts/seed.ps1
. "$PSScriptRoot\common.ps1"

$seeds = Get-ChildItem (Get-SeedsDir) -Filter "*.sql" | Sort-Object Name
if (-not $seeds) {
    Write-Host "No seed files found in $(Get-SeedsDir)"
    exit 0
}
foreach ($seed in $seeds) {
    Write-Host "Applying seed $($seed.Name) ..."
    Invoke-PsqlFile -Path $seed.FullName
}
Write-Host "Seeds applied."
