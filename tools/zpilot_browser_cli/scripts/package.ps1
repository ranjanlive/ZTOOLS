[CmdletBinding()]
param([ValidatePattern('^[a-z0-9-]*$')][string]$Variant = '')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$version = (Get-Content -LiteralPath (Join-Path $projectRoot 'extension/manifest.json') -Raw | ConvertFrom-Json).version
$packageName = "zpilot-browser-cli-v$version"
if ($Variant) { $packageName += "-$Variant" }
$releaseDir = Join-Path $projectRoot 'releases'
$archivePath = Join-Path $releaseDir "$packageName.zip"
if (Test-Path -LiteralPath $archivePath) { throw "Archive already exists: $archivePath. Preserve or rename it before packaging again." }
$folders = @('generate_image','generate_music','web_search','webpage_read','zagent_in_chrome','extension','scripts','agent_tools')
$documents = @('README.md','CLI-USAGE.md','RELEASE-NOTES.md','setup.ps1','setup.cmd','setup.sh','SHA256SUMS','.gitignore')
$files = @()
foreach ($folder in $folders) {
    $files += Get-ChildItem -LiteralPath (Join-Path $projectRoot $folder) -File -Recurse
}
foreach ($tool in $folders[0..4]) {
    foreach ($config in Get-ChildItem -LiteralPath (Join-Path $projectRoot $tool) -Filter config.json -File -Recurse) {
        $settings = Get-Content -LiteralPath $config.FullName -Raw | ConvertFrom-Json
        if ($settings.extension_id) { throw "Release config contains a Connection ID: $($config.FullName). Clear it before sharing." }
    }
}
& (Join-Path $PSScriptRoot 'checksums.ps1')
foreach ($document in $documents) {
    $files += Get-Item -LiteralPath (Join-Path $projectRoot $document) -Force
}
New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
Add-Type -AssemblyName System.IO.Compression
$stream = [System.IO.File]::Open($archivePath, [System.IO.FileMode]::CreateNew)
try {
    $archive = [System.IO.Compression.ZipArchive]::new($stream, [System.IO.Compression.ZipArchiveMode]::Create, $true)
    try {
        foreach ($file in $files | Sort-Object FullName) {
            $relative = [System.IO.Path]::GetRelativePath($projectRoot, $file.FullName).Replace('\','/')
            $entry = $archive.CreateEntry("$packageName/$relative", [System.IO.Compression.CompressionLevel]::Optimal)
            $entry.LastWriteTime = $file.LastWriteTime
            $entryStream = $entry.Open()
            $inputStream = [System.IO.File]::OpenRead($file.FullName)
            try { $inputStream.CopyTo($entryStream) } finally { $inputStream.Dispose(); $entryStream.Dispose() }
        }
    } finally { $archive.Dispose() }
} finally { $stream.Dispose() }
$hash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllText("$archivePath.sha256", "$hash  $packageName.zip`n", [System.Text.UTF8Encoding]::new($false))
Write-Output "Created $archivePath"
Write-Output "Files: $($files.Count)"
Write-Output "SHA256: $hash"
