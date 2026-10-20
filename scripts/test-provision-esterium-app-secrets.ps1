#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Token provisioning test requires an ephemeral GitHub-hosted Windows runner.'
}
$base = Join-Path $env:ProgramData 'Esterium'
$target = Join-Path $base ('SyntheticSecrets-' + [Guid]::NewGuid().ToString('N'))
$helper = Join-Path $PSScriptRoot 'provision-esterium-app-secrets.ps1'
try {
    $plan = @(& $helper -DestinationDir $target) -join [Environment]::NewLine
    if ($plan -notmatch 'no files or services changed' -or (Test-Path -LiteralPath $target)) {
        throw 'Default mode is not non-destructive.'
    }
    $realRejected = $false
    try { & $helper -DestinationDir $target -Apply | Out-Null }
    catch { $realRejected = $_.Exception.Message -match 'independently approved publisher identity' }
    if (-not $realRejected) { throw 'Direct secret provisioner accepted unsigned real Apply.' }
    $env:ESTERIUM_SECRET_DISPOSABLE_CI = '1'
    $output = @(& $helper -DestinationDir $target -Apply -UnsignedDisposableCIFixture) -join [Environment]::NewLine
    if ($output -notmatch 'PASS: generated two application credentials') {
        throw 'Credential generation did not report success.'
    }
    $secrets = @('jwt.secret', 'metrics.secret')
    $tokens = @()
    foreach ($name in $secrets) {
        $file = Join-Path $target $name
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
            throw "Credential was not created: $name"
        }
        $value = (Get-Content -LiteralPath $file -Raw).Trim()
        if ($value.Length -lt 64 -or $value -notmatch '^[A-Za-z0-9_-]{64}$') {
            throw "Credential entropy/format is incorrect: $name"
        }
        if ($output.Contains($value) -or $plan.Contains($value)) {
            throw "Generated secret leaked in script output: $name"
        }
        $tokens += $value
        $acl = Get-Acl -LiteralPath $file
        $localServiceCanRead = $false
        foreach ($entry in $acl.Access) {
            if ($entry.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) {
                continue
            }
            $sid = $entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
            $canRead = ($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0
            if (-not $canRead) { continue }
            if ($sid -in @('S-1-1-0','S-1-5-11','S-1-5-32-545','S-1-5-7')) {
                throw "Broad group has access to $name"
            }
            if ($sid -eq 'S-1-5-19') { $localServiceCanRead = $true }
        }
        if (-not $localServiceCanRead) {
            throw "LocalService cannot read $name"
        }
    }
    if ($tokens[0] -eq $tokens[1]) {
        throw 'JWT and metrics credentials are identical.'
    }
    $secondRejected = $false
    try { & $helper -DestinationDir $target -Apply -UnsignedDisposableCIFixture | Out-Null }
    catch { $secondRejected = $_.Exception.Message -match 'already exists' }
    if (-not $secondRejected) { throw 'Existing credential folder was overwritten.' }
    $after = (Get-Content -LiteralPath (Join-Path $target 'jwt.secret') -Raw).Trim()
    if ($after -ne $tokens[0]) {
        throw 'Credential contents changed after rejected overwrite.'
    }
    Write-Output 'PASS: random tokens, LocalService read ACL, no stdout leak, dry-run and overwrite refusal.'
} finally {
    Remove-Item Env:ESTERIUM_SECRET_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Recurse -Force
    }
}
