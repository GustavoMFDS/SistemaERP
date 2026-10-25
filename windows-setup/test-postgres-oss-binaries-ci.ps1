#requires -Version 5.1
<#
  Tests official EDB PostgreSQL Windows 16.15 binary ZIP on a disposable
  GitHub-hosted runner. NO INSTALL, no new Windows service, no initdb,
  no database files, no customer system modification. Requires verified
  vendor SHA pin obtained independently from EnterpriseDB/edb-installers #731.
#>
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1') {
    throw 'PostgreSQL ZIP smoke is permitted only on hosted disposable Windows CI.'
}
$version='16.15-5'
$url='https://get.enterprisedb.com/postgresql/postgresql-16.15-5-windows-x64-binaries.zip'
# EDB contributor-published SHA-256 of the exact 16.15-5 ZIP:
# https://github.com/EnterpriseDB/edb-installers/issues/731
# Do not confuse it with the DIFFERENT SHA for PostgreSQL 17.11-5.
$expected='43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'
$root=Join-Path $env:RUNNER_TEMP ('esterium-postgres-zip-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($root) | Out-Null
$package=Join-Path $root ('postgresql-'+$version+'-windows-x64-binaries.zip')
& curl.exe -fLsS --retry 2 --connect-timeout 30 --max-time 480 -o $package $url
if ($LASTEXITCODE -ne 0) { throw 'Official EDB ZIP download failed.' }
if ((Get-FileHash -LiteralPath $package -Algorithm SHA256).Hash -cne $expected) {
    throw 'FAIL CLOSED: official PostgreSQL ZIP did not match independent EDB SHA-256.'
}
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive=[IO.Compression.ZipFile]::OpenRead($package)
try {
    $found=@{}
    $sum=[long]0
    $count=0
    foreach ($entry in $archive.Entries) {
        $count++
        if ($count -gt 30000) { throw 'Unexpected EDB ZIP file count.' }
        $name=$entry.FullName.Replace('\','/')
        $parts=$name.TrimEnd('/').Split('/')
        if (-not $name.StartsWith('pgsql/',[StringComparison]::Ordinal) -or
            $name.Contains(':') -or $name.Contains('\') -or
            $parts.Count -lt 2 -or
            @($parts | Where-Object { $_ -in @('', '.', '..') }).Count -gt 0 -or
            (($entry.ExternalAttributes -shr 16) -band 0xf000) -eq 0xa000) {
            throw 'Unsafe or unexpected ZIP content.'
        }
        if ($found.ContainsKey($name)) { throw 'Duplicate path in PostgreSQL archive.' }
        $found[$name]=$true
        $sum+=$entry.Length
        if ($sum -gt 1300000000) { throw 'Unexpected expansion size for PostgreSQL binary ZIP.' }
    }
    foreach ($needed in @('pgsql/bin/postgres.exe','pgsql/bin/initdb.exe',
        'pgsql/bin/pg_ctl.exe','pgsql/bin/psql.exe','pgsql/bin/pg_isready.exe')) {
        if (-not $found.ContainsKey($needed)) {
            throw 'Required PostgreSQL binary missing from vendor ZIP: '+$needed
        }
    }
    Write-Output ('PASS: EDB PostgreSQL '+$version+' binary ZIP SHA-256 pinned.')
    Write-Output ('PASS: validated '+$count+' ZIP entries, no links/traversal, expanded bytes '+$sum+'.')
} finally { $archive.Dispose() }
$extract=Join-Path $root 'extracted'
[IO.Directory]::CreateDirectory($extract) | Out-Null
[IO.Compression.ZipFile]::ExtractToDirectory($package,$extract)
$bin=Join-Path $extract 'pgsql\bin'
foreach ($name in @('postgres.exe','initdb.exe','pg_ctl.exe','psql.exe')) {
    $exe=Join-Path $bin $name
    if (-not (Test-Path -LiteralPath $exe -PathType Leaf) -or
        ((Get-Item -LiteralPath $exe).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Expected binary absent after ZIP extraction: '+$name
    }
    $output=@(& $exe '--version' 2>&1)
    if ($LASTEXITCODE -ne 0 -or
        (($output -join ' ') -notmatch '16\.15')) {
        throw 'PostgreSQL binary failed read-only --version check: '+$name
    }
    Write-Output ('PASS: extracted '+$name+' reports PostgreSQL 16.15.')
}
$services=@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue)
if ($services.Count -ne 0) {throw 'Read-only PostgreSQL ZIP smoke unexpectedly created services.'}
Write-Output 'PASS: ZIP binaries usable on Windows; no initdb, service, password or SQL was created.'
