# Runs the SQL test suite in db/tests.
# Usage: pwsh -File db/scripts/test.ps1
. "$PSScriptRoot\common.ps1"

$tests = Get-ChildItem (Get-TestsDir) -Filter "*.sql" | Sort-Object Name
if (-not $tests) {
    Write-Host "No test files found in $(Get-TestsDir)"
    exit 0
}
foreach ($test in $tests) {
    Write-Host "Running $($test.Name) ..."
    Invoke-PsqlFile -Path $test.FullName
}
Write-Host "All database tests passed."
