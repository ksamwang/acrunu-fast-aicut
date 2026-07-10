param(
    [string]$Version = "8.1.1",
    [string]$Platform = "windows-x64"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$toolsRoot = Join-Path $repoRoot ".tools"
$downloadsDir = Join-Path $toolsRoot "downloads"
$extractRoot = Join-Path $toolsRoot "ffmpeg"
$platformRoot = Join-Path $extractRoot $Platform
$platformBinDir = Join-Path $platformRoot "bin"
$archivePath = Join-Path $toolsRoot "ffmpeg-$Version-essentials_build.zip"
$downloadUrl = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-$Version-essentials_build.zip"

New-Item -ItemType Directory -Force -Path $downloadsDir | Out-Null
New-Item -ItemType Directory -Force -Path $extractRoot | Out-Null
New-Item -ItemType Directory -Force -Path $platformBinDir | Out-Null

if (-not (Test-Path -LiteralPath $archivePath)) {
    $archivePath = Join-Path $downloadsDir "ffmpeg-$Version-essentials_build.zip"
}

if (-not (Test-Path -LiteralPath $archivePath)) {
    Write-Host "Downloading ffmpeg $Version ..."
    Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath
} else {
    Write-Host "Using existing archive:" $archivePath
}

$expandedDir = Join-Path $extractRoot "ffmpeg-$Version-essentials_build"
if (-not (Test-Path -LiteralPath $expandedDir)) {
    Write-Host "Extracting archive ..."
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractRoot -Force
} else {
    Write-Host "Using existing extracted directory:" $expandedDir
}

$sourceBinDir = Join-Path $expandedDir "bin"
foreach ($fileName in @("ffmpeg.exe", "ffprobe.exe", "ffplay.exe")) {
    $sourceFile = Join-Path $sourceBinDir $fileName
    if (Test-Path -LiteralPath $sourceFile) {
        Copy-Item -LiteralPath $sourceFile -Destination (Join-Path $platformBinDir $fileName) -Force
    }
}

Write-Host "Bundled ffmpeg binaries prepared at:" $platformBinDir
Write-Host "FFMPEG_PATH =" (Join-Path $platformBinDir "ffmpeg.exe")
Write-Host "FFPROBE_PATH =" (Join-Path $platformBinDir "ffprobe.exe")
