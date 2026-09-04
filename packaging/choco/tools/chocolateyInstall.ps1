$ErrorActionPreference = 'Stop'

$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition

$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -eq 'ARM64') {
    $asset = 'goducky-windows-arm64.exe'
} else {
    $asset = 'goducky-windows-amd64.exe'
}

$url = "https://github.com/Go-Ducky/cli/releases/latest/download/$asset"

# Download the release binary
$exe = Join-Path $toolsDir 'goducky.exe'
Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing

# Place it on PATH via shim
Install-BinFile -Name 'goducky' -Path $exe
