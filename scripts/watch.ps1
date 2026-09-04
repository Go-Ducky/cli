<#
.SYNOPSIS
Watches the Go source and rebuilds automatically whenever a .go file, go.mod,
or go.sum changes. Rebuilds the local goducky.exe AND all platform binaries in
dist/ (via build.ps1). Run from a terminal in the repo root:

    powershell -ExecutionPolicy Bypass -File scripts\watch.ps1

Press Ctrl+C to stop.
#>
[CmdletBinding()]
param(
    [string]$Output = "goducky.exe",
    [switch]$SkipDist
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$OutputPath = Join-Path $Root $Output
$PollMs = 700
$SettleMs = 400

function Invoke-Build {
    Write-Host "[watch] rebuilding goducky..." -ForegroundColor DarkGray
    Push-Location $Root
    try {
        & go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o $OutputPath ./cmd/goducky
        if ($LASTEXITCODE -eq 0) {
            Write-Host "[watch] built $Output" -ForegroundColor Green
        } else {
            Write-Host "[watch] build failed" -ForegroundColor Red
        }
    } finally {
        Pop-Location
    }
    if (-not $SkipDist) {
        Write-Host "[watch] rebuilding dist/ for all platforms..." -ForegroundColor DarkGray
        & (Join-Path $PSScriptRoot "build.ps1")
        Write-Host "[watch] dist/ done" -ForegroundColor Green
    }
}

# Signature of every tracked source file (the .go files plus go.mod/go.sum).
# The output binary is never included, so building can't re-trigger a build.
function Get-TreeState {
    $items = @()
    $files = @()
    $files += Get-ChildItem -LiteralPath (Join-Path $Root "cmd") -Recurse -File -Filter "*.go" -ErrorAction SilentlyContinue
    $files += Get-ChildItem -LiteralPath (Join-Path $Root "internal") -Recurse -File -Filter "*.go" -ErrorAction SilentlyContinue
    foreach ($extra in @("go.mod", "go.sum")) {
        $p = Join-Path $Root $extra
        if (Test-Path -LiteralPath $p) {
            $files += Get-Item -LiteralPath $p
        }
    }
    foreach ($f in $files) {
        $sig = (Get-FileHash -LiteralPath $f.FullName -Algorithm SHA256).Hash
        $items += ($f.FullName.Substring($Root.Length) + "=" + $sig)
    }
    return ($items -join "|")
}

Write-Host ("[watch] watching {0}" -f $Root) -ForegroundColor Cyan
Write-Host "[watch] rebuilding on every save. Press Ctrl+C to stop." -ForegroundColor Cyan
Invoke-Build

$last = Get-TreeState
try {
    while ($true) {
        Start-Sleep -Milliseconds $PollMs
        $now = Get-TreeState
        if ($now -ne $last) {
            $last = $now
            Start-Sleep -Milliseconds $SettleMs
            Invoke-Build
            $last = Get-TreeState
        }
    }
} finally {
    Write-Host ""
    Write-Host "[watch] stopped" -ForegroundColor Cyan
}