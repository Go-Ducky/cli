$ErrorActionPreference = 'Stop'

$Repo = 'Go-Ducky/cli'
$InstallDir = Join-Path $HOME '.goducky\bin'
$Version = 'latest'

$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -eq 'AMD64') { $Asset = 'goducky-windows-amd64.exe' }
elseif ($Arch -eq 'ARM64') { $Asset = 'goducky-windows-arm64.exe' }
else { throw "Unsupported architecture: $Arch" }

Write-Host "Installing GoDucky CLI" -ForegroundColor Cyan

if ($Version -eq 'latest') {
    $Releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1"
    $Tag = $Releases[0].tag_name
    $DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/$Asset"
    $LatestVer = $Releases[0].name -replace '^GoDucky\s+', ''
    Write-Host "Latest release: $Tag" -ForegroundColor Green
} else {
    $Version = $Version.TrimStart('v')
    $DownloadUrl = "https://github.com/$Repo/releases/download/v$Version/$Asset"
    $LatestVer = $Version
}

$Existing = $null
$Cmd = Get-Command goducky -ErrorAction SilentlyContinue
if ($Cmd) {
    $Existing = $Cmd.Source
} elseif (Test-Path -LiteralPath $InstallDir) {
    $ExePath = Join-Path $InstallDir 'goducky.exe'
    if (Test-Path -LiteralPath $ExePath) { $Existing = $ExePath }
}
$CurVer = ''
if ($Existing) {
    $CurVer = (& $Existing --version 2>$null | Select-Object -First 1) -replace '^goducky\s+', ''
}
if ($CurVer -and $CurVer -eq $LatestVer) {
    Write-Host "GoDucky $CurVer is already installed - you're good to go!" -ForegroundColor Green
    exit 0
} elseif ($CurVer) {
    Write-Host "You have GoDucky $CurVer, but the latest is $LatestVer." -ForegroundColor Yellow
    Write-Host "Update with: goducky update   (re-running this script also updates)" -ForegroundColor Yellow
}

Write-Host "Downloading $Asset v$Version ..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$TempFile = Join-Path $env:TEMP $Asset
Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempFile -UseBasicParsing

Move-Item -Force $TempFile (Join-Path $InstallDir 'goducky.exe')

$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($UserPath -notlike "*$InstallDir*") {
    $NewPath = $InstallDir.TrimEnd('\') + ';' + $UserPath
    [Environment]::SetEnvironmentVariable('PATH', $NewPath, 'User')
    $env:PATH = "$InstallDir;$env:PATH"
    Write-Host "Added $InstallDir to user PATH." -ForegroundColor Yellow
    Write-Host "New terminals pick it up automatically; this terminal works right now." -ForegroundColor Yellow
}

$Installed = Join-Path $InstallDir 'goducky.exe'
Write-Host "Verifying install..." -ForegroundColor Cyan
& $Installed --version

Write-Host ""
Write-Host "GoDucky CLI installed successfully!" -ForegroundColor Green
Write-Host "Run 'goducky' to start. (Restart your terminal if 'goducky' isn't recognized.)"
