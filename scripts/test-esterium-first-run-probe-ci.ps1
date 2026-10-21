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
$tokens=$null
$parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile(
    $probe,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('First-run runtime probe PowerShell parser errors: '+
        (@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
$opts=@{BundleDir=$env:RUNNER_TEMP;Apply=$true}
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
    'backupRestoreApproved=$false','automaticStartupApproved=$false','probe-result.json',
    "Set-Service -Name 'EsteriumERP' -StartupType Disabled")) {
    if (-not $text.Contains($required)) {
        throw 'First-run probe lost a required fail-closed security contract.'
    }
}
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
if ($denied -notmatch 'valid HTTPS origin') {
    throw 'Combined installer allowed insecure/origin-ambiguous local settings.'
}
$code=[IO.File]::ReadAllText($coordinator)
foreach ($required in @('stage-esterium-approved-windows-release.ps1',
    'bootstrap-esterium-local-schema.ps1','register-esterium-windows-service.ps1',
    'probe-esterium-local-first-run.ps1','provision-esterium-app-secrets.ps1')) {
    if (-not $code.Contains($required)) {
        throw 'Combined installer lost an essential signed first-install step.'
    }
}
Write-Output 'PASS: combined first-install coordinator parses, blocks bad publisher/origin and retains expected safe ordering.'
Write-Output 'PASS: first-run runtime probe parses and refuses unsigned Apply before touching Windows services.'
Write-Output 'NOT EXECUTED: genuine signed EDB/NSSM SCM startup remains a vendor pilot gate.'
