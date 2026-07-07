$ErrorActionPreference = "Stop"

function Test-Command {
    param([string]$Name)

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) {
        Write-Error "$Name is not available on PATH"
    }

    & $Name -version | Select-Object -First 1
}

Test-Command "ffmpeg"
Test-Command "ffprobe"
