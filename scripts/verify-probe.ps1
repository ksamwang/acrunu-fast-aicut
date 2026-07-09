param(
    [Parameter(Mandatory = $true)]
    [string]$InputPath
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $InputPath)) {
    throw "Input file not found: $InputPath"
}

$ffprobe = if ($env:FFPROBE_PATH) { $env:FFPROBE_PATH } else { "ffprobe" }

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
