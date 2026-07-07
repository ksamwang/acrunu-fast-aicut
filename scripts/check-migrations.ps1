$ErrorActionPreference = "Stop"

$migrationFiles = Get-ChildItem -Path "./migrations" -Filter "*.sql" | Sort-Object Name
if ($migrationFiles.Count -eq 0) {
    Write-Error "No migration files found"
}

foreach ($file in $migrationFiles) {
    $content = Get-Content -Raw -Encoding UTF8 $file.FullName
    if ($content -notmatch "-- \+goose Up") {
        Write-Error "$($file.Name) is missing -- +goose Up"
    }
    if ($content -notmatch "-- \+goose Down") {
        Write-Error "$($file.Name) is missing -- +goose Down"
    }
}

Write-Output "Checked $($migrationFiles.Count) migration files"
