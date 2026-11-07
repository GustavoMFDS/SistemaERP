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
   $env:ESTERIUM_CI_DATABASE_URL_FILE -notmatch 'database-url(?:-first-admin)?\.secret$')){
    throw 'ERP startup smoke requires the integrated PostgreSQL secret handoff.'
}
if($RunERPStartupSmoke -and (-not $FromIntegratedStage -or
   $env:ESTERIUM_CI_DATABASE_URL_FILE -notmatch 'database-url(?:-first-admin)?\.secret$')){
    throw 'ERP startup smoke requires the integrated PostgreSQL secret handoff.'
}
$id=[Guid]::NewGuid().ToString('N')
$root=Join-Path $env:RUNNER_TEMP ('esterium-garnet-aof-'+$id)
$destination=Join-Path $env:ProgramData ('Esterium\GarnetAofCI-'+$id)
$zip=Join-Path $root 'garnet.zip'
$winswService='EstGarCI'+$id.Substring(0,12)
$winswExe=Join-Path $root ($winswService+'.exe')
$winswRegistrationAttempted=$false
$bin=Join-Path $root 'bin'
$dotnetRuntimeRoot=Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage\tools\dotnet'
$script:dotnetRuntimeCopy=$null
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
function Assert-PrivateDotnetRuntime([int]$workerPid,[string]$phase) {
    if(-not $FromIntegratedStage){return}
    $expectedCoreClr=Join-Path $script:dotnetRuntimeCopy 'shared\Microsoft.NETCore.App\10.0.14\coreclr.dll'
    if(-not(Test-Path -LiteralPath $expectedCoreClr -PathType Leaf)){
        throw 'Private .NET runtime does not contain the pinned CoreCLR library.'
    }
    $loadedCoreClr=@(Get-Process -Id $workerPid -Module -ErrorAction Stop |
        Where-Object {$_.ModuleName -ieq 'coreclr.dll'})
    if($loadedCoreClr.Count -ne 1){
        throw ('Could not prove the '+$phase+' Garnet process loaded the private CoreCLR.')
    }
    $actualCoreClr=[IO.Path]::GetFullPath([string]$loadedCoreClr[0].FileName)
    $expectedCoreClr=[IO.Path]::GetFullPath($expectedCoreClr)
    if(-not [String]::Equals($actualCoreClr,$expectedCoreClr,[StringComparison]::OrdinalIgnoreCase)){
        throw ('The '+$phase+' Garnet process loaded CoreCLR outside the pinned private runtime. actual='+$actualCoreClr+' expected='+$expectedCoreClr)
    }
    Write-Output ('PASS: '+$phase+' Garnet process loaded CoreCLR from the pinned private runtime: '+$actualCoreClr)
}
function Assert-PrivateDotnetRuntime([int]$workerPid,[string]$phase) {
    if(-not $FromIntegratedStage){return}
    $expectedCoreClr=Join-Path $script:dotnetRuntimeCopy 'shared\Microsoft.NETCore.App\10.0.14\coreclr.dll'
    if(-not(Test-Path -LiteralPath $expectedCoreClr -PathType Leaf)){
        throw 'Private .NET runtime does not contain the pinned CoreCLR library.'
    }
    $loadedCoreClr=@(Get-Process -Id $workerPid -Module -ErrorAction Stop |
        Where-Object {$_.ModuleName -ieq 'coreclr.dll'})
    if($loadedCoreClr.Count -ne 1){
        throw ('Could not prove the '+$phase+' Garnet process loaded the private CoreCLR.')
    }
    $actualCoreClr=[IO.Path]::GetFullPath([string]$loadedCoreClr[0].FileName)
    $expectedCoreClr=[IO.Path]::GetFullPath($expectedCoreClr)
    if(-not [String]::Equals($actualCoreClr,$expectedCoreClr,[StringComparison]::OrdinalIgnoreCase)){
        throw ('The '+$phase+' Garnet process loaded CoreCLR outside the pinned private runtime. actual='+$actualCoreClr+' expected='+$expectedCoreClr)
    }
    Write-Output ('PASS: '+$phase+' Garnet process loaded CoreCLR from the pinned private runtime: '+$actualCoreClr)
}
function Start-Garnet([string]$conf,[string]$phase) {
    $exes=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.exe')
    if ($exes.Count -ne 1) { throw 'Expected exactly one GarnetServer.exe.' }
    $out=Join-Path $root ($phase+'.out.log')
    $err=Join-Path $root ($phase+'.err.log')
    if($FromIntegratedStage){
        if($null -eq $script:dotnetRuntimeCopy){
            $sourceRuntime=Join-Path $dotnetRuntimeRoot ''
            $targetRuntime=Join-Path $root 'dotnet-runtime'
            if(-not(Test-Path -LiteralPath (Join-Path $sourceRuntime 'dotnet.exe') -PathType Leaf) -or
               (Get-Item -LiteralPath $sourceRuntime -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){
                throw 'Integrated Garnet source has no ordinary bundled .NET runtime.'
            }
            Copy-Item -LiteralPath $sourceRuntime -Destination $targetRuntime -Recurse -ErrorAction Stop
            $sourceFiles=@(Get-ChildItem -LiteralPath $sourceRuntime -Recurse -File)
            $targetFiles=@(Get-ChildItem -LiteralPath $targetRuntime -Recurse -File)
            if($sourceFiles.Count -eq 0 -or $sourceFiles.Count -ne $targetFiles.Count){
                throw 'Disposable service runtime copy is incomplete.'
            }
            foreach($sourceFile in $sourceFiles){
                if(($sourceFile.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
                    throw 'Bundled .NET runtime contains a reparse point.'
                }
                $relative=$sourceFile.FullName.Substring($sourceRuntime.Length).TrimStart('\')
                $copyFile=Join-Path $targetRuntime $relative
                if(-not(Test-Path -LiteralPath $copyFile -PathType Leaf) -or
                   (Get-FileHash -LiteralPath $sourceFile.FullName -Algorithm SHA256).Hash -cne
                   (Get-FileHash -LiteralPath $copyFile -Algorithm SHA256).Hash){
                    throw 'Disposable .NET runtime copy differs from verified staged bytes.'
                }
            }
            $script:dotnetRuntimeCopy=$targetRuntime
        }
        $dotnetExe=Join-Path $script:dotnetRuntimeCopy 'dotnet.exe'
        $expectedCoreClr=Join-Path $script:dotnetRuntimeCopy 'shared\Microsoft.NETCore.App\10.0.14\coreclr.dll'
        $apphostExists=Test-Path -LiteralPath $exes[0].FullName -PathType Leaf
        $dotnetExists=Test-Path -LiteralPath $dotnetExe -PathType Leaf
        $coreClrExists=Test-Path -LiteralPath $expectedCoreClr -PathType Leaf
        if(-not $apphostExists -or -not $dotnetExists -or -not $coreClrExists){
            $candidates=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer*' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName.Substring($bin.Length).TrimStart('\\') })
            throw ('Integrated Garnet requires GarnetServer.exe and its bundled .NET runtime. apphost='+$apphostExists+' dotnet='+$dotnetExists+' coreclr='+$coreClrExists+' appDir='+$exes[0].DirectoryName+' candidates='+($candidates -join ','))
        }
        $runtimeList=@(& $dotnetExe --list-runtimes)
        if($LASTEXITCODE -ne 0 -or -not($runtimeList -match 'Microsoft\.NETCore\.App 10\.0\.14')){
            throw 'Integrated Garnet did not find the bundled .NET 10 runtime.'
        }
        $processArgs=@('--config-import-path',('"'+$conf+'"'))
        $oldDotnetRootX64=[Environment]::GetEnvironmentVariable('DOTNET_ROOT_X64','Process')
        $oldDotnetRoot=[Environment]::GetEnvironmentVariable('DOTNET_ROOT','Process')
        try{
            [Environment]::SetEnvironmentVariable('DOTNET_ROOT_X64',$script:dotnetRuntimeCopy,'Process')
            [Environment]::SetEnvironmentVariable('DOTNET_ROOT',$script:dotnetRuntimeCopy,'Process')
            $proc=Start-Process -FilePath $exes[0].FullName -WorkingDirectory $exes[0].DirectoryName -ArgumentList $processArgs -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
        }finally{
            [Environment]::SetEnvironmentVariable('DOTNET_ROOT_X64',$oldDotnetRootX64,'Process')
            [Environment]::SetEnvironmentVariable('DOTNET_ROOT',$oldDotnetRoot,'Process')
        }
    }else{
        $proc=Start-Process -FilePath $exes[0].FullName -WorkingDirectory $exes[0].DirectoryName -ArgumentList @('--config-import-path',('"'+$conf+'"')) -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
    }
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
        $sourceWinSW=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage\winsw.exe'
        if(-not(Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage\bundle-manifest.json') -PathType Leaf) -or
           -not(Test-Path -LiteralPath $sourceZip -PathType Leaf) -or
           -not(Test-Path -LiteralPath $sourceWinSW -PathType Leaf) -or
           (Get-FileHash -LiteralPath $sourceWinSW -Algorithm SHA256).Hash -cne
             '923111C7142B3DC783A3C722B19B8A21BCB78222D7A136AC33F0CA8A29F4CB66'){
            throw 'Protected native EXE Garnet/WinSW staged bytes not present or pin invalid.'
        }
        Copy-Item -LiteralPath $sourceZip -Destination $zip -ErrorAction Stop
        # The original vendor staging is admin-only by design. A service
        # running as NetworkService must get RX, never write, to the WinSW
        # executable. Copy pinned bytes into this isolated CI fixture whose
        # parent has explicit NetworkService RX, rather than relaxing the
        # protected publisher staging ACL.
        Copy-Item -LiteralPath $sourceWinSW -Destination $winswExe -ErrorAction Stop
        if((Get-FileHash -LiteralPath $winswExe -Algorithm SHA256).Hash -cne
          '923111C7142B3DC783A3C722B19B8A21BCB78222D7A136AC33F0CA8A29F4CB66') {
            throw 'Copied native-exe WinSW file failed pinned SHA-256 verification.'
        }
        Write-Output 'PASS: Garnet CI uses the Garnet/WinSW bytes extracted from the native MIT EXE.'
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
    Assert-PrivateDotnetRuntime -workerPid $server.Id -phase 'console'
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
    Assert-PrivateDotnetRuntime -workerPid $server.Id -phase 'recovered console'
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
    # The console process passed AOF recovery. Now prove the protected Garnet
    # executable/configuration under real Windows SCM with the pinned WinSW.
    Stop-Process -Id $server.Id -Force
    $server.WaitForExit()
    $server=$null
    if(-not $FromIntegratedStage){
        Invoke-WebRequest -Uri 'https://github.com/winsw/winsw/releases/download/v2.12.0/WinSW.NET4.exe' -OutFile $winswExe -TimeoutSec 90 -ErrorAction Stop
        if ((Get-FileHash -LiteralPath $winswExe -Algorithm SHA256).Hash -cne
            '923111C7142B3DC783A3C722B19B8A21BCB78222D7A136AC33F0CA8A29F4CB66') {
            throw 'WinSW release asset hash differs from the fixed upstream SHA-256.'
        }
    }else{
        if(-not (Test-Path -LiteralPath $winswExe -PathType Leaf) -or
           (Get-FileHash -LiteralPath $winswExe -Algorithm SHA256).Hash -cne
             '923111C7142B3DC783A3C722B19B8A21BCB78222D7A136AC33F0CA8A29F4CB66'){
            throw 'WinSW from native MIT EXE does not match the pinned SHA-256.'
        }
    }
    if ($null -ne (Get-Service -Name $winswService -ErrorAction SilentlyContinue)) {
        throw 'Disposable Garnet service identity is already registered.'
    }
    $garnetExe=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.exe')
    if ($garnetExe.Count -ne 1) { throw 'Expected one pinned GarnetServer.exe for SCM.' }
    if($FromIntegratedStage){
        $dotnetRuntimeDir=[IO.Path]::GetFullPath($script:dotnetRuntimeCopy)
        $dotnetExe=Join-Path $dotnetRuntimeDir 'dotnet.exe'
        if(-not(Test-Path -LiteralPath $dotnetExe -PathType Leaf)){
            throw 'Integrated Garnet service requires the staged private dotnet host.'
        }
        $assemblyFiles=@(Get-ChildItem -LiteralPath $bin -Recurse -File -Filter 'GarnetServer.dll')
        if($assemblyFiles.Count -gt 1){throw 'Integrated Garnet ZIP contains multiple GarnetServer.dll assemblies.'}
        $frameworkDependent=$false
        $garnetServerDll=''
        if($assemblyFiles.Count -eq 1){
            $garnetServerDll=$assemblyFiles[0].FullName
            $assemblyDir=$assemblyFiles[0].DirectoryName
            if(-not(Test-Path -LiteralPath (Join-Path $assemblyDir 'GarnetServer.runtimeconfig.json') -PathType Leaf) -or
               -not(Test-Path -LiteralPath (Join-Path $assemblyDir 'GarnetServer.deps.json') -PathType Leaf)){
                throw 'Integrated Garnet ZIP has an incomplete framework-dependent assembly set.'
            }
            $frameworkDependent=$true
            $launchExecutable=$dotnetExe
            $launchArguments=@($garnetServerDll,'--config-import-path',$confPath)
            $serviceWorkingDir=$assemblyDir
            $workerExecutable=$dotnetExe
        }else{
            $frameworkMetadata=@(Get-ChildItem -LiteralPath $bin -Recurse -File | Where-Object {$_.Name -in @('GarnetServer.runtimeconfig.json','GarnetServer.deps.json')})
            if($frameworkMetadata.Count -gt 0){throw 'Integrated Garnet ZIP has framework metadata but no GarnetServer.dll.'}
            $launchExecutable=$garnetExe[0].FullName
            $launchArguments=@('--config-import-path',$confPath)
            $serviceWorkingDir=$garnetExe[0].DirectoryName
            $workerExecutable=$garnetExe[0].FullName
        }
        $quotePowerShellLiteral={param([string]$value) return ("'"+$value.Replace("'","''")+"'")}
        $launcherArgumentText=(@($launchArguments | ForEach-Object {& $quotePowerShellLiteral ([string]$_)}) -join ',')
        $launcherLines=@(
            '$ErrorActionPreference = ''Stop''',
            ('$env:DOTNET_ROOT_X64 = '+(& $quotePowerShellLiteral $dotnetRuntimeDir)),
            ('$env:DOTNET_ROOT = '+(& $quotePowerShellLiteral $dotnetRuntimeDir)),
            '$env:DOTNET_MULTILEVEL_LOOKUP = ''0''',
            ('$launchArguments = @('+$launcherArgumentText+')'),
            ('& '+(& $quotePowerShellLiteral $launchExecutable)+' @launchArguments'),
            'exit $LASTEXITCODE'
        )
        $launcherScriptPath=Join-Path $root 'esterium-garnet-launcher.ps1'
        [IO.File]::WriteAllText($launcherScriptPath,(($launcherLines -join [Environment]::NewLine)+[Environment]::NewLine),[Text.UTF8Encoding]::new($true))
        $serviceApplication=Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
        if(-not(Test-Path -LiteralPath $serviceApplication -PathType Leaf)){
            throw 'Windows PowerShell launcher is unavailable for the private Garnet runtime.'
        }
        $appParameters='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$launcherScriptPath+'"'
        $serviceWorkingDir=$root
    }else{
        $frameworkDependent=$false
        $garnetServerDll=''
        $serviceApplication=$garnetExe[0].FullName
        $serviceWorkingDir=$garnetExe[0].DirectoryName
        $appParameters='--config-import-path "'+$confPath+'"'
    }
    $serviceLogs=Join-Path $root 'winsw-logs'
    New-Item -ItemType Directory -Path $serviceLogs -ErrorAction Stop | Out-Null
    & icacls.exe $serviceLogs '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)M' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Could not protect WinSW disposable service logs.'}
    $xmlName=[Security.SecurityElement]::Escape($winswService)
    $xmlExe=[Security.SecurityElement]::Escape($serviceApplication)
    $xmlArguments=[Security.SecurityElement]::Escape($appParameters)
    $xmlWorkingDirectory=[Security.SecurityElement]::Escape($serviceWorkingDir)
    $xmlLogs=[Security.SecurityElement]::Escape($serviceLogs)
    $serviceConfigPath=Join-Path $root ($winswService+'.xml')
    $serviceConfig=@"
<?xml version="1.0" encoding="utf-8"?>
<service>
  <id>$xmlName</id>
  <name>Esterium Garnet CI</name>
  <description>Disposable Garnet integration test service</description>
  <executable>$xmlExe</executable>
  <arguments>$xmlArguments</arguments>
  <workingdirectory>$xmlWorkingDirectory</workingdirectory>
  <startmode>Manual</startmode>
  <stoptimeout>30sec</stoptimeout>
  <onfailure action="restart" delay="5sec" />
  <serviceaccount><domain>NT AUTHORITY</domain><user>NetworkService</user></serviceaccount>
  <logpath>$xmlLogs</logpath>
  <log mode="roll" />
</service>
"@
    [IO.File]::WriteAllText($serviceConfigPath,$serviceConfig,[Text.UTF8Encoding]::new($false))
    # Arm cleanup before install because WinSW can register then fail while
    # reporting success; cleanup only removes an SCM path owned by this fixture.
    $winswRegistrationAttempted=$true
    & $winswExe install | Out-Null
    $installExit=$LASTEXITCODE
    $registeredService=Get-CimInstance Win32_Service -Filter "Name='$winswService'" -ErrorAction SilentlyContinue
    if($null -ne $registeredService -and
       $registeredService.PathName.IndexOf($winswExe,[StringComparison]::OrdinalIgnoreCase) -ge 0){
    }
    if ($installExit -ne 0 -or $null -eq $registeredService -or
        -not $registeredService.PathName -or
        $registeredService.PathName.IndexOf($winswExe,[StringComparison]::OrdinalIgnoreCase) -lt 0) {
        throw 'WinSW did not register Garnet Windows service.'
    }
    Set-Service -Name $winswService -StartupType Disabled -ErrorAction Stop
    if($FromIntegratedStage){
        $scmCommand=@(Get-CimInstance Win32_Service -Filter "Name='$winswService'" -ErrorAction Stop)
        $serviceConfigData=[xml][IO.File]::ReadAllText($serviceConfigPath)
        $serviceParameters=[string]$serviceConfigData.service.arguments
        $launcherContent=[IO.File]::ReadAllText($launcherScriptPath)
        $expectedAssembly=if($frameworkDependent){[IO.Path]::GetFullPath($garnetServerDll)}else{''}
        $expectedRuntimeLiteral=& $quotePowerShellLiteral $dotnetRuntimeDir
        $expectedConfigLiteral=& $quotePowerShellLiteral $confPath
        $expectedExecutableLiteral=& $quotePowerShellLiteral $launchExecutable
        $expectedAssemblyLiteral=if($frameworkDependent){& $quotePowerShellLiteral $expectedAssembly}else{''}
        $appPathMatches=[String]::Equals([string]$serviceConfigData.service.executable,$serviceApplication,[StringComparison]::OrdinalIgnoreCase)
        $hasLauncherPath=$serviceParameters.IndexOf($launcherScriptPath,[StringComparison]::OrdinalIgnoreCase) -ge 0
        $hasConfigSwitch=$launcherContent -match '(?i)--config-import-path'
        $hasConfigPath=$launcherContent.IndexOf($expectedConfigLiteral,[StringComparison]::OrdinalIgnoreCase) -ge 0
        $hasAssemblyPath=(-not $frameworkDependent) -or $launcherContent.IndexOf($expectedAssemblyLiteral,[StringComparison]::OrdinalIgnoreCase) -ge 0
        $hasExecutable=$launcherContent.IndexOf($expectedExecutableLiteral,[StringComparison]::OrdinalIgnoreCase) -ge 0
        $hasRuntimeX64=$launcherContent.IndexOf(('$env:DOTNET_ROOT_X64 = '+$expectedRuntimeLiteral),[StringComparison]::OrdinalIgnoreCase) -ge 0
        $hasRuntime=$launcherContent.IndexOf(('$env:DOTNET_ROOT = '+$expectedRuntimeLiteral),[StringComparison]::OrdinalIgnoreCase) -ge 0
        if($scmCommand.Count -ne 1 -or
           $scmCommand[0].PathName -notlike ('*'+$winswExe+'*') -or
           -not $appPathMatches -or -not $hasLauncherPath -or -not $hasAssemblyPath -or
           -not $hasExecutable -or -not $hasConfigSwitch -or -not $hasConfigPath -or
           -not $hasRuntimeX64 -or -not $hasRuntime){
            throw ('WinSW Garnet launcher mismatch: serviceCount='+$scmCommand.Count+
                ' appPathMatches='+$appPathMatches+' launcherPath='+$hasLauncherPath+
                ' assemblyPath='+$hasAssemblyPath+' executable='+$hasExecutable+
                ' configSwitch='+$hasConfigSwitch+' configPath='+$hasConfigPath+
                ' runtimeX64='+$hasRuntimeX64+' runtime='+$hasRuntime+
                ' appPath='+[string]$serviceConfigData.service.executable+' parameters='+$serviceParameters)
        }
        Write-Output 'PASS: real Garnet SCM command uses a PowerShell launcher that sets the private .NET runtime before Garnet.'
    }
    if($FromIntegratedStage){
        & icacls.exe $script:dotnetRuntimeCopy '/grant:r' '*S-1-5-20:(OI)(CI)RX' '/T' '/C' | Out-Null
        if($LASTEXITCODE -ne 0){
            throw 'Could not grant the disposable NetworkService account read access to the private Garnet runtime.'
        }
    }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$winswService'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].StartMode -cne 'Disabled' -or
        $scm[0].State -cne 'Stopped' -or $scm[0].StartName -notin
        @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE')) {
        throw 'WinSW Garnet service is not disabled/stopped with NetworkService identity.'
    }
    # Only on an ephemeral CI runner, turn the previously disabled fixture on
    # long enough to validate its real TLS/ACL/AOF runtime and SCM transitions.
    Set-Service -Name $winswService -StartupType Manual -ErrorAction Stop
    Start-Service -Name $winswService -ErrorAction Stop
    (Get-Service -Name $winswService).WaitForStatus(
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
    if (-not $ready) { throw 'WinSW Garnet service did not expose local TLS on its expected port.' }
    if($FromIntegratedStage){
        $serviceScm=@(Get-CimInstance Win32_Service -Filter "Name='$winswService'" -ErrorAction Stop)
        if($serviceScm.Count -ne 1){throw 'Could not identify the WinSW Garnet service process.'}
        $launcherName=Split-Path -Leaf $serviceApplication
        $launchers=@(Get-CimInstance Win32_Process -Filter "Name='$launcherName'" -ErrorAction Stop |
            Where-Object {$_.ParentProcessId -eq [int]$serviceScm[0].ProcessId -and
                [String]::Equals([string]$_.ExecutablePath,$serviceApplication,[StringComparison]::OrdinalIgnoreCase) -and
                ([string]$_.CommandLine).IndexOf($launcherScriptPath,[StringComparison]::OrdinalIgnoreCase) -ge 0})
        if($launchers.Count -ne 1){throw 'Could not identify the private runtime launcher child of the WinSW service.'}
        $workerName=Split-Path -Leaf $workerExecutable
        $garnetWorkers=@(Get-CimInstance Win32_Process -Filter "Name='$workerName'" -ErrorAction Stop |
            Where-Object {$_.ParentProcessId -eq [int]$launchers[0].ProcessId -and
                [String]::Equals([string]$_.ExecutablePath,$workerExecutable,[StringComparison]::OrdinalIgnoreCase) -and
                ((-not $frameworkDependent) -or ([string]$_.CommandLine).IndexOf($expectedAssembly,[StringComparison]::OrdinalIgnoreCase) -ge 0)})
        if($garnetWorkers.Count -ne 1){
            throw 'Could not identify the pinned Garnet worker launched by the WinSW private runtime script.'
        }
        Assert-PrivateDotnetRuntime -workerPid ([int]$garnetWorkers[0].ProcessId) -phase 'WinSW service'
    }
    $serviceConnection=Open-Redis
    try {
        Assert-RedisDenied $serviceConnection.ssl $serviceConnection.reader @('PING')
        if ((Send-Redis $serviceConnection.ssl $serviceConnection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Real NetworkService Garnet could not use its protected ACL.'
        }
        if ((Send-Redis $serviceConnection.ssl $serviceConnection.reader @('GET',('esterium:smoke:'+$id))) -ne 'committed-value') {
            throw 'WinSW-wrapped Garnet lost its durable AOF data.'
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
            $env:ESTERIUM_CI_GARNET_SERVICE=$winswService
            & (Join-Path $PSScriptRoot '..\windows-setup\test-oss-erp-runtime-ci.ps1')
            if(-not $?){throw 'Live ERP service start, login or dependency restart smoke failed.'}
        }finally{
            foreach($entry in $oldLab.GetEnumerator()){[Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')}
        }
    }
    Stop-Service -Name $winswService -ErrorAction Stop
    (Get-Service -Name $winswService).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
    Set-Service -Name $winswService -StartupType Disabled -ErrorAction Stop
    # SCM can report 'Stopped' slightly before the wrapped child closes TCP.
    $portClosed=$false
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        if (@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -eq 0) {
            $portClosed=$true
            break
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $portClosed) { throw 'Stopped WinSW wrapper left the child Garnet TLS port open.' }
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$winswService'" -ErrorAction Stop)
    if ($scm.Count -ne 1 -or $scm[0].State -cne 'Stopped' -or
        $scm[0].StartMode -cne 'Disabled' -or [int]$scm[0].ProcessId -ne 0 -or
        @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -ne 0) {
        throw 'SCM service shutdown left Garnet running or its local TLS port open.'
    }
    Write-Output 'PASS: pinned WinSW runs authenticated Garnet TLS under NetworkService and stops disabled with no listener.'
    Write-Output 'PASS: pinned Garnet TLS, default user rejection, ACL, atomic Lua and AOF recovery survived hard Windows restart.'
} finally {
    if ($winswRegistrationAttempted) {
        # Never leave a CI service startable on error; only this randomly named
        # fixture can be removed, never any pre-existing Windows service.
        try {
            $svc=Get-Service -Name $winswService -ErrorAction SilentlyContinue
            if ($null -ne $svc) {
                $serviceRecord=Get-CimInstance Win32_Service -Filter "Name='$winswService'" -ErrorAction Stop
                if($null -eq $serviceRecord -or -not $serviceRecord.PathName -or
                   $serviceRecord.PathName.IndexOf($winswExe,[StringComparison]::OrdinalIgnoreCase) -lt 0){
                    throw 'Refuse to remove a service not registered by this WinSW fixture.'
                }
                Set-Service -Name $winswService -StartupType Disabled -ErrorAction Stop
                if ($svc.Status -ne 'Stopped') {
                    Stop-Service -Name $winswService -Force -ErrorAction Stop
                    (Get-Service -Name $winswService).WaitForStatus(
                        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
                }
                & $winswExe uninstall | Out-Null
                if ($LASTEXITCODE -ne 0) { throw 'WinSW did not remove disposable Garnet CI service.' }
            }
        } catch {
            Write-Warning 'WinSW CI service cleanup failed: discard this hosted runner.'
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
