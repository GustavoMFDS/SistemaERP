#requires -Version 5.1
<#
  Install the REAL vendor EDB PostgreSQL 16.15 EXE on an ephemeral,
  GitHub-hosted Windows runner. This is deliberately independent of the
  commercial signed Esterium coordinator: never bypass its approval policy.
  WinGet's version-pinned public manifest supplies the installer SHA-256.
  No customer data, permanent activation or release approval is involved.
#>
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_EDB_REAL_DISPOSABLE_CI -ne '1') {
    throw 'Real vendor EXE smoke is restricted to disposable hosted Windows CI.'
}
if (-not [Security.Principal.WindowsPrincipal]::new(
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'EDB vendor installation smoke requires elevated Windows CI.'
}
$release='16.15-1'
$uri='https://get.enterprisedb.com/postgresql/postgresql-'+$release+'-windows-x64.exe'
# Independently recorded WinGet community catalog, version-specific manifest:
# https://github.com/microsoft/winget-pkgs/blob/master/manifests/p/PostgreSQL/PostgreSQL/16/16.15-1/PostgreSQL.PostgreSQL.16.installer.yaml
$expectedHash='DE926FEFAD00E313E212CD438C0F04BF033E200099AD56C012724EFCEBED79F2'
$id=[Guid]::NewGuid().ToString('N').Substring(0,10)
$root=Join-Path $env:RUNNER_TEMP ('esterium-edb-exe-'+$id)
$installer=Join-Path $root ('postgresql-'+$release+'-windows-x64.exe')
$options=Join-Path $root 'edb-install.options'
$installDir=Join-Path $env:ProgramFiles ('EsteriumPgEdbCITest-'+$id)
$dataDir=Join-Path $env:ProgramData ('Esterium\EdbCITest-'+$id)
$svc='EstEDB'+$id
$svcAccount='estpg'+$id
$portListener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
$port=0
try {
    $portListener.Start()
    $port=([Net.IPEndPoint]$portListener.LocalEndpoint).Port
} finally { $portListener.Stop() }
if ($port -lt 1024 -or $port -gt 65535) { throw 'No safe temporary PostgreSQL port.' }
if ((Get-Service -Name $svc -ErrorAction SilentlyContinue) -or
    (Test-Path -LiteralPath $installDir) -or (Test-Path -LiteralPath $dataDir) -or
    @(Get-CimInstance Win32_UserAccount -Filter "Name='$svcAccount' AND LocalAccount=TRUE").Count -gt 0) {
    throw 'Disposable EDB identity/path already exists; never reuse.'
}
$installed=$false
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    & icacls.exe $root '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'CI secret staging permissions invalid.' }
    # EDB is large: a corrupted or unavailable download must fail hard.
    Invoke-WebRequest -Uri $uri -OutFile $installer -MaximumRedirection 3 -TimeoutSec 1200 -ErrorAction Stop
    if ((Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash -cne $expectedHash) {
        throw 'EDB EXE does not match the independently pinned WinGet SHA-256.'
    }
    $signature=Get-AuthenticodeSignature -LiteralPath $installer
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Subject -notmatch '(?i)EnterpriseDB') {
        throw 'EDB EXE Authenticode chain/signer was not independently validated.'
    }
    Write-Output 'PASS: genuine PostgreSQL 16.15-1 EDB EXE SHA-256 and trusted Authenticode signer.'
    # Different unpredictable credentials, never echoed to CI logs, command
    # line or source control. EDB option file is restricted and deleted.
    $pgSecret=[Convert]::ToBase64String(
        [Security.Cryptography.RandomNumberGenerator]::GetBytes(30))
    $svcSecret=[Convert]::ToBase64String(
        [Security.Cryptography.RandomNumberGenerator]::GetBytes(30))
    $lines=@(
        'mode=unattended'
        'unattendedmodeui=none'
        'prefix='+$installDir
        'datadir='+$dataDir
        'serverport='+$port
        'servicename='+$svc
        'serviceaccount='+$svcAccount
        'superaccount=postgres'
        'superpassword='+$pgSecret
        'servicepassword='+$svcSecret
        'enable-components=server,commandlinetools'
        'enable_acledit=1'
        'create_shortcuts=0'
    )
    [IO.File]::WriteAllLines($options,$lines,[Text.UTF8Encoding]::new($false))
    $pgSecret=$null
    $svcSecret=$null
    [Array]::Clear($lines,0,$lines.Length)
    Write-Output 'Installing EDB vendor EXE with a newly named service on disposable Windows CI...'
    $process=Start-Process -FilePath $installer -ArgumentList @(
        '--mode','unattended','--unattendedmodeui','none',
        '--optionfile',('"' + $options + '"')
    ) -Wait -PassThru -ErrorAction Stop
    if ($null -eq $process -or $process.ExitCode -ne 0) {
        throw 'Real EDB EXE installation failed; see disposable CI logs.'
    }
    # EDB starts its new service on installation. Fail closed: stop/disable
    # immediately, do not expose a working database to the network.
    $service=Get-Service -Name $svc -ErrorAction Stop
    Set-Service -Name $svc -StartupType Disabled -ErrorAction Stop
    if ($service.Status -ne 'Stopped') {
        Stop-Service -Name $svc -Force -ErrorAction Stop
        (Get-Service -Name $svc).WaitForStatus(
            [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))
    }
    $dbVersion=(Join-Path $dataDir 'PG_VERSION')
    $psql=Join-Path $installDir 'bin\psql.exe'
    $pg=Join-Path $installDir 'bin\postgres.exe'
    if (-not (Test-Path -LiteralPath $dbVersion -PathType Leaf) -or
        [IO.File]::ReadAllText($dbVersion).Trim() -cne '16' -or
        -not (Test-Path -LiteralPath $psql -PathType Leaf) -or
        -not (Test-Path -LiteralPath $pg -PathType Leaf)) {
        throw 'EDB installer did not produce PostgreSQL 16 binaries and cluster.'
    }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$svc'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].StartMode -cne 'Disabled' -or
        $scm[0].State -cne 'Stopped' -or [int]$scm[0].ProcessId -ne 0) {
        throw 'Real EDB PostgreSQL service was not stopped and disabled.'
    }
    $identities=@($svcAccount,('.\'+$svcAccount),($env:COMPUTERNAME+'\'+$svcAccount))
    if ([string]$scm[0].StartName -notin $identities) {
        throw 'Real EDB service identity differs from the selected account.'
    }
    if (@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -gt 0) {
        throw 'Real EDB PostgreSQL still listens after service shutdown.'
    }
    $installed=$true
    Write-Output 'PASS: EDB EXE unattended install, PostgreSQL 16 cluster, dedicated Windows service/account, stopped/disabled SCM and closed port.'
    Write-Output 'NOT PROVEN: signed Esterium bundle, Garnet NSSM, ERP TLS/SQL bootstrap, two-box LAN, external backup or commercial release.'
} finally {
    # If EDB registered a partially configured service, disable and stop it
    # even if installation, verification or shutdown failed. Hosted runners
    # are discarded; never attempt destructive rollback on customer machines.
    $s=Get-Service -Name $svc -ErrorAction SilentlyContinue
    if ($null -ne $s) {
        try {
            Set-Service -Name $svc -StartupType Disabled -ErrorAction Stop
            if ((Get-Service -Name $svc).Status -ne 'Stopped') {
                Stop-Service -Name $svc -Force -ErrorAction Stop
                (Get-Service -Name $svc).WaitForStatus(
                    [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))
            }
        } catch {
            Write-Warning 'EDB CI service cleanup failed; hosted runner must be discarded.'
            if ($installed) { throw 'EDB smoke could not leave its service stopped.' }
        }
    }
    if (Test-Path -LiteralPath $options -PathType Leaf) {
        Remove-Item -LiteralPath $options -Force -ErrorAction Stop
    }
}
