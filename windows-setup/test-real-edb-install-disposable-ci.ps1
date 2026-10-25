#requires -Version 5.1
<#
  Real EnterpriseDB PostgreSQL 16 unattended install on an EPHEMERAL hosted
  Windows runner only. No actual user's machine, no commercial deployment.
  Reuses the approved EDB unattended option schema, but cannot bypass any
  Authenticode requirement in the signed production chain.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PostgresInstaller,
    [Parameter(Mandatory=$true)][string]$ExpectedSHA256,
    [Parameter(Mandatory=$true)][string]$ExpectedSignerThumbprint
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_REAL_EDB_DISPOSABLE_CI -ne '1') {
    throw 'Real unsigned PostgreSQL installation is ONLY permitted on a disposable GitHub-hosted Windows runner.'
}
$identity=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $identity.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Disposable Windows runner requires Administrator.'
}
if ($ExpectedSHA256 -cnotmatch '^[a-fA-F0-9]{64}$' -or
    $ExpectedSignerThumbprint -cnotmatch '^[a-fA-F0-9]{40}$' -or
    -not [IO.Path]::IsPathRooted($PostgresInstaller) -or
    $PostgresInstaller.StartsWith('\\') -or
    -not (Test-Path -LiteralPath $PostgresInstaller -PathType Leaf) -or
    (Get-FileHash -LiteralPath $PostgresInstaller -Algorithm SHA256).Hash -cne $ExpectedSHA256.ToUpperInvariant()) {
    throw 'EDB installer does not match independent pinned file hash.'
}
$signature=Get-AuthenticodeSignature -LiteralPath $PostgresInstaller
if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
    $null -eq $signature.SignerCertificate -or
    $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
      $ExpectedSignerThumbprint.ToUpperInvariant()) {
    throw 'EDB signer is not Authenticode-valid and pinned.'
}
$serviceName='EsteriumCIpg16'
$account='esterium_ci_pg'
$port=55439
$installDir=Join-Path $env:ProgramFiles 'EsteriumCI-PostgreSQL-16'
$parent=Join-Path $env:ProgramData 'EsteriumCIPostgreSQL'
$dataDir=Join-Path $parent 'Data'
$optionsDir=Join-Path $parent 'Options'
$optionFile=Join-Path $optionsDir 'postgresql-unattended.options'
$identities=@($account,('.\'+$account),($env:COMPUTERNAME+'\'+$account))
foreach ($p in @($installDir,$parent,$dataDir,$optionsDir)) {
    if (Test-Path -LiteralPath $p) {throw 'CI EDB test must never overwrite existing files.'}
}
if ($null -ne (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) -or
    @(Get-CimInstance Win32_UserAccount -Filter "Name='$account' AND LocalAccount=TRUE").Count -ne 0 -or
    @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
    throw 'CI EDB test requires a fresh Windows identity, service and port.'
}
function New-CIPassword {
    $rng=[Security.Cryptography.RandomNumberGenerator]::Create()
    $bytes=New-Object byte[] 48
    try {
        $rng.GetBytes($bytes)
        return [BitConverter]::ToString($bytes).Replace('-','')
    } finally {
        $rng.Dispose()
        [Array]::Clear($bytes,0,$bytes.Length)
    }
}
$super=New-CIPassword
$servicePassword=New-CIPassword
if ($super -ceq $servicePassword) { throw 'Password generator reused a secret.' }
$serviceBefore=$null
$installer=$null
try {
    [IO.Directory]::CreateDirectory($optionsDir) | Out-Null
    & icacls.exe $parent '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Cannot protect CI PostgreSQL configuration directory.'}
    $lines=@(
        'mode=unattended'
        ('prefix='+$installDir)
        ('datadir='+$dataDir)
        ('serverport='+$port)
        ('servicename='+$serviceName)
        ('serviceaccount='+$account)
        'superaccount=postgres'
        ('superpassword='+$super)
        ('servicepassword='+$servicePassword)
        'enable-components=server,commandlinetools'
        'enable_acledit=1'
        'create_shortcuts=0'
        'unattendedmodeui=minimal'
    )
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes(($lines -join [Environment]::NewLine)+[Environment]::NewLine)
    try {
        $stream=[IO.File]::Open($optionFile,[IO.FileMode]::CreateNew,
            [IO.FileAccess]::Write,[IO.FileShare]::None)
        try {$stream.Write($bytes,0,$bytes.Length);$stream.Flush($true)}
        finally {$stream.Dispose()}
    } finally { [Array]::Clear($bytes,0,$bytes.Length) }
    & icacls.exe $optionFile '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'CI EDB password-bearing options are not protected.'}
    # One more signature/hash check immediately before launching privileged bytes.
    if ((Get-FileHash -LiteralPath $PostgresInstaller -Algorithm SHA256).Hash -cne
        $ExpectedSHA256.ToUpperInvariant()) {throw 'EDB bytes changed before execution.'}
    $signature=Get-AuthenticodeSignature -LiteralPath $PostgresInstaller
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
            $ExpectedSignerThumbprint.ToUpperInvariant()) {
        throw 'EDB signature changed before execution.'
    }
    Write-Output 'CI ONLY: starting real EDB PostgreSQL 16 unattended installation on disposable hosted runner.'
    $start=[Diagnostics.ProcessStartInfo]::new()
    $start.FileName=$PostgresInstaller
    $start.Arguments='--mode unattended --unattendedmodeui none --optionfile "'+$optionFile+'"'
    $start.UseShellExecute=$false
    $installer=[Diagnostics.Process]::Start($start)
    if ($null -eq $installer) {throw 'Could not start real EDB installer.'}
    if (-not $installer.WaitForExit(8*60*1000)) {
        try {$installer.Kill()} catch {}
        throw 'EDB installer timed out in disposable CI.'
    }
    if ($installer.ExitCode -ne 0) {
        throw ('Real EDB unattended installer returned error code '+$installer.ExitCode)
    }
    Write-Output 'CI ONLY: EDB unattended process returned success; checking service and database version.'
} finally {
    $super=$null
    $servicePassword=$null
    # EDB may have created an auto-starting service even on failure.
    # Always disable and stop our CI-unique service before any assertions.
    $svc=Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($null -ne $svc) {
        try {
            Set-Service -Name $serviceName -StartupType Disabled -ErrorAction Stop
            $svc=Get-Service -Name $serviceName -ErrorAction Stop
            if ($svc.Status -ne [ServiceProcess.ServiceControllerStatus]::Stopped) {
                Stop-Service -Name $serviceName -Force -ErrorAction Stop
                (Get-Service -Name $serviceName).WaitForStatus(
                    [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(90))
            }
        } catch {
            throw 'CI EDB service could not be disabled and stopped; do not reuse this runner.'
        }
    }
    if (Test-Path -LiteralPath $optionFile -PathType Leaf) {
        Remove-Item -LiteralPath $optionFile -Force -ErrorAction Stop
    }
    if ($null -ne $installer) {$installer.Dispose()}
}
$pgBin=Join-Path $installDir 'bin\pg_isready.exe'
$version=Join-Path $dataDir 'PG_VERSION'
$svc=@(Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop)
if (-not(Test-Path -LiteralPath $pgBin -PathType Leaf) -or
    -not(Test-Path -LiteralPath $version -PathType Leaf) -or
    ([IO.File]::ReadAllText($version)).Trim() -cne '16' -or
    $svc.Count -ne 1 -or $svc[0].StartMode -cne 'Disabled' -or
    $svc[0].State -cne 'Stopped' -or [int]$svc[0].ProcessId -ne 0 -or
    $svc[0].StartName -notin $identities -or
    @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -ne 0) {
    throw 'EDB installed but did not meet fresh-cluster, identity, disabled service and no-listener gates.'
}
Write-Output 'PASS: real signed EDB PostgreSQL 16 EXE installed on disposable Windows, fresh PG_VERSION=16.'
Write-Output 'PASS: dedicated Windows service identity, stopped/disabled SCM, no TCP listener; no credentials logged.'
Write-Output 'NOT READY: no production use, SQL migrations, Garnet start, TLS or backup was authorized by this isolated test.'
