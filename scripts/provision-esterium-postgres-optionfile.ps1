#requires -Version 5.1
<#
  Generate a protected option file for a NEW PostgreSQL 16 EDB Windows
  installer. Never execute, download or trust the third-party installer.
  Default is read-only. Apply creates ONLY a new restricted configuration.
  Superuser/OS-service passwords are written to an ACL-protected option
  file because EDB reads them there; never put them on a command line.
#>
param(
    [Parameter(Mandatory=$true)][string]$SuperPasswordFile,
    [Parameter(Mandatory=$true)][string]$ServicePasswordFile,
    [string]$DestinationDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$PostgresInstallDir=(Join-Path $env:ProgramFiles 'PostgreSQL\16'),
    [string]$DataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$ServiceName='EsteriumPostgreSQL16',
    [string]$ServiceAccount='esterium_pgsvc',
    [ValidateRange(1,65535)][int]$Port=5432,
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows is required.' }
if ($ServiceName -cnotmatch '^[a-zA-Z][a-zA-Z0-9_-]{2,63}$' -or
    $ServiceAccount -cnotmatch '^[a-zA-Z][a-zA-Z0-9_-]{2,30}$' -or
    $ServiceAccount -in @('postgres','administrator','system','networkservice','localservice')) {
    throw 'Invalid or unsafe dedicated PostgreSQL Windows service/account name.'
}
$programFiles=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
$programData=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$target=[IO.Path]::GetFullPath($DestinationDir).TrimEnd('\')
$prefix=[IO.Path]::GetFullPath($PostgresInstallDir).TrimEnd('\')
$data=[IO.Path]::GetFullPath($DataDir).TrimEnd('\')
if (-not [IO.Path]::IsPathRooted($DestinationDir) -or
    -not [IO.Path]::IsPathRooted($PostgresInstallDir) -or
    -not [IO.Path]::IsPathRooted($DataDir) -or
    -not $target.StartsWith($programData + '\',[StringComparison]::OrdinalIgnoreCase) -or
    -not $data.StartsWith($programData + '\',[StringComparison]::OrdinalIgnoreCase) -or
    -not $prefix.StartsWith($programFiles + '\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'PostgreSQL options/data must be new paths under ProgramData\Esterium; binaries must be under Program Files.'
}
foreach ($dir in @($target,$prefix,$data)) {
    if (Test-Path -LiteralPath $dir) {
        throw 'Existing PostgreSQL install/data/options path cannot be overwritten.'
    }
    $cursor=Split-Path -Parent $dir
    while (-not [string]::IsNullOrWhiteSpace($cursor)) {
        if (Test-Path -LiteralPath $cursor) {
            $item=Get-Item -LiteralPath $cursor -Force
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw 'PostgreSQL install/data/options ancestors must not be symbolic links.'
            }
        }
        $next=Split-Path -Parent $cursor
        if ($next -eq $cursor) { break }
        $cursor=$next
    }
}
if ($target.StartsWith($data + '\',[StringComparison]::OrdinalIgnoreCase) -or
    $data.StartsWith($target + '\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Options and PostgreSQL data paths must be separate.'
}
if ($Port -eq 8080 -or $Port -eq 6379) {
    throw 'PostgreSQL may not take the Esterium web or local cache port.'
}
if ((Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) -ne $null) {
    throw 'PostgreSQL Windows service already exists.'
}
if (@(Get-CimInstance -ClassName Win32_UserAccount -Filter "Name='$ServiceAccount' AND LocalAccount=TRUE" -ErrorAction Stop).Count -ne 0) {
    throw 'PostgreSQL Windows account already exists: never change its password.'
}
if (@(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue).Count -gt 0) {
    throw 'PostgreSQL TCP port already occupied.'
}
$secrets=@($SuperPasswordFile,$ServicePasswordFile)
$normalized=@()
foreach ($source in $secrets) {
    if (-not [IO.Path]::IsPathRooted($source) -or
        -not (Test-Path -LiteralPath $source -PathType Leaf)) {
        throw 'Both PostgreSQL passwords require existing absolute protected files.'
    }
    $path=[IO.Path]::GetFullPath($source)
    if ($path.StartsWith($target + '\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Credential sources must be outside the generated option file.'
    }
    $file=Get-Item -LiteralPath $path -Force
    if ($file.Length -le 0 -or ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'PostgreSQL password source is empty or a symbolic link.'
    }
    $acl=Get-Acl -LiteralPath $path
    foreach ($entry in $acl.Access) {
        if ($entry.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        try { $sid=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value }
        catch { throw 'Cannot verify PostgreSQL password ACL identities.' }
        if (($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0 -and
            $sid -notin @('S-1-5-18','S-1-5-32-544')) {
            throw 'PostgreSQL bootstrap credentials may be read only by SYSTEM and Administrators.'
        }
    }
    $normalized+= $path
}
if ($normalized[0] -eq $normalized[1]) {
    throw 'Superuser and Windows service passwords must be distinct files.'
}
if (-not $Apply) {
    Write-Output 'PLAN: generate an ACL-protected offline PostgreSQL 16 installer option file for a new service and empty data location.'
    Write-Output 'PLAN: unattended server and command-line tools only; no pgAdmin/StackBuilder or shortcuts.'
    Write-Output 'PLAN: passwords will be stored only in the restricted file, not shell arguments or logs.'
    Write-Output 'PLAN: no database, Windows service, program, network or fiscal action performed.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator approval required for PostgreSQL option file creation.'
}
$passwords=@()
foreach ($path in $normalized) {
    $secret=[IO.File]::ReadAllText($path).TrimEnd([char]13,[char]10)
    if ($secret -cnotmatch '^[a-zA-Z0-9._!-]{24,128}$') {
        throw 'PostgreSQL installer password must contain 24-128 safe non-whitespace characters.'
    }
    $passwords+= $secret
}
if ($passwords[0] -ceq $passwords[1]) {
    throw 'PostgreSQL and service passwords must be different.'
}
$icacls=Join-Path $env:SystemRoot 'System32\icacls.exe'
$parent=Split-Path -Parent $target
[IO.Directory]::CreateDirectory($parent) | Out-Null
$stage=Join-Path $parent ('.esterium-postgresql-options-' + [Guid]::NewGuid().ToString('N'))
$published=$false
try {
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    & $icacls $stage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not restrict PostgreSQL installer option directory.' }
    $text=@(
        'mode=unattended'
        ('prefix=' + $prefix)
        ('datadir=' + $data)
        ('serverport=' + $Port)
        ('servicename=' + $ServiceName)
        ('serviceaccount=' + $ServiceAccount)
        'superaccount=postgres'
        ('superpassword=' + $passwords[0])
        ('servicepassword=' + $passwords[1])
        'enable-components=server,commandlinetools'
        'create_shortcuts=0'
        'unattendedmodeui=minimal'
    ) -join [Environment]::NewLine
    $out=Join-Path $stage 'postgresql-unattended.options'
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes($text + [Environment]::NewLine)
    $handle=[IO.File]::Open($out,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $handle.Write($bytes,0,$bytes.Length); $handle.Flush($true) }
    finally { $handle.Dispose(); [Array]::Clear($bytes,0,$bytes.Length) }
    & $icacls $out '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not restrict PostgreSQL installer options.' }
    if (Test-Path -LiteralPath $target) { throw 'Another process created PostgreSQL options.' }
    Move-Item -LiteralPath $stage -Destination $target -ErrorAction Stop
    $published=$true
    Write-Output 'PASS: new protected PostgreSQL 16 unattended installer option file created.'
    Write-Output 'NOT INSTALLED: PostgreSQL service, data cluster, TLS, application database or SEFAZ.'
    Write-Output 'Before any vendor installer execution, verify its signed origin and ensure passwords/options are securely retired afterward.'
} finally {
    $passwords=@()
    if (-not $published -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force
    }
}
