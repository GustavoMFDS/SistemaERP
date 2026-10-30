#requires -Version 7.2
<#
  Starts the actual ERP Windows service against PostgreSQL and Garnet already
  running in the disposable integrated Windows workflow. Uses random CI-local
  secrets and the explicitly guarded demo seed; never ships seed data.
#>
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1' -or $env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1' -or $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1'){
    throw 'ERP runtime install is restricted to the disposable hosted Windows lab.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Disposable ERP runtime lab requires Administrator.'}
$required=@{
    database=$env:ESTERIUM_CI_DATABASE_URL_FILE
    redisPassword=$env:ESTERIUM_CI_GARNET_PASSWORD
    redisCert=$env:ESTERIUM_CI_GARNET_CERT_FILE
    redisPort=$env:ESTERIUM_CI_GARNET_PORT
    redisService=$env:ESTERIUM_CI_GARNET_SERVICE
    pgPort=$env:ESTERIUM_CI_POSTGRES_PORT
}
if(@($required.Values | Where-Object { [string]::IsNullOrWhiteSpace([string]$_) }).Count -gt 0){throw 'Integrated PostgreSQL/Garnet handoff is incomplete.'}
$guid=[Guid]::NewGuid().ToString('N')
$serviceName='EsteriumERP'
$source=Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage'
$install=Join-Path $env:ProgramFiles ('EsteriumERP-CILab-'+$guid)
$root=Join-Path $env:ProgramData ('Esterium\RuntimeCILab-'+$guid)
$secrets=Join-Path $root 'secrets'
$tls=Join-Path $root 'https'
$databaseURL=Join-Path $secrets 'database-url.secret'
$redisURL=Join-Path $secrets 'redis-url.secret'
$redisCA=Join-Path $secrets 'garnet-ca.cer'
$pgCA=Join-Path $secrets 'postgres-ca.crt'
$jwt=Join-Path $secrets 'jwt.secret'
$metrics=Join-Path $secrets 'metrics.secret'
$oldPg=@{}
foreach($key in @('PGPASSWORD','PGHOST','PGPORT','PGUSER','PGDATABASE','PGSSLROOTCERT','PGSSLMODE','PGHOSTADDR')){$oldPg[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
function Protect-File([string]$path){
    & icacls.exe $path '/inheritance:r' '/grant:r' '*S-1-5-19:R' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Could not apply LocalService-only credential ACL.'}
}
function Protect-Tree([string]$path){
    & icacls.exe $path '/inheritance:r' '/grant:r' '*S-1-5-19:(OI)(CI)RX' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '/T' '/C' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Could not apply LocalService read-only lab ACL.'}
}
function Get-FreeLoopbackPort{
    $listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
    try{$listener.Start();return ([Net.IPEndPoint]$listener.LocalEndpoint).Port}
    finally{$listener.Stop()}
}
function Wait-ERPReady([int]$port,[int]$seconds){
    $uri="https://localhost:$port/health/ready"
    for($attempt=0;$attempt -lt $seconds;$attempt++){
        $svc=Get-Service -Name $serviceName -ErrorAction SilentlyContinue
        if($null -ne $svc -and $svc.Status -eq 'Running'){
            try{$result=Invoke-RestMethod -Uri $uri -Method Get -SkipCertificateCheck -TimeoutSec 5;if($result.status -eq 'ready'){return}}catch{}
        }
        Start-Sleep -Seconds 1
    }
    $svc=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
    throw "ERP HTTPS readiness failed after $seconds seconds; SCM state=$($svc.State)."
}
function Assert-Login([int]$port){
    $uri="https://localhost:$port/api/v1/auth/login"
    $body=@{email='admin@sistema.local';password='admin123'}|ConvertTo-Json -Compress
    $login=Invoke-RestMethod -Uri $uri -Method Post -SkipCertificateCheck -TimeoutSec 15 -ContentType 'application/json' -Body $body
    $access=[string]$login.token.access_token
    if([string]::IsNullOrWhiteSpace($access)){throw 'Synthetic first-admin login returned no access token.'}
    $me=Invoke-RestMethod -Uri "https://localhost:$port/api/v1/auth/me" -Method Get -SkipCertificateCheck -TimeoutSec 15 -Headers @{Authorization="Bearer $access"}
    if($null -eq $me){throw 'Authenticated first-admin profile was empty.'}
}
try{
    if((Get-Service -Name $serviceName -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath $install) -or (Test-Path -LiteralPath $root)){throw 'Disposable lab install destination or ERP service already exists.'}
    $verify=Join-Path $source 'tools\verify-esterium-bundle.ps1'
    $registrar=Join-Path $source 'tools\register-esterium-windows-service.ps1'
    $manifest=Join-Path $source 'bundle-manifest.json'
    foreach($file in @($verify,$registrar,$manifest,(Join-Path $source 'web\index.html'),(Join-Path $source 'esterium-server.exe'),$required.database,$required.redisCert)){
        if(-not(Test-Path -LiteralPath $file -PathType Leaf)){throw "Required verified lab file missing: $file"}
    }
    & $verify -BundleDir $source | Out-Null
    if(-not $?){throw 'Source ERP bundle integrity check failed.'}

    # The demo seed is explicitly guarded and runs only against this empty CI database.
    $dbText=[IO.File]::ReadAllText($required.database).Trim()
    $dbUri=[Uri]::new($dbText)
    if($dbUri.Scheme -cne 'postgres' -or $dbUri.Host -cne 'localhost' -or [int]$dbUri.Port -ne [int]$required.pgPort){throw 'Disposable database URL is not the verified loopback TLS endpoint.'}
    $userInfo=[Uri]::UnescapeDataString($dbUri.UserInfo)
    $separator=$userInfo.IndexOf(':')
    if($separator -lt 1){throw 'Disposable database URL has no application credential.'}
    $pgUser=$userInfo.Substring(0,$separator)
    $pgPassword=$userInfo.Substring($separator+1)
    if($pgUser -cne 'esterium_app' -or [string]::IsNullOrWhiteSpace($pgPassword)){throw 'Disposable database URL is not the least-privileged ERP role.'}
    $query=@{}
    foreach($part in $dbUri.Query.TrimStart('?').Split('&',[StringSplitOptions]::RemoveEmptyEntries)){
        $pair=$part.Split('=',2)
        if($pair.Count -eq 2){$query[[Uri]::UnescapeDataString($pair[0])]=[Uri]::UnescapeDataString($pair[1])}
    }
    if($query.sslmode -cne 'verify-full' -or -not $query.sslrootcert -or -not(Test-Path -LiteralPath $query.sslrootcert -PathType Leaf)){throw 'Disposable ERP database URL must use verify-full TLS and a local CA.'}
    $env:PGHOST='localhost';$env:PGPORT=[string]$required.pgPort;$env:PGUSER=$pgUser;$env:PGPASSWORD=$pgPassword
    $env:PGDATABASE='esterium';$env:PGSSLMODE='verify-full';$env:PGSSLROOTCERT=$query.sslrootcert
    Remove-Item Env:PGHOSTADDR -ErrorAction SilentlyContinue
    $psql=Join-Path $env:ProgramFiles 'EsteriumOpenSourcePostgresBin\pgsql\bin\psql.exe'
    $seed=Join-Path $env:GITHUB_WORKSPACE 'backend\seed\seed.sql'
    if(-not(Test-Path -LiteralPath $psql -PathType Leaf) -or -not(Test-Path -LiteralPath $seed -PathType Leaf)){throw 'Pinned PostgreSQL client or guarded synthetic seed is unavailable.'}
    $userCount=@(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' 'SELECT count(*) FROM users;') -join ''
    if($LASTEXITCODE -ne 0 -or $userCount.Trim() -cne '0'){throw 'First-admin CI seed requires an empty migrated application database.'}
    & $psql '-X' '-q' '-v' 'ON_ERROR_STOP=1' '-v' 'ALLOW_DEMO_SEED=1' '-f' $seed | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Guarded CI-only synthetic ERP seed failed.'}
    Write-Output 'PASS: empty migrated schema received only the guarded synthetic CI tenant/admin/catalog seed.'

    [IO.Directory]::CreateDirectory($secrets)|Out-Null
    [IO.Directory]::CreateDirectory($tls)|Out-Null
    Protect-Tree $root
    Copy-Item -LiteralPath $query.sslrootcert -Destination $pgCA -ErrorAction Stop
    $dbText=[regex]::Replace($dbText,'(?<=\?|&)sslrootcert=[^&]+','sslrootcert='+[Uri]::EscapeDataString($pgCA),1)
    [IO.File]::WriteAllText($databaseURL,$dbText+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    $redisPassword=[string]$required.redisPassword
    if($redisPassword -notmatch '^ci-only-garnet-[0-9a-f]{32}$'){throw 'Garnet credential is not an isolated random CI secret.'}
    $redisText="rediss://esteriumcache:$redisPassword@localhost:$($required.redisPort)/0"
    [IO.File]::WriteAllText($redisURL,$redisText+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    Copy-Item -LiteralPath $required.redisCert -Destination $redisCA -ErrorAction Stop
    $rng=[Security.Cryptography.RandomNumberGenerator]::Create()
    try{
        $jwtBytes=[byte[]]::new(48);$metricsBytes=[byte[]]::new(48)
        $rng.GetBytes($jwtBytes);$rng.GetBytes($metricsBytes)
        [IO.File]::WriteAllText($jwt,[Convert]::ToBase64String($jwtBytes),[Text.UTF8Encoding]::new($false))
        [IO.File]::WriteAllText($metrics,[Convert]::ToBase64String($metricsBytes),[Text.UTF8Encoding]::new($false))
        [Array]::Clear($jwtBytes,0,$jwtBytes.Length);[Array]::Clear($metricsBytes,0,$metricsBytes.Length)
    }finally{$rng.Dispose()}
    foreach($file in @($databaseURL,$redisURL,$redisCA,$pgCA,$jwt,$metrics)){Protect-File $file}

    $generator=Join-Path $source 'tools\esterium-pg-tls-certgen.exe'
    if(-not(Test-Path -LiteralPath $generator -PathType Leaf)){throw 'Pinned local TLS certificate generator missing.'}
    & $generator '-out-dir' $tls | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Local HTTPS fixture certificate generation failed.'}
    foreach($file in @((Join-Path $tls 'server.crt'),(Join-Path $tls 'server.key'),(Join-Path $tls 'root.crt'))){
        if(-not(Test-Path -LiteralPath $file -PathType Leaf)){throw 'TLS generator returned an incomplete localhost certificate.'}
        Protect-File $file
    }
    $httpPort=Get-FreeLoopbackPort
    if($httpPort -in @(8080,[int]$required.pgPort,[int]$required.redisPort)){throw 'OS selected a reserved lab port.'}
    $registerParams=@{
        BundleDir=$source;DatabaseURLFile=$databaseURL;RedisURLFile=$redisURL
        RedisCACertFile=$redisCA;JWTSecretFile=$jwt;MetricsBearerTokenFile=$metrics
        AllowedOrigin='https://esterium.ci.invalid';ListenAddress="127.0.0.1:$httpPort"
        TLSCertFile=(Join-Path $tls 'server.crt');TLSKeyFile=(Join-Path $tls 'server.key')
        InstallDir=$install;UnsignedDisposableCIFixture=$true
    }
    & $registrar @registerParams | Out-Null
    if(-not $?){throw 'Disposable lab ERP service registration failed.'}
    $scm=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop
    if($scm.StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE') -or $scm.StartMode -cne 'Manual' -or $scm.State -cne 'Stopped' -or $scm.PathName -notlike ('*'+[IO.Path]::Combine($install,'esterium-server.exe')+'*')){throw 'ERP service identity, initial state or executable path is unsafe.'}
    $failure=& sc.exe qfailure $serviceName 2>&1
    if($LASTEXITCODE -ne 0 -or ($failure -join ' ') -notmatch 'RESTART'){throw 'ERP service recovery actions are not configured.'}

    Set-Service -Name $serviceName -StartupType Manual -ErrorAction Stop
    Start-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(30))
    Wait-ERPReady $httpPort 75

    $origin="https://localhost:$httpPort"
    $homeResponse=Invoke-WebRequest -Uri "$origin/" -Method Get -SkipCertificateCheck -TimeoutSec 15
    if($homeResponse.StatusCode -ne 200 -or $homeResponse.Content -notmatch '(?i)<div\s+id=["'']root["'']'){throw 'Installed HTTPS origin did not serve the compiled React application shell.'}
    $asset=[regex]::Match($homeResponse.Content,'(?i)src=["'']([^"'']+\.js(?:\?[^"'']*)?)["'']')
    if(-not $asset.Success){throw 'Compiled React JavaScript asset is missing from local index.html.'}
    $assetPath=$asset.Groups[1].Value
    if($assetPath.StartsWith('/')){$assetUri=$origin+$assetPath}else{$assetUri=$origin+'/'+$assetPath}
    $js=Invoke-WebRequest -Uri $assetUri -Method Get -SkipCertificateCheck -TimeoutSec 20
    if($js.StatusCode -ne 200 -or $js.RawContentLength -lt 10000){throw 'Installed frontend JavaScript asset failed HTTPS delivery.'}
    Assert-Login $httpPort
    Write-Output 'PASS: LocalService ERP served the compiled React shell/assets over HTTPS; synthetic admin login and auth profile passed.'

    Stop-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(30))
    Start-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(30))
    Wait-ERPReady $httpPort 60
    Assert-Login $httpPort
    Write-Output 'PASS: ERP service stop/start returned to HTTPS-ready state and login remained usable.'

    $svc=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop
    if([int]$svc.ProcessId -le 0){throw 'SCM did not expose a live ERP service process for failure recovery test.'}
    Stop-Process -Id ([int]$svc.ProcessId) -Force -ErrorAction Stop
    Wait-ERPReady $httpPort 75
    Assert-Login $httpPort
    Write-Output 'PASS: forced ERP process failure recovered through Windows SCM restart policy; HTTPS and login returned.'

    $depName=[string]$required.redisService
    if(-not(Get-Service -Name $depName -ErrorAction SilentlyContinue)){throw 'Expected disposable Garnet Windows service disappeared before recovery test.'}
    Stop-Service -Name $depName -ErrorAction Stop
    (Get-Service -Name $depName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
    Start-Service -Name $depName -ErrorAction Stop
    (Get-Service -Name $depName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    Wait-ERPReady $httpPort 75
    Assert-Login $httpPort
    Write-Output 'PASS: ERP reconnected after real Garnet service restart; readiness and login recovered.'
}finally{
    $service=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
    if($null -ne $service -and $service.PathName -like ('*'+[IO.Path]::Combine($install,'esterium-server.exe')+'*')){
        if($service.State -ne 'Stopped'){try{Stop-Service -Name $serviceName -Force -ErrorAction Stop}catch{Write-Warning 'ERP CI cleanup could not stop its owned service.'}}
        & sc.exe delete $serviceName | Out-Null
        for($i=0;$i -lt 30;$i++){if(-not(Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){break};Start-Sleep -Milliseconds 500}
    }
    if(-not(Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){
        foreach($path in @($install,$root)){if(Test-Path -LiteralPath $path){Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue}}
    }
    foreach($entry in $oldPg.GetEnumerator()){[Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')}
}

