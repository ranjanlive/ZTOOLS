[CmdletBinding()]
param([switch]$CurrentPlatformOnly)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$toolNames = @('web_search','webpage_read','generate_image','generate_music','zagent_in_chrome')
$canonical = Join-Path $projectRoot 'web_search/source_code/internal/browsercli'
# Source releases stay independent; maintain shared code once and copy at release time.
foreach ($toolName in $toolNames) {
    if ($toolName -ne 'web_search') {
        Copy-Item -Path (Join-Path $canonical '*.go') -Destination (Join-Path $projectRoot "$toolName/source_code/internal/browsercli") -Force
    }
}
$targets = @('windows-amd64','windows-arm64','linux-amd64','linux-arm64','darwin-amd64','darwin-arm64')
if ($CurrentPlatformOnly) {
    $targets = @((& go env GOOS) + '-' + (& go env GOARCH))
}
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    foreach ($target in $targets) {
        $env:GOOS, $env:GOARCH = $target.Split('-')
        foreach ($toolName in $toolNames) {
            $destination = Join-Path $projectRoot "$toolName/binaries/$target"
            New-Item -ItemType Directory -Path $destination -Force | Out-Null
            $executable = $toolName
            if ($env:GOOS -eq 'windows') { $executable += '.exe' }
            Push-Location (Join-Path $projectRoot "$toolName/source_code")
            try {
                & go build -trimpath -ldflags '-s -w' -o (Join-Path $destination $executable) .
                if ($LASTEXITCODE -ne 0) { throw "Build failed: $toolName ($target)" }
            } finally { Pop-Location }
            Copy-Item -LiteralPath (Join-Path $projectRoot "$toolName/config.json") -Destination (Join-Path $destination 'config.json') -Force
            Copy-Item -LiteralPath (Join-Path $projectRoot "agent_tools/$toolName/$toolName.json") -Destination (Join-Path $destination "$toolName.json") -Force
        }
        Write-Output "Built $target"
    }
} finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
}
& (Join-Path $PSScriptRoot 'checksums.ps1')
