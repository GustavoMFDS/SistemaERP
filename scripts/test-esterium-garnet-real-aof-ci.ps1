#requires -Version 5.1
# Real Garnet smoke: disposable Windows CI, pinned upstream ZIP, no ERP data.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Garnet real AOF smoke only supports disposable hosted Windows CI.'
}
$id=[Guid]::NewGuid().ToString('N')
$root=Join-Path $env:RUNNER_TEMP ('esterium-garnet-aof-'+$id)
$destination=Join-Path $env:ProgramData ('Esterium\GarnetAofCI-'+$id)
$zip=Join-Path $root 'garnet.zip'
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
    if ($line.StartsWith('+')) { return $line.Substring(1) }
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
    Write-Output ('Garnet CI diagnostic: '+$diagnostic)
    foreach ($log in @($out,$err)) {
        if (Test-Path -LiteralPath $log) {
            Write-Output ('Garnet CI log '+[IO.Path]::GetFileName($log)+' bytes='+((Get-Item -LiteralPath $log).Length))
            Get-Content -LiteralPath $log -Tail 35
        }
    }
    throw "Garnet $phase failed to bind its local TLS port."
}
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($acl,('user default off'+[Environment]::NewLine+'user esteriumcache on >'+$password+' ~* +@all'))
    [IO.File]::WriteAllText($pfxPassword,('ci-only-pfx-'+$id))
    $cert=New-SelfSignedCertificate -DnsName localhost -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddDays(3) -ErrorAction Stop
    $thumbprint=$cert.Thumbprint
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
    $url='https://github.com/microsoft/garnet/releases/download/v2.2.3/win-x64-based-readytorun.zip'
    Invoke-WebRequest -Uri $url -OutFile $zip -ErrorAction Stop
    if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -cne
        '5cd130686ab415b13c59d36d048a6faded81e707ffe6da5953f98beeb79f9f16') {
        throw 'Upstream official Garnet ZIP hash differs from independently pinned CI fixture.'
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $bin -ErrorAction Stop
    $server=Start-Garnet $fresh 'fresh'
    $connection=Open-Redis
    try {
        if ((Send-Redis $connection.ssl $connection.reader @('AUTH','esteriumcache',$password)) -ne 'OK') {
            throw 'Garnet refused its authenticated user.'
        }
        if ((Send-Redis $connection.ssl $connection.reader @('SET',('esterium:smoke:'+$id),'committed-value')) -ne 'OK') {
            throw 'Durable test write was not acknowledged.'
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
    } finally {
        $connection.reader.Dispose();$connection.ssl.Dispose();$connection.tcp.Dispose()
    }
    Write-Output 'PASS: pinned official Garnet TLS/ACL and AOF-acknowledged data survived hard Windows process restart.'
} finally {
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
