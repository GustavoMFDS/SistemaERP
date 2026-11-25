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
    $inputGate=@($ast.FindAll({param($n)
        $n -is [Management.Automation.Language.IfStatementAst] -and
        $n.Clauses[0].Item1.Extent.Text -ceq '$Apply -or $ValidateTLSInput'
    },$true))
    $planReturn=@($ast.FindAll({param($n)
        $n -is [Management.Automation.Language.IfStatementAst] -and
        $n.Clauses[0].Item1.Extent.Text -ceq '-not $Apply'
    },$true))
    if($inputGate.Count -ne 1 -or $planReturn.Count -ne 1 -or
        $inputGate[0].Extent.EndOffset -ge $planReturn[0].Extent.StartOffset){throw 'Preflight must precede PLAN return.'}
    $sourceHelper=@($ast.FindAll({param($n)
        $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Assert-TLSPEMSource'
    },$true))
    if($sourceHelper.Count -ne 1){throw 'Missing input source helper.'}
    . ([scriptblock]::Create($sourceHelper[0].Extent.Text))
    $gate=[scriptblock]::Create($inputGate[0].Extent.Text)
    $Apply=$false
    $ValidateTLSInput=$false
    $TLSCertFile=''
    $TLSKeyFile=''
    & $gate
    $ValidateTLSInput=$true
    $TLSCertFile=$leaf
    $TLSKeyFile=Join-Path $dir 'server.key'
    & $gate
    $TLSCertFile=$bad
    $stage=''
    try {& $gate}catch{$stage=[string]$_.Exception.Data['EsteriumInstallStage']}
    if($stage -cne 'tls-certificate-time'){throw 'Malformed input did not receive TLS stage.'}
    $ct=$null;$ce=$null
    $complete=[Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'scripts\install-esterium-local-complete.ps1'),[ref]$ct,[ref]$ce)
    if($ce.Count){throw 'Complete installer AST invalid.'}
    $preflight=@($complete.FindAll({param($n)
        $n -is [Management.Automation.Language.TryStatementAst] -and $n.Body.Extent.Text.Contains('-ValidateTLSInput')
    },$true))
    if($preflight.Count -ne 1 -or $preflight[0].Extent.EndOffset -ge $complete.Extent.Text.IndexOf('$setupArgs.Apply=$true',[StringComparison]::Ordinal)){throw 'Input gate must precede dependency Apply.'}
    $fixture=Join-Path $dir 'preflight-fixture.ps1'
    $prefix=@(
        'param([string]$Case)',
        '$ErrorActionPreference=''Stop''',
        '$Apply=$true',
        '$GuiFailureExitCodes=$true',
        '$finishArgs=@{}',
        'function Invoke-FinishFixture {',
        'param([switch]$ValidateTLSInput)',
        'if(-not $ValidateTLSInput){throw ''Preflight missing''}',
        'if($Case -eq ''valid''){return}',
        '$err=[Exception]::new(''Synthetic failure'')',
        'if($Case -eq ''tls''){$err.Data[''EsteriumInstallStage'']=''tls-certificate-time''}',
        'if($Case -eq ''wrapped''){$err.Data[''EsteriumInstallStage'']=''tls-certificate-time'';$err=[Exception]::new(''Outer synthetic failure'',$err)}',
        'throw $err',
        '}',
        '$finish=''Invoke-FinishFixture'''
    ) -join [Environment]::NewLine
    [IO.File]::WriteAllText($fixture,$prefix+[Environment]::NewLine+$preflight[0].Extent.Text+[Environment]::NewLine+'exit 0')
    foreach($case in @(@{Name='valid';Exit=0},@{Name='tls';Exit=29},@{Name='wrapped';Exit=29},@{Name='unknown';Exit=21})){
        $result=@(& (Join-Path $PSHOME 'powershell.exe') -NoProfile -File $fixture -Case $case.Name 2>&1)
        if($LASTEXITCODE -ne $case.Exit -or $result.Count){throw ('Preflight fixture failed: '+$case.Name)}
    }
    Write-Output 'PASS: 3 input-gate cases, 4 fixed child exit cases and dependency ordering; no real Apply.'
} finally {
    if(Test-Path -LiteralPath $dir){Remove-Item -LiteralPath $dir -Recurse -Force}
}
