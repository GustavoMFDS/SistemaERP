#requires -Version 5.1
<#
  Bootstrap ONLY a brand-new, already TLS-validated PostgreSQL 16 cluster.
  No command-line passwords, no SQL with secrets in .sql files. Refuse existing
  role or database, invalid TLS trust and unapproved publisher. Does not run
  migrations, start/enable services or affect an existing shop installation.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PsqlExe,
    [Parameter(Mandatory=$true)][string]$SuperPasswordFile,
    [Parameter(Mandatory=$true)][string]$AppPasswordFile,
    [Parameter(Mandatory=$true)][string]$RootCertificateFile,
    [string]$DestinationDir=(Join-Path $env:ProgramData 'Esterium\PostgresApplication'),
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'A inicializacao local do banco exige Windows.' }
if ($UnsignedDisposableCIFixture -and ($env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_DB_BOOTSTRAP_DISPOSABLE_CI -ne '1')) {
    throw 'Inicializacao sem assinatura somente em runner Windows descartavel.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$') {
        throw 'Editor aprovado necessario antes de inicializar banco.'
    }
    $signature=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
          $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Inicializador de banco exige assinatura Authenticode aprovada.'
    }
}
if ($PostgresPort -eq 8080 -or $PostgresPort -eq 6379) {
    throw 'Porta da aplicacao ou cache nao pode ser usada pelo banco.'
}
function Assert-RegularLocalFile([string]$value) {
    if (-not [IO.Path]::IsPathRooted($value) -or $value.StartsWith('\\')) {
        throw 'Arquivo de PostgreSQL deve usar caminho absoluto em disco local.'
    }
    $full=[IO.Path]::GetFullPath($value)
    $cursor=$full
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Arquivos PostgreSQL nao podem usar links ou junctions.'
        }
        $next=Split-Path -Parent $cursor
        if ($next -eq $cursor) { break }
        $cursor=$next
    }
    $item=Get-Item -LiteralPath $full -Force
    if ($item.PSIsContainer -or $item.Length -lt 1 -or $item.Length -gt 1048576) {
        throw 'Arquivo PostgreSQL vazio, ausente ou excessivamente grande.'
    }
    return $full
}
$psql=Assert-RegularLocalFile $PsqlExe
$superFile=Assert-RegularLocalFile $SuperPasswordFile
$appFile=Assert-RegularLocalFile $AppPasswordFile
$ca=Assert-RegularLocalFile $RootCertificateFile
if ($superFile -eq $appFile -or $ca -in @($superFile,$appFile)) {
    throw 'Senha SQL e certificado CA devem estar em arquivos independentes.'
}
if (-not $UnsignedDisposableCIFixture) {
    $prefix=[IO.Path]::GetFullPath((Join-Path $env:ProgramFiles 'PostgreSQL\16\bin')).TrimEnd('\')+'\'
    if (-not $psql.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($psql) -cne 'psql.exe') {
        throw 'psql.exe precisa pertencer ao PostgreSQL 16 aprovado.'
    }
}
foreach ($secret in @($superFile,$appFile)) {
    foreach ($ace in (Get-Acl -LiteralPath $secret).Access) {
        if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        if (($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -eq 0) { continue }
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if ($sid -notin @('S-1-5-18','S-1-5-32-544') -and -not $UnsignedDisposableCIFixture) {
            throw 'Senha PostgreSQL lida por identidade Windows nao aprovada.'
        }
    }
}
$base=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
if (-not [IO.Path]::IsPathRooted($DestinationDir) -or $DestinationDir.StartsWith('\\')) {
    throw 'Destino do banco deve usar disco local.'
}
$target=[IO.Path]::GetFullPath($DestinationDir).TrimEnd('\')
if (-not $target.StartsWith($base+'\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Arquivo de conexao SQL deve ficar sob ProgramData\Esterium.'
}
if (Test-Path -LiteralPath $target) {
    throw 'Banco ja foi inicializado nesta pasta: nao substituir credenciais.'
}
$cursor=Split-Path -Parent $target
while ($cursor) {
    if (Test-Path -LiteralPath $cursor) {
        if (((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Destino de senha SQL usa link ou junction proibido.'
        }
    }
    $next=Split-Path -Parent $cursor
    if ($next -eq $cursor) { break }
    $cursor=$next
}
if (-not $Apply) {
    Write-Output 'PLAN: verificar servidor PostgreSQL 16 com TLS verify-full, certificado CA e regras SCRAM.'
    Write-Output 'PLAN: criar esterium_app sem privilegios e banco esterium somente se nao existirem.'
    Write-Output 'PLAN: gravar DATABASE_URL_FILE com ACL LocalService, sem expor senhas em argumentos ou logs.'
    Write-Output 'PLAN: nenhum usuario, banco, arquivo, migracao ou servico modificado.'
    return
}
if (-not [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Criacao comercial do banco requer Administrador.'
}
$super=[IO.File]::ReadAllText($superFile).TrimEnd([char]13,[char]10)
$appPassword=[IO.File]::ReadAllText($appFile).TrimEnd([char]13,[char]10)
if ($super -cnotmatch '^[A-Za-z0-9_-]{48,128}$' -or
    $appPassword -cnotmatch '^[A-Za-z0-9_-]{48,128}$' -or
    $super -ceq $appPassword) {
    throw 'Senhas SQL precisam ser aleatorias, longas e independentes.'
}
function Invoke-Sql([string]$statement,[string]$username,[string]$dbName,[string]$secret) {
    $info=[Diagnostics.ProcessStartInfo]::new()
    $info.FileName=$psql
    $info.Arguments="-X -q -t -A -v ON_ERROR_STOP=1 -h localhost -p $PostgresPort -U $username -d $dbName -f -"
    $info.UseShellExecute=$false
    $info.CreateNoWindow=$true
    $info.RedirectStandardInput=$true
    $info.RedirectStandardOutput=$true
    $info.RedirectStandardError=$true
    $info.EnvironmentVariables['PGPASSWORD']=$secret
    $info.EnvironmentVariables['PGSSLMODE']='verify-full'
    $info.EnvironmentVariables['PGSSLROOTCERT']=$ca
    $info.EnvironmentVariables['PGCONNECT_TIMEOUT']='5'
    $proc=[Diagnostics.Process]::Start($info)
    if ($null -eq $proc) { throw 'Nao foi possivel iniciar cliente SQL seguro.' }
    try {
        $proc.StandardInput.WriteLine($statement)
        $proc.StandardInput.Close()
        # Queries here return at most one short line. Never log stderr:
        # rejected SQL can include the password statement in diagnostics.
        if (-not $proc.WaitForExit(20000)) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
            throw 'Consulta PostgreSQL nao terminou no prazo seguro.'
        }
        $output=$proc.StandardOutput.ReadToEnd().Trim()
        $null=$proc.StandardError.ReadToEnd()
        if ($proc.ExitCode -ne 0) {
            throw 'Consulta PostgreSQL recusada: revisar TLS, conta, role e estado parcial sem repetir.'
        }
        if ($output.Length -gt 1024) { throw 'Resposta SQL inesperadamente longa.' }
        return $output
    } finally { $proc.Dispose() }
}
try {
    if ((Invoke-Sql 'SELECT ssl FROM pg_stat_ssl WHERE pid=pg_backend_pid();' 'postgres' 'postgres' $super) -cne 't' -or
        (Invoke-Sql 'SHOW password_encryption;' 'postgres' 'postgres' $super) -cne 'scram-sha-256' -or
        (Invoke-Sql 'SHOW listen_addresses;' 'postgres' 'postgres' $super) -cne 'localhost' -or
        (Invoke-Sql 'SELECT count(*) FROM pg_hba_file_rules WHERE error IS NOT NULL;' 'postgres' 'postgres' $super) -cne '0') {
        throw 'Banco comercial nao foi aprovado para TLS/SCRAM exclusivamente local.'
    }
    if ((Invoke-Sql "SELECT count(*) FROM pg_roles WHERE rolname='esterium_app';" 'postgres' 'postgres' $super) -cne '0' -or
        (Invoke-Sql "SELECT count(*) FROM pg_database WHERE datname='esterium';" 'postgres' 'postgres' $super) -cne '0') {
        throw 'Conta ou banco Esterium ja existem: nao reutilizar ou sobrescrever.'
    }
    # Password is sent over a verified TLS session through stdin only.
    # Restricted alphabet prevents SQL metacharacters. No SQL files or argv
    # expose the secret. A partially created role/db fails closed for review.
    $null=Invoke-Sql ("CREATE ROLE esterium_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '"+$appPassword+"';") 'postgres' 'postgres' $super
    $null=Invoke-Sql "CREATE DATABASE esterium OWNER esterium_app ENCODING 'UTF8' TEMPLATE template0;" 'postgres' 'postgres' $super
    if ((Invoke-Sql "SELECT rolname FROM pg_roles WHERE rolname='esterium_app' AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolreplication AND NOT rolbypassrls;" 'postgres' 'postgres' $super) -cne 'esterium_app') {
        throw 'Conta do banco recebeu privilegios maiores que os aprovados.'
    }
    if ((Invoke-Sql 'SELECT current_user;' 'esterium_app' 'esterium' $appPassword) -cne 'esterium_app') {
        throw 'Conta do ERP nao autenticou com TLS e senha exclusiva.'
    }
    $parent=Split-Path -Parent $target
    [IO.Directory]::CreateDirectory($parent) | Out-Null
    $stage=Join-Path $parent ('.esterium-sql-'+[Guid]::NewGuid().ToString('N'))
    $published=$false
    try {
        [IO.Directory]::CreateDirectory($stage) | Out-Null
        & icacls.exe $stage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-19:(OI)(CI)RX' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Impossivel proteger pasta de conexao SQL.' }
        $url=('postgres://esterium_app:'+$appPassword+'@localhost:'+$PostgresPort+
            '/esterium?sslmode=verify-full&sslrootcert='+[Uri]::EscapeDataString($ca))
        $out=Join-Path $stage 'database-url.secret'
        $bytes=[Text.UTF8Encoding]::new($false).GetBytes($url+[Environment]::NewLine)
        try {
            $stream=[IO.File]::Open($out,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
            try { $stream.Write($bytes,0,$bytes.Length);$stream.Flush($true) }
            finally { $stream.Dispose() }
        } finally { [Array]::Clear($bytes,0,$bytes.Length) }
        & icacls.exe $out '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' '*S-1-5-19:R' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Impossivel proteger DATABASE_URL_FILE.' }
        if (Test-Path -LiteralPath $target) { throw 'Destino criado por outro processo: nao sobrescrever.' }
        Move-Item -LiteralPath $stage -Destination $target -ErrorAction Stop
        $published=$true
    } finally {
        if (-not $published -and (Test-Path -LiteralPath $stage)) {
            Remove-Item -LiteralPath $stage -Force -Recurse -ErrorAction SilentlyContinue
        }
    }
    Write-Output 'PASS: nova conta SQL sem privilegios, banco Esterium e DATABASE_URL_FILE TLS protegida criados.'
    Write-Output 'NOT READY: migracoes, backups, servicos e homologacao real ainda nao foram habilitados.'
} finally { $super=$null; $appPassword=$null }
