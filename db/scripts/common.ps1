# Shared helpers for the scripts in db/scripts.
# Dot-source this file; do not run it directly.

Set-StrictMode -Version Latest
# NOTE: intentionally not "Stop". Native tools (docker, goose, psql, sqlc)
# write progress and warnings to stderr, and PowerShell 5.1 turns those into
# terminating errors under the Stop preference. Every call site checks
# $LASTEXITCODE explicitly instead.
$ErrorActionPreference = "Continue"

# Make the Go toolchain and tools installed with `go install` available even in
# shells that were opened before Go was installed.
function Initialize-ToolPath {
    $candidates = @(
        (Join-Path $env:USERPROFILE "go\bin"),
        "C:\Program Files\Go\bin"
    )
    foreach ($dir in $candidates) {
        if ((Test-Path $dir) -and ($env:Path -notlike "*$dir*")) {
            $env:Path = "$dir;$env:Path"
        }
    }
}

function Get-DbDir { return (Split-Path $PSScriptRoot -Parent) }
function Get-RepoRoot { return (Split-Path (Split-Path $PSScriptRoot -Parent) -Parent) }
function Get-MigrationsDir { return (Join-Path (Get-DbDir) "migrations") }
function Get-QueriesDir { return (Join-Path (Get-DbDir) "queries") }
function Get-SeedsDir { return (Join-Path (Get-DbDir) "seeds") }
function Get-TestsDir { return (Join-Path (Get-DbDir) "tests") }
function Get-ComposeFile { return (Join-Path (Get-RepoRoot) "deploy\compose\docker-compose.yml") }

function Require-Tool {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string]$Hint = ""
    )
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        $message = "Required tool '$Name' was not found on PATH."
        if ($Hint) { $message += " $Hint" }
        throw $message
    }
}

# Reads db/.env if present. Falls back to the compose defaults so the scripts
# work out of the box.
function Get-DbUrl {
    $envFile = Join-Path (Get-DbDir) ".env"
    if (Test-Path $envFile) {
        Get-Content $envFile | ForEach-Object {
            if ($_ -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$') {
                $name = $Matches[1]
                $value = $Matches[2].Trim().Trim('"')
                Set-Item -Path "Env:$name" -Value $value
            }
        }
    }
    if (-not $env:DATABASE_URL) {
        return "postgres://cutoptics:cutoptics@localhost:5432/cutoptics?sslmode=disable"
    }
    return $env:DATABASE_URL
}

function Get-DbUser {
    if ($env:POSTGRES_USER) { return $env:POSTGRES_USER }
    return "cutoptics"
}

function Get-DbName {
    if ($env:POSTGRES_DB) { return $env:POSTGRES_DB }
    return "cutoptics"
}

function Assert-DockerUp {
    $compose = Get-ComposeFile
    if (-not (Test-Path $compose)) { throw "compose file not found: $compose" }
    $running = & docker compose -f $compose ps --status running --quiet postgres
    if ($LASTEXITCODE -ne 0 -or -not $running) {
        throw "The postgres container is not running. Start it with: db/scripts/up.ps1"
    }
}

# Runs one SQL statement inside the postgres container.
function Invoke-PsqlCommand {
    param([Parameter(Mandatory = $true)][string]$Sql)
    Assert-DockerUp
    $compose = Get-ComposeFile
    & docker compose -f $compose exec -T postgres `
        psql -U (Get-DbUser) -d (Get-DbName) -v ON_ERROR_STOP=1 -q -c $Sql
    if ($LASTEXITCODE -ne 0) { throw "psql command failed" }
}

# Pipes a host .sql file into psql inside the container.
function Invoke-PsqlFile {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path $Path)) { throw "SQL file not found: $Path" }
    Assert-DockerUp
    $compose = Get-ComposeFile
    Get-Content -Raw -Path $Path | & docker compose -f $compose exec -T postgres `
        psql -U (Get-DbUser) -d (Get-DbName) -v ON_ERROR_STOP=1
    if ($LASTEXITCODE -ne 0) { throw "psql failed on $Path" }
}
