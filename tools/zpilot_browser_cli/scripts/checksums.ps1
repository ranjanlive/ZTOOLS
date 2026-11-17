$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$entries = Get-ChildItem -LiteralPath $projectRoot -File -Recurse | Where-Object {
    $relative = [System.IO.Path]::GetRelativePath($projectRoot, $_.FullName).Replace('\','/')
    $relative -ne 'SHA256SUMS' -and $relative -notmatch '(^|/)(\.git|\.test-results|releases)/'
} | ForEach-Object {
    $relative = [System.IO.Path]::GetRelativePath($projectRoot, $_.FullName).Replace('\','/')
    $hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $relative"
} | Sort-Object
[System.IO.File]::WriteAllLines((Join-Path $projectRoot 'SHA256SUMS'), $entries, [System.Text.UTF8Encoding]::new($false))
Write-Output "Updated SHA256SUMS ($($entries.Count) files)"
