param(
    [Parameter(Mandatory = $true)]
    [string]$InputPath
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $InputPath)) {
    throw "Input file not found: $InputPath"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$bundledFfprobe = Join-Path $repoRoot ".tools\\ffmpeg\\windows-x64\\bin\\ffprobe.exe"
$ffprobe = if ($env:FFPROBE_PATH) {
    $env:FFPROBE_PATH
} elseif (Test-Path -LiteralPath $bundledFfprobe) {
    $bundledFfprobe
} else {
    "ffprobe"
}

Write-Host "Using ffprobe:" $ffprobe
Write-Host "Input:" $InputPath

& $ffprobe `
  -v error `
  -print_format json `
  -show_format `
  -show_streams `
  $InputPath

if ($LASTEXITCODE -ne 0) {
    throw "ffprobe failed with exit code $LASTEXITCODE"
}
