#Requires -Version 5.1
<#
.SYNOPSIS
    Compila Balanza POS y genera el instalador con Inno Setup.

.DESCRIPTION
    1. Compila el ejecutable Go (con manifest embebido, sin ventana de consola).
    2. Invoca Inno Setup para generar el instalador en dist\BalanzaPOS-Setup.exe.

.PARAMETER SkipBuild
    Salta la compilacion Go (usa el .exe ya existente).

.PARAMETER SkipInstaller
    Salta la generacion del instalador.

.EXAMPLE
    .\build.ps1
    .\build.ps1 -SkipBuild
#>
param(
    [switch]$SkipBuild,
    [switch]$SkipInstaller
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ProjectDir = $PSScriptRoot
$OutputExe  = Join-Path $ProjectDir "balanza-pos.exe"
$DistDir    = Join-Path $ProjectDir "dist"
$IssScript  = Join-Path $ProjectDir "installer.iss"

# ── 1. Compilacion Go ─────────────────────────────────────────────────────────
if (-not $SkipBuild) {
    Write-Host "► Compilando balanza-pos.exe..." -ForegroundColor Cyan

    $env:GOOS   = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "1"

    $ldflags = "-H windowsgui -s -w"
    & go build -ldflags $ldflags -o $OutputExe .
    if ($LASTEXITCODE -ne 0) { throw "Error al compilar el proyecto Go." }

    $size = (Get-Item $OutputExe).Length / 1MB
    Write-Host "  OK  balanza-pos.exe  ($([math]::Round($size,1)) MB)" -ForegroundColor Green
} else {
    Write-Host "► Compilacion omitida (-SkipBuild)." -ForegroundColor Yellow
    if (-not (Test-Path $OutputExe)) { throw "No existe balanza-pos.exe. Ejecuta sin -SkipBuild primero." }
}

# ── 2. Inno Setup ─────────────────────────────────────────────────────────────
if (-not $SkipInstaller) {
    # Rutas tipicas de instalacion de Inno Setup
    $isccCandidates = @(
        "C:\Program Files (x86)\Inno Setup 6\ISCC.exe",
        "C:\Program Files\Inno Setup 6\ISCC.exe",
        "C:\Program Files (x86)\Inno Setup 5\ISCC.exe",
        "C:\Program Files\Inno Setup 5\ISCC.exe"
    )
    $iscc = $isccCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1

    if (-not $iscc) {
        Write-Host ""
        Write-Warning "Inno Setup no encontrado. Descargalo desde https://jrsoftware.org/isdl.php"
        Write-Warning "Luego ejecuta:  & '<ruta>\ISCC.exe' '$IssScript'"
        exit 0
    }

    if (-not (Test-Path $DistDir)) { New-Item -ItemType Directory -Path $DistDir | Out-Null }

    Write-Host "► Generando instalador con Inno Setup..." -ForegroundColor Cyan
    & $iscc $IssScript
    if ($LASTEXITCODE -ne 0) { throw "Inno Setup termino con error $LASTEXITCODE." }

    $installer = Get-ChildItem $DistDir -Filter "BalanzaPOS-Setup*.exe" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if ($installer) {
        $sizeMB = [math]::Round($installer.Length / 1MB, 1)
        Write-Host "  OK  $($installer.Name)  ($sizeMB MB)" -ForegroundColor Green
        Write-Host "      $($installer.FullName)" -ForegroundColor DarkGray
    }
} else {
    Write-Host "► Instalador omitido (-SkipInstaller)." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Listo." -ForegroundColor Green
