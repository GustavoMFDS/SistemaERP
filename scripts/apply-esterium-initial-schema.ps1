#requires -Version 5.1
<#
  NEW-DATABASE-ONLY initial migrations. Requires nonprivileged SQL role and
  previously verified TLS. Password is only in child process environment,
  never command-line arguments, console output or migration files.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$DatabaseURLFile,
    [Parameter(Mandatory=$true)][string]$PsqlExe,
    [Parameter(Mandatory=$true)][string]$MigrateExe,
    [Parameter(Mandatory=$true)][string]$MigrationsDir,
    [string]$BundleDir='',
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Migracoes locais exigem Windows.' }
if ($UnsignedDisposableCIFixture -and ($env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_INITIAL_MIGRATIONS_DISPOSABLE_CI -ne '1')) {
    throw 'Migracoes sem assinatura somente em Windows CI descartavel.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$' -or
        $ApprovedManifestSHA256 -cnotmatch '^[a-fA-F0-9]{64}$') {
        throw 'Migracao real exige assinatura e manifesto aprovados.'
    }
    $sig=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($sig.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $sig.SignerCertificate -or
        $sig.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
          $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Inicializador de schema nao esta assinado pelo editor aprovado.'
    }
    if (-not $BundleDir) { throw 'Bundle comercial assinado obrigatorio.' }
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') -BundleDir $BundleDir -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
    $installed=[IO.Path]::GetFullPath($BundleDir).TrimEnd('\')
    $prefix=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')+'\'
    if (-not $installed.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or
        -not [string]::Equals([IO.Path]::GetFullPath($MigrateExe),(Join-Path $installed 'tools\esterium-migrate.exe'),[StringComparison]::OrdinalIgnoreCase) -or
        -not [string]::Equals([IO.Path]::GetFullPath($MigrationsDir),(Join-Path $installed 'migrations'),[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Migrador e SQL devem vir do bundle aprovado em Program Files.'
    }
}
function Assert-Local([string]$value,[bool]$dir) {
    if (-not [IO.Path]::IsPathRooted($value) -or $value.StartsWith('\\')) {
        throw 'Caminho de migracao exige disco local.'
    }
    $full=[IO.Path]::GetFullPath($value)
    $cursor=$full
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Nao usar links ou junctions nos arquivos SQL.'
        }
        $parent=Split-Path -Parent $cursor
        if ($parent -eq $cursor) { break }
        $cursor=$parent
    }
    if ((Get-Item -LiteralPath $full).PSIsContainer -ne $dir) {
        throw 'Caminho de migracao possui tipo incorreto.'
    }
    return $full
}
$urlFile=Assert-Local $DatabaseURLFile $false
$psql=Assert-Local $PsqlExe $false
$migrator=Assert-Local $MigrateExe $false
$folder=Assert-Local $MigrationsDir $true
$versions=@{}
$ups=@(Get-ChildItem -LiteralPath $folder -File -Filter '*.up.sql')
foreach ($file in $ups) {
    if ($file.Name -cnotmatch '^(?<version>\d{4})_[a-z0-9_]+\.up\.sql$' -or
        $file.Length -lt 1 -or $file.Length -gt 1048576 -or
        ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Arquivo SQL inicial invalido ou modificado.'
    }
    $version=[int]$Matches['version']
    if ($versions.ContainsKey($version) -or
        -not (Test-Path -LiteralPath (Join-Path $folder ($file.Name.Replace('.up.sql','.down.sql'))) -PathType Leaf)) {
        throw 'Versao de SQL duplicada ou sem par de reversao.'
    }
    $versions[$version]=$true
}
if ($versions.Count -lt 51) { throw 'Cadeia inicial SQL incompleta.' }
for ($v=1;$v -le $versions.Count;$v++) {
    if (-not $versions.ContainsKey($v)) { throw 'Versoes SQL nao sequenciais.' }
}
if (@(Get-ChildItem -LiteralPath $folder -File -Filter '*.sql').Count -ne (2*$versions.Count)) {
    throw 'Pacote contem SQL extra ou sem par up/down.'
}
if (-not $Apply) {
    Write-Output 'PLAN: aplicar migracoes versionadas completas somente em banco novo e vazio.'
    Write-Output 'PLAN: usar TLS verify-full e conta esterium_app sem enviar senha em argumentos.'
    Write-Output 'PLAN: nao permitir schema ja criado, dirty ou SQL fora de pacote aprovado.'
    Write-Output 'PLAN: nenhum dado, conexao ou servico alterado.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Migracao inicial exige Administrador.'
}
$url=[IO.File]::ReadAllText($urlFile).TrimEnd([char]13,[char]10)
if ($url -cnotmatch '^postgres://esterium_app:([A-Za-z0-9_-]{48,128})@localhost:([1-9][0-9]{0,4})/esterium\?sslmode=verify-full&sslrootcert=([^&]+)$') {
    throw 'DATABASE_URL_FILE deve usar localhost, esterium_app e TLS verify-full.'
}
$password=$Matches[1]
$port=[int]$Matches[2]
$ca=Assert-Local ([Uri]::UnescapeDataString($Matches[3])) $false
if ($port -lt 1 -or $port -gt 65535 -or
    [IO.Path]::GetExtension($ca).ToLowerInvariant() -ne '.crt') {
    throw 'Porta ou raiz TLS PostgreSQL invalida.'
}
$safeUrl=('postgres://esterium_app@localhost:'+$port+'/esterium?sslmode=verify-full&sslrootcert='+[Uri]::EscapeDataString($ca)+'&search_path=public')
if ($safeUrl.Contains($password)) { throw 'A URL do migrador nao pode conter senha.' }
function Run-Child([string]$exe,[string]$args,[bool]$returnOutput,[string]$phase='readiness',[string]$sqlInput='') {
    $info=[Diagnostics.ProcessStartInfo]::new()
    $info.FileName=$exe
    $info.Arguments=$args
    $info.UseShellExecute=$false
    $info.CreateNoWindow=$true
    $info.RedirectStandardInput=$true
    $info.RedirectStandardOutput=$true
    $info.RedirectStandardError=$true
    # Never inherit the administrator's PostgreSQL connection identity.
    # A reused PGUSER/PGDATABASE/PGHOSTADDR could run migrations against
    # a different database despite a validated passwordless URL.
    foreach ($key in @('PGHOSTADDR','PGSERVICE','PGSERVICEFILE','PGPASSFILE','PGOPTIONS')) {
        $info.EnvironmentVariables.Remove($key)
    }
    $info.EnvironmentVariables['PGUSER']='esterium_app'
    $info.EnvironmentVariables['PGDATABASE']='esterium'
    $info.EnvironmentVariables['PGHOST']='localhost'
    $info.EnvironmentVariables['PGPORT']=[string]$port
    $info.EnvironmentVariables['PGPASSWORD']=$password
    $info.EnvironmentVariables['PGSSLMODE']='verify-full'
    $info.EnvironmentVariables['PGSSLROOTCERT']=$ca
    $info.EnvironmentVariables['PGCONNECT_TIMEOUT']='5'
    $proc=[Diagnostics.Process]::Start($info)
    if ($null -eq $proc) { throw 'Cliente de migracoes indisponivel.' }
    try {
        if ($sqlInput) { $proc.StandardInput.WriteLine($sqlInput) }
        $proc.StandardInput.Close()
        # Drain child streams concurrently. A full migration prints more
        # than a pipe buffer on Windows; waiting before reading can deadlock.
        $outputTask=$proc.StandardOutput.ReadToEndAsync()
        $errorTask=$proc.StandardError.ReadToEndAsync()
        if (-not $proc.WaitForExit(120000)) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
            throw 'Consulta/migracao SQL excedeu limite de tempo.'
        }
        $out=$outputTask.GetAwaiter().GetResult().Trim()
        $errorText=$errorTask.GetAwaiter().GetResult()
        if ($proc.ExitCode -ne 0) {
            if ($UnsignedDisposableCIFixture) {
                # Synthetic disposable CI only: bound and redact every stderr
                # message. Never reveal SQL or connection errors on a shop PC.
                $diagnostic=$errorText.Replace($password,'[REDACTED]')
                $diagnostic=$diagnostic.Substring(0,[Math]::Min(1200,$diagnostic.Length))
                Write-Host ('PostgreSQL CI phase '+$phase+' rejected with exit '+$proc.ExitCode)
                Write-Host ('PostgreSQL CI diagnostic: '+$diagnostic)
            }
            throw 'Migracao SQL recusada: preservar schema e inspecionar estado dirty, sem tentar novamente.'
        }
        if ($out.Length -gt 4096) { throw 'Resposta SQL inesperadamente longa.' }
        if ($returnOutput) { return $out }
        return ''
    } finally { $proc.Dispose() }
}
try {
    $query="COPY (SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE') TO STDOUT;"
    $preflight=('-X -q -t -A -v ON_ERROR_STOP=1 -h localhost -p '+$port+' -U esterium_app -d esterium -f -')
    $count=Run-Child $psql $preflight $true 'empty database preflight' $query
    if ($UnsignedDisposableCIFixture) { Write-Host ('PostgreSQL CI: public table preflight count='+$count) }
    if ($count -cne '0') { throw 'Banco esterium nao esta vazio: migracao inicial proibida.' }
    $migrateArgs=('-path "'+$folder+'" -database "'+$safeUrl+'" up')
    $null=Run-Child $migrator $migrateArgs $false 'initial migration'
    $verifySQL="COPY (SELECT version::text||':'||dirty::text FROM public.schema_migrations) TO STDOUT;"
    $verifyArgs=('-X -q -t -A -v ON_ERROR_STOP=1 -h localhost -p '+$port+' -U esterium_app -d esterium -f -')
    $result=Run-Child $psql $verifyArgs $true 'schema version verification' $verifySQL
    if ($result -cne ($versions.Count.ToString()+':false')) {
        throw 'Migracoes iniciais nao terminaram na versao esperada ou estao dirty.'
    }
    Write-Output 'PASS: schema inicial Esterium aplicado com migrador versionado e conta nao privilegiada.'
    Write-Output 'NOT READY: backup externo, servicos Windows, PDV e homologacao ainda exigem aceite.'
} finally { $password=$null; $url=$null }
