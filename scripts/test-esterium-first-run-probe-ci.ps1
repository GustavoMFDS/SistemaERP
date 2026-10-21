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
if ($denied -notmatch 'not signed by approved publisher') {
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
if ($denied -notmatch 'not signed by approved publisher') {
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
Write-Output 'PASS: first-run runtime probe parses and refuses unsigned Apply before touching Windows services.'
Write-Output 'NOT EXECUTED: genuine signed EDB/NSSM SCM startup remains a vendor pilot gate.'
