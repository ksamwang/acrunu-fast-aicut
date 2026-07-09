param(
    [string]$HostName = "10.168.10.23",
    [string]$UserName = "acrunu",
    [string]$RemoteDir = "/home/acrunu/acrunu-fast-aicut",
    [string[]]$Services = @("api", "worker"),
    [switch]$AllowDirty,
    [switch]$RunMigrations,
    [string]$DatabaseUrl = "postgres://aicut:aicut@localhost:5432/aicut?sslmode=disable"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Require-Command {
    param([string]$Name)

    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $Name"
    }
}

Require-Command git
Require-Command ssh
Require-Command scp

$repoRoot = (git rev-parse --show-toplevel).Trim()
Set-Location $repoRoot

$branch = (git branch --show-current).Trim()
$commit = (git rev-parse --short HEAD).Trim()
$dirty = git status --porcelain

if ($dirty -and -not $AllowDirty) {
    Write-Host "Working tree has uncommitted changes. This script deploys committed HEAD only." -ForegroundColor Yellow
    Write-Host "Commit your changes first, or pass -AllowDirty if you intentionally want to deploy current HEAD." -ForegroundColor Yellow
    Write-Host ""
    $dirty
    exit 1
}

$timestamp = Get-Date -Format "yyyyMMddHHmmss"
$archiveName = "aicut-$commit-$timestamp.tar"
$archivePath = Join-Path $env:TEMP $archiveName
$remoteArchivePath = "/tmp/$archiveName"
$target = "$UserName@$HostName"
$serviceArgs = ($Services -join " ")

Write-Host "Deploying $branch@$commit to ${target}:$RemoteDir" -ForegroundColor Cyan
Write-Host "Creating archive from committed HEAD..."
git archive --format=tar -o $archivePath HEAD

try {
    Write-Host "Uploading archive..."
    scp $archivePath "${target}:$remoteArchivePath"

    $remoteCommands = @(
        "set -e",
        "mkdir -p '$RemoteDir'",
        "tar -xf '$remoteArchivePath' -C '$RemoteDir'",
        "cd '$RemoteDir'",
        "docker compose up -d --build $serviceArgs",
        "docker compose ps $serviceArgs",
        "rm -f '$remoteArchivePath'"
    )

    if ($RunMigrations) {
        $migrationCommand = "goose -dir ./migrations postgres '$DatabaseUrl' up"
        $remoteCommands = @(
            "set -e",
            "mkdir -p '$RemoteDir'",
            "tar -xf '$remoteArchivePath' -C '$RemoteDir'",
            "cd '$RemoteDir'",
            $migrationCommand,
            "docker compose up -d --build $serviceArgs",
            "docker compose ps $serviceArgs",
            "rm -f '$remoteArchivePath'"
        )
    }

    Write-Host "Rebuilding services on server..."
    ssh $target ($remoteCommands -join "; ")

    Write-Host "Deployment finished." -ForegroundColor Green
}
finally {
    if (Test-Path $archivePath) {
        Remove-Item -LiteralPath $archivePath -Force
    }
}
