#requires -Version 5.1
# Fail-closed, network-free negative checks. All real trust checks must run
# separately on each actual POS Windows host and remain a commercial gate.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'LAN preflight tests run on a disposable hosted Windows CI worker.'
}
$tool=Join-Path $PSScriptRoot 'check-esterium-lan-terminal.ps1'
$tokens=$null; $parseErrors=$null
[void][Management.Automation.Language.Parser]::ParseFile(
    $tool,[ref]$tokens,[ref]$parseErrors)
if (@($parseErrors).Count -gt 0) {
    throw ('LAN checker PowerShell syntax: '+
        (@($parseErrors | Select-Object -ExpandProperty Message) -join '; '))
}
function Require-Rejection([string]$url,[string]$hash,[string]$expected) {
    $message=''
    try { & $tool -ERPUrl $url -ExpectedCertificateSHA256 $hash | Out-Null }
    catch { $message=$_.Exception.Message }
    if ($message -notmatch $expected) {
        throw ('Unsafe URL/check failed to block: '+$url+'; reason: '+$message)
    }
}
$approved='a'*64
Require-Rejection 'http://erp.loja.local:8443/' $approved 'exige URL HTTPS'
Require-Rejection 'https://localhost:8443/' $approved 'exige URL HTTPS'
Require-Rejection 'https://127.0.0.1:8443/' $approved 'exige URL HTTPS'
Require-Rejection 'https://erp.loja.local:5432/' $approved 'porta HTTPS'
Require-Rejection 'https://erp.loja.local:6379/' $approved 'porta HTTPS'
Require-Rejection 'https://erp.example:8443/' $approved 'exige URL HTTPS'
Require-Rejection 'https://admin@erp.loja.local:8443/' $approved 'exige URL HTTPS'
Require-Rejection 'https://erp.loja.local:8443/?token=secret' $approved 'exige URL HTTPS'
Require-Rejection 'https://erp.loja.local:8443/' 'invalid-digest' 'Hash SHA-256'
Write-Output 'PASS: Windows terminal LAN checker parses and rejects unsafe HTTP, localhost, public/test names, leaked credentials, DB ports and missing certificate SHA-256 without network access.'
Write-Output 'NOT PROVEN: true DNS, TLS chain/pin, physical LAN computers, firewall and browser trust require an onsite run.'
