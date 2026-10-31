#requires -Version 5.1
# Fail-closed FIRST-RUN probe contract, hosted Windows CI only. No services
# or customer databases are started, registered or modified by this test.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'First-run probe safety test requires disposable Windows CI.'
}
$probe=Join-Path $PSScriptRoot 'probe-esterium-local-first-run.ps1'
$activation=Join-Path $PSScriptRoot 'activate-esterium-local-first-install.ps1'
$tokens=$null
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile(
    $probe,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('First-run runtime probe PowerShell parser errors: '+
        (@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$opts=@{
    BundleDir=$env:RUNNER_TEMP
    AllowedOrigin='https://esterium.ci.invalid:8443'
    ListenAddress='127.0.0.1:8443'
    TLSCertFile=(Join-Path $env:RUNNER_TEMP 'synthetic-erp-cert.pem')
    TLSKeyFile=(Join-Path $env:RUNNER_TEMP 'synthetic-erp-key.pem')
    Apply=$true
}
$denied=''
try { & $probe @opts | Out-Null }
catch { $denied=$_.Exception.Message }
if ($denied -notmatch 'independently approved signature and release digest') {
    throw 'Unsigned first-run probe accepted an unknown publisher/release.'
}
$opts.ApprovedManifestSHA256='0'*64
$opts.TrustedPublisherThumbprint='0'*40
$denied=''
try { & $probe @opts | Out-Null }
catch { $denied=$_.Exception.Message }
if ($denied -notmatch '(not signed by approved publisher|cannot be trusted)') {
    throw 'Unsigned first-run probe passed a fake approved digest and thumbprint.'
}
# Retain a static rule to catch accidental removal of the safety finalizer.
$text=[IO.File]::ReadAllText($probe)
foreach ($required in @('finally {','-StartupType Disabled','--check-dependencies',
    'NFCE_SEFAZ_PRODUCTION_ENABLED','FailOnRecoveryError','-RequireServiceOwnership',
    'First-run probe required local item is missing:','GetFileName($missingCursor)','TrimEnd([char[]]@([IO.Path]::DirectorySeparatorChar',
    'backupRestoreApproved=$false','automaticStartupApproved=$false','probe-result.json',
    "Set-Service -Name 'EsteriumERP' -StartupType Disabled")) {
    if (-not $text.Contains($required)) {
        throw 'First-run probe lost a required fail-closed security contract.'
    }
}
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile($activation,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('Final HTTPS activation parser errors: '+(@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$activationText=[IO.File]::ReadAllText($activation)
foreach ($required in @('ESTERIUM_FINAL_ACTIVATION_DISPOSABLE_CI','TrustedRemoteAddress',
    'Get-NetFirewallProfile','New-NetFirewallRule','Get-NetFirewallApplicationFilter',
    'WaitForStatus','Move-Item -LiteralPath $staging -Destination $activationPath',
    'automaticStartupApproved=$true','fiscalEmissionApproved=$false')) {
    if (-not $activationText.Contains($required)) {
        throw 'Final activation lost a required HTTPS/firewall/startup contract.'
    }
}
$repair=Join-Path $PSScriptRoot 'repair-esterium-local-install.ps1'
$uninstall=Join-Path $PSScriptRoot 'uninstall-esterium-local-app.ps1'
foreach($path in @($repair,$uninstall)){
    $parseErrors=$null
    [void][Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$parseErrors)
    if(@($parseErrors).Count -gt 0){
        throw ('Repair/uninstall PowerShell parser error: '+$path+' '+(@($parseErrors|Select-Object -ExpandProperty Message)-join '; '))
    }
}
$repairText=[IO.File]::ReadAllText($repair)
foreach($required in @('ESTERIUM_REPAIR_DISPOSABLE_CI','ConfirmRepair','bundle-manifest.json',
    'Get-NetFirewallRule','/health/ready','backupRestoreApproved -cne $false')){
    if(-not $repairText.Contains($required)){throw 'Assisted repair lost its signed/receipt/health/non-fiscal guard.'}
}
$uninstallText=[IO.File]::ReadAllText($uninstall)
foreach($required in @('ESTERIUM_UNINSTALL_DISPOSABLE_CI','ConfirmUninstall',
    'unregister-esterium-windows-service.ps1','Move-Item -LiteralPath $sourceDir',
    'Remove-Item -LiteralPath $install -Recurse -Force','ProgramData','backups stay intact')){
    if(-not $uninstallText.Contains($required)){throw 'Conservative uninstall lost its confirmation/data-preservation guard.'}
}
Write-Output 'PASS: assisted repair and conservative uninstall parse and retain signed receipt, HTTPS, confirmation and data-preservation gates.'
# SQL coordinator must reject unsigned entry points before starting SQL.
$sqlTool=Join-Path $PSScriptRoot 'bootstrap-esterium-local-schema.ps1'
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile($sqlTool,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('Initial SQL coordinator syntax invalid: '+(@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$denied=''
try { & $sqlTool -BundleDir $env:RUNNER_TEMP -Apply | Out-Null }
catch { $denied=$_.Exception.Message }
if ($denied -notmatch 'independent release hash and publisher approval') {
    throw 'Unsigned SQL installer tried to proceed without approval.'
}
$denied=''
try {
    & $sqlTool -BundleDir $env:RUNNER_TEMP -Apply -ApprovedManifestSHA256 ('0'*64) -TrustedPublisherThumbprint ('0'*40) | Out-Null
} catch { $denied=$_.Exception.Message }
if ($denied -notmatch '(not signed by approved publisher|cannot be trusted)') {
    throw 'SQL installer accepted a fake signing identity.'
}
$sqlText=[IO.File]::ReadAllText($sqlTool)
foreach ($must in @('-StartupType Disabled','finally {','apply-esterium-initial-schema.ps1',
    'initialize-esterium-postgres-app-db.ps1','TLS_POLICY_STAGED_NEVER_STARTED','ProgramData','Get-CimInstance')) {
    if (-not $sqlText.Contains($must)) {
        throw 'Initial SQL bootstrap lost a protected first-install contract.'
    }
}
Write-Output 'PASS: first-install SQL bootstrap parses and refuses unknown publisher before accessing Windows services.'
# Fresh signed release staging never accepts an untrusted Publisher or hash.
$stageTool=Join-Path $PSScriptRoot 'stage-esterium-approved-windows-release.ps1'
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile($stageTool,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('Approved release staging parser errors: '+(@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$denied=''
try { & $stageTool -SourceBundleDir $env:RUNNER_TEMP -Apply | Out-Null }
catch { $denied=$_.Exception.Message }
if ($denied -notmatch 'independently approved digest and publisher') {
    throw 'New-install release staging accepted an unapproved manifest.'
}
$denied=''
try {
    & $stageTool -SourceBundleDir $env:RUNNER_TEMP -Apply -ApprovedManifestSHA256 ('0'*64) -TrustedPublisherThumbprint ('0'*40) | Out-Null
} catch { $denied=$_.Exception.Message }
if ($denied -notmatch 'not signed by approved publisher') {
    throw 'Release staging accepted an unapproved signer.'
}
Write-Output 'PASS: new release staging parser and approved-signature pre-write gate reject unknown publishers.'
# Combined coordinator must be syntactically valid and must refuse unsigned
# initial installation without writing any PostgreSQL/SCM/ERP state.
$coordinator=Join-Path $PSScriptRoot 'finish-esterium-local-first-install.ps1'
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile($coordinator,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('Combined first-install coordinator parser error: '+(@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$denied=''
try {
    & $coordinator -SourceBundleDir $env:RUNNER_TEMP -AllowedOrigin 'https://ci.invalid' -Apply | Out-Null
} catch { $denied=$_.Exception.Message }
if ($denied -notmatch 'independently approved manifest SHA-256 and publisher') {
    throw 'Combined installer allowed missing release publisher approval.'
}
$denied=''
try {
    & $coordinator -SourceBundleDir $env:RUNNER_TEMP -AllowedOrigin 'https://ci.invalid' -Apply -ApprovedManifestSHA256 ('0'*64) -TrustedPublisherThumbprint ('0'*40) | Out-Null
} catch { $denied=$_.Exception.Message }
if ($denied -notmatch '(not signed by approved publisher|not signed by independently approved publisher)') {
    throw 'Combined installer allowed synthetic invalid code-signing certificate.'
}
$denied=''
try {
    & $coordinator -SourceBundleDir $env:RUNNER_TEMP -AllowedOrigin 'http://localhost:8080' | Out-Null
} catch { $denied=$_.Exception.Message }
if ($denied -notmatch '(?i)(valid (?:root )?HTTPS origin|origem HTTPS)') {
    throw 'Combined installer allowed insecure/origin-ambiguous local settings.'
}
$code=[IO.File]::ReadAllText($coordinator)
foreach ($required in @('stage-esterium-approved-windows-release.ps1',
    'bootstrap-esterium-local-schema.ps1','register-esterium-windows-service.ps1',
    'probe-esterium-local-first-run.ps1','activate-esterium-local-first-install.ps1',
    'provision-esterium-app-secrets.ps1',
    "Set-Service -Name 'EsteriumERP' -StartupType Disabled")) {
    if (-not $code.Contains($required)) {
        throw 'Combined installer lost an essential signed first-install step.'
    }
}
$probeStart=$code.IndexOf('$probe=@{')
$probeEnd=$code.IndexOf("`n# No registered", $probeStart)
if ($probeStart -lt 0 -or $probeEnd -le $probeStart) {
    throw 'Combined installer probe argument block is missing or malformed.'
}
$probeArguments=$code.Substring($probeStart,$probeEnd-$probeStart)
foreach ($required in @('AllowedOrigin=$AllowedOrigin','ListenAddress=$ListenAddress',
    'TLSCertFile=$TLSCertFile','TLSKeyFile=$TLSKeyFile',
    'TrustedRemoteAddress=$TrustedRemoteAddress')) {
    if (-not $probeArguments.Contains($required)) {
        throw 'Combined installer failed to pass approved HTTPS inputs to the first-run probe.'
    }
}
Write-Output 'PASS: combined first-install coordinator parses, blocks bad publisher/origin and passes approved HTTPS inputs to the probe.'
# Combined entry point must be signed, safely deny bad approvals and retain
# the non-mutating PLAN before either vendor or ERP initialization.
$one=Join-Path $PSScriptRoot 'install-esterium-local-complete.ps1'
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile(
    $one,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('One-command Windows installer parser errors: '+
        (@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$entryText=[IO.File]::ReadAllText($one)
foreach ($part in @('setup-esterium-local-dependencies.ps1',
                    'finish-esterium-local-first-install.ps1',
                    'Get-AuthenticodeSignature',
                    '& $setup @setupArgs | Out-Null',
                    '& $finish @finishArgs | Out-Null',
                    '$setupArgs.Apply=$true',
                    '$finishArgs.Apply=$true',
                    'nao repetir Apply')) {
    if (-not $entryText.Contains($part)) {
        throw 'Single-command installer lost a required fail-closed operation.'
    }
}
$fakeRoot=Join-Path $env:RUNNER_TEMP ('esterium-first-install-entry-'+[Guid]::NewGuid().ToString('N'))
$fakeTools=Join-Path $fakeRoot 'tools'
[IO.Directory]::CreateDirectory($fakeTools) | Out-Null
try {
    foreach ($file in @('activate-esterium-local-first-install.ps1','install-esterium-local-complete.ps1',
                        'setup-esterium-local-dependencies.ps1',
                        'finish-esterium-local-first-install.ps1')) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $file) -Destination $fakeTools -ErrorAction Stop
    }
    $fakeEntry=Join-Path $fakeTools 'install-esterium-local-complete.ps1'
    $args=@{
        SourceBundleDir=$fakeRoot
        AllowedOrigin='https://esterium.ci.invalid:8443'
        PostgresInstaller=(Join-Path $fakeRoot 'postgres.exe')
        ExpectedPostgresSHA256=('0'*64)
        ExpectedPostgresPublisherThumbprint=('0'*40)
        GarnetPackage=(Join-Path $fakeRoot 'garnet.zip')
        ExpectedGarnetSHA256=('0'*64)
        ServiceWrapperExe=(Join-Path $fakeRoot 'nssm.exe')
        ExpectedServiceWrapperSHA256=('0'*64)
        ApprovedManifestSHA256=('0'*64)
        TrustedPublisherThumbprint=('0'*40)
        Apply=$true
    }
    $denied=''
    try { & $fakeEntry @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'sem assinatura do editor aprovado') {
        throw 'One-command setup did not refuse an untrusted publisher before side effects.'
    }
    $args.ApprovedManifestSHA256=''
    $denied=''
    try { & $fakeEntry @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'manifesto e editor aprovados independentemente') {
        throw 'One-command setup accepted missing release approval.'
    }
    $args.Apply=$false
    $args.AllowedOrigin='http://localhost:8080'
    $denied=''
    try { & $fakeEntry @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'origem HTTPS aprovada') {
        throw 'One-command setup accepted an insecure browser origin.'
    }
    $args.AllowedOrigin='https://esterium.ci.invalid:8443'
    $args.GarnetPort=5432
    $denied=''
    try { & $fakeEntry @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'precisam ser distintas') {
        throw 'One-command setup accepted conflicting database/cache ports.'
    }
    if (@(Get-ChildItem -LiteralPath $fakeTools -File).Count -ne 4) {
        throw 'One-command negative tests mutated disposable source tools.'
    }
} finally {
    Remove-Item -LiteralPath $fakeRoot -Recurse -Force -ErrorAction Stop
}
Write-Output 'PASS: single-command installer parses and refuses unsigned Apply, missing approval and unsafe origin/ports.'

Write-Output 'PASS: first-run runtime probe parses and refuses unsigned Apply before touching Windows services.'
Write-Output 'NOT EXECUTED: genuine signed EDB/NSSM SCM startup remains a vendor pilot gate.'
