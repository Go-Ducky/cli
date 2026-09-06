$ErrorActionPreference = 'Stop'

$Repo = 'Go-Ducky/cli'
$InstallDir = Join-Path $HOME '.goducky\bin'

Write-Host "Uninstalling GoDucky CLI" -ForegroundColor Cyan

$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($UserPath) {
    $clean = ($UserPath.Split(';') | Where-Object { $_ -and $_ -notlike "*$InstallDir*" }) -join ';'
    if ($clean -ne $UserPath) {
        [Environment]::SetEnvironmentVariable('PATH', $clean, 'User')
        $env:PATH = $clean
        Write-Host "Removed $InstallDir from user PATH." -ForegroundColor Yellow
        Write-Host "New terminals pick it up automatically." -ForegroundColor Yellow
    }
}

$Exe = Join-Path $InstallDir 'goducky.exe'
$Cmd = Get-Command goducky -ErrorAction SilentlyContinue
if (-not $Cmd -and -not (Test-Path -LiteralPath $Exe)) {
    Write-Host ""
    Write-Host "GoDucky CLI isn't installed - nothing to do." -ForegroundColor Yellow
    exit 0
}

if (Test-Path -LiteralPath $Exe) {
    Remove-Item -LiteralPath $Exe -Force
    Write-Host "Removed $Exe" -ForegroundColor Green
} else {
    Write-Host "No binary found at $Exe (nothing to remove)." -ForegroundColor Yellow
}
if (Test-Path -LiteralPath $InstallDir) {
    Remove-Item -LiteralPath $InstallDir -Force -ErrorAction SilentlyContinue
}
if ((Test-Path -LiteralPath (Join-Path $HOME '.goducky')) -and -not (Get-ChildItem -LiteralPath (Join-Path $HOME '.goducky'))) {
    Remove-Item -LiteralPath (Join-Path $HOME '.goducky') -Force -ErrorAction SilentlyContinue
}

$ConfigDir = Join-Path $env:APPDATA 'goducky'
if (Test-Path -LiteralPath $ConfigDir) {
    if ([Console]::IsInputRedirected) {
        Write-Host "Tip: to also delete saved chats and config, run this again in a terminal." -ForegroundColor Yellow
    } else {
        $ans = Read-Host "Remove saved chats and config ($ConfigDir)? [y/N]"
        if ($ans -match '^(y|yes)$') {
            Remove-Item -LiteralPath $ConfigDir -Recurse -Force
            Write-Host "Removed $ConfigDir" -ForegroundColor Green
        } else {
            Write-Host "Kept $ConfigDir" -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "GoDucky CLI uninstalled. Reinstall anytime with:" -ForegroundColor Green
Write-Host "  irm https://raw.githubusercontent.com/$Repo/main/scripts/install.ps1 -OutFile `$env:TEMP\goducky-install.ps1; powershell -ExecutionPolicy Bypass -File `$env:TEMP\goducky-install.ps1"