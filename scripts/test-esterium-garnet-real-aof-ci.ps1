#requires -Version 5.1
[CmdletBinding()]
param([switch]$FromIntegratedStage,[switch]$RunERPStartupSmoke)
# Real Garnet smoke: disposable Windows CI, pinned upstream ZIP, no ERP data.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Garnet real AOF smoke only supports disposable hosted Windows CI.'
}
if($RunERPStartupSmoke -and (-not $FromIntegratedStage -or
   $env:ESTERIUM_CI_DATABASE_URL_FILE -notmatch 'database-url(?:-first-admin)?\.secret)){
    throw 'ERP startup smoke requires the integrated PostgreSQL secret handoff.'
}
$id=[Guid]::NewGuid().ToString('N')
$root=Join-Path $env:RUNNER_TEMP ('esterium-garnet-aof-'+$id)
$destination=Join-Path $env:ProgramData ('Esterium\GarnetAofCI-'+$id)
$zip=Join-Path $root 'garnet.zip'
$nssmZip=Join-Path $root 'nssm.zip'
$nssmDir=Join-Path $root 'nssm'
$nssmExe=Join-Path $nssmDir 'nssm-2.24-101-g897c7ad\\win64\\nssm.exe'
$nssmService='EstGarCI'+$id.Substring(0,12)
$nssmRegistered=$false
$bin=Join-Path $root 'bin'
$acl=Join-Path $root 'garnet.acl'
$pfx=Join-Path $root 'garnet.pfx'
$pfxPassword=Join-Path $root 'pfx.secret'
$fresh=Join-Path $root 'fresh.conf'
$password='ci-only-garnet-'+$id
$cert=$null
$server=$null
$thumbprint=''
$port=64325
$env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI='1'
function Protect-Fixture([string]$path,[bool]$networkRead) {
    $grants=@('/inheritance:r','/grant:r','*S-1-5-18:F','*S-1-5-32-544:F')
    if ($networkRead) { $grants+='*S-1-5-20:R' }
    & icacls.exe $path @grants | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to restrict ephemeral Garnet fixture.' }
}
function Read-Reply([IO.StreamReader]$reader) {
    $line=$reader.ReadLine()
    if ([string]::IsNullOrEmpty($line)) { throw 'Missing Redis response over TLS.' }
    if ($line.StartsWith('-')) { throw ('Redis command rejected: '+$line.Substring(1)) }
    if ($line.StartsWith('+') -or $line.StartsWith(':')) { return $line.Substring(1) }
    if (-not $line.StartsWith('$')) { throw 'Unexpected Redis reply format.' }
    $n=[int]::Parse($line.Substring(1))
    if ($n -lt 0) { return $null }
    if ($n -gt 8192) { throw 'Redis reply too large.' }
    $chars=New-Object char[] $n
    $offset=0
    while ($offset -lt $n) {
        $count=$reader.Read($chars,$offset,$n-$offset)
        if ($count -le 0) { throw 'Truncated Redis reply.' }
        $offset+=$count
    }
    if ($reader.Read() -ne 13 -or $reader.Read() -ne 10) { throw 'Bad Redis line termination.' }
    return (-join $chars)
}
function Send-Redis([IO.Stream]$stream,[IO.StreamReader]$reader,[string[]]$items) {
    $nl=[string][char]13+[string][char]10
    $builder=[Text.StringBuilder]::new()
    [void]$builder.Append('*').Append($items.Count).Append($nl)
    foreach ($item in $items) {
        [void]$builder.Append('$').Append([Text.Encoding]::UTF8.GetByteCount($item)).Append($nl)
        [void]$builder.Append($item).Append($nl)
    }
    $bytes=[Text.Encoding]::UTF8.GetBytes($builder.ToString())
    $stream.Write($bytes,0,$bytes.Length)
    $stream.Flush()
    return (Read-Reply $reader)
}
function Assert-RedisDenied([IO.Stream]$stream,[IO.StreamReader]$reader,[string[]]$command) {
    $rejected=$false
    try { Send-Redis $stream $reader $command | Out-Null }
    catch {
        if ($_.Exception.Message -match '^Redis command rejected:') { $rejected=$true }
        else { throw }
    }
    if (-not $rejected) {
        throw 'Garnet accepted a command without a valid authenticated user.'
    }
}
function Open-Redis {
    $tcp=[Net.Sockets.TcpClient]::new()
    try {
        $tcp.ReceiveTimeout=5000
        $tcp.SendTimeout=5000
        $tcp.Connect('127.0.0.1',$port)
        $callback=[Net.Security.RemoteCertificateValidationCallback]{
            param($sender,$crt,$chain,$errors)
            return $null -ne $crt -and $crt.GetCertHashString() -eq $script:thumbprint
        }
        $ssl=[Net.Security.SslStream]::new($tcp.GetStream(),$false,$callback)
        $ssl.AuthenticateAsClient('localhost')
        $reader=[IO.StreamReader]::new($ssl,[Text.Encoding]::UTF8,$false,4096,$true)
        return @{tcp=$tcp;ssl=$ssl;reader=$reader}
    } catch {
        $tcp.Dispose()
        throw
    }
}
function Start-Garnet([string]$conf,[string]$phase) {
    $exes=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.exe')
    if ($exes.Count -ne 1) { throw 'Expected exactly one GarnetServer.exe.' }
    $out=Join-Path $root ($phase+'.out.log')
    $err=Join-Path $root ($phase+'.err.log')
    $proc=Start-Process -FilePath $exes[0].FullName -WorkingDirectory $exes[0].DirectoryName -ArgumentList @('--config-import-path',('"'+$conf+'"')) -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
    for ($attempt=0;$attempt -lt 40;$attempt++) {
        $proc.Refresh()
        if ($proc.HasExited) { break }
        $tcp=[Net.Sockets.TcpClient]::new()
        try {
            $task=$tcp.ConnectAsync('127.0.0.1',$port)
            if ($task.Wait(500) -and $tcp.Connected) { return $proc }
        } catch { } finally { $tcp.Dispose() }
        Start-Sleep -Milliseconds 900
    }
    $proc.Refresh()
    $diagnostic=if ($proc.HasExited) { 'vendor process exited '+$proc.ExitCode } else { 'vendor process stayed running but did not listen' }
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
    $proc.WaitForExit()
    Write-Host ('Garnet CI diagnostic: '+$diagnostic)
    foreach ($log in @($out,$err)) {
        if (Test-Path -LiteralPath $log) {
            Write-Host ('Garnet CI log '+[IO.Path]::GetFileName($log)+' bytes='+((Get-Item -LiteralPath $log).Length))
            foreach ($line in @(Get-Content -LiteralPath $log -Tail 35)) { Write-Host $line }
        }
    }
    throw "Garnet $phase failed to bind its local TLS port."
}
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    # Service binaries and TLS fixture must be traversable by NetworkService;
    # only the disposable runner hosts this ACL. Secrets are separately ACL
    # protected and are NOT packaged into customer releases.
    & icacls.exe $root '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not protect disposable Garnet service fixture root.' }
    [IO.File]::WriteAllText($acl,('user default off'+[Environment]::NewLine+'user esteriumcache on >'+$password+' ~* +@all'))
    [IO.File]::WriteAllText($pfxPassword,('ci-only-pfx-'+$id))
    $cert=New-SelfSignedCertificate -DnsName localhost -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddDays(3) -ErrorAction Stop
    $thumbprint=$cert.Thumbprint
    $garnetPublicCert=Join-Path $root 'garnet-server.cer'
    Export-Certificate -Cert $cert -FilePath $garnetPublicCert -ErrorAction Stop | Out-Null
    Protect-Fixture $garnetPublicCert $true
    $secure=ConvertTo-SecureString ([IO.File]::ReadAllText($pfxPassword)) -AsPlainText -Force
    Export-PfxCertificate -Cert $cert -FilePath $pfx -Password $secure -ErrorAction Stop | Out-Null
    foreach ($file in @($acl,$pfx,$pfxPassword)) { Protect-Fixture $file $true }
    & (Join-Path $PSScriptRoot 'provision-esterium-garnet-config.ps1') -AclFile $acl -CertificatePfxFile $pfx -CertificatePasswordFile $pfxPassword -DestinationDir $destination -Port $port -Apply -UnsignedDisposableCIFixture | Out-Null
    $confPath=Join-Path $destination 'garnet.conf'
    $config=Get-Content -LiteralPath $confPath -Raw | ConvertFrom-Json
    if (-not $config.EnableAOF -or -not $config.WaitForCommit -or
        -not $config.Recover -or -not $config.FailOnRecoveryError) {
        throw 'Generated Garnet durability configuration is incomplete.'
    }
    # Fresh profile is a transient *CI-only* exception. The service config
    # stays Recover=true; production first-run is never automatically enabled.
    $config.Recover=$false
    [IO.File]::WriteAllText($fresh,($config | ConvertTo-Json -Depth 5))
    Protect-Fixture $fresh $false
    if($FromIntegratedStage){
        if($env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1' -or
           $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1'){
            throw 'Integrated Garnet smoke requires the disposable hosted Windows MIT setup CI guards.'
        }
        $sourceZip=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage\garnet.zip'
        $sourceNssm=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage\nssm.exe'
        if(-not(Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage\bundle-manifest.json') -PathType Leaf) -or
           -not(Test-Path -LiteralPath $sourceZip -PathType Leaf) -or
           -not(Test-Path -LiteralPath $sourceNssm -PathType Leaf) -or
           (Get-FileHash -LiteralPath $sourceNssm -Algorithm SHA256).Hash -cne
             'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'){
            throw 'Protected native EXE Garnet/NSSM staged bytes not present or pin invalid.'
        }
        Copy-Item -LiteralPath $sourceZip -Destination $zip -ErrorAction Stop
        # The original vendor staging is admin-only by design. A service
        # running as NetworkService must get RX, never write, to the NSSM
        # executable. Copy pinned bytes into this isolated CI fixture whose
        # parent has explicit NetworkService RX, rather than relaxing the
        # protected publisher staging ACL.
        $nssmExe=Join-Path $root 'nssm.exe'
        Copy-Item -LiteralPath $sourceNssm -Destination $nssmExe -ErrorAction Stop
        if((Get-FileHash -LiteralPath $nssmExe -Algorithm SHA256).Hash -cne
          'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848') {
            throw 'Copied native-exe NSSM file failed independent SHA-256 verification.'
        }
        Write-Output 'PASS: Garnet CI uses the Garnet/NSSM bytes extracted from the native MIT EXE.'
    }else{
        $url='https://github.com/microsoft/garnet/releases/download/v2.2.3/win-x64-based-readytorun.zip'
        Invoke-WebRequest -Uri $url -OutFile $zip -ErrorAction Stop
    }
    if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -cne
        '5cd130686ab415b13c59d36d048a6faded81e707ffe6da5953f98beeb79f9f16') {
        throw 'Upstream official Garnet ZIP hash differs from independently pinned CI fixture.'
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $bin -ErrorAction Stop
    $server=Start-Garnet $fresh 'fresh'
    $connection=Open-Redis
    try {
        Assert-RedisDenied $connection.ssl $connection.reader @('PING')
        Assert-RedisDenied $connection.ssl $connection.reader @('AUTH','esteriumcache','wrong-ci-password')
        if ((Send-Redis $connection.ssl $connection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Garnet refused its authenticated user.'
        }
        if ((Send-Redis $connection.ssl $connection.reader @('SET',('esterium:smoke:'+$id),'committed-value')) -ne 'OK') {
            throw 'Durable test write was not acknowledged.'
        }
        $luaKey='esterium:lua:'+ $id
        if ((Send-Redis $connection.ssl $connection.reader @('EVAL',"return redis.call('INCR', KEYS[1])",'1',$luaKey)) -ne '1') {
            throw 'Garnet did not execute the required atomic Redis Lua counter.'
        }
    } finally {
        $connection.reader.Dispose();$connection.ssl.Dispose();$connection.tcp.Dispose()
    }
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
    $server=$null
    $server=Start-Garnet $confPath 'recovered'
    $connection=Open-Redis
    try {
        if ((Send-Redis $connection.ssl $connection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Recovered Garnet refused its authenticated user.'
        }
        $value=Send-Redis $connection.ssl $connection.reader @('GET',('esterium:smoke:'+$id))
        if ($value -ne 'committed-value') {
            throw 'Garnet failed to recover an AOF-acknowledged write after a forced crash.'
        }
        if ((Send-Redis $connection.ssl $connection.reader @('GET',('esterium:lua:'+$id))) -ne '1') {
            throw 'Atomic Lua write disappeared after AOF recovery.'
        }
    } finally {
        $connection.reader.Dispose();$connection.ssl.Dispose();$connection.tcp.Dispose()
    }
    # The console process passed AOF recovery. Now prove exactly the same
    # protected Garnet executable/configuration under REAL Windows SCM/NSSM.
    # NSSM 2.24-101 is the Windows 10+ compatible upstream pre-release;
    # the SHA-256 was independently pinned by WinGet's version manifest.
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
    $server=$null
    if(-not $FromIntegratedStage){
        $nssmHash='99F5045FFFBFFB745D67FE3A065A953C4A3D9C253B868892D9B685B0EE7D07B8'
        $cacheRoot=Join-Path $env:GITHUB_WORKSPACE '.vendor-cache'
        $cacheFile=Join-Path $cacheRoot 'nssm-2.24-101-g897c7ad.zip'
        $downloaded=$false
        # GitHub Actions cache is an UNTRUSTED byte source. Its content must pass
        # the same independently pinned vendor SHA-256 as direct upstream bytes.
        if (Test-Path -LiteralPath $cacheFile -PathType Leaf) {
            $cacheHash=(Get-FileHash -LiteralPath $cacheFile -Algorithm SHA256).Hash
            if ($cacheHash -cne $nssmHash) {
                throw 'Cached NSSM ZIP differs from approved upstream SHA-256; refuse cache and installation.'
            }
            Copy-Item -LiteralPath $cacheFile -Destination $nssmZip -ErrorAction Stop
            $downloaded=$true
            Write-Output 'PASS: reused SHA-256 verified official NSSM ZIP from ephemeral CI cache.'
        }
        # Publisher-only fallback for a cold cache. The downloaded binary is
        # cached only AFTER immutable SHA-256 verification and a full successful
        # live Windows service test; it is never signed on behalf of the vendor.
        if (-not $downloaded) {
          foreach ($nssmUrl in @(
            'https://www.nssm.cc/ci/nssm-2.24-101-g897c7ad.zip',
            'https://nssm.cc/ci/nssm-2.24-101-g897c7ad.zip'
        )) {
            for ($attempt=0; $attempt -lt 3; $attempt++) {
                try {
                    if (Test-Path -LiteralPath $nssmZip) {
                        Remove-Item -LiteralPath $nssmZip -Force
                    }
                    Invoke-WebRequest -Uri $nssmUrl -OutFile $nssmZip -TimeoutSec 90 -ErrorAction Stop
                    $downloaded=$true
                    break
                } catch {
                    if ($attempt -lt 2) { Start-Sleep -Seconds @(2,6,14)[$attempt] }
                }
            }
            if ($downloaded) { break }
          }
        }
        if (-not $downloaded) {
            throw 'Publisher unavailable and no verified NSSM cache exists; service proof is blocked.'
        }
        if ((Get-FileHash -LiteralPath $nssmZip -Algorithm SHA256).Hash -cne $nssmHash) {
            throw 'NSSM package differs from independently approved upstream WinGet SHA-256.'
        }
        Expand-Archive -LiteralPath $nssmZip -DestinationPath $nssmDir -ErrorAction Stop
        if (-not (Test-Path -LiteralPath $nssmExe -PathType Leaf)) {
            throw 'Pinned NSSM ZIP has no required Windows x64 executable.'
        }
    }else{
        if(-not (Test-Path -LiteralPath $nssmExe -PathType Leaf) -or
           (Get-FileHash -LiteralPath $nssmExe -Algorithm SHA256).Hash -cne
             'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'){
            throw 'NSSM from native MIT EXE does not match the approved x64 SHA-256.'
        }
    }
    if ($null -ne (Get-Service -Name $nssmService -ErrorAction SilentlyContinue)) {
        throw 'Disposable Garnet service identity is already registered.'
    }
    $garnetExe=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.exe')
    if ($garnetExe.Count -ne 1) { throw 'Expected one pinned GarnetServer.exe for SCM.' }
    & $nssmExe install $nssmService $garnetExe[0].FullName '--config-import-path' $confPath | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'NSSM did not register Garnet Windows service.' }
    $nssmRegistered=$true
    foreach ($cmd in @(
        @('set',$nssmService,'ObjectName','NT AUTHORITY\NetworkService'),
        @('set',$nssmService,'Start','SERVICE_DISABLED'),
        @('set',$nssmService,'AppDirectory',$garnetExe[0].DirectoryName),
        @('set',$nssmService,'AppExit','Default','Restart'),
        @('set',$nssmService,'AppRestartDelay','5000')
    )) {
        & $nssmExe @cmd | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'NSSM refused restricted Garnet SCM settings.' }
    }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$nssmService'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].StartMode -cne 'Disabled' -or
        $scm[0].State -cne 'Stopped' -or $scm[0].StartName -notin
        @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE')) {
        throw 'NSSM Garnet service is not disabled/stopped with NetworkService identity.'
    }
    # Only on an ephemeral CI runner, turn the previously disabled fixture on
    # long enough to validate its real TLS/ACL/AOF runtime and SCM transitions.
    Set-Service -Name $nssmService -StartupType Manual -ErrorAction Stop
    Start-Service -Name $nssmService -ErrorAction Stop
    (Get-Service -Name $nssmService).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    $ready=$false
    for($attempt=0; $attempt -lt 45; $attempt++) {
        $tcp=[Net.Sockets.TcpClient]::new()
        try {
            $task=$tcp.ConnectAsync('127.0.0.1',$port)
            if ($task.Wait(400) -and $tcp.Connected) { $ready=$true;break }
        } catch {} finally { $tcp.Dispose() }
        Start-Sleep -Milliseconds 800
    }
    if (-not $ready) { throw 'NSSM Garnet service did not expose local TLS on its expected port.' }
    $serviceConnection=Open-Redis
    try {
        Assert-RedisDenied $serviceConnection.ssl $serviceConnection.reader @('PING')
        if ((Send-Redis $serviceConnection.ssl $serviceConnection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Real NetworkService Garnet could not use its protected ACL.'
        }
        if ((Send-Redis $serviceConnection.ssl $serviceConnection.reader @('GET',('esterium:smoke:'+$id))) -ne 'committed-value') {
            throw 'NSSM-wrapped Garnet lost its durable AOF data.'
        }
    } finally {
        $serviceConnection.reader.Dispose()
        $serviceConnection.ssl.Dispose()
        $serviceConnection.tcp.Dispose()
    }
    if($RunERPStartupSmoke){
        $labKeys=@('ESTERIUM_CI_GARNET_PASSWORD','ESTERIUM_CI_GARNET_CERT_FILE','ESTERIUM_CI_GARNET_PORT','ESTERIUM_CI_GARNET_SERVICE')
        $oldLab=@{}
        foreach($key in $labKeys){$oldLab[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
        try{
            $env:ESTERIUM_CI_GARNET_PASSWORD=$password
            $env:ESTERIUM_CI_GARNET_CERT_FILE=$garnetPublicCert
            $env:ESTERIUM_CI_GARNET_PORT=[string]$port
            $env:ESTERIUM_CI_GARNET_SERVICE=$nssmService
            & (Join-Path $PSScriptRoot '..\windows-setup\test-oss-erp-runtime-ci.ps1')
            if(-not $?){throw 'Live ERP service start, login or dependency restart smoke failed.'}
        }finally{
            foreach($entry in $oldLab.GetEnumerator()){[Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')}
        }
    }
    Stop-Service -Name $nssmService -ErrorAction Stop
    (Get-Service -Name $nssmService).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
    Set-Service -Name $nssmService -StartupType Disabled -ErrorAction Stop
    # SCM can report 'Stopped' slightly before the wrapped child closes TCP.
    $portClosed=$false
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        if (@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -eq 0) {
            $portClosed=$true
            break
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $portClosed) { throw 'Stopped NSSM wrapper left the child Garnet TLS port open.' }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$nssmService'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].State -cne 'Stopped' -or
        $scm[0].StartMode -cne 'Disabled' -or [int]$scm[0].ProcessId -ne 0 -or
        @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -ne 0) {
        throw 'SCM service shutdown left Garnet running or its local TLS port open.'
    }
    # The integrated path uses the already SHA-256-approved NSSM executable
    # provided by the MIT EXE. It has NO upstream NSSM archive or cacheFile;
    # do not read undefined variables or fabricate an archive from the EXE.
    if (-not $FromIntegratedStage) {
        if (-not (Test-Path -LiteralPath $cacheFile)) {
            if (Test-Path -LiteralPath $cacheRoot) {
                $cacheDir=Get-Item -LiteralPath $cacheRoot -Force -ErrorAction Stop
                if (($cacheDir.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                    throw 'Refuse NSSM CI cache through a symbolic link or junction.'
                }
            } else {
                [IO.Directory]::CreateDirectory($cacheRoot) | Out-Null
            }
            # Copy only after the genuine SCM/TLS/ACL/AOF proof passed.
            Copy-Item -LiteralPath $nssmZip -Destination $cacheFile -ErrorAction Stop
            if ((Get-FileHash -LiteralPath $cacheFile -Algorithm SHA256).Hash -cne $nssmHash) {
                throw 'NSSM vendor cache bytes changed while preparing GitHub upload.'
            }
        }
    }
    Write-Output 'PASS: upstream WinGet-pinned NSSM runs authenticated Garnet TLS under NetworkService and stops disabled with no listener.'
    Write-Output 'PASS: pinned Garnet TLS, default user rejection, ACL, atomic Lua and AOF recovery survived hard Windows restart.'
} finally {
    if ($nssmRegistered) {
        # Never leave a CI service startable on error; only this randomly named
        # fixture can be removed, never any pre-existing Windows service.
        try {
            $svc=Get-Service -Name $nssmService -ErrorAction SilentlyContinue
            if ($null -ne $svc) {
                Set-Service -Name $nssmService -StartupType Disabled -ErrorAction Stop
                if ($svc.Status -ne 'Stopped') {
                    Stop-Service -Name $nssmService -Force -ErrorAction Stop
                    (Get-Service -Name $nssmService).WaitForStatus(
                        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
                }
                & $nssmExe remove $nssmService confirm | Out-Null
                if ($LASTEXITCODE -ne 0) { throw 'NSSM did not remove disposable Garnet CI service.' }
            }
        } catch {
            Write-Warning 'NSSM CI service cleanup failed: discard this hosted runner.'
            throw
        }
    }
    if ($null -ne $server) {
        $server.Refresh()
        if (-not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
    }
    if ($null -ne $cert) {
        Remove-Item -LiteralPath ('Cert:\CurrentUser\My\'+$thumbprint) -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI -ErrorAction SilentlyContinue
    foreach ($path in @($destination,$root)) {
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue }
    }
}
)){
    throw 'ERP startup smoke requires the integrated PostgreSQL secret handoff.'
}
$id=[Guid]::NewGuid().ToString('N')
$root=Join-Path $env:RUNNER_TEMP ('esterium-garnet-aof-'+$id)
$destination=Join-Path $env:ProgramData ('Esterium\GarnetAofCI-'+$id)
$zip=Join-Path $root 'garnet.zip'
$nssmZip=Join-Path $root 'nssm.zip'
$nssmDir=Join-Path $root 'nssm'
$nssmExe=Join-Path $nssmDir 'nssm-2.24-101-g897c7ad\\win64\\nssm.exe'
$nssmService='EstGarCI'+$id.Substring(0,12)
$nssmRegistered=$false
$bin=Join-Path $root 'bin'
$acl=Join-Path $root 'garnet.acl'
$pfx=Join-Path $root 'garnet.pfx'
$pfxPassword=Join-Path $root 'pfx.secret'
$fresh=Join-Path $root 'fresh.conf'
$password='ci-only-garnet-'+$id
$cert=$null
$server=$null
$thumbprint=''
$port=64325
$env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI='1'
function Protect-Fixture([string]$path,[bool]$networkRead) {
    $grants=@('/inheritance:r','/grant:r','*S-1-5-18:F','*S-1-5-32-544:F')
    if ($networkRead) { $grants+='*S-1-5-20:R' }
    & icacls.exe $path @grants | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to restrict ephemeral Garnet fixture.' }
}
function Read-Reply([IO.StreamReader]$reader) {
    $line=$reader.ReadLine()
    if ([string]::IsNullOrEmpty($line)) { throw 'Missing Redis response over TLS.' }
    if ($line.StartsWith('-')) { throw ('Redis command rejected: '+$line.Substring(1)) }
    if ($line.StartsWith('+') -or $line.StartsWith(':')) { return $line.Substring(1) }
    if (-not $line.StartsWith('$')) { throw 'Unexpected Redis reply format.' }
    $n=[int]::Parse($line.Substring(1))
    if ($n -lt 0) { return $null }
    if ($n -gt 8192) { throw 'Redis reply too large.' }
    $chars=New-Object char[] $n
    $offset=0
    while ($offset -lt $n) {
        $count=$reader.Read($chars,$offset,$n-$offset)
        if ($count -le 0) { throw 'Truncated Redis reply.' }
        $offset+=$count
    }
    if ($reader.Read() -ne 13 -or $reader.Read() -ne 10) { throw 'Bad Redis line termination.' }
    return (-join $chars)
}
function Send-Redis([IO.Stream]$stream,[IO.StreamReader]$reader,[string[]]$items) {
    $nl=[string][char]13+[string][char]10
    $builder=[Text.StringBuilder]::new()
    [void]$builder.Append('*').Append($items.Count).Append($nl)
    foreach ($item in $items) {
        [void]$builder.Append('$').Append([Text.Encoding]::UTF8.GetByteCount($item)).Append($nl)
        [void]$builder.Append($item).Append($nl)
    }
    $bytes=[Text.Encoding]::UTF8.GetBytes($builder.ToString())
    $stream.Write($bytes,0,$bytes.Length)
    $stream.Flush()
    return (Read-Reply $reader)
}
function Assert-RedisDenied([IO.Stream]$stream,[IO.StreamReader]$reader,[string[]]$command) {
    $rejected=$false
    try { Send-Redis $stream $reader $command | Out-Null }
    catch {
        if ($_.Exception.Message -match '^Redis command rejected:') { $rejected=$true }
        else { throw }
    }
    if (-not $rejected) {
        throw 'Garnet accepted a command without a valid authenticated user.'
    }
}
function Open-Redis {
    $tcp=[Net.Sockets.TcpClient]::new()
    try {
        $tcp.ReceiveTimeout=5000
        $tcp.SendTimeout=5000
        $tcp.Connect('127.0.0.1',$port)
        $callback=[Net.Security.RemoteCertificateValidationCallback]{
            param($sender,$crt,$chain,$errors)
            return $null -ne $crt -and $crt.GetCertHashString() -eq $script:thumbprint
        }
        $ssl=[Net.Security.SslStream]::new($tcp.GetStream(),$false,$callback)
        $ssl.AuthenticateAsClient('localhost')
        $reader=[IO.StreamReader]::new($ssl,[Text.Encoding]::UTF8,$false,4096,$true)
        return @{tcp=$tcp;ssl=$ssl;reader=$reader}
    } catch {
        $tcp.Dispose()
        throw
    }
}
function Start-Garnet([string]$conf,[string]$phase) {
    $exes=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.exe')
    if ($exes.Count -ne 1) { throw 'Expected exactly one GarnetServer.exe.' }
    $out=Join-Path $root ($phase+'.out.log')
    $err=Join-Path $root ($phase+'.err.log')
    $proc=Start-Process -FilePath $exes[0].FullName -WorkingDirectory $exes[0].DirectoryName -ArgumentList @('--config-import-path',('"'+$conf+'"')) -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
    for ($attempt=0;$attempt -lt 40;$attempt++) {
        $proc.Refresh()
        if ($proc.HasExited) { break }
        $tcp=[Net.Sockets.TcpClient]::new()
        try {
            $task=$tcp.ConnectAsync('127.0.0.1',$port)
            if ($task.Wait(500) -and $tcp.Connected) { return $proc }
        } catch { } finally { $tcp.Dispose() }
        Start-Sleep -Milliseconds 900
    }
    $proc.Refresh()
    $diagnostic=if ($proc.HasExited) { 'vendor process exited '+$proc.ExitCode } else { 'vendor process stayed running but did not listen' }
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
    $proc.WaitForExit()
    Write-Host ('Garnet CI diagnostic: '+$diagnostic)
    foreach ($log in @($out,$err)) {
        if (Test-Path -LiteralPath $log) {
            Write-Host ('Garnet CI log '+[IO.Path]::GetFileName($log)+' bytes='+((Get-Item -LiteralPath $log).Length))
            foreach ($line in @(Get-Content -LiteralPath $log -Tail 35)) { Write-Host $line }
        }
    }
    throw "Garnet $phase failed to bind its local TLS port."
}
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    # Service binaries and TLS fixture must be traversable by NetworkService;
    # only the disposable runner hosts this ACL. Secrets are separately ACL
    # protected and are NOT packaged into customer releases.
    & icacls.exe $root '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not protect disposable Garnet service fixture root.' }
    [IO.File]::WriteAllText($acl,('user default off'+[Environment]::NewLine+'user esteriumcache on >'+$password+' ~* +@all'))
    [IO.File]::WriteAllText($pfxPassword,('ci-only-pfx-'+$id))
    $cert=New-SelfSignedCertificate -DnsName localhost -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddDays(3) -ErrorAction Stop
    $thumbprint=$cert.Thumbprint
    $garnetPublicCert=Join-Path $root 'garnet-server.cer'
    Export-Certificate -Cert $cert -FilePath $garnetPublicCert -ErrorAction Stop | Out-Null
    Protect-Fixture $garnetPublicCert $true
    $secure=ConvertTo-SecureString ([IO.File]::ReadAllText($pfxPassword)) -AsPlainText -Force
    Export-PfxCertificate -Cert $cert -FilePath $pfx -Password $secure -ErrorAction Stop | Out-Null
    foreach ($file in @($acl,$pfx,$pfxPassword)) { Protect-Fixture $file $true }
    & (Join-Path $PSScriptRoot 'provision-esterium-garnet-config.ps1') -AclFile $acl -CertificatePfxFile $pfx -CertificatePasswordFile $pfxPassword -DestinationDir $destination -Port $port -Apply -UnsignedDisposableCIFixture | Out-Null
    $confPath=Join-Path $destination 'garnet.conf'
    $config=Get-Content -LiteralPath $confPath -Raw | ConvertFrom-Json
    if (-not $config.EnableAOF -or -not $config.WaitForCommit -or
        -not $config.Recover -or -not $config.FailOnRecoveryError) {
        throw 'Generated Garnet durability configuration is incomplete.'
    }
    # Fresh profile is a transient *CI-only* exception. The service config
    # stays Recover=true; production first-run is never automatically enabled.
    $config.Recover=$false
    [IO.File]::WriteAllText($fresh,($config | ConvertTo-Json -Depth 5))
    Protect-Fixture $fresh $false
    if($FromIntegratedStage){
        if($env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1' -or
           $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1'){
            throw 'Integrated Garnet smoke requires the disposable hosted Windows MIT setup CI guards.'
        }
        $sourceZip=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage\garnet.zip'
        $sourceNssm=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage\nssm.exe'
        if(-not(Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage\bundle-manifest.json') -PathType Leaf) -or
           -not(Test-Path -LiteralPath $sourceZip -PathType Leaf) -or
           -not(Test-Path -LiteralPath $sourceNssm -PathType Leaf) -or
           (Get-FileHash -LiteralPath $sourceNssm -Algorithm SHA256).Hash -cne
             'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'){
            throw 'Protected native EXE Garnet/NSSM staged bytes not present or pin invalid.'
        }
        Copy-Item -LiteralPath $sourceZip -Destination $zip -ErrorAction Stop
        # The original vendor staging is admin-only by design. A service
        # running as NetworkService must get RX, never write, to the NSSM
        # executable. Copy pinned bytes into this isolated CI fixture whose
        # parent has explicit NetworkService RX, rather than relaxing the
        # protected publisher staging ACL.
        $nssmExe=Join-Path $root 'nssm.exe'
        Copy-Item -LiteralPath $sourceNssm -Destination $nssmExe -ErrorAction Stop
        if((Get-FileHash -LiteralPath $nssmExe -Algorithm SHA256).Hash -cne
          'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848') {
            throw 'Copied native-exe NSSM file failed independent SHA-256 verification.'
        }
        Write-Output 'PASS: Garnet CI uses the Garnet/NSSM bytes extracted from the native MIT EXE.'
    }else{
        $url='https://github.com/microsoft/garnet/releases/download/v2.2.3/win-x64-based-readytorun.zip'
        Invoke-WebRequest -Uri $url -OutFile $zip -ErrorAction Stop
    }
    if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -cne
        '5cd130686ab415b13c59d36d048a6faded81e707ffe6da5953f98beeb79f9f16') {
        throw 'Upstream official Garnet ZIP hash differs from independently pinned CI fixture.'
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $bin -ErrorAction Stop
    $server=Start-Garnet $fresh 'fresh'
    $connection=Open-Redis
    try {
        Assert-RedisDenied $connection.ssl $connection.reader @('PING')
        Assert-RedisDenied $connection.ssl $connection.reader @('AUTH','esteriumcache','wrong-ci-password')
        if ((Send-Redis $connection.ssl $connection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Garnet refused its authenticated user.'
        }
        if ((Send-Redis $connection.ssl $connection.reader @('SET',('esterium:smoke:'+$id),'committed-value')) -ne 'OK') {
            throw 'Durable test write was not acknowledged.'
        }
        $luaKey='esterium:lua:'+ $id
        if ((Send-Redis $connection.ssl $connection.reader @('EVAL',"return redis.call('INCR', KEYS[1])",'1',$luaKey)) -ne '1') {
            throw 'Garnet did not execute the required atomic Redis Lua counter.'
        }
    } finally {
        $connection.reader.Dispose();$connection.ssl.Dispose();$connection.tcp.Dispose()
    }
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
    $server=$null
    $server=Start-Garnet $confPath 'recovered'
    $connection=Open-Redis
    try {
        if ((Send-Redis $connection.ssl $connection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Recovered Garnet refused its authenticated user.'
        }
        $value=Send-Redis $connection.ssl $connection.reader @('GET',('esterium:smoke:'+$id))
        if ($value -ne 'committed-value') {
            throw 'Garnet failed to recover an AOF-acknowledged write after a forced crash.'
        }
        if ((Send-Redis $connection.ssl $connection.reader @('GET',('esterium:lua:'+$id))) -ne '1') {
            throw 'Atomic Lua write disappeared after AOF recovery.'
        }
    } finally {
        $connection.reader.Dispose();$connection.ssl.Dispose();$connection.tcp.Dispose()
    }
    # The console process passed AOF recovery. Now prove exactly the same
    # protected Garnet executable/configuration under REAL Windows SCM/NSSM.
    # NSSM 2.24-101 is the Windows 10+ compatible upstream pre-release;
    # the SHA-256 was independently pinned by WinGet's version manifest.
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
    $server=$null
    if(-not $FromIntegratedStage){
        $nssmHash='99F5045FFFBFFB745D67FE3A065A953C4A3D9C253B868892D9B685B0EE7D07B8'
        $cacheRoot=Join-Path $env:GITHUB_WORKSPACE '.vendor-cache'
        $cacheFile=Join-Path $cacheRoot 'nssm-2.24-101-g897c7ad.zip'
        $downloaded=$false
        # GitHub Actions cache is an UNTRUSTED byte source. Its content must pass
        # the same independently pinned vendor SHA-256 as direct upstream bytes.
        if (Test-Path -LiteralPath $cacheFile -PathType Leaf) {
            $cacheHash=(Get-FileHash -LiteralPath $cacheFile -Algorithm SHA256).Hash
            if ($cacheHash -cne $nssmHash) {
                throw 'Cached NSSM ZIP differs from approved upstream SHA-256; refuse cache and installation.'
            }
            Copy-Item -LiteralPath $cacheFile -Destination $nssmZip -ErrorAction Stop
            $downloaded=$true
            Write-Output 'PASS: reused SHA-256 verified official NSSM ZIP from ephemeral CI cache.'
        }
        # Publisher-only fallback for a cold cache. The downloaded binary is
        # cached only AFTER immutable SHA-256 verification and a full successful
        # live Windows service test; it is never signed on behalf of the vendor.
        if (-not $downloaded) {
          foreach ($nssmUrl in @(
            'https://www.nssm.cc/ci/nssm-2.24-101-g897c7ad.zip',
            'https://nssm.cc/ci/nssm-2.24-101-g897c7ad.zip'
        )) {
            for ($attempt=0; $attempt -lt 3; $attempt++) {
                try {
                    if (Test-Path -LiteralPath $nssmZip) {
                        Remove-Item -LiteralPath $nssmZip -Force
                    }
                    Invoke-WebRequest -Uri $nssmUrl -OutFile $nssmZip -TimeoutSec 90 -ErrorAction Stop
                    $downloaded=$true
                    break
                } catch {
                    if ($attempt -lt 2) { Start-Sleep -Seconds @(2,6,14)[$attempt] }
                }
            }
            if ($downloaded) { break }
          }
        }
        if (-not $downloaded) {
            throw 'Publisher unavailable and no verified NSSM cache exists; service proof is blocked.'
        }
        if ((Get-FileHash -LiteralPath $nssmZip -Algorithm SHA256).Hash -cne $nssmHash) {
            throw 'NSSM package differs from independently approved upstream WinGet SHA-256.'
        }
        Expand-Archive -LiteralPath $nssmZip -DestinationPath $nssmDir -ErrorAction Stop
        if (-not (Test-Path -LiteralPath $nssmExe -PathType Leaf)) {
            throw 'Pinned NSSM ZIP has no required Windows x64 executable.'
        }
    }else{
        if(-not (Test-Path -LiteralPath $nssmExe -PathType Leaf) -or
           (Get-FileHash -LiteralPath $nssmExe -Algorithm SHA256).Hash -cne
             'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'){
            throw 'NSSM from native MIT EXE does not match the approved x64 SHA-256.'
        }
    }
    if ($null -ne (Get-Service -Name $nssmService -ErrorAction SilentlyContinue)) {
        throw 'Disposable Garnet service identity is already registered.'
    }
    $garnetExe=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.exe')
    if ($garnetExe.Count -ne 1) { throw 'Expected one pinned GarnetServer.exe for SCM.' }
    & $nssmExe install $nssmService $garnetExe[0].FullName '--config-import-path' $confPath | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'NSSM did not register Garnet Windows service.' }
    $nssmRegistered=$true
    foreach ($cmd in @(
        @('set',$nssmService,'ObjectName','NT AUTHORITY\NetworkService'),
        @('set',$nssmService,'Start','SERVICE_DISABLED'),
        @('set',$nssmService,'AppDirectory',$garnetExe[0].DirectoryName),
        @('set',$nssmService,'AppExit','Default','Restart'),
        @('set',$nssmService,'AppRestartDelay','5000')
    )) {
        & $nssmExe @cmd | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'NSSM refused restricted Garnet SCM settings.' }
    }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$nssmService'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].StartMode -cne 'Disabled' -or
        $scm[0].State -cne 'Stopped' -or $scm[0].StartName -notin
        @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE')) {
        throw 'NSSM Garnet service is not disabled/stopped with NetworkService identity.'
    }
    # Only on an ephemeral CI runner, turn the previously disabled fixture on
    # long enough to validate its real TLS/ACL/AOF runtime and SCM transitions.
    Set-Service -Name $nssmService -StartupType Manual -ErrorAction Stop
    Start-Service -Name $nssmService -ErrorAction Stop
    (Get-Service -Name $nssmService).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    $ready=$false
    for($attempt=0; $attempt -lt 45; $attempt++) {
        $tcp=[Net.Sockets.TcpClient]::new()
        try {
            $task=$tcp.ConnectAsync('127.0.0.1',$port)
            if ($task.Wait(400) -and $tcp.Connected) { $ready=$true;break }
        } catch {} finally { $tcp.Dispose() }
        Start-Sleep -Milliseconds 800
    }
    if (-not $ready) { throw 'NSSM Garnet service did not expose local TLS on its expected port.' }
    $serviceConnection=Open-Redis
    try {
        Assert-RedisDenied $serviceConnection.ssl $serviceConnection.reader @('PING')
        if ((Send-Redis $serviceConnection.ssl $serviceConnection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Real NetworkService Garnet could not use its protected ACL.'
        }
        if ((Send-Redis $serviceConnection.ssl $serviceConnection.reader @('GET',('esterium:smoke:'+$id))) -ne 'committed-value') {
            throw 'NSSM-wrapped Garnet lost its durable AOF data.'
        }
    } finally {
        $serviceConnection.reader.Dispose()
        $serviceConnection.ssl.Dispose()
        $serviceConnection.tcp.Dispose()
    }
    if($RunERPStartupSmoke){
        $labKeys=@('ESTERIUM_CI_GARNET_PASSWORD','ESTERIUM_CI_GARNET_CERT_FILE','ESTERIUM_CI_GARNET_PORT','ESTERIUM_CI_GARNET_SERVICE')
        $oldLab=@{}
        foreach($key in $labKeys){$oldLab[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
        try{
            $env:ESTERIUM_CI_GARNET_PASSWORD=$password
            $env:ESTERIUM_CI_GARNET_CERT_FILE=$garnetPublicCert
            $env:ESTERIUM_CI_GARNET_PORT=[string]$port
            $env:ESTERIUM_CI_GARNET_SERVICE=$nssmService
            & (Join-Path $PSScriptRoot '..\windows-setup\test-oss-erp-runtime-ci.ps1')
            if(-not $?){throw 'Live ERP service start, login or dependency restart smoke failed.'}
        }finally{
            foreach($entry in $oldLab.GetEnumerator()){[Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')}
        }
    }
    Stop-Service -Name $nssmService -ErrorAction Stop
    (Get-Service -Name $nssmService).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
    Set-Service -Name $nssmService -StartupType Disabled -ErrorAction Stop
    # SCM can report 'Stopped' slightly before the wrapped child closes TCP.
    $portClosed=$false
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        if (@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -eq 0) {
            $portClosed=$true
            break
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $portClosed) { throw 'Stopped NSSM wrapper left the child Garnet TLS port open.' }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$nssmService'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].State -cne 'Stopped' -or
        $scm[0].StartMode -cne 'Disabled' -or [int]$scm[0].ProcessId -ne 0 -or
        @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -ne 0) {
        throw 'SCM service shutdown left Garnet running or its local TLS port open.'
    }
    # The integrated path uses the already SHA-256-approved NSSM executable
    # provided by the MIT EXE. It has NO upstream NSSM archive or cacheFile;
    # do not read undefined variables or fabricate an archive from the EXE.
    if (-not $FromIntegratedStage) {
        if (-not (Test-Path -LiteralPath $cacheFile)) {
            if (Test-Path -LiteralPath $cacheRoot) {
                $cacheDir=Get-Item -LiteralPath $cacheRoot -Force -ErrorAction Stop
                if (($cacheDir.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                    throw 'Refuse NSSM CI cache through a symbolic link or junction.'
                }
            } else {
                [IO.Directory]::CreateDirectory($cacheRoot) | Out-Null
            }
            # Copy only after the genuine SCM/TLS/ACL/AOF proof passed.
            Copy-Item -LiteralPath $nssmZip -Destination $cacheFile -ErrorAction Stop
            if ((Get-FileHash -LiteralPath $cacheFile -Algorithm SHA256).Hash -cne $nssmHash) {
                throw 'NSSM vendor cache bytes changed while preparing GitHub upload.'
            }
        }
    }
    Write-Output 'PASS: upstream WinGet-pinned NSSM runs authenticated Garnet TLS under NetworkService and stops disabled with no listener.'
    Write-Output 'PASS: pinned Garnet TLS, default user rejection, ACL, atomic Lua and AOF recovery survived hard Windows restart.'
} finally {
    if ($nssmRegistered) {
        # Never leave a CI service startable on error; only this randomly named
        # fixture can be removed, never any pre-existing Windows service.
        try {
            $svc=Get-Service -Name $nssmService -ErrorAction SilentlyContinue
            if ($null -ne $svc) {
                Set-Service -Name $nssmService -StartupType Disabled -ErrorAction Stop
                if ($svc.Status -ne 'Stopped') {
                    Stop-Service -Name $nssmService -Force -ErrorAction Stop
                    (Get-Service -Name $nssmService).WaitForStatus(
                        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
                }
                & $nssmExe remove $nssmService confirm | Out-Null
                if ($LASTEXITCODE -ne 0) { throw 'NSSM did not remove disposable Garnet CI service.' }
            }
        } catch {
            Write-Warning 'NSSM CI service cleanup failed: discard this hosted runner.'
            throw
        }
    }
    if ($null -ne $server) {
        $server.Refresh()
        if (-not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
    }
    if ($null -ne $cert) {
        Remove-Item -LiteralPath ('Cert:\CurrentUser\My\'+$thumbprint) -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI -ErrorAction SilentlyContinue
    foreach ($path in @($destination,$root)) {
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue }
    }
}
