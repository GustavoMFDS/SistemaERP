#requires -Version 5.1
<#
  Read-only Windows host suitability gate for a technical Esterium bundle.
  It does not install or start PostgreSQL, Redis, services, certificates, SQL
  migrations, backup tasks, firewall rules or fiscal transmission.
  A green result is NOT pilot acceptance or proof of release provenance.
#>
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'),
    [ValidateRange(1, 512)][int]$MinimumFreeGiB = 10,
    [ValidateRange(1, 65535)][int]$ListenPort = 8080,
    [switch]$RequireSignedServer
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:OS -ne 'Windows_NT') {
    throw 'Esterium Local host preflight supports only Windows x64.'
}
if (-not [Environment]::Is64BitOperatingSystem) {
    throw 'Esterium Local requires a 64-bit Windows installation.'
}
$version = [Environment]::OSVersion.Version
if ($version.Major -lt 10 -or ($version.Major -eq 10 -and $version.Build -lt 19045)) {
    throw 'This Windows version is outside the preliminary Windows 10/11 x64 support baseline.'
}
if ($PSVersionTable.PSVersion.Major -lt 5) {
    throw 'PowerShell 5.1 or newer is required for technical installation.'
}
if (-not [IO.Path]::IsPathRooted($InstallDir)) {
    throw 'InstallDir must be absolute.'
}
$installPath = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\', '/')
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\', '/')
if (-not $installPath.StartsWith($programFiles + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'InstallDir must be a dedicated subdirectory of Program Files.'
}
if (Test-Path -LiteralPath $installPath) {
    throw 'An Esterium installation directory already exists; never overwrite it.'
}
if ($null -ne (Get-Service -Name 'EsteriumERP' -ErrorAction SilentlyContinue)) {
    throw 'An Esterium Windows service already exists; refusing a parallel install.'
}
$driveRoot = [IO.Path]::GetPathRoot($installPath)
$driveInfo = [IO.DriveInfo]::new($driveRoot)
if (-not $driveInfo.IsReady) {
    throw 'The installation volume is not accessible.'
}
$minimumBytes = [int64]$MinimumFreeGiB * 1024 * 1024 * 1024
if ($driveInfo.AvailableFreeSpace -lt $minimumBytes) {
    throw 'Not enough free disk capacity for the requested technical installation threshold.'
}
# Reading the OS TCP listener table is safe: never bind a port in a preflight.
$tcp = @(Get-NetTCPConnection -State Listen -LocalPort $ListenPort -ErrorAction SilentlyContinue)
if ($tcp.Count -gt 0) {
    throw 'The application listener port is already occupied.'
}
$bundle = (Resolve-Path -LiteralPath $BundleDir -ErrorAction Stop).Path
$bundleItem = Get-Item -LiteralPath $bundle -Force
if (-not $bundleItem.PSIsContainer -or
    ($bundleItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw 'Technical bundle must be a real local directory, not a symbolic link.'
}
$verifier = Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'
& $verifier -BundleDir $bundle | Out-Null
$serverBinary = Join-Path $bundle 'esterium-server.exe'
if ($RequireSignedServer) {
    $signature = Get-AuthenticodeSignature -LiteralPath $serverBinary
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid) {
        throw 'Production signature check failed: the executable is not Authenticode-valid.'
    }
} else {
    Write-Warning 'Technical package SHA-256 passed, but publisher authenticity was NOT verified. Not cleared for production.'
}
Write-Output 'PASS: Windows x64 host, disk, listener port, new-install isolation and SHA-256 technical bundle check.'
Write-Output 'NOT CHECKED: PostgreSQL/Redis TLS, LAN client trust, backup restore, printers, signature unless requested, or SEFAZ.'
