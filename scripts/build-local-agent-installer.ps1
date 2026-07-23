param(
    [string]$Version = "0.1.0",
    [string]$OutputDirectory = "",
    [string]$ISCCPath = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Require-File {
    param([string]$Path, [string]$Description)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Description not found: $Path"
    }
    return (Resolve-Path -LiteralPath $Path).Path
}

function Find-ISCC {
    param([string]$ExplicitPath)
    $candidates = @(
        $ExplicitPath,
        $env:ISCC_PATH,
        (Join-Path ${env:ProgramFiles(x86)} "Inno Setup 6\ISCC.exe"),
        (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe"),
        (Join-Path $repoRoot ".tools\inno\ISCC.exe")
    ) | Where-Object { $_ }
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    $command = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    throw "Inno Setup 6 compiler was not found. Install Inno Setup or pass -ISCCPath."
}

if ($Version -notmatch '^\d+\.\d+\.\d+([.-][0-9A-Za-z.-]+)?$') {
    throw "Version must be a semantic version, for example 0.1.0."
}

$repoRoot = (git rev-parse --show-toplevel).Trim()
Set-Location $repoRoot

if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $repoRoot "storage\client-releases\local-agent\windows-x64"
}
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$stagingRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot ".tmp\local-agent-installer"))
$safeTempRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot ".tmp")) + [IO.Path]::DirectorySeparatorChar
if (-not $stagingRoot.StartsWith($safeTempRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to clean staging directory outside repository temp root: $stagingRoot"
}

$ffmpegPath = if ($env:FFMPEG_PATH) { $env:FFMPEG_PATH } else { Join-Path $repoRoot ".tools\ffmpeg\windows-x64\bin\ffmpeg.exe" }
$ffprobePath = if ($env:FFPROBE_PATH) { $env:FFPROBE_PATH } else { Join-Path $repoRoot ".tools\ffmpeg\windows-x64\bin\ffprobe.exe" }
$ffmpegPath = Require-File $ffmpegPath "FFmpeg"
$ffprobePath = Require-File $ffprobePath "FFprobe"
$ffmpegDistributionRoot = Join-Path $repoRoot ".tools\ffmpeg\ffmpeg-8.1.1-essentials_build"
$ffmpegLicense = Require-File (Join-Path $ffmpegDistributionRoot "LICENSE") "FFmpeg license"
$ffmpegReadme = Require-File (Join-Path $ffmpegDistributionRoot "README.txt") "FFmpeg readme"
$trayIcon = Require-File (Join-Path $repoRoot "apps\local-agent\tray.ico") "Local Agent tray icon"
$iscc = Find-ISCC $ISCCPath

if (Test-Path -LiteralPath $stagingRoot) {
    Remove-Item -LiteralPath $stagingRoot -Recurse -Force
}
$ffmpegStage = Join-Path $stagingRoot "ffmpeg"
$ffmpegBinStage = Join-Path $ffmpegStage "bin"
New-Item -ItemType Directory -Path $ffmpegBinStage -Force | Out-Null
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null

$agentBinary = Join-Path $stagingRoot "local-agent.exe"
$ldflags = "-H=windowsgui -X main.version=$Version -X main.buildMode=installer"
& go build -trimpath -ldflags $ldflags -o $agentBinary .\apps\local-agent
if ($LASTEXITCODE -ne 0) {
    throw "Local Agent build failed with exit code $LASTEXITCODE"
}

Copy-Item -LiteralPath $ffmpegPath -Destination (Join-Path $ffmpegBinStage "ffmpeg.exe")
Copy-Item -LiteralPath $ffprobePath -Destination (Join-Path $ffmpegBinStage "ffprobe.exe")
Copy-Item -LiteralPath $ffmpegLicense -Destination (Join-Path $ffmpegStage "LICENSE")
Copy-Item -LiteralPath $ffmpegReadme -Destination (Join-Path $ffmpegStage "README.txt")
Copy-Item -LiteralPath $trayIcon -Destination (Join-Path $stagingRoot "tray.ico")

$installerScript = Join-Path $repoRoot "deploy\local-agent\installer.iss"
& $iscc "/DMyAppVersion=$Version" "/DSourceDir=$stagingRoot" "/DOutputDir=$OutputDirectory" $installerScript
if ($LASTEXITCODE -ne 0) {
    throw "Inno Setup compilation failed with exit code $LASTEXITCODE"
}

$installerName = "ACRUNU-Fast-Cut-Local-Agent-Setup-x64.exe"
$installerPath = Require-File (Join-Path $OutputDirectory $installerName) "Local Agent installer"
$sha256 = (Get-FileHash -LiteralPath $installerPath -Algorithm SHA256).Hash.ToLowerInvariant()
$manifest = [ordered]@{
    version = $Version
    platform = "windows-x64"
    protocol_version = 1
    sha256 = $sha256
    filename = $installerName
}
$manifestPath = Join-Path $OutputDirectory "release.json"
$manifestJSON = $manifest | ConvertTo-Json
$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($manifestPath, $manifestJSON, $utf8WithoutBom)

Write-Host "Local Agent installer built:" -ForegroundColor Green
Write-Host $installerPath
Write-Host "SHA256: $sha256"
