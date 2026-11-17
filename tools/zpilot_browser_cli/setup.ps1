[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "ZPilotBrowserCLI"),
    [string]$ExtensionID = "",
    [string]$Profile = "",
    [string]$BrowserPath = "",
    [switch]$NoPath
)
$ErrorActionPreference = "Stop"
$packageRoot = $PSScriptRoot
if ($ExtensionID -and $ExtensionID -cnotmatch '^[a-p]{32}\.[a-f0-9]{64}$') {
    throw 'ExtensionID must be the full Connection ID copied from the extension popup.'
}
$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
if ($architecture -notin @("amd64", "x64", "arm64")) { throw "Unsupported Windows architecture: $architecture" }
if ($architecture -eq "x64") { $architecture = "amd64" }
$platform = "windows-$architecture"
$InstallDir = [System.IO.Path]::GetFullPath($InstallDir)
if ($InstallDir.TrimEnd('\') -eq $packageRoot.TrimEnd('\')) { throw "Choose an installation directory separate from the release package" }
$toolNames = @("generate_image", "generate_music", "web_search", "webpage_read", "zagent_in_chrome")
# Validate the package before installing anything.
foreach ($toolName in $toolNames) {
    $binary = Join-Path $packageRoot "$toolName/binaries/$platform/$toolName.exe"
    if (!(Test-Path -LiteralPath $binary -PathType Leaf)) { throw "Missing packaged binary: $binary" }
}
if (!(Test-Path -LiteralPath (Join-Path $packageRoot "extension/manifest.json"))) { throw "Packaged extension is missing" }
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
$binDir = Join-Path $InstallDir "bin"
New-Item -ItemType Directory -Path $binDir -Force | Out-Null
foreach ($toolName in $toolNames) {
    $target = Join-Path $InstallDir "tools/$toolName"
    New-Item -ItemType Directory -Path $target -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $packageRoot "$toolName/binaries/$platform/$toolName.exe") -Destination (Join-Path $target "$toolName.exe") -Force
    Copy-Item -LiteralPath (Join-Path $packageRoot "$toolName/binaries/$platform/$toolName.json") -Destination (Join-Path $target "$toolName.json") -Force
    $config = Join-Path $target "config.json"
    if (!(Test-Path -LiteralPath $config)) {
        Copy-Item -LiteralPath (Join-Path $packageRoot "$toolName/config.json") -Destination $config
    }
    if ($ExtensionID -or $Profile -or $BrowserPath) {
        $settings = Get-Content -LiteralPath $config -Raw | ConvertFrom-Json
        if ($ExtensionID) {
            $settings | Add-Member -NotePropertyName extension_id -NotePropertyValue $ExtensionID -Force
            $settings.PSObject.Properties.Remove('socket')
        }
        if ($Profile) {
            $settings.PSObject.Properties.Remove('chrome_profile')
            $settings | Add-Member -NotePropertyName profile -NotePropertyValue $Profile -Force
        }
        if ($BrowserPath) {
            $settings.PSObject.Properties.Remove('browser_path')
            $settings | Add-Member -NotePropertyName path -NotePropertyValue $BrowserPath -Force
        }
        [System.IO.File]::WriteAllText($config, (($settings | ConvertTo-Json -Depth 10) + "`n"), [System.Text.UTF8Encoding]::new($false))
    }
    # Relative launchers preserve an independent config beside each real executable.
    $launcher = "@echo off`r`n`"%~dp0..\tools\$toolName\$toolName.exe`" %*`r`nexit /b %errorlevel%`r`n"
    [System.IO.File]::WriteAllText((Join-Path $binDir "$toolName.cmd"), $launcher, [System.Text.Encoding]::ASCII)
}
$extensionDir = Join-Path $InstallDir "extension"
New-Item -ItemType Directory -Path $extensionDir -Force | Out-Null
Get-ChildItem -LiteralPath (Join-Path $packageRoot "extension") | Copy-Item -Destination $extensionDir -Recurse -Force
if (!$NoPath) {
    $oldPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @($oldPath -split ';' | Where-Object { $_ -and $_.TrimEnd('\') -ine $binDir.TrimEnd('\') })
    [Environment]::SetEnvironmentVariable("Path", (($binDir + ';' + ($parts -join ';')).TrimEnd(';')), "User")
    $env:PATH = $binDir + ';' + $env:PATH
}
Write-Output "Installed $platform tools in $InstallDir"
Write-Output "Configs: $InstallDir\tools\<tool>\config.json (existing configs preserved)"
Write-Output "Load unpacked extension from: $extensionDir"
Write-Output "Open chrome://extensions, enable Developer mode, select Load unpacked, and choose that folder. Reload if already installed."
Write-Output "Open the extension popup, copy its Connection ID, and paste it into extension_id in each tool config."
Write-Output "Try: web_search --query 'Go tutorials'"
Write-Output "No native host, browser registration, or separate service is needed."
Write-Output "PATH directory: $binDir"
Write-Output "If an older command still resolves first, inspect Get-Command generate_image -All and remove or reorder the older PATH entry."
