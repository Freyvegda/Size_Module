# Convenience wrapper for the project-local Graphify CLI.
#
# Graphify is installed in .venv (see AGENTS.md); it is not on PATH, so use:
#   .\graphify.ps1 query "how does the beam solver work"
#   .\graphify.ps1 update .
#   .\graphify.ps1 explain "Solver"
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$GraphifyArgs
)

& "$PSScriptRoot\.venv\Scripts\graphify.exe" @GraphifyArgs
exit $LASTEXITCODE
