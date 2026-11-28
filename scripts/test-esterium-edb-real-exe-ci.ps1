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
    Initialize-FreshPostgresAclNative
    $locks=[Collections.Generic.List[object]]::new()
    $objects=[Collections.Generic.List[object]]::new()
    $identities=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    try {
        # Hold each physical ancestor from volume root down. No share-delete.
        $ancestors=[Collections.Generic.Stack[string]]::new()
        $cursor=Split-Path -Parent $expected
        while($cursor){
            $ancestors.Push($cursor)
            $parent=Split-Path -Parent $cursor
            if($parent -eq $cursor){break}
            $cursor=$parent
        }
        while($ancestors.Count){
            $handle=[EsteriumFreshPgAclNative]::Open($ancestors.Pop(),$true,$false)
            $locks.Add($handle)
        }
        $pending=[Collections.Generic.Stack[string]]::new()
        $pending.Push($expected)
        $rootVolume=''
        while($pending.Count){
            $path=$pending.Pop()
            $entry=Get-Item -LiteralPath $path -Force -ErrorAction Stop
            $directory=[bool]$entry.PSIsContainer
            $handle=[EsteriumFreshPgAclNative]::Open($path,$directory,$true)
            $locks.Add($handle)
            $identity=[EsteriumFreshPgAclNative]::Identity($handle,$directory)
            if($path -ceq $expected){$rootVolume=$identity.Split(':')[0]}
            if($identity.Split(':')[0] -cne $rootVolume -or -not $identities.Add($identity)){throw 'Unexpected fixture volume or repeated identity.'}
            $objects.Add([pscustomobject]@{Path=$path;Directory=$directory;Handle=$handle;Identity=$identity})
            if($directory){
                foreach($child in @(Get-ChildItem -LiteralPath $path -Force -ErrorAction Stop)){$pending.Push($child.FullName)}
            }
        }
        # No writes until every object has a validated, retained handle.
        # Apply to those handles, never reopen an untrusted path for mutation.
        foreach($object in $objects){
            if([EsteriumFreshPgAclNative]::Identity($object.Handle,$object.Directory) -cne $object.Identity){throw 'Fixture identity changed.'}
            $raw=New-FreshPostgresDataDescriptor $ServiceSid $object.Directory
            $bytes=New-Object byte[] $raw.BinaryLength
            $raw.GetBinaryForm($bytes,0)
            [EsteriumFreshPgAclNative]::Apply($object.Handle,$object.Directory,$bytes)
        }
        # Re-enumerate while handles remain held; reject added/missing objects.
        $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
        $pending.Push($expected)
        while($pending.Count){
            $path=$pending.Pop()
            $entry=Get-Item -LiteralPath $path -Force -ErrorAction Stop
            $directory=[bool]$entry.PSIsContainer
            $handle=[EsteriumFreshPgAclNative]::Open($path,$directory,$false)
            try{
                $identity=[EsteriumFreshPgAclNative]::Identity($handle,$directory)
                if(-not $identities.Contains($identity) -or -not $seen.Add($identity)){throw 'Fixture tree changed during ACL normalization.'}
            }finally{$handle.Dispose()}
            if($directory){foreach($child in @(Get-ChildItem -LiteralPath $path -Force -ErrorAction Stop)){$pending.Push($child.FullName)}}
        }
        if($seen.Count -ne $objects.Count){throw 'Fixture tree size changed during ACL normalization.'}
        Assert-FreshPostgresParentBoundary $expected
        Assert-FreshPostgresDataAcl $expected $ServiceSid
        Write-Output ('PASS: disposable EDB cluster normalized through retained handles and validated objects='+$objects.Count)
    } finally {
        for($i=$locks.Count-1;$i -ge 0;$i--){$locks[$i].Dispose()}
    }
}

$preinstallLocks=[Collections.Generic.List[object]]::new()
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
    # Validate that EDB accepts an atomically created, empty protected
    # data directory and preserves its physical identity. Fixture only.
    $candidateInstaller=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
    $aclTokens=$null;$aclErrors=$null
    $aclAst=[Management.Automation.Language.Parser]::ParseFile($candidateInstaller,[ref]$aclTokens,[ref]$aclErrors)
    if($aclErrors.Count){throw 'Candidate ACL validator syntax invalid.'}
    foreach($functionName in @('Test-FreshPostgresServiceCommand','Test-PostgresDataDescriptor','New-FreshPostgresDataDescriptor','Initialize-FreshPostgresAclNative','New-PrivateFreshPostgresDirectory','Test-FreshPostgresParentDescriptor','Assert-FreshPostgresParentBoundary','Assert-FreshPostgresDataAcl')){
        $defs=@($aclAst.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $functionName},$true))
        if($defs.Count -ne 1){throw 'Candidate ACL function absent or ambiguous.'}
        . ([scriptblock]::Create($defs[0].Extent.Text))
    }
    Initialize-FreshPostgresAclNative
    $parentDir=Split-Path -Parent $dataDir
    if(-not (Test-Path -LiteralPath $parentDir)){
        $null=New-PrivateFreshPostgresDirectory $parentDir
    }
    $ancestorStack=[Collections.Generic.Stack[string]]::new()
    $cursor=$parentDir
    while($cursor){
        $ancestorStack.Push($cursor)
        $parent=Split-Path -Parent $cursor
        if($parent -eq $cursor){break}
        $cursor=$parent
    }
    while($ancestorStack.Count){$preinstallLocks.Add([EsteriumFreshPgAclNative]::Open($ancestorStack.Pop(),$true,$false))}
    Assert-FreshPostgresParentBoundary $dataDir
    $freshIdentity=New-PrivateFreshPostgresDirectory $dataDir
    $freshRootHandle=[EsteriumFreshPgAclNative]::Open($dataDir,$true,$false)
    $preinstallLocks.Add($freshRootHandle)
    if([EsteriumFreshPgAclNative]::Identity($freshRootHandle,$true) -cne $freshIdentity){throw 'Fresh root changed before vendor invocation.'}
    Write-Output 'PASS: atomically created protected empty EDB data directory.'
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
    $verifyRoot=[EsteriumFreshPgAclNative]::Open($dataDir,$true,$false)
    try{
        if([EsteriumFreshPgAclNative]::Identity($verifyRoot,$true) -cne $freshIdentity -or [EsteriumFreshPgAclNative]::Identity($freshRootHandle,$true) -cne $freshIdentity){throw 'EDB did not preserve the pre-created directory identity.'}
    }finally{$verifyRoot.Dispose()}
    Write-Output 'PASS: EDB preserved the protected directory physical identity.'
    Assert-FreshPostgresParentBoundary $dataDir
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
        # Apply only after an equivalent loopback baseline has passed.
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
    # Use an explicit argument vector and a private child-process libpq
    # environment. Do not inherit runner PG* settings or write credentials
    # to the command line, stdout, stderr or global environment.
    function Invoke-DisposablePgClient([string]$Executable,[string[]]$Arguments) {
        if($Executable -cne $readyExe -and $Executable -cne $psql){throw 'Unexpected EDB client executable.'}
        $info=[Diagnostics.ProcessStartInfo]::new()
        $info.FileName=$Executable
        $info.WorkingDirectory=Split-Path -Parent $Executable
        $info.UseShellExecute=$false
        $info.CreateNoWindow=$true
        $info.RedirectStandardOutput=$true
        $info.RedirectStandardError=$true
        foreach($key in @($info.Environment.Keys | Where-Object {$_ -match '^PG'})){[void]$info.Environment.Remove($key)}
        $info.Environment['PGPASSFILE']=$probePassFile
        $info.Environment['PGCONNECT_TIMEOUT']='2'
        $info.Environment['PGSSLMODE']='disable'
        $info.Environment['PG_COLOR']='never'
        foreach($argument in $Arguments){[void]$info.ArgumentList.Add($argument)}
        $process=[Diagnostics.Process]::Start($info)
        if($null -eq $process){throw 'Disposable EDB client could not launch.'}
        try{
            $stdout=$process.StandardOutput.ReadToEndAsync()
            $stderr=$process.StandardError.ReadToEndAsync()
            if(-not $process.WaitForExit(12000)){
                $process.Kill()
                $process.WaitForExit()
                throw 'Disposable EDB client process timed out.'
            }
            $output=$stdout.GetAwaiter().GetResult()
            $errorText=$stderr.GetAwaiter().GetResult()
            $reason='none'
            if($process.ExitCode -ne 0){
                $reason='other'
                if($errorText -match '(?i)invalid.*(option|parameter|connection|value)|unrecognized option|too many command.line arguments|missing.*argument'){$reason='invalid-parameters'}
                elseif($errorText -match '(?i)password authentication failed|no password supplied'){$reason='authentication'}
                elseif($errorText -match '(?i)connection refused|timeout expired|could not connect'){$reason='connection'}
                elseif($errorText -match '(?i)SSL|GSSAPI'){$reason='transport'}
                elseif($errorText -match '(?i)permission denied|access is denied'){$reason='permission'}
            }
            return [pscustomobject]@{Code=[int]$process.ExitCode;Output=$output.Trim();Reason=$reason}
        }finally{$process.Dispose()}
    }

    $connInfo="host=127.0.0.1 port=$port user=postgres dbname=postgres sslmode=disable connect_timeout=2"
    function Write-DisposableEdbStartDiagnostic([datetime]$Since,[string]$Phase,[int]$ProbeCode) {
        $record=Get-CimInstance Win32_Service -Filter "Name='$svc'" -ErrorAction SilentlyContinue
        if($null -ne $record){
            $state=if($record.State -in @('Running','Stopped','Start Pending','Stop Pending')){$record.State}else{'other'}
            Write-Output ('EDB start diagnostic phase='+$Phase+' state='+$state+' win32='+[int]$record.ExitCode+' specific='+[int]$record.ServiceSpecificExitCode+' probe='+$ProbeCode)
        }
        $listeners=@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue)
        Write-Output ('EDB start diagnostic expected-port listeners='+$listeners.Count+' loopback='+@($listeners | Where-Object {$_.LocalAddress -eq '127.0.0.1'}).Count)
        $messages=[Collections.Generic.List[string]]::new()
        foreach($event in @(Get-WinEvent -FilterHashtable @{LogName='Application';StartTime=$Since} -ErrorAction SilentlyContinue | Where-Object {$_.ProviderName -match '^(PostgreSQL|pg_ctl|'+[regex]::Escape($svc)+')$'} | Select-Object -First 30)){
            $messages.Add([string]$event.Message)
        }
        foreach($logDir in @((Join-Path $dataDir 'log'),(Join-Path $dataDir 'pg_log'))){
            if((Test-Path -LiteralPath $logDir -PathType Container) -and -not ((Get-Item -LiteralPath $logDir -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){
                foreach($logFile in @(Get-ChildItem -LiteralPath $logDir -File -ErrorAction SilentlyContinue | Where-Object {$_.LastWriteTime -ge $Since -and -not ($_.Attributes -band [IO.FileAttributes]::ReparsePoint)} | Sort-Object LastWriteTime -Descending | Select-Object -First 3)){
                    foreach($line in @(Get-Content -LiteralPath $logFile.FullName -Tail 80 -ErrorAction SilentlyContinue)){$messages.Add([string]$line)}
                }
            }
        }
        $categories=[ordered]@{
            'permission-denied'='(?i)permission denied|access is denied|access denied'
            'invalid-data-permissions'='(?i)data directory.*(invalid permissions|wrong ownership)'
            'configuration-error'='(?i)configuration file.*(error|invalid)|syntax error|unrecognized configuration parameter'
            'bind-error'='(?i)could not bind|could not create.*socket|address already in use'
            'lock-file-error'='(?i)could not (create|open|write).*lock file'
            'recovery'='(?i)database system was interrupted|redo starts|recovery is in progress'
            'ready-message'='(?i)database system is ready to accept connections'
            'fatal'='(?i)\bFATAL\b|\bPANIC\b'
        }
        foreach($category in $categories.Keys){
            $count=@($messages | Where-Object {$_ -match $categories[$category]}).Count
            if($count){Write-Output ('EDB diagnostic category='+$category+' count='+$count)}
        }
        Write-Output ('EDB diagnostic inspected messages='+$messages.Count)
    }

    function Start-DisposableEdbAndCheck([string]$Phase) {
        $since=Get-Date
        $probeCode=-1
        try{
            Set-Service -Name $svc -StartupType Manual -ErrorAction Stop
            Start-Service -Name $svc -ErrorAction Stop
            (Get-Service -Name $svc).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(60))
            $ready=$false
            for($attempt=0;$attempt -lt 30;$attempt++){
                $probe=Invoke-DisposablePgClient $readyExe @(('--dbname=' + $connInfo),'--timeout=2','--quiet')
                $probeCode=$probe.Code
                if($probeCode -eq 0){$ready=$true;break}
                if($probeCode -eq 3){
                    Write-Output ('EDB readiness client no-attempt category='+$probe.Reason)
                    $sqlProbe=Invoke-DisposablePgClient $psql @('-X','-q','-t','-A','-w',('--dbname='+$connInfo),'-v','ON_ERROR_STOP=1','-c','SELECT 1')
                    Write-Output ('EDB independent SQL probe code='+$sqlProbe.Code+' category='+$sqlProbe.Reason+' exact-result='+($sqlProbe.Code -eq 0 -and $sqlProbe.Output -ceq '1'))
                    throw 'EDB readiness client did not attempt connection.'
                }
                Start-Sleep -Milliseconds 500
            }
            if(-not $ready){throw 'Disposable EDB cluster not ready.'}
        }catch{
            try{Write-DisposableEdbStartDiagnostic $since $Phase $probeCode}catch{Write-Warning 'EDB diagnostic collection incomplete.'}
            throw
        }
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
        $result=Invoke-DisposablePgClient $psql @('-X','-q','-t','-A','-w',('--dbname='+$connInfo),'-v','ON_ERROR_STOP=1','-c',$Sql)
        if($result.Code -ne 0){throw ('Disposable EDB SQL failed: code='+$result.Code+' category='+$result.Reason)}
        return $result.Output
    }
    # This is a loopback-only vendor fixture, not commercial TLS.
    Start-DisposableEdbAndCheck 'before-normalization'
    if((Invoke-DisposableSql 'SELECT 1') -cne '1'){throw 'EDB baseline SQL failed.'}
    Stop-DisposableEdbAndCheck
    Write-Output 'PASS: vendor EDB baseline loopback SQL before ACL normalization.'
    if(-not $aclOk){Normalize-DisposableEdbAcl $dataDir $serviceSid $id}
    Start-DisposableEdbAndCheck 'after-normalization'
    if((Invoke-DisposableSql "SHOW listen_addresses") -cne '127.0.0.1'){throw 'SQL reports unexpected listener configuration.'}
    $null=Invoke-DisposableSql "CREATE TABLE ci_acl_durable (id integer PRIMARY KEY, payload text NOT NULL); INSERT INTO ci_acl_durable VALUES (1,'acl-normalized');"
    if((Invoke-DisposableSql "SELECT payload FROM ci_acl_durable WHERE id=1") -cne 'acl-normalized'){throw 'Initial SQL persistence failed.'}
    Stop-DisposableEdbAndCheck
    Start-DisposableEdbAndCheck 'after-restart'
    if((Invoke-DisposableSql "SELECT payload FROM ci_acl_durable WHERE id=1") -cne 'acl-normalized'){throw 'SQL durability after restart failed.'}
    Stop-DisposableEdbAndCheck
    Assert-FreshPostgresDataAcl $dataDir $serviceSid
    Write-Output 'PASS: normalized real EDB service ran under dedicated SID; committed SQL survived restart and recursive ACL recheck.'


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
    for($i=$preinstallLocks.Count-1;$i -ge 0;$i--){$preinstallLocks[$i].Dispose()}
    if (Test-Path -LiteralPath $probePassFile -PathType Leaf) { Remove-Item -LiteralPath $probePassFile -Force -ErrorAction Stop }
    if (Test-Path -LiteralPath $options -PathType Leaf) {
        Remove-Item -LiteralPath $options -Force -ErrorAction Stop
    }
}
