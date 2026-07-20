param(
    [string]$HostName = "10.168.10.23",
    [string]$UserName = "acrunu",
    [string]$RemoteDir = "/home/acrunu/acrunu-fast-aicut",
    [string]$ReleaseDirectory = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

foreach ($commandName in @("git", "ssh", "scp")) {
    if (-not (Get-Command $commandName -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $commandName"
    }
}

$repoRoot = (git rev-parse --show-toplevel).Trim()
if (-not $ReleaseDirectory) {
    $ReleaseDirectory = Join-Path $repoRoot "storage\client-releases\local-agent\windows-x64"
}
$ReleaseDirectory = (Resolve-Path -LiteralPath $ReleaseDirectory).Path
$manifestPath = Join-Path $ReleaseDirectory "release.json"
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    throw "Release manifest not found. Build the installer first: $manifestPath"
}
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace($manifest.filename) -or $manifest.filename -notmatch '^[A-Za-z0-9._-]+\.exe$') {
    throw "Release manifest contains an invalid installer filename."
}
$installerPath = Join-Path $ReleaseDirectory $manifest.filename
if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
    throw "Installer not found: $installerPath"
}

$target = "$UserName@$HostName"
$remoteReleaseDir = "$RemoteDir/storage/client-releases/local-agent/windows-x64"
$uploadID = [Guid]::NewGuid().ToString("N")
$remoteInstallerTemp = "/tmp/local-agent-$uploadID.exe"
$remoteManifestTemp = "/tmp/local-agent-$uploadID.json"

& scp $installerPath "${target}:$remoteInstallerTemp"
if ($LASTEXITCODE -ne 0) { throw "Installer upload failed." }
& scp $manifestPath "${target}:$remoteManifestTemp"
if ($LASTEXITCODE -ne 0) { throw "Manifest upload failed." }

$remoteCommand = "set -e; mkdir -p '$remoteReleaseDir'; mv '$remoteInstallerTemp' '$remoteReleaseDir/$($manifest.filename)'; mv '$remoteManifestTemp' '$remoteReleaseDir/release.json'; chmod 644 '$remoteReleaseDir/$($manifest.filename)' '$remoteReleaseDir/release.json'"
& ssh $target $remoteCommand
if ($LASTEXITCODE -ne 0) { throw "Publishing release on the server failed." }

Write-Host "Published Local Agent $($manifest.version) to $target" -ForegroundColor Green
