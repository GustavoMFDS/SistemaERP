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
$probePassFile=Join-Path $root 'pgpass-ci.txt'
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
# This experiment is NOT an installer or repair entrypoint.
# It can run only within the vendor EXE's freshly created hosted CI fixture.
function Normalize-DisposableEdbAcl([string]$DataDir,[string]$ServiceSid,[string]$FixtureId) {
    if($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:ESTERIUM_EDB_REAL_DISPOSABLE_CI -ne '1'){
        throw 'EDB normalization experiment requires hosted disposable CI.'
    }
    if($FixtureId -cnotmatch '^[a-f0-9]{10}$' -or $ServiceSid -cnotmatch '^S-1-5-21-(\d+-){3}\d+$'){throw 'Invalid fixture identity.'}
    $expected=Join-Path $env:ProgramData ('Esterium\EdbCITest-'+$FixtureId)
    if([IO.Path]::GetFullPath($DataDir) -cne $expected){throw 'Unexpected fixture data path.'}
    $service=Get-CimInstance Win32_Service -Filter "Name='EstEDB$FixtureId'" -ErrorAction Stop
    if($null -eq $service -or $service.State -cne 'Stopped' -or $service.StartMode -cne 'Disabled' -or [int]$service.ProcessId -ne 0){throw 'Fixture service must remain stopped and disabled.'}
    $expectedSid=[string](Get-LocalUser -Name ('estpg'+$FixtureId) -ErrorAction Stop).SID.Value
    if($expectedSid -cne $ServiceSid){throw 'Fixture service SID mismatch.'}
    $cursor=$expected
    while($cursor){
        $entry=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if(-not $entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Fixture ancestor must be physical.'}
        $parent=Split-Path -Parent $cursor
        if($parent -eq $cursor){break}
        $cursor=$parent
    }
    # This is a private, single-purpose runner. Reject links before any writes.
    # fsutil reports one name per hard link for ordinary files. A query failure
    # is an error, not permission to proceed.
    $pending=[Collections.Generic.Stack[string]]::new()
    $pending.Push($expected)
    $objects=[Collections.Generic.List[object]]::new()
    while($pending.Count){
        $path=$pending.Pop()
        $entry=Get-Item -LiteralPath $path -Force -ErrorAction Stop
        if($entry.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Fixture reparse point rejected.'}
        if(-not $entry.PSIsContainer){
            $links=@(& (Join-Path $env:SystemRoot 'System32\fsutil.exe') hardlink list $path 2>&1)
            if($LASTEXITCODE -ne 0 -or $links.Count -ne 1){throw 'Fixture file link count could not be established as one.'}
        }
        $objects.Add([pscustomobject]@{Path=$path;Directory=[bool]$entry.PSIsContainer})
        if($entry.PSIsContainer){foreach($child in @(Get-ChildItem -LiteralPath $path -Force -ErrorAction Stop)){$pending.Push($child.FullName)}}
    }
    # Parent first, exact protected DACL and approved owner on every object.
    # No recovery of an existing cluster is authorized by this experiment.
    foreach($object in $objects){
        $entry=Get-Item -LiteralPath $object.Path -Force -ErrorAction Stop
        if([bool]$entry.PSIsContainer -ne $object.Directory -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Fixture object changed before normalization.'}
        $raw=New-FreshPostgresDataDescriptor $ServiceSid $object.Directory
        $bytes=New-Object byte[] $raw.BinaryLength
        $raw.GetBinaryForm($bytes,0)
        $acl=Get-Acl -LiteralPath $object.Path -ErrorAction Stop
        $acl.SetSecurityDescriptorBinaryForm($bytes,([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
        Set-Acl -LiteralPath $object.Path -AclObject $acl -ErrorAction Stop
    }
    Assert-FreshPostgresDataAcl $expected $ServiceSid
    Write-Output ('PASS: disposable EDB cluster normalized and validated objects='+$objects.Count)
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
    # Restrict the synthetic probe passfile before use; never put it on CLI.
    [IO.File]::WriteAllText($probePassFile,('127.0.0.1:'+ $port +':postgres:postgres:'+ $pgSecret +[Environment]::NewLine),[Text.UTF8Encoding]::new($false))
    & icacls.exe $probePassFile '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Disposable probe passfile ACL failed.'}
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
    # Read-only inspection of the vendor-created service and cluster using
    # the exact same validator as the proposed fresh-install acceptance gate.
    $candidateInstaller=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
    $aclTokens=$null;$aclErrors=$null
    $aclAst=[Management.Automation.Language.Parser]::ParseFile($candidateInstaller,[ref]$aclTokens,[ref]$aclErrors)
    if($aclErrors.Count){throw 'Candidate ACL validator syntax invalid.'}
    foreach($functionName in @('Test-FreshPostgresServiceCommand','Test-PostgresDataDescriptor','New-FreshPostgresDataDescriptor','Assert-FreshPostgresDataAcl')){
        $defs=@($aclAst.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $functionName},$true))
        if($defs.Count -ne 1){throw 'Candidate ACL function absent or ambiguous.'}
        . ([scriptblock]::Create($defs[0].Extent.Text))
    }
    $bindingOk=Test-FreshPostgresServiceCommand ([string]$scm[0].PathName) $installDir $dataDir $svc $false
    Write-Output ('EDB actual service binding accepted='+$bindingOk)
    $aclOk=$false
    $aclReason='inspection-error'
    try {
        $serviceSid=[string](Get-LocalUser -Name $svcAccount -ErrorAction Stop).SID.Value
        Assert-FreshPostgresDataAcl $dataDir $serviceSid
        $aclOk=$true
        $aclReason='pass'
    } catch {
        if($_.Exception.Message -match '^PostgreSQL fresh data ACL rejected: (owner|null-dacl|root-inheritance|unsupported-ace|unexpected-principal|excessive-rights|missing-effective-rights|missing-child-rights)\.'){
            $aclReason=$Matches[1]
        }
    }
    Write-Output ('EDB actual data ACL accepted='+$aclOk+' reason='+$aclReason)
    if(-not $bindingOk){throw 'EDB vendor command does not match approved fresh service binding.'}
    if(-not $aclOk){
        if($aclReason -notin @('owner','root-inheritance','unexpected-principal','excessive-rights','missing-effective-rights','missing-child-rights')){throw 'EDB ACL inspection failed outside supported normalization experiment.'}
        Normalize-DisposableEdbAcl $dataDir $serviceSid $id
    }
    # Re-check the stopped state before the live disposable fixture.
    $scm=Get-CimInstance Win32_Service -Filter "Name='$svc'" -ErrorAction Stop
    if($scm.State -ne 'Stopped' -or $scm.StartMode -ne 'Disabled' -or [int]$scm.ProcessId -ne 0){throw 'EDB fixture service changed during ACL experiment.'}

    # Live proof on this newly created, disposable cluster only. Restrict
    # listeners before enabling the service; do not change firewall/trust.
    $autoConfig=Join-Path $dataDir 'postgresql.auto.conf'
    if(-not (Test-Path -LiteralPath $autoConfig -PathType Leaf) -or ((Get-Item -LiteralPath $autoConfig -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'Expected physical fresh auto configuration missing.'}
    [IO.File]::AppendAllText($autoConfig,[Environment]::NewLine+"listen_addresses = '127.0.0.1'"+[Environment]::NewLine+"port = $port"+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    $readyExe=Join-Path $installDir 'bin\pg_isready.exe'
    function Start-DisposableEdbAndCheck {
        Set-Service -Name $svc -StartupType Manual -ErrorAction Stop
        Start-Service -Name $svc -ErrorAction Stop
        (Get-Service -Name $svc).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(60))
        $ready=$false
        for($attempt=0;$attempt -lt 30;$attempt++){
            & $readyExe -h 127.0.0.1 -p $port -t 2 -q
            if($LASTEXITCODE -eq 0){$ready=$true;break}
            Start-Sleep -Milliseconds 500
        }
        if(-not $ready){throw 'Normalized EDB cluster not ready.'}
        $listeners=@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction Stop)
        if($listeners.Count -lt 1 -or @($listeners | Where-Object {$_.LocalAddress -cne '127.0.0.1'}).Count){throw 'Disposable EDB listener is not restricted to loopback.'}
        foreach($listener in $listeners){
            $worker=Get-CimInstance Win32_Process -Filter "ProcessId=$($listener.OwningProcess)" -ErrorAction Stop
            if($null -eq $worker -or [string]$worker.ExecutablePath -ine $pg){throw 'Unexpected EDB listener process.'}
            $owner=Invoke-CimMethod -InputObject $worker -MethodName GetOwnerSid -ErrorAction Stop
            if($owner.ReturnValue -ne 0 -or [string]$owner.Sid -cne $serviceSid){throw 'EDB listener does not run under expected dedicated service SID.'}
        }
    }
    function Stop-DisposableEdbAndCheck {
        Set-Service -Name $svc -StartupType Disabled -ErrorAction Stop
        Stop-Service -Name $svc -ErrorAction Stop
        (Get-Service -Name $svc).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))
        if(@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count){throw 'Disposable EDB still listening after stop.'}
    }
    function Invoke-DisposableSql([string]$Sql) {
        $result=@(& $psql -X -q -t -A -w -h 127.0.0.1 -p $port -U postgres -d postgres -v ON_ERROR_STOP=1 -c $Sql)
        if($LASTEXITCODE -ne 0){throw 'Disposable normalized EDB SQL failed.'}
        return ($result -join '').Trim()
    }
    $oldPgEnv=@{}
    foreach($key in @('PGPASSFILE','PGPASSWORD','PGSSLMODE','PGSERVICE','PGSERVICEFILE','PGHOSTADDR','PGOPTIONS')){
        $oldPgEnv[$key]=[Environment]::GetEnvironmentVariable($key,'Process')
        [Environment]::SetEnvironmentVariable($key,$null,'Process')
    }
    try {
        $env:PGPASSFILE=$probePassFile
        # This is a loopback-only vendor fixture, not the commercial TLS policy.
        $env:PGSSLMODE='disable'
        Start-DisposableEdbAndCheck
        if((Invoke-DisposableSql "SHOW listen_addresses") -cne '127.0.0.1'){throw 'SQL reports unexpected listener configuration.'}
        $null=Invoke-DisposableSql "CREATE TABLE ci_acl_durable (id integer PRIMARY KEY, payload text NOT NULL); INSERT INTO ci_acl_durable VALUES (1,'acl-normalized');"
        if((Invoke-DisposableSql "SELECT payload FROM ci_acl_durable WHERE id=1") -cne 'acl-normalized'){throw 'Initial SQL persistence failed.'}
        Stop-DisposableEdbAndCheck
        Start-DisposableEdbAndCheck
        if((Invoke-DisposableSql "SELECT payload FROM ci_acl_durable WHERE id=1") -cne 'acl-normalized'){throw 'SQL durability after restart failed.'}
        Stop-DisposableEdbAndCheck
        Assert-FreshPostgresDataAcl $dataDir $serviceSid
        Write-Output 'PASS: normalized real EDB service ran under dedicated SID; committed SQL survived restart and recursive ACL recheck.'
    } finally {
        foreach($key in $oldPgEnv.Keys){[Environment]::SetEnvironmentVariable($key,$oldPgEnv[$key],'Process')}
    }

    $installed=$true
    Write-Output 'PASS: EDB EXE unattended install, PostgreSQL 16 cluster, dedicated Windows service/account, stopped/disabled SCM and closed port.'
    Write-Output 'NOT PROVEN: signed Esterium bundle, Garnet WinSW, ERP TLS/SQL bootstrap, two-box LAN, external backup or commercial release.'
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
    if (Test-Path -LiteralPath $probePassFile -PathType Leaf) { Remove-Item -LiteralPath $probePassFile -Force -ErrorAction Stop }
    if (Test-Path -LiteralPath $options -PathType Leaf) {
        Remove-Item -LiteralPath $options -Force -ErrorAction Stop
    }
}
