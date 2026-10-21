#requires -Version 5.1
<#
  Opt-in, NEW-install-only Garnet configuration. Not a vendor installer.
  Default is read-only. -Apply creates a private config, does not start services,
  touch PostgreSQL, open sockets, update old settings or enable fiscal traffic.
  Garnet needs CertPassword in its JSON config: protect the resulting file.
#>
param(
    [Parameter(Mandatory=$true)][string]$AclFile,
    [Parameter(Mandatory=$true)][string]$CertificatePfxFile,
    [Parameter(Mandatory=$true)][string]$CertificatePasswordFile,
    [string]$DestinationDir = (Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
    [ValidateRange(1,65535)][int]$Port = 6379,
    [switch]$Apply,
    [string]$TrustedPublisherThumbprint = '',
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Garnet config tool requires Windows.' }
# A direct -Apply bypasses the signed orchestration script. Each helper
# therefore authenticates its own publisher before reading private inputs
# or creating new credential/configuration files.
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned configuration fixture requires isolated GitHub-hosted Windows CI.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Garnet configuration requires an independently approved publisher identity.'
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
            $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Garnet configuration requires the original helper signed by the approved publisher.'
    }
}

$base = [IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$target = [IO.Path]::GetFullPath($DestinationDir).TrimEnd('\')
if (-not [IO.Path]::IsPathRooted($DestinationDir) -or
    -not $target.StartsWith($base + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Destination must be a new folder under ProgramData\Esterium.'
}
if (Test-Path -LiteralPath $target) { throw 'Target exists; refusing to replace Garnet settings.' }
$parent = Split-Path -Parent $target
$cursor = $parent
while (-not [string]::IsNullOrWhiteSpace($cursor)) {
    if (Test-Path -LiteralPath $cursor) {
        $item = Get-Item -LiteralPath $cursor -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Garnet config path contains a link or junction.'
        }
    }
    $next = Split-Path -Parent $cursor
    if ($next -eq $cursor) { break }
    $cursor = $next
}
$inputs = @(
    @{label='ACL'; value=$AclFile},
    @{label='PFX'; value=$CertificatePfxFile},
    @{label='password'; value=$CertificatePasswordFile}
)
$paths = @{}
foreach ($source in $inputs) {
    $label = [string]$source.label
    $value = [string]$source.value
    if (-not [IO.Path]::IsPathRooted($value) -or
        -not (Test-Path -LiteralPath $value -PathType Leaf)) {
        throw "$label requires an existing absolute file."
    }
    $path = [IO.Path]::GetFullPath($value)
    if ($path.StartsWith($target + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw "$label must be outside the new config."
    }
    $item = Get-Item -LiteralPath $path -Force
    if ($item.Length -le 0 -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "$label must be a nonempty ordinary file."
    }
    $acl = Get-Acl -LiteralPath $path
    $networkServiceRead = $false
    foreach ($rule in $acl.Access) {
        if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        try { $sid = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value }
        catch { throw "Cannot resolve access control for $label." }
        $reads = ($rule.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0
        if ($sid -eq 'S-1-5-20') {
            $writes = [Security.AccessControl.FileSystemRights]::WriteData -bor
                [Security.AccessControl.FileSystemRights]::AppendData -bor
                [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
                [Security.AccessControl.FileSystemRights]::Delete -bor
                [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
                [Security.AccessControl.FileSystemRights]::TakeOwnership
            if (($rule.FileSystemRights -band $writes) -ne 0) {
                throw "NETWORK SERVICE must not be able to change $label."
            }
            if ($reads) { $networkServiceRead = $true }
        }
        if ($reads -and $sid -notin @('S-1-5-20','S-1-5-18','S-1-5-32-544')) {
            throw "$label may not be read by other Windows accounts."
        }
    }
    if (-not $networkServiceRead) { throw "$label requires NETWORK SERVICE read access." }
    $paths[$label] = $path
}
# Never accept the default account or a passwordless ACL user. The file is
# still vendor-owned: check syntax and then test real Garnet ACL behavior.
$aclText = [IO.File]::ReadAllText($paths['ACL'])
if ($aclText.Length -gt 131072 -or
    $aclText -notmatch '(?im)^\s*user\s+default\s+off(?:\s|$)' -or
    $aclText -match '(?im)^\s*user\s+default\s+on(?:\s|$)' -or
    $aclText -match '(?i)\bnopass\b' -or
    $aclText -notmatch '(?im)^\s*user\s+(?!default\b)\S+\s+on\b[^\r\n]*[>#]\S+') {
    throw 'Garnet ACL must disable default, prohibit nopass, and enable a named password-protected account.'
}
if ([IO.Path]::GetExtension($paths['PFX']).ToLowerInvariant() -ne '.pfx') {
    throw 'A PFX certificate is required for this Garnet setup.'
}
if ($Port -eq 8080) { throw 'Garnet must not use the Esterium web port.' }
if (@(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue).Count -gt 0) {
    throw 'The selected Garnet port is occupied.'
}
if (-not $Apply) {
    Write-Output 'PLAN: new Garnet JSON with loopback TLS, ACL, Lua, AOF synchronous commit and fail-closed recovery.'
    Write-Output 'PLAN: cert password will exist only in an ACL-protected config, never in a process argument or ZIP.'
    Write-Output 'PLAN: NETWORK SERVICE read-only; SYSTEM and Administrators full control. No changes made.'
    return
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator permission is required to provision Garnet configuration.'
}
$password = [IO.File]::ReadAllText($paths['password']).TrimEnd([char]13,[char]10)
if ([string]::IsNullOrWhiteSpace($password) -or $password.Length -gt 4096) {
    throw 'PFX password file is empty or invalid.'
}
$cert = $null
try {
    $cert = [Security.Cryptography.X509Certificates.X509Certificate2]::new(
        $paths['PFX'], $password,
        [Security.Cryptography.X509Certificates.X509KeyStorageFlags]::EphemeralKeySet)
    if (-not $cert.HasPrivateKey -or $cert.NotAfter.ToUniversalTime() -le [DateTime]::UtcNow -or
        $cert.NotBefore.ToUniversalTime() -gt [DateTime]::UtcNow) {
        throw 'TLS certificate requires a valid current private key.'
    }
} catch { throw 'TLS PFX or password invalid, or private key missing/expired.' }
finally { if ($null -ne $cert) { $cert.Dispose() } }
$icacls = Join-Path $env:SystemRoot 'System32\icacls.exe'
[IO.Directory]::CreateDirectory($parent) | Out-Null
$stage = Join-Path $parent ('.esterium-garnet-' + [Guid]::NewGuid().ToString('N'))
$committed = $false
try {
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    & $icacls $stage '/inheritance:r' '/grant:r' '*S-1-5-20:(OI)(CI)RX' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict Garnet config folder ACL.' }
    # The read-only configuration has separate mutable persistence storage.
    # The service is NOT started here. Recover=true is kept for first and
    # subsequent starts; disposable Windows CI checks both with the same config.
    $state=Join-Path $stage 'state'
    [IO.Directory]::CreateDirectory($state) | Out-Null
    & $icacls $state '/inheritance:r' '/grant:r' '*S-1-5-20:(OI)(CI)M' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict Garnet persistence directory ACL.' }
    [IO.Directory]::CreateDirectory((Join-Path $state 'checkpoints')) | Out-Null
    $config = [ordered]@{
        Address='127.0.0.1'; Port=$Port; EnableCluster=$false
        AuthenticationMode='ACL'; AclFile=$paths['ACL']
        EnableTLS=$true; ClientCertificateRequired=$false; CertFileName=$paths['PFX']; CertPassword=$password
        EnableLua=$true; LuaTransactionMode=$true
        LuaMemoryManagementMode='Tracked'; LuaScriptMemoryLimit='8m'; LuaScriptTimeoutMs=5000
        EnableAOF=$true; WaitForCommit=$true; CommitFrequencyMs=0
        Recover=$true; FailOnRecoveryError=$true; UseAofNullDevice=$false
        CheckpointDir=(Join-Path $target 'state\checkpoints')
    }
    $out = Join-Path $stage 'garnet.conf'
    $bytes = [Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $config -Depth 3) + [Environment]::NewLine)
    $handle = [IO.File]::Open($out,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $handle.Write($bytes,0,$bytes.Length); $handle.Flush($true) }
    finally { $handle.Dispose(); [Array]::Clear($bytes,0,$bytes.Length) }
    & $icacls $out '/inheritance:r' '/grant:r' '*S-1-5-20:R' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict Garnet config file ACL.' }
    if (Test-Path -LiteralPath $target) { throw 'Another process created the target.' }
    Move-Item -LiteralPath $stage -Destination $target -ErrorAction Stop
    $committed = $true
    Write-Output 'PASS: new protected Garnet configuration and separate AOF data folders created; no service or database changed.'
    Write-Output 'NEXT: install an independently approved Garnet service and verify real TLS/ACL/Lua compatibility before use.'
} finally {
    $password = $null
    if (-not $committed -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force
    }
}
