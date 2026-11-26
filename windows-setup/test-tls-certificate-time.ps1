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
    # Recovery media must reject invalid public certificate times before SCM mutation.
    $rt=$null;$re=$null
    $recovery=[Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'scripts\inspect-esterium-partial-install.ps1'),[ref]$rt,[ref]$re)
    if($re.Count){throw 'Recovery AST invalid.'}
    foreach($fn in @('Assert-Physical','Assert-RecoveryCertificateTime')){
        $functions=@($recovery.FindAll({param($n)
            $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $fn
        },$true))
        if($functions.Count -ne 1){throw 'Missing recovery helper.'}
        . ([scriptblock]::Create($functions[0].Extent.Text))
    }
    $statements=@($recovery.EndBlock.Statements)
    $first=-1;$last=-1;$mutation=-1
    for($j=0;$j -lt $statements.Count;$j++){
        $statement=$statements[$j].Extent.Text
        if($statement -ceq '$certDir=Join-Path $base ''PostgresTlsPolicy\certificates'''){$first=$j}
        if($statement -ceq 'Assert-RecoveryCertificateTime (Join-Path $certDir ''root.crt'') $now'){$last=$j}
        if($statement -ceq '$completed=$false'){$mutation=$j}
    }
    if($first -lt 0 -or $last -lt $first -or $mutation -le $last){throw 'Recovery gate must precede service mutation.'}
    $recoveryGate=[scriptblock]::Create(($statements[$first..$last] | ForEach-Object {$_.Extent.Text}) -join [Environment]::NewLine)
    function Expect-RecoveryRefusal([string]$Path,[datetime]$At,[string]$Code){
        $message=''
        try{Assert-RecoveryCertificateTime $Path $At}catch{$message=$_.Exception.Message}
        if(-not $message.StartsWith($Code+':')){throw ('Expected recovery rejection: '+$Code)}
    }
    Assert-RecoveryCertificateTime $leaf $from
    Assert-RecoveryCertificateTime $leaf $until
    Expect-RecoveryRefusal $leaf ($from.AddSeconds(-1)) 'RECOVERY_TLS_NOT_YET_VALID'
    Expect-RecoveryRefusal $rootCert ($now.AddHours(-4)) 'RECOVERY_TLS_NOT_YET_VALID'
    Expect-RecoveryRefusal $leaf ($until.AddSeconds(1)) 'RECOVERY_TLS_EXPIRED'
    Expect-RecoveryRefusal $bad $now 'RECOVERY_TLS_INVALID'
    Expect-RecoveryRefusal (Join-Path $dir 'missing.crt') $now 'RECOVERY_TLS_INVALID'
    $base=Join-Path $dir 'recovery-fixture'
    $certDir=Join-Path $base 'PostgresTlsPolicy\certificates'
    [IO.Directory]::CreateDirectory($certDir)|Out-Null
    Copy-Item $leaf (Join-Path $certDir 'server.crt')
    Copy-Item $rootCert (Join-Path $certDir 'root.crt')
    $conf=Join-Path $base 'postgresql.conf'
    $configured=(Join-Path $certDir 'server.crt').Replace('\','/')
    [IO.File]::WriteAllText($conf,("ssl_cert_file = '"+$configured+"'"))
    & $recoveryGate
    [IO.File]::WriteAllText($conf,"ssl_cert_file = 'other.crt'")
    $observed=''
    try{& $recoveryGate}catch{$observed=$_.Exception.Message}
    if(-not $observed.StartsWith('RECOVERY_TLS_INVALID:')){throw 'Unexpected certificate path was accepted.'}
    Write-Output 'PASS: 7 recovery certificate cases and 2 gate cases; before SCM mutation.'
    # Only synthetic files and a stub ACL command in the temporary fixture.
    foreach($fn in @('Copy-ApprovedTLSFile','Write-TLSStagingEvidence')){
        $functions=@($ast.FindAll({param($n)
            $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $fn
        },$true))
        if($functions.Count -ne 1){throw 'Missing provenance helper.'}
        . ([scriptblock]::Create($functions[0].Extent.Text))
    }
    $prov=Join-Path $dir 'provenance-fixture'
    [IO.Directory]::CreateDirectory($prov)|Out-Null
    $sourceChain=Join-Path $prov 'source-chain.txt'
    $sourceKey=Join-Path $prov 'source-key.txt'
    [IO.File]::WriteAllText($sourceChain,'synthetic public material')
    [IO.File]::WriteAllText($sourceKey,'synthetic inert fixture, not a key')
    $chainPin=(Get-FileHash $sourceChain).Hash
    $keyPin=(Get-FileHash $sourceKey).Hash
    $manifestPin='A'*64
    $checked=[datetime]::UtcNow.ToString('o')
    $aclState=@{fail=$false;calls=0}
    function icacls.exe {
        $aclState.calls++
        $global:LASTEXITCODE=if($aclState.fail){1}else{0}
    }
    function Expect-ProvRefusal([scriptblock]$operation,[string]$expected){
        $observed=''
        try{& $operation | Out-Null}catch{$observed=$_.Exception.Message}
        if(-not $observed.Contains($expected)){throw 'Expected provenance refusal.'}
    }
    $targetChain=Join-Path $prov 'staged-chain.txt'
    $targetKey=Join-Path $prov 'staged-key.txt'
    Expect-ProvRefusal {Copy-ApprovedTLSFile $sourceChain $targetChain 'fixture' ('B'*64)} 'changed since'
    if(Test-Path $targetChain){throw 'Mismatch must precede copy.'}
    $null=Copy-ApprovedTLSFile $sourceChain $targetChain 'fixture' $chainPin
    $null=Copy-ApprovedTLSFile $sourceKey $targetKey 'fixture' $keyPin
    Expect-ProvRefusal {Copy-ApprovedTLSFile $sourceChain $targetChain 'fixture' $chainPin} 'destination must be new'
    $receipt=Join-Path $prov 'tls-staging-evidence.json'
    Expect-ProvRefusal {Write-TLSStagingEvidence $prov $targetChain $targetKey ('B'*64) $keyPin $checked $manifestPin} 'differs'
    if(Test-Path $receipt){throw 'Mismatch must precede receipt.'}
    $aclState.fail=$true
    Expect-ProvRefusal {Write-TLSStagingEvidence $prov $targetChain $targetKey $chainPin $keyPin $checked $manifestPin} 'protect TLS staging evidence'
    if((Get-Item $receipt).Length -ne 0){throw 'ACL failure wrote evidence payload.'}
    Remove-Item -LiteralPath $receipt
    $aclState.fail=$false
    if(@(Write-TLSStagingEvidence $prov $targetChain $targetKey $chainPin $keyPin $checked $manifestPin).Count){throw 'Unexpected provenance output.'}
    $record=Get-Content -LiteralPath $receipt -Raw|ConvertFrom-Json
    if($record.kind -cne 'ESTERIUM_TLS_STAGING_EVIDENCE' -or $record.independentApproval -ne $false -or
        $record.chainSHA256 -cne $chainPin -or $record.keySHA256 -cne $keyPin -or $record.releaseManifestSHA256 -cne $manifestPin){
        throw 'Incorrect provenance record.'
    }
    $receiptHash=(Get-FileHash $receipt).Hash
    $refused=$false
    try{Write-TLSStagingEvidence $prov $targetChain $targetKey $chainPin $keyPin $checked $manifestPin}catch{$refused=$true}
    if(-not $refused -or (Get-FileHash $receipt).Hash -cne $receiptHash){throw 'Existing evidence overwritten.'}
    Write-Output 'PASS: 7 provenance cases; synthetic files and stubbed ACL command only.'
} finally {
    if(Test-Path -LiteralPath $dir){Remove-Item -LiteralPath $dir -Recurse -Force}
}
