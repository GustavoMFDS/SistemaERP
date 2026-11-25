#requires -Version 5.1
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$GeneratorExe)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
# Only exercise the exact pure certificate-time helper, never real Apply.
$root=Split-Path -Parent $PSScriptRoot
$t=$null;$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $root 'scripts\finish-esterium-local-first-install.ps1'),[ref]$t,[ref]$e)
if($e.Count){throw 'Coordinator AST invalid.'}
$nodes=@($ast.FindAll({param($n)
    $n -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $n.Name -ceq 'Assert-TLSCertificateTime'
},$true))
if($nodes.Count -ne 1){throw 'Expected one temporal validation helper.'}
. ([scriptblock]::Create($nodes[0].Extent.Text))
# Structural regression: the signed coordinator must check the certificates
# before schema, administrator startup and readiness probing. These checks
# inspect syntax only; they never execute the installation.
$gates=@($ast.FindAll({param($n)
    $n -is [Management.Automation.Language.CommandAst] -and
    $n.GetCommandName() -ceq 'Assert-InstallTLSCertificateTimes'
},$true) | Sort-Object {$_.Extent.StartOffset})
if($gates.Count -ne 3){throw 'Expected three TLS time gates.'}
$source=$ast.Extent.Text
$anchors=@(
    'bootstrap-esterium-local-schema.ps1',
    'Set-Service -Name $PostgresServiceName -StartupType Manual',
    'probe-esterium-local-first-run.ps1'
)
for($index=0;$index -lt $anchors.Count;$index++){
    $offset=$source.IndexOf($anchors[$index],[StringComparison]::Ordinal)
    if($offset -lt 0 -or $gates[$index].Extent.StartOffset -ge $offset){
        throw ('Missing preceding TLS gate '+$index)
    }
}
$stageChecks=@($ast.FindAll({param($n)
    $n -is [Management.Automation.Language.TryStatementAst] -and
    $n.Body.Extent.Text.Contains("Assert-TLSCertificateTime "+'$TLSCertFile') -and
    $n.Extent.Text.Contains("['EsteriumInstallStage']='tls-certificate-time'")
},$true))
if($stageChecks.Count -ne 1){throw 'HTTPS input rejection must carry fixed TLS failure stage.'}
Write-Output 'PASS: 3 structural TLS gate-order checks and HTTPS error-stage mapping.'
$dir=Join-Path ([IO.Path]::GetTempPath()) ('esterium-tls-time-'+[guid]::NewGuid().ToString('N'))
try {
    [IO.Directory]::CreateDirectory($dir)|Out-Null
    & $GeneratorExe -out-dir $dir | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Fixture generation failed.'}
    $leaf=Join-Path $dir 'server.crt'
    $rootCert=Join-Path $dir 'root.crt'
    $chain=Join-Path $dir 'chain.pem'
    [IO.File]::WriteAllText($chain,([IO.File]::ReadAllText($leaf)+[Environment]::NewLine+[IO.File]::ReadAllText($rootCert)))
    $cert=[Security.Cryptography.X509Certificates.X509Certificate2]::new($leaf)
    try {$from=$cert.NotBefore.ToUniversalTime();$until=$cert.NotAfter.ToUniversalTime()}finally{$cert.Dispose()}
    function Expect-Refusal([string]$path,[datetime]$at,[string]$code){
        $observed=''
        try {Assert-TLSCertificateTime $path 'fixture' $at}catch{$observed=$_.Exception.Message}
        if(-not $observed.StartsWith($code+':')){throw ('Expected rejection: '+$code)}
    }
    $now=[datetime]::UtcNow
    Assert-TLSCertificateTime $chain 'fixture' $now
    Assert-TLSCertificateTime $leaf 'fixture' $from
    Assert-TLSCertificateTime $leaf 'fixture' $until
    Expect-Refusal $leaf ($from.AddSeconds(-1)) 'TLS_CERTIFICATE_NOT_YET_VALID'
    Expect-Refusal $chain ($now.AddHours(-4)) 'TLS_CERTIFICATE_NOT_YET_VALID'
    Expect-Refusal $leaf ($until.AddSeconds(1)) 'TLS_CERTIFICATE_EXPIRED'
    $bad=Join-Path $dir 'bad.pem'
    [IO.File]::WriteAllText($bad,'-----BEGIN CERTIFICATE-----broken!-----END CERTIFICATE-----')
    Expect-Refusal $bad $now 'TLS_CERTIFICATE_INVALID'
    [IO.File]::AppendAllText($chain,[IO.File]::ReadAllText($bad))
    Expect-Refusal $chain $now 'TLS_CERTIFICATE_INVALID'
    Write-Output 'PASS: 8 synthetic certificate time/format cases; no services, trust store or clock changes.'
} finally {
    if(Test-Path -LiteralPath $dir){Remove-Item -LiteralPath $dir -Recurse -Force}
}
