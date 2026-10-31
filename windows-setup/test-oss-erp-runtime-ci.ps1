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
    pgAccount=$env:ESTERIUM_CI_POSTGRES_ACCOUNT
}
if(@($required.Values | Where-Object { [string]::IsNullOrWhiteSpace([string]$_) }).Count -gt 0){throw 'Integrated PostgreSQL/Garnet handoff is incomplete.'}
$guid=[Guid]::NewGuid().ToString('N')
$serviceName='EsteriumERP'
$source=Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage'
$install=Join-Path $env:ProgramFiles ('EsteriumERP-CILab-'+$guid)
$root=Join-Path $env:ProgramData ('Esterium\RuntimeCILab-'+$guid)
$readinessReceiptDir=Join-Path $root 'FirstRunReadiness'
$activationReceiptDir=Join-Path $root 'Installation'
$activationRuleName='EsteriumERP-HTTPS'
$shortcutPath=Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)) 'Esterium ERP.url'
$activationRuleCreated=$false
$activationShortcutCreated=$false
$serverTrustThumbprint=''
$serverTrustAdded=$false
$hostsPath=Join-Path $env:SystemRoot 'System32\drivers\etc\hosts'
$hostsOriginalBytes=$null
$hostsAppendedBytes=$null
$hostsEdited=$false
$labHost='esterium.ci.invalid'
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
function ConvertTo-Pem([string]$label,[byte[]]$bytes){
    $encoded=[Convert]::ToBase64String($bytes)
    $chunks=@(for($offset=0;$offset -lt $encoded.Length;$offset+=64){
        $length=[Math]::Min(64,$encoded.Length-$offset)
        $encoded.Substring($offset,$length)
    })
    return "-----BEGIN $label-----`n$($chunks -join "`n")`n-----END $label-----`n"
}

function Test-FirewallRemoteScope([string[]]$Actual,[string]$Expected){
    if($Actual.Count -ne 1){return $false}
    $observed=[string]$Actual[0]
    if([string]::Equals($observed,$Expected,[StringComparison]::OrdinalIgnoreCase)){return $true}
    $slash=$Expected.LastIndexOf('/')
    if($slash -gt 0 -and $Expected.Substring($slash+1) -ceq '32'){
        return [string]::Equals($observed,$Expected.Substring(0,$slash),[StringComparison]::OrdinalIgnoreCase)
    }
    return $false
}
function Wait-ERPReady([int]$port,[int]$seconds){
    $uri="https://$labHost`:$port/health/ready"
    for($attempt=0;$attempt -lt $seconds;$attempt++){
        $svc=Get-Service -Name $serviceName -ErrorAction SilentlyContinue
        if($null -ne $svc -and $svc.Status -eq 'Running'){
            try{$result=Invoke-RestMethod -Uri $uri -Method Get -TimeoutSec 5;if($result.status -eq 'ready'){return}}catch{}
        }
        Start-Sleep -Seconds 1
    }
    $svc=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
    throw "ERP HTTPS readiness failed after $seconds seconds; SCM state=$($svc.State)."
}
function Assert-Login([int]$port){
    $uri="https://$labHost`:$port/api/v1/auth/login"
    $body=@{email=$script:adminEmail;password=$script:adminPassword}|ConvertTo-Json -Compress
    $login=Invoke-RestMethod -Uri $uri -Method Post -TimeoutSec 15 -ContentType 'application/json' -Body $body
    $access=[string]$login.token.access_token
    if([string]::IsNullOrWhiteSpace($access)){throw 'Synthetic first-admin login returned no access token.'}
    $me=Invoke-RestMethod -Uri "https://$labHost`:$port/api/v1/auth/me" -Method Get -TimeoutSec 15 -Headers @{Authorization="Bearer $access"}
    if($null -eq $me){throw 'Authenticated first-admin profile was empty.'}
}
function Invoke-AdminBootstrap([bool]$expectSuccess){
    $info=[Diagnostics.ProcessStartInfo]::new()
    $info.FileName=$script:bootstrap
    $info.Arguments='--database-url-file "'+$script:databaseURL.Replace('"','')+'" --password-stdin'
    $info.UseShellExecute=$false;$info.CreateNoWindow=$true
    $info.RedirectStandardInput=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
    foreach($entry in $script:adminVariables.GetEnumerator()){$info.EnvironmentVariables[$entry.Key]=$entry.Value}
    $proc=[Diagnostics.Process]::Start($info)
    if($null -eq $proc){throw 'First-admin CLI could not start in the disposable Windows lab.'}
    try{
        $stdout=$proc.StandardOutput.ReadToEndAsync()
        $stderr=$proc.StandardError.ReadToEndAsync()
        $proc.StandardInput.WriteLine($script:adminPassword)
        $proc.StandardInput.Close()
        if(-not $proc.WaitForExit(60000)){Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue;throw 'First-admin CLI timed out.'}
        $output=$stdout.GetAwaiter().GetResult().Trim()
        $diagnostic=$stderr.GetAwaiter().GetResult().Trim()
        if($expectSuccess -and ($proc.ExitCode -ne 0 -or $output -cne 'PASS: first administrator created for the new company.')){
            if($diagnostic -match '^FAIL: first administrator was not created: [A-Za-z0-9 ,.;:-]+$'){
                throw ('First-admin CLI rejected the fresh synthetic database: '+$diagnostic)
            }
            throw 'First-admin CLI rejected the fresh synthetic database; no safe diagnostic was returned.'
        }
        if(-not $expectSuccess -and $proc.ExitCode -eq 0){throw 'First-admin CLI accepted a repeated setup attempt.'}
    }finally{$proc.Dispose()}
}
function Get-PsqlScalar([string]$name,[string]$query){
    $lines=@(& $script:psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' $query 2>$null)
    $exitCode=$LASTEXITCODE
    if($exitCode -ne 0){throw "First-admin invariant query failed: $name (psql exit code $exitCode)."}
    return (($lines -join '').Trim())
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

    # Apply the same production-safe RBAC seed shipped in the verified ERP bundle.
    $dbText=[IO.File]::ReadAllText($required.database).Trim()
    $dbUri=[Uri]::new($dbText)
    $databaseName=[Uri]::UnescapeDataString($dbUri.AbsolutePath.TrimStart('/'))
    if($dbUri.Scheme -cne 'postgres' -or $dbUri.Host -cne 'localhost' -or [int]$dbUri.Port -ne [int]$required.pgPort -or $databaseName -cne 'esterium_first_admin_ci'){throw 'Disposable database URL is not the verified loopback TLS first-admin database.'}
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
    $env:PGDATABASE=$databaseName;$env:PGSSLMODE='verify-full';$env:PGSSLROOTCERT=$query.sslrootcert
    Remove-Item Env:PGHOSTADDR -ErrorAction SilentlyContinue
    $psql=Join-Path $env:ProgramFiles 'EsteriumOpenSourcePostgresBin\pgsql\bin\psql.exe'
    $script:psql=$psql
    $seed=Join-Path $source 'tools\seed.prod.sql'
    $bootstrap=Join-Path $source 'tools\esterium-bootstrap-admin.exe'
    if(-not(Test-Path -LiteralPath $psql -PathType Leaf) -or -not(Test-Path -LiteralPath $seed -PathType Leaf) -or -not(Test-Path -LiteralPath $bootstrap -PathType Leaf)){throw 'Pinned PostgreSQL client or production bootstrap tool is unavailable.'}
    $userCount=@(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' 'SELECT count(*) FROM users;') -join ''
    if($LASTEXITCODE -ne 0 -or $userCount.Trim() -cne '0'){throw 'First-admin CI seed requires an empty migrated application database.'}
    & $psql '-X' '-q' '-v' 'ON_ERROR_STOP=1' '-f' $seed | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Production-safe role and permission seed failed.'}
    Write-Output 'PASS: empty migrated schema received the production-safe RBAC seed; no demo account or product was inserted.'

    [IO.Directory]::CreateDirectory($secrets)|Out-Null
    [IO.Directory]::CreateDirectory($tls)|Out-Null
    Protect-Tree $root
    Copy-Item -LiteralPath $query.sslrootcert -Destination $pgCA -ErrorAction Stop
    $dbText=[regex]::Replace($dbText,'(?<=\?|&)sslrootcert=[^&]+','sslrootcert='+[Uri]::EscapeDataString($pgCA),1)
    [IO.File]::WriteAllText($databaseURL,$dbText+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    $script:adminEmail='ci-admin@esterium.invalid'
    $script:adminPassword='CIonly-'+[Guid]::NewGuid().ToString('N')
    $script:bootstrap=$bootstrap
    $script:databaseURL=$databaseURL
    $script:adminVariables=@{
        ESTERIUM_BOOTSTRAP_LEGAL_NAME='Esterium CI Synthetic LTDA'
        ESTERIUM_BOOTSTRAP_TRADE_NAME='Esterium CI Lab'
        ESTERIUM_BOOTSTRAP_CNPJ='00000000000191'
        ESTERIUM_BOOTSTRAP_ADMIN_NAME='Synthetic Lab Administrator'
        ESTERIUM_BOOTSTRAP_ADMIN_EMAIL=$script:adminEmail
    }
    Invoke-AdminBootstrap $true
    Invoke-AdminBootstrap $false
    $identityCount=Get-PsqlScalar 'synthetic identity' "SELECT count(*) FROM users WHERE email='ci-admin@esterium.invalid';"
    $demoCount=Get-PsqlScalar 'no-demo invariant' "SELECT count(*) FROM users WHERE email IN ('admin@sistema.local','gerente@sistema.local','caixa@sistema.local');"
    $adminPerms=Get-PsqlScalar 'tenant admin role' "SELECT count(*) FROM user_tenant_roles utr JOIN roles r ON r.id=utr.role_id JOIN users u ON u.id=utr.user_id WHERE r.name='admin' AND u.email='ci-admin@esterium.invalid';"
    $auditCount=Get-PsqlScalar 'first-admin audit event' "SELECT count(*) FROM audit_logs al JOIN users u ON u.id=al.actor_user_id WHERE al.action='install.first_admin.create' AND u.email='ci-admin@esterium.invalid';"
    Write-Output ("First-admin invariant counts: identity={0}, demo={1}, tenantAdmin={2}, audit={3}" -f $identityCount,$demoCount,$adminPerms,$auditCount)
    if($identityCount -cne '1' -or $demoCount -cne '0' -or $adminPerms -cne '1' -or $auditCount -cne '1'){throw ("Synthetic first-admin invariant failed: identity={0}, demo={1}, tenantAdmin={2}, audit={3}." -f $identityCount,$demoCount,$adminPerms,$auditCount)}
    Write-Output 'PASS: first-admin CLI created one synthetic tenant admin, assigned RBAC, rejected replay and created no demo account.'
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

    $dnsName=$labHost
    $serverCertPath=Join-Path $tls 'server.crt'
    $serverKeyPath=Join-Path $tls 'server.key'
    $rootKey=$null;$rootCert=$null;$rootPublicCert=$null;$leafKey=$null;$leafCert=$null;$leafWithKey=$null
    try{
        $now=[DateTimeOffset]::UtcNow
        $rootKey=[Security.Cryptography.RSA]::Create(2048)
        $rootRequest=[Security.Cryptography.X509Certificates.CertificateRequest]::new(
            'CN=Esterium disposable CI root '+$guid,$rootKey,
            [Security.Cryptography.HashAlgorithmName]::SHA256,
            [Security.Cryptography.RSASignaturePadding]::Pkcs1)
        $rootRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($true,$false,0,$true))
        $rootRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new(
                [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::KeyCertSign -bor
                [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::CrlSign,$true))
        $rootRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509SubjectKeyIdentifierExtension]::new($rootRequest.PublicKey,$false))
        $rootCert=$rootRequest.CreateSelfSigned($now.AddMinutes(-5),$now.AddDays(3))

        $leafKey=[Security.Cryptography.RSA]::Create(2048)
        $leafRequest=[Security.Cryptography.X509Certificates.CertificateRequest]::new(
            'CN='+$dnsName,$leafKey,
            [Security.Cryptography.HashAlgorithmName]::SHA256,
            [Security.Cryptography.RSASignaturePadding]::Pkcs1)
        $leafRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($false,$false,0,$true))
        $leafRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new(
                [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::DigitalSignature -bor
                [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::KeyEncipherment,$true))
        $serverAuth=[Security.Cryptography.Oid]::new('1.3.6.1.5.5.7.3.1','Server Authentication')
        $serverAuthOids=[Security.Cryptography.OidCollection]::new();[void]$serverAuthOids.Add($serverAuth)
        $leafRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]::new($serverAuthOids,$true))
        $san=[Security.Cryptography.X509Certificates.SubjectAlternativeNameBuilder]::new()
        $san.AddDnsName($dnsName)
        $leafRequest.CertificateExtensions.Add($san.Build())
        $leafRequest.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509SubjectKeyIdentifierExtension]::new($leafRequest.PublicKey,$false))
        $serial=[byte[]]::new(16);$serialRng=[Security.Cryptography.RandomNumberGenerator]::Create()
        try{$serialRng.GetBytes($serial);$serial[0]=$serial[0] -band 0x7f;if(@($serial|Where-Object {$_ -ne 0}).Count -eq 0){$serial[15]=1}}
        finally{$serialRng.Dispose()}
        $leafCert=$leafRequest.Create($rootCert,$now.AddMinutes(-5),$now.AddDays(2),$serial)
        $leafWithKey=[Security.Cryptography.X509Certificates.RSACertificateExtensions]::CopyWithPrivateKey($leafCert,$leafKey)
        $serverCertPem=ConvertTo-Pem 'CERTIFICATE' $leafWithKey.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert)
        $rootCertPem=ConvertTo-Pem 'CERTIFICATE' $rootCert.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert)
        $privateKeyBytes=$leafKey.ExportPkcs8PrivateKey()
        try{
            [IO.File]::WriteAllText($serverCertPath,$serverCertPem+$rootCertPem,[Text.UTF8Encoding]::new($false))
            [IO.File]::WriteAllText($serverKeyPath,(ConvertTo-Pem 'PRIVATE KEY' $privateKeyBytes),[Text.UTF8Encoding]::new($false))
        }finally{[Array]::Clear($privateKeyBytes,0,$privateKeyBytes.Length)}
        $rootPublicCert=[Security.Cryptography.X509Certificates.X509Certificate2]::new(
            $rootCert.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert))
        $serverTrustThumbprint=$rootPublicCert.Thumbprint
    }finally{
        if($null -ne $leafWithKey){$leafWithKey.Dispose()}
        if($null -ne $leafCert){$leafCert.Dispose()}
        if($null -ne $leafKey){$leafKey.Dispose()}
        if($null -ne $rootKey){$rootKey.Dispose()}
    }
    foreach($file in @($serverCertPath,$serverKeyPath)){Protect-File $file}
    $rootStore=[Security.Cryptography.X509Certificates.X509Store]::new(
        [Security.Cryptography.X509Certificates.StoreName]::Root,
        [Security.Cryptography.X509Certificates.StoreLocation]::LocalMachine)
    try{
        $rootStore.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
        if(@($rootStore.Certificates | Where-Object {$_.Thumbprint -eq $serverTrustThumbprint}).Count -ne 0){
            throw 'Random CI HTTPS root thumbprint unexpectedly exists in the system trust store.'
        }
        $rootStore.Add($rootPublicCert);$serverTrustAdded=$true
    }finally{$rootStore.Close();$rootPublicCert.Dispose();$rootCert.Dispose()}
    $hostsOriginalBytes=[IO.File]::ReadAllBytes($hostsPath)
    $hostsText=[Text.Encoding]::Default.GetString($hostsOriginalBytes)
    if($hostsText -match '(?im)^\s*(?:127\.(?:\d{1,3}\.){2}\d{1,3}|::1)\s+esterium\.ci\.invalid(?:\s|$)'){
        throw 'Disposable CI host alias already exists; refusing to edit a pre-existing mapping.'
    }
    $hostsPrefix=''
    if($hostsOriginalBytes.Length -gt 0 -and $hostsOriginalBytes[$hostsOriginalBytes.Length-1] -notin @(10,13)){$hostsPrefix="`r`n"}
    $hostsAppendedBytes=[Text.Encoding]::ASCII.GetBytes($hostsPrefix+"127.0.0.1 $labHost # Esterium CI $guid`r`n")
    $hostStream=[IO.FileStream]::new($hostsPath,[IO.FileMode]::Append,[IO.FileAccess]::Write,[IO.FileShare]::Read)
    try{$hostStream.Write($hostsAppendedBytes,0,$hostsAppendedBytes.Length);$hostStream.Flush($true);$hostsEdited=$true}
    finally{$hostStream.Dispose()}
    & ipconfig.exe '/flushdns' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Could not flush the disposable CI DNS cache after adding the HTTPS lab alias.'}
    $httpPort=Get-FreeLoopbackPort
    if($httpPort -in @(8080,[int]$required.pgPort,[int]$required.redisPort)){throw 'OS selected a reserved lab port.'}
    $origin="https://$labHost`:$httpPort"
    if(@(Get-NetFirewallRule -Name $activationRuleName -ErrorAction SilentlyContinue).Count -ne 0){throw 'Managed ERP HTTPS firewall name already exists before the isolated activation test.'}
    if(Test-Path -LiteralPath $shortcutPath){throw 'All-users ERP shortcut already exists before the isolated activation test.'}
    $registerParams=@{
        BundleDir=$source;DatabaseURLFile=$databaseURL;RedisURLFile=$redisURL
        RedisCACertFile=$redisCA;JWTSecretFile=$jwt;MetricsBearerTokenFile=$metrics
        AllowedOrigin=$origin;ListenAddress="0.0.0.0:$httpPort";TrustedRemoteAddress='127.0.0.1/32'
        TLSCertFile=$serverCertPath;TLSKeyFile=$serverKeyPath
        InstallDir=$install;UnsignedDisposableCIFixture=$true
    }
    & $registrar @registerParams | Out-Null
    if(-not $?){throw 'Disposable lab ERP service registration failed.'}
    $serviceRegistry="HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName"
    $serviceEnvironment=@((Get-ItemProperty -LiteralPath $serviceRegistry -Name Environment -ErrorAction Stop).Environment)
    if(@($serviceEnvironment | Where-Object {$_ -ceq 'FISCAL_PROVIDER=disabled'}).Count -ne 1 -or
       @($serviceEnvironment | Where-Object {$_ -ceq 'APP_ENV=prod'}).Count -ne 1 -or
       @($serviceEnvironment | Where-Object {$_ -match '^NFCE_SEFAZ_(?:PRODUCTION|HOMOLOGATION)_ENABLED=true$'}).Count -ne 0){
        throw 'Disposable ERP service did not retain the production-safe disabled fiscal/SEFAZ configuration.'
    }
    $activationServiceEnvironment=@($serviceEnvironment)
    $scm=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop
    if($scm.StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE') -or $scm.StartMode -cne 'Manual' -or $scm.State -cne 'Stopped' -or $scm.PathName -notlike ('*'+[IO.Path]::Combine($install,'esterium-server.exe')+'*')){throw 'ERP service identity, initial state or executable path is unsafe.'}
    $failure=& sc.exe qfailure $serviceName 2>&1
    if($LASTEXITCODE -ne 0 -or ($failure -join ' ') -notmatch 'RESTART'){throw 'ERP service recovery actions are not configured.'}
    $receipt=@{
        kind='ESTERIUM_LOCAL_DEPENDENCY_PROBE';schemaVersion=1
        stage='dependencies-validated-services-disabled';checkedUtc=[DateTime]::UtcNow.ToString('o')
        releaseManifestSHA256=(Get-FileHash -LiteralPath (Join-Path $install 'bundle-manifest.json') -Algorithm SHA256).Hash
        postgresqlConfigSHA256=('0'*64);garnetConfigSHA256=('0'*64)
        postgresService='EsteriumPostgreSQL16';garnetService=[string]$required.redisService
        automaticStartupApproved=$false;backupRestoreApproved=$false;fiscalEmissionApproved=$false
    }
    [IO.Directory]::CreateDirectory($readinessReceiptDir) | Out-Null
    & icacls.exe $readinessReceiptDir '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Synthetic CI probe receipt directory ACL failed.'}
    $receiptFile=Join-Path $readinessReceiptDir 'probe-result.json'
    [IO.File]::WriteAllText($receiptFile,(ConvertTo-Json $receipt -Depth 3)+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    & icacls.exe $receiptFile '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Synthetic CI probe receipt ACL failed.'}
    $activationScript=Join-Path $source 'tools\activate-esterium-local-first-install.ps1'
    if(-not(Test-Path -LiteralPath $activationScript -PathType Leaf)){throw 'Signed product activation helper is missing from the bundle.'}
    $manifestHash=$receipt.releaseManifestSHA256
    & $activationScript -BundleDir $install -AllowedOrigin $origin -ListenAddress "0.0.0.0:$httpPort" `
        -TLSCertFile $serverCertPath -TLSKeyFile $serverKeyPath `
        -TrustedRemoteAddress '127.0.0.1/32' -ReadinessReceiptDir $readinessReceiptDir `
        -ActivationReceiptDir $activationReceiptDir -PostgresServiceName 'EsteriumPostgreSQL16' `
        -GarnetServiceName ([string]$required.redisService) -ApprovedManifestSHA256 $manifestHash `
        -UnsignedDisposableCIFixture -Apply | Out-Null
    if(-not $?){throw 'Disposable first-install activation helper failed.'}
    $activationRuleCreated=$true
    $activationShortcutCreated=$true
    $scm=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop
    $pgScm=Get-CimInstance Win32_Service -Filter "Name='EsteriumPostgreSQL16'" -ErrorAction Stop
    $garnetScm=Get-CimInstance Win32_Service -Filter "Name='$($required.redisService)'" -ErrorAction Stop
    $expectedPgAccount=[string]$required.pgAccount
    $pgAccount=@(Get-LocalUser -Name $expectedPgAccount -ErrorAction SilentlyContinue)
    $expectedPgSid=if($pgAccount.Count -eq 1){[string]$pgAccount[0].SID.Value}else{''}
    $pgAdminMembership=@(Get-LocalGroupMember -Group 'Administrators' -ErrorAction Stop |
        Where-Object {$pgAccount.Count -eq 1 -and $_.SID.Value -ceq [string]$pgAccount[0].SID.Value})
    $serviceStartName=([string]$pgScm.StartName).Trim()
    $normalizedStartName=$serviceStartName
    if($normalizedStartName.StartsWith('.\',[StringComparison]::Ordinal)){
        $normalizedStartName=$env:COMPUTERNAME+$normalizedStartName.Substring(1)
    }elseif($normalizedStartName.IndexOf('\') -lt 0 -and $normalizedStartName.IndexOf('/') -lt 0){
        $normalizedStartName=$env:COMPUTERNAME+'\'+$normalizedStartName
    }
    $servicePgSid=''
    $pgIdentityMatches=$false
    try{
        $resolvedIdentity=[Security.Principal.NTAccount]::new($normalizedStartName).Translate([Security.Principal.SecurityIdentifier])
        $servicePgSid=[string]$resolvedIdentity.Value
        $pgIdentityMatches=$expectedPgSid -ne '' -and $servicePgSid -ceq $expectedPgSid
    }catch{
        $servicePgSid=''
        $pgIdentityMatches=$false
    }
    if($expectedPgAccount -cnotmatch '^epg[0-9a-f]{12}$' -or $pgAccount.Count -ne 1 -or
       $pgAdminMembership.Count -ne 0 -or -not $pgIdentityMatches){
        throw ('Integrated PostgreSQL service identity mismatch (StartName={0}; expectedLocalAccount={1}; localAccountFound={2}; nonAdmin={3}; identityMatches={4}).' -f
            $serviceStartName,$expectedPgAccount,($pgAccount.Count -eq 1),($pgAdminMembership.Count -eq 0),$pgIdentityMatches)
    }
    if($pgScm.State -cne 'Running'){
        throw ('PostgreSQL dependency service is not running after activation (state={0}).' -f [string]$pgScm.State)
    }
    $dependencies=@((Get-ItemProperty -LiteralPath "HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName" -Name DependOnService -ErrorAction Stop).DependOnService)
    if($scm.StartMode -cne 'Auto' -or $scm.State -cne 'Running' -or
       $pgScm.StartMode -cne 'Auto' -or $garnetScm.StartMode -cne 'Auto' -or
       $dependencies -notcontains 'EsteriumPostgreSQL16' -or $dependencies -notcontains ([string]$required.redisService)){
        throw 'Activation did not configure automatic startup and service dependency ordering.'
    }
    $firewall=@(Get-NetFirewallRule -Name $activationRuleName -PolicyStore ActiveStore -ErrorAction Stop)
    if($firewall.Count -ne 1){throw 'Disposable activation did not create exactly one managed firewall rule.'}
    $remoteFilter=$firewall[0] | Get-NetFirewallAddressFilter
    if(-not (Test-FirewallRemoteScope -Actual @($remoteFilter.RemoteAddress) -Expected '127.0.0.1/32') -or
       -not(Test-Path -LiteralPath $shortcutPath -PathType Leaf)){
        throw 'LAN firewall scope or all-users Start Menu shortcut was not installed.'
    }
    Wait-ERPReady $httpPort 75
    $homeResponse=Invoke-WebRequest -Uri "$origin/" -Method Get -TimeoutSec 15
    if($homeResponse.StatusCode -ne 200 -or $homeResponse.Content -notmatch '(?i)<div\s+id=["'']root["'']'){throw 'Installed HTTPS origin did not serve the compiled React application shell.'}
    $asset=[regex]::Match($homeResponse.Content,'(?i)src=["'']([^"'']+\.js(?:\?[^"'']*)?)["'']')
    if(-not $asset.Success){throw 'Compiled React JavaScript asset is missing from local index.html.'}
    $assetPath=$asset.Groups[1].Value
    if($assetPath.StartsWith('/')){$assetUri=$origin+$assetPath}else{$assetUri=$origin+'/'+$assetPath}
    $js=Invoke-WebRequest -Uri $assetUri -Method Get -TimeoutSec 20
    if($js.StatusCode -ne 200 -or $js.RawContentLength -lt 10000){throw 'Installed frontend JavaScript asset failed HTTPS delivery.'}
    Assert-Login $httpPort
    Write-Output 'PASS: LocalService ERP served the compiled React shell/assets over HTTPS; synthetic admin login and auth profile passed.'

    # First-install activation and its initial HTTPS readiness run with the
    # production-safe fiscal-disabled configuration. Only after that proof,
    # this disposable synthetic tenant switches to dev + local MVP XML so the
    # real fiscal API can be exercised without any SEFAZ transmission.
    # Synthetic logins and protected-route reloads exercise refresh rotation in
    # this disposable browser/runtime proof. Keep safe first boot at repository
    # defaults; raise limits only for the temporary dev + local XML CI service.
    $serviceEnvironment=@($serviceEnvironment | Where-Object {
        $_ -cnotmatch '^(?:APP_ENV|FISCAL_PROVIDER|RATE_LIMIT_LOGIN_PER_MINUTE|RATE_LIMIT_LOGIN_IDENTIFIER_PER_MINUTE|RATE_LIMIT_LOGIN_IP_IDENTIFIER_PER_MINUTE|RATE_LIMIT_REFRESH_PER_MINUTE)='
    })+@(
        'APP_ENV=dev',
        'FISCAL_PROVIDER=mvp',
        'RATE_LIMIT_LOGIN_PER_MINUTE=1000',
        'RATE_LIMIT_LOGIN_IDENTIFIER_PER_MINUTE=1000',
        'RATE_LIMIT_LOGIN_IP_IDENTIFIER_PER_MINUTE=1000',
        'RATE_LIMIT_REFRESH_PER_MINUTE=1000'
    )
    Set-ItemProperty -LiteralPath $serviceRegistry -Name Environment -Value $serviceEnvironment -ErrorAction Stop
    $registeredEnvironment=@((Get-ItemProperty -LiteralPath $serviceRegistry -Name Environment -ErrorAction Stop).Environment)
    if(@($registeredEnvironment | Where-Object {$_ -ceq 'RATE_LIMIT_LOGIN_PER_MINUTE=1000'}).Count -ne 1 -or
       @($registeredEnvironment | Where-Object {$_ -ceq 'RATE_LIMIT_LOGIN_IDENTIFIER_PER_MINUTE=1000'}).Count -ne 1 -or
       @($registeredEnvironment | Where-Object {$_ -ceq 'RATE_LIMIT_LOGIN_IP_IDENTIFIER_PER_MINUTE=1000'}).Count -ne 1 -or
       @($registeredEnvironment | Where-Object {$_ -ceq 'RATE_LIMIT_REFRESH_PER_MINUTE=1000'}).Count -ne 1){
        throw 'Disposable CI authentication rate-limit overrides were not installed.'
    }
    if(@($registeredEnvironment | Where-Object {$_ -ceq 'APP_ENV=dev'}).Count -ne 1 -or
       @($registeredEnvironment | Where-Object {$_ -ceq 'FISCAL_PROVIDER=mvp'}).Count -ne 1 -or
       @($registeredEnvironment | Where-Object {$_ -match '^NFCE_SEFAZ_(?:PRODUCTION|HOMOLOGATION)_ENABLED=true$'}).Count -ne 0){
        throw 'Disposable local XML provider override or SEFAZ-disabled assertions failed.'
    }
    Restart-Service -Name $serviceName -Force -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(30))
    Wait-ERPReady $httpPort 60
    Assert-Login $httpPort
    Write-Output 'PASS: disposable CI switched the installed service to dev + local XML provider; SEFAZ transmission stayed disabled.'

    if($script:adminEmail -cne 'ci-admin@esterium.invalid' -or $script:adminPassword -notmatch '^CIonly-[0-9a-f]{32}$'){
        throw 'Installed browser smoke requires the random synthetic first-admin identity.'
    }
    $webRoot=Join-Path (Split-Path -Parent $PSScriptRoot) 'web'
    $evidence=Join-Path $env:RUNNER_TEMP 'esterium-installed-lab-evidence'
    if(-not(Test-Path -LiteralPath (Join-Path $webRoot 'package.json') -PathType Leaf) -or
       -not(Test-Path -LiteralPath (Join-Path $webRoot 'node_modules\@playwright\test') -PathType Container)){
        throw 'Playwright package must be installed by the verified frontend build before the real ERP smoke.'
    }
    [IO.Directory]::CreateDirectory($evidence) | Out-Null
    $browserEnv=@{}
    foreach($key in @('ESTERIUM_INSTALLED_LAB','ESTERIUM_LAB_ADMIN_EMAIL','ESTERIUM_LAB_ADMIN_PASSWORD','ESTERIUM_LAB_EVIDENCE_DIR','E2E_BASE_URL','E2E_IGNORE_HTTPS_ERRORS')){
        $browserEnv[$key]=[Environment]::GetEnvironmentVariable($key,'Process')
    }
    try{
        $env:ESTERIUM_INSTALLED_LAB='1'
        $env:ESTERIUM_LAB_ADMIN_EMAIL=$script:adminEmail
        $env:ESTERIUM_LAB_ADMIN_PASSWORD=$script:adminPassword
        $env:ESTERIUM_LAB_EVIDENCE_DIR=$evidence
        $env:E2E_BASE_URL=$origin
        $env:E2E_IGNORE_HTTPS_ERRORS='0'
        Push-Location $webRoot
        try{
            & npx.cmd playwright install chromium firefox
            if($LASTEXITCODE -ne 0){throw 'Playwright Chromium/Firefox installation failed in the disposable Windows lab.'}
            & npx.cmd playwright test --config playwright.installed-lab.config.ts `
                --project=chromium --project=firefox --project=chromium-mobile --reporter=line
            if($LASTEXITCODE -ne 0){throw 'Installed ERP route, mobile or real synthetic business Playwright smoke failed.'}
        }finally{Pop-Location}
    }finally{
        foreach($entry in $browserEnv.GetEnumerator()){[Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')}
    }
    Write-Output 'PASS: Installed HTTPS ERP login and all 17 screens were opened in Chromium and Firefox with system trust enabled.'
    Write-Output 'PASS: Chromium mobile route matrix had no horizontal overflow; real synthetic catalog, procurement, cash, sale, return, finance, fiscal XML and two-terminal race passed.'

    Stop-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(30))
    $recoveryService=Get-Service -Name $serviceName -ErrorAction Stop
    if($recoveryService.Status -eq [ServiceProcess.ServiceControllerStatus]::Stopped){
        Start-Service -Name $serviceName -ErrorAction Stop
    }
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
    # SCM will not stop Garnet while its declared ERP dependent is running.
    # Stop the ERP service first, restart Garnet, then recover ERP in order.
    Stop-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(30))
    Stop-Service -Name $depName -ErrorAction Stop
    (Get-Service -Name $depName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
    Start-Service -Name $depName -ErrorAction Stop
    (Get-Service -Name $depName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    Start-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(30))
    Wait-ERPReady $httpPort 75
    Assert-Login $httpPort
    Write-Output 'PASS: ERP service recovered after a real Garnet service restart; readiness and login returned.'

    Stop-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(30))
    Set-Service -Name $serviceName -StartupType Manual -ErrorAction Stop
    if((Get-Item -LiteralPath $serviceRegistry -ErrorAction Stop).GetValueNames() -contains 'DependOnService'){
        Remove-ItemProperty -LiteralPath $serviceRegistry -Name DependOnService -ErrorAction Stop
    }
    Remove-NetFirewallRule -Name $activationRuleName -ErrorAction Stop
    $activationRuleCreated=$false
    Remove-Item -LiteralPath $shortcutPath -Force -ErrorAction Stop
    $activationShortcutCreated=$false
    $protectedFiles=@(Get-ChildItem -LiteralPath $secrets -Recurse -File)+@(Get-ChildItem -LiteralPath $tls -Recurse -File)
    $protectedHashes=@{}
    foreach($file in $protectedFiles){$protectedHashes[$file.FullName]=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash}
    # Repair must inspect the same fiscal-safe service environment as activation.
    # Restore the captured fixture after synthetic MVP sales; never weaken repair guards.
    $serviceEnvironment=@($activationServiceEnvironment)
    Set-ItemProperty -LiteralPath $serviceRegistry -Name Environment -Value $serviceEnvironment -ErrorAction Stop
    $restoredServiceEnvironment=@((Get-ItemProperty -LiteralPath $serviceRegistry -Name Environment -ErrorAction Stop).Environment)
    if(@($restoredServiceEnvironment | Where-Object {$_ -ceq 'APP_ENV=prod'}).Count -ne 1 -or
       @($restoredServiceEnvironment | Where-Object {$_ -ceq 'FISCAL_PROVIDER=disabled'}).Count -ne 1 -or
       @($restoredServiceEnvironment | Where-Object {$_ -ceq 'NFCE_SEFAZ_PRODUCTION_ENABLED=false'}).Count -ne 1 -or
       @($restoredServiceEnvironment | Where-Object {$_ -ceq 'NFCE_SEFAZ_HOMOLOGATION_ENABLED=false'}).Count -ne 1 -or
       @($restoredServiceEnvironment | Where-Object {$_ -match '^RATE_LIMIT_(?:LOGIN|REFRESH)'}).Count -ne 0){
        throw 'Disposable CI did not restore the activated fiscal-safe service environment before repair.'
    }
    $repair=Join-Path $source 'tools\repair-esterium-local-install.ps1'
    if(-not(Test-Path -LiteralPath $repair -PathType Leaf)){throw 'Assisted repair helper is missing from the verified bundle.'}
    $oldRepairGate=$env:ESTERIUM_REPAIR_DISPOSABLE_CI
    try{
        $env:ESTERIUM_REPAIR_DISPOSABLE_CI='1'
        & $repair -InstallDir $install -ActivationReceiptDir $activationReceiptDir `
            -PostgresServiceName 'EsteriumPostgreSQL16' -GarnetServiceName $depName `
            -ApprovedManifestSHA256 $manifestHash -UnsignedDisposableCIFixture `
            -Apply -ConfirmRepair | Out-Null
        if(-not $?){throw 'Assisted repair helper returned a failure code.'}
    }finally{$env:ESTERIUM_REPAIR_DISPOSABLE_CI=$oldRepairGate}
    $repairedScm=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop
    $repairedDependencies=@((Get-ItemProperty -LiteralPath $serviceRegistry -Name DependOnService -ErrorAction Stop).DependOnService)
    $repairedRule=@(Get-NetFirewallRule -Name $activationRuleName -PolicyStore ActiveStore -ErrorAction Stop)
    $repairedAddress=@($repairedRule[0] | Get-NetFirewallAddressFilter)
    if($repairedScm.StartMode -cne 'Auto' -or $repairedScm.State -cne 'Running' -or
       $repairedDependencies -notcontains 'EsteriumPostgreSQL16' -or $repairedDependencies -notcontains $depName -or
       $repairedRule.Count -ne 1 -or
       -not(Test-FirewallRemoteScope -Actual @($repairedAddress[0].RemoteAddress) -Expected '127.0.0.1/32') -or
       -not(Test-Path -LiteralPath $shortcutPath -PathType Leaf)){
        throw 'Assisted repair did not restore the exact ERP service dependencies, loopback-only firewall rule and shortcut.'
    }
    Wait-ERPReady $httpPort 75
    Assert-Login $httpPort
    foreach($path in $protectedHashes.Keys){
        if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -cne $protectedHashes[$path]){
            throw 'Assisted repair modified an application secret or TLS file.'
        }
    }
    Write-Output 'PASS: repair restored only ERP SCM policy, HTTPS firewall, shortcut and service health; secret/TLS bytes were unchanged.'

    Stop-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(30))
    $uninstall=Join-Path $source 'tools\uninstall-esterium-local-app.ps1'
    if(-not(Test-Path -LiteralPath $uninstall -PathType Leaf)){throw 'Conservative uninstall helper is missing from the verified bundle.'}
    $archive=Join-Path $root 'Uninstalled'
    $oldUninstallGate=$env:ESTERIUM_UNINSTALL_DISPOSABLE_CI
    try{
        $env:ESTERIUM_UNINSTALL_DISPOSABLE_CI='1'
        & $uninstall -InstallDir $install -ActivationReceiptDir $activationReceiptDir `
            -ReadinessReceiptDir $readinessReceiptDir -ReceiptArchiveDir $archive `
            -ApprovedManifestSHA256 $manifestHash -UnsignedDisposableCIFixture `
            -Apply -ConfirmUninstall | Out-Null
        if(-not $?){throw 'Conservative uninstall helper returned a failure code.'}
    }finally{$env:ESTERIUM_UNINSTALL_DISPOSABLE_CI=$oldUninstallGate}
    $uninstalledService=@(Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue)
    if($uninstalledService.Count -ne 0 -or (Test-Path -LiteralPath $install) -or
       @(Get-NetFirewallRule -Name $activationRuleName -ErrorAction SilentlyContinue).Count -ne 0 -or
       (Test-Path -LiteralPath $shortcutPath) -or
       -not(Test-Path -LiteralPath $archive -PathType Container)){
        throw 'Uninstall left the ERP app/service/firewall/shortcut behind or failed to archive its receipts.'
    }
    if(-not(Get-Service -Name 'EsteriumPostgreSQL16' -ErrorAction SilentlyContinue) -or
       -not(Get-Service -Name $depName -ErrorAction SilentlyContinue) -or
       (Get-Service -Name 'EsteriumPostgreSQL16').Status -ne 'Running' -or
       (Get-Service -Name $depName).Status -ne 'Running'){
        throw 'Conservative uninstall changed or removed a PostgreSQL/Garnet dependency service.'
    }
    foreach($path in $protectedHashes.Keys){
        if(-not(Test-Path -LiteralPath $path -PathType Leaf) -or
           (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -cne $protectedHashes[$path]){
            throw 'Conservative uninstall removed or changed protected ProgramData secrets/TLS material.'
        }
    }
    $remainingAdmin=@(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' "SELECT count(*) FROM users WHERE email='ci-admin@esterium.invalid';") -join ''
    if($LASTEXITCODE -ne 0 -or $remainingAdmin.Trim() -cne '1'){
        throw 'Conservative uninstall changed the synthetic PostgreSQL application data.'
    }
    Write-Output 'PASS: uninstall removed only the ERP app/service/firewall/shortcut, archived receipts and preserved dependency services, ProgramData secrets and SQL data.'
}finally{
    if($activationRuleCreated){
        try{Remove-NetFirewallRule -Name $activationRuleName -ErrorAction Stop}catch{Write-Warning 'Could not remove the owned synthetic CI firewall rule.'}
    }
    if($activationShortcutCreated -and (Test-Path -LiteralPath $shortcutPath)){
        try{Remove-Item -LiteralPath $shortcutPath -Force -ErrorAction Stop}catch{Write-Warning 'Could not remove the owned synthetic CI Start Menu shortcut.'}
    }
    if($serverTrustAdded -and $serverTrustThumbprint){
        try{
            $rootStore=[Security.Cryptography.X509Certificates.X509Store]::new(
                [Security.Cryptography.X509Certificates.StoreName]::Root,
                [Security.Cryptography.X509Certificates.StoreLocation]::LocalMachine)
            $rootStore.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
            foreach($cert in @($rootStore.Certificates | Where-Object {$_.Thumbprint -eq $serverTrustThumbprint})){
                $rootStore.Remove($cert);$cert.Dispose()
            }
            $rootStore.Close()
        }catch{Write-Warning 'Could not remove the owned synthetic CI HTTPS root certificate.'}
    }
    if($hostsEdited -and $null -ne $hostsOriginalBytes -and $null -ne $hostsAppendedBytes){
        try{
            $currentHosts=[IO.File]::ReadAllBytes($hostsPath)
            $matchesOwnedAppend=$currentHosts.Length -eq ($hostsOriginalBytes.Length+$hostsAppendedBytes.Length)
            if($matchesOwnedAppend){
                for($i=0;$i -lt $hostsOriginalBytes.Length;$i++){
                    if($currentHosts[$i] -ne $hostsOriginalBytes[$i]){$matchesOwnedAppend=$false;break}
                }
            }
            if($matchesOwnedAppend){
                for($i=0;$i -lt $hostsAppendedBytes.Length;$i++){
                    if($currentHosts[$hostsOriginalBytes.Length+$i] -ne $hostsAppendedBytes[$i]){$matchesOwnedAppend=$false;break}
                }
            }
            if($matchesOwnedAppend){
                [IO.File]::WriteAllBytes($hostsPath,$hostsOriginalBytes)
                & ipconfig.exe '/flushdns' | Out-Null
            }else{Write-Warning 'Disposable CI hosts file changed outside the owned appended entry; preserving current file for runner disposal.'}
        }catch{Write-Warning 'Could not safely remove the owned synthetic CI host alias.'}
    }
    $service=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
    if($null -ne $service -and $service.PathName -like ('*'+[IO.Path]::Combine($install,'esterium-server.exe')+'*')){
        if($service.State -ne 'Stopped'){try{Stop-Service -Name $serviceName -Force -ErrorAction Stop}catch{Write-Warning 'ERP CI cleanup could not stop its owned service.'}}
        try{Set-Service -Name $serviceName -StartupType Disabled -ErrorAction Stop}catch{Write-Warning 'ERP CI cleanup could not disable its owned service.'}
        & sc.exe delete $serviceName | Out-Null
        for($i=0;$i -lt 30;$i++){if(-not(Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){break};Start-Sleep -Milliseconds 500}
    }
    if(-not(Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){
        foreach($path in @($install,$root)){if(Test-Path -LiteralPath $path){Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue}}
    }
    foreach($entry in $oldPg.GetEnumerator()){[Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')}
    $script:adminPassword=$null;$script:adminEmail=$null;$script:adminVariables=$null
}



