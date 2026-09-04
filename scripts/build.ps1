[CmdletBinding()]
param(
    [string]$Version = "0.1.0"
)
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$targets = @(
    @{ OS = "windows"; ARCH = "amd64"; EXT = ".exe" }
    @{ OS = "windows"; ARCH = "arm64"; EXT = ".exe" }
    @{ OS = "darwin";  ARCH = "amd64"; EXT = "" }
    @{ OS = "darwin";  ARCH = "arm64"; EXT = "" }
    @{ OS = "linux";   ARCH = "amd64"; EXT = "" }
    @{ OS = "linux";   ARCH = "arm64"; EXT = "" }
)

foreach ($t in $targets) {
    $name = "goducky-$($t.OS)-$($t.ARCH)$($t.EXT)"
    $out = Join-Path $dist $name
    $env:GOOS = $t.OS
    $env:GOARCH = $t.ARCH
    $env:CGO_ENABLED = "0"
    Write-Host "Building $name ..." -ForegroundColor Cyan
    & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $out ./cmd/goducky
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Build failed for $name - the target may be locked by Windows Defender. Close any running copy and try again."
        continue
    }
    Write-Host "  OK" -ForegroundColor Green
}

Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
Write-Host ""
Write-Host "Binaries written to $dist" -ForegroundColor Green