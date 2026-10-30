#requires -Version 5.1
<#
  One-time first administrator bootstrap for a fresh migrated Esterium store.
  Prompts for company identity and a new password; the password travels to the
  bundled bootstrap CLI through stdin only and is never passed as an argument.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$DatabaseURLFile,
    [Parameter(Mandatory=$true)][string]$BootstrapAdminExe,
    [Parameter(Mandatory=$true)][string]$TrustedPublisherThumbprint,
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'A criacao do primeiro administrador exige Windows.' }
if (-not $Apply) { throw 'A criacao do administrador exige execucao explicitamente aprovada pelo instalador.' }
if ($TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$') { throw 'Editor aprovado invalido.' }
$scriptSignature=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
if ($scriptSignature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
    $null -eq $scriptSignature.SignerCertificate -or
    $scriptSignature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()) {
    throw 'Bootstrap do primeiro administrador sem assinatura do editor aprovado.'
}
$tools=[IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\')
$expectedExe=[IO.Path]::GetFullPath((Join-Path $tools 'esterium-bootstrap-admin.exe'))
if (-not [string]::Equals([IO.Path]::GetFullPath($BootstrapAdminExe),$expectedExe,[StringComparison]::OrdinalIgnoreCase) -or
    -not (Test-Path -LiteralPath $expectedExe -PathType Leaf)) {
    throw 'CLI de bootstrap deve pertencer ao pacote instalado e verificado.'
}
$exeSignature=Get-AuthenticodeSignature -LiteralPath $expectedExe
if ($exeSignature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
    $null -eq $exeSignature.SignerCertificate -or
    $exeSignature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()) {
    throw 'CLI do primeiro administrador sem assinatura do editor aprovado.'
}
$dbFile=[IO.Path]::GetFullPath($DatabaseURLFile)
if (-not [IO.Path]::IsPathRooted($DatabaseURLFile) -or $DatabaseURLFile.StartsWith('\\') -or
    -not (Test-Path -LiteralPath $dbFile -PathType Leaf)) {
    throw 'Arquivo de conexao PostgreSQL protegido ausente ou invalido.'
}
$dbBase=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium\PostgresApplication')).TrimEnd('\')+'\'
if (-not $dbFile.StartsWith($dbBase,[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Arquivo de conexao deve ficar na pasta protegida do PostgreSQL Esterium.'
}
$cursor=$dbFile
while ($cursor) {
    $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Arquivo ou diretorio de conexao usa link/junction proibido.'
    }
    $parent=Split-Path -Parent $cursor
    if ($parent -eq $cursor) { break }
    $cursor=$parent
}
$localServiceRead=$false
foreach ($ace in (Get-Acl -LiteralPath $dbFile).Access) {
    if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow -or
        ($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -eq 0) { continue }
    try { $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value }
    catch { throw 'ACL do arquivo PostgreSQL contem identidade nao reconhecida.' }
    if ($sid -notin @('S-1-5-18','S-1-5-32-544','S-1-5-19')) {
        throw 'Arquivo de conexao PostgreSQL e legivel por identidade nao aprovada.'
    }
    if ($sid -eq 'S-1-5-19') { $localServiceRead=$true }
}
if (-not $localServiceRead) { throw 'A conta LocalService precisa de leitura para iniciar o ERP.' }
$companyName=Read-Host 'Razao social da empresa'
$tradeName=Read-Host 'Nome fantasia (opcional)'
$cnpj=Read-Host 'CNPJ (14 digitos)'
$adminName=Read-Host 'Nome do administrador'
$adminEmail=Read-Host 'E-mail do administrador'
$secret=Read-Host 'Crie uma senha forte (12 a 72 bytes UTF-8)' -AsSecureString
$confirm=Read-Host 'Confirme a senha' -AsSecureString
$secretBstr=[IntPtr]::Zero
$confirmBstr=[IntPtr]::Zero
$plain=''
$confirmed=''
$process=$null
try {
    $secretBstr=[Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
    $confirmBstr=[Runtime.InteropServices.Marshal]::SecureStringToBSTR($confirm)
    $plain=[Runtime.InteropServices.Marshal]::PtrToStringBSTR($secretBstr)
    $confirmed=[Runtime.InteropServices.Marshal]::PtrToStringBSTR($confirmBstr)
    if (-not [string]::Equals($plain,$confirmed,[StringComparison]::Ordinal)) { throw 'As senhas nao coincidem.' }

    $info=[Diagnostics.ProcessStartInfo]::new()
    $info.FileName=$expectedExe
    $quotedPath='"'+$dbFile.Replace('"','')+'"'
    $info.Arguments='--database-url-file '+$quotedPath+' --password-stdin'
    $info.UseShellExecute=$false
    $info.CreateNoWindow=$true
    $info.RedirectStandardInput=$true
    $info.RedirectStandardOutput=$true
    $info.RedirectStandardError=$true
    $info.EnvironmentVariables['ESTERIUM_BOOTSTRAP_LEGAL_NAME']=$companyName
    $info.EnvironmentVariables['ESTERIUM_BOOTSTRAP_TRADE_NAME']=$tradeName
    $info.EnvironmentVariables['ESTERIUM_BOOTSTRAP_CNPJ']=$cnpj
    $info.EnvironmentVariables['ESTERIUM_BOOTSTRAP_ADMIN_NAME']=$adminName
    $info.EnvironmentVariables['ESTERIUM_BOOTSTRAP_ADMIN_EMAIL']=$adminEmail
    $process=[Diagnostics.Process]::Start($info)
    if ($null -eq $process) { throw 'CLI de bootstrap indisponivel.' }
    $outputTask=$process.StandardOutput.ReadToEndAsync()
    $errorTask=$process.StandardError.ReadToEndAsync()
    $process.StandardInput.WriteLine($plain)
    $process.StandardInput.Close()
    if (-not $process.WaitForExit(60000)) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        throw 'Bootstrap excedeu o limite de tempo; preserve o banco para inspecao.'
    }
    $output=$outputTask.GetAwaiter().GetResult().Trim()
    $null=$errorTask.GetAwaiter().GetResult()
    if ($process.ExitCode -ne 0 -or $output -cne 'PASS: first administrator created for the new company.') {
        throw 'Bootstrap recusado. Preserve o banco e consulte o diagnostico protegido.'
    }
    Write-Output 'PASS: primeiro administrador criado; senha nunca foi enviada em argumento ou arquivo.'
} finally {
    if ($null -ne $process) { $process.Dispose() }
    if ($secretBstr -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($secretBstr) }
    if ($confirmBstr -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($confirmBstr) }
    $plain=$null
    $confirmed=$null
    if ($null -ne $secret) { $secret.Dispose() }
    if ($null -ne $confirm) { $confirm.Dispose() }
}
