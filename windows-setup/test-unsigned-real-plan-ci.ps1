#requires -Version 5.1
<#
  Disposable hosted Windows CI ONLY. Downloads exact upstream bytes, builds
  the real ERP bundle, and runs the unsigned READ-ONLY two-phase plan.
  NEVER use -Apply or execute this test on a user's device.
#>
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1') {
    throw 'Real unsigned plan is restricted to disposable GitHub-hosted Windows CI.'
}
$root=(Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$dest=Join-Path $env:RUNNER_TEMP ('esterium-unsigned-plan-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($dest) | Out-Null
function DownloadPinned([string]$url,[string]$file,[string]$expected) {
    & curl.exe -fLsS --retry 2 --connect-timeout 25 --max-time 240 -o $file $url
    if ($LASTEXITCODE -ne 0) {throw "Pinned upstream download unavailable: $url"}
    if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -cne $expected) {
        throw "Pinned vendor hash mismatch: $file"
    }
}
$edbHash='DE926FEFAD00E313E212CD438C0F04BF033E200099AD56C012724EFCEBED79F2'
$garnetHash='5CD130686AB415B13C59D36D048A6FADED81E707FFE6DA5953F98BEEB79F9F16'
$nssmArchiveHash='99F5045FFFBFFB745D67FE3A065A953C4A3D9C253B868892D9B685B0EE7D07B8'
$nssmHash='EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'
$edbCert='7BEDD1269FCCF7A5D95F18274750B79893C06C70'
$pg=Join-Path $dest 'postgresql.exe'
$gn=Join-Path $dest 'garnet.zip'
$nupkg=Join-Path $dest 'nssm.nupkg'
$nz=Join-Path $dest 'nssm.zip'
$nssm=Join-Path $dest 'nssm.exe'
DownloadPinned 'https://get.enterprisedb.com/postgresql/postgresql-16.15-1-windows-x64.exe' $pg $edbHash
DownloadPinned 'https://github.com/microsoft/garnet/releases/download/v2.2.3/win-x64-based-readytorun.zip' $gn $garnetHash
# Chocolatey is only a byte transport; the extracted NSSM ZIP must match
# the independently pinned WinGet hash.
& curl.exe -fLsS --retry 2 --connect-timeout 25 --max-time 120 -o $nupkg 'https://community.chocolatey.org/api/v2/package/NSSM/2.24.101-g897c7ad'
if ($LASTEXITCODE -ne 0) {throw 'NSSM upstream transport unavailable'}
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip=[IO.Compression.ZipFile]::OpenRead($nupkg)
try {
    $entry=@($zip.Entries | Where-Object { $_.FullName -ceq 'tools/nssm-2.24-101-g897c7ad.zip' })
    if ($entry.Count -ne 1) { throw 'Chocolatey transport lacks pinned NSSM ZIP' }
    $input=$entry[0].Open(); $output=[IO.File]::Create($nz)
    try {$input.CopyTo($output)} finally {$input.Dispose();$output.Dispose()}
} finally { $zip.Dispose() }
if ((Get-FileHash -LiteralPath $nz -Algorithm SHA256).Hash -cne $nssmArchiveHash) {
    throw 'NSSM archive differs from independently pinned WinGet hash'
}
$extract=Join-Path $dest 'nssm-unpacked'
Expand-Archive -LiteralPath $nz -DestinationPath $extract -ErrorAction Stop
Copy-Item -LiteralPath (Join-Path $extract 'nssm-2.24-101-g897c7ad\win64\nssm.exe') -Destination $nssm -ErrorAction Stop
if ((Get-FileHash -LiteralPath $nssm -Algorithm SHA256).Hash -cne $nssmHash) {throw 'Unexpected x64 NSSM'}
$sig=Get-AuthenticodeSignature -LiteralPath $pg
if ($sig.Status -ne [Management.Automation.SignatureStatus]::Valid -or
    $null -eq $sig.SignerCertificate -or
    $sig.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $edbCert) {
    throw 'EDB publisher identity is invalid'
}
$bundle=Join-Path $dest 'erp-bundle'
& (Join-Path $root 'scripts/build-esterium-local-windows.ps1') -OutputDir $bundle | Out-Null
$manifest=(Get-FileHash -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Algorithm SHA256).Hash
$before=@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue)
if ($before.Count -ne 0) {throw 'Runner has unexpected existing services'}
$args=@{ SourceBundleDir=$bundle; AllowedOrigin='https://erp.example.org';
    PostgresInstaller=$pg; ExpectedPostgresSHA256=$edbHash;
    ExpectedPostgresPublisherThumbprint=$edbCert; GarnetPackage=$gn;
    ExpectedGarnetSHA256=$garnetHash; ServiceWrapperExe=$nssm;
    ExpectedServiceWrapperSHA256=$nssmHash; ApprovedManifestSHA256=$manifest }
# Invoke the tools embedded in the built ERP bundle, never unbuilt source scripts.
& (Join-Path $bundle 'tools/install-esterium-local-complete.ps1') @args
$after=@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue)
if ($after.Count -ne 0) {throw 'Read-only plan unexpectedly modified services'}
Write-Output 'PASS: unsigned real ERP + EDB/Garnet/NSSM plan on disposable Windows.'
Write-Output 'NOT INSTALLED: no -Apply, no bypass of privileged Authenticode checks.'

# PHASE ONE ONLY: integrated unsigned native EXE may copy verified ERP to a
# new admin-only Program Files staging directory, but must NOT install vendors,
# register services, migrate database, or enable fiscal/sales features.
$stageDir=Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage'
if (Test-Path -LiteralPath $stageDir) { throw 'Disposable runner already has an ERP open-source stage' }
$exeDir=Join-Path $dest 'oss-exe'
$build=@{
    OutputDir=$exeDir;BundleDir=$bundle;TechnicalPreviewPayload=$true
    ApprovedManifestSHA256=$manifest
    PostgresInstaller=$pg;ExpectedPostgresSHA256=$edbHash
    ExpectedPostgresPublisherThumbprint=$edbCert
    GarnetPackage=$gn;ExpectedGarnetSHA256=$garnetHash
    ServiceWrapperExe=$nssm;ExpectedServiceWrapperSHA256=$nssmHash
}
& (Join-Path $root 'windows-setup/build-esterium-setup-exe.ps1') @build | Out-Null
$exe=Join-Path $exeDir 'EsteriumERP-Setup-TECHNICAL-PREVIEW.exe'
if (-not (Test-Path $exe -PathType Leaf)) {throw 'Missing integrated unsigned EXE'}
foreach ($case in @(@('--has-payload',0),@('--has-preview-approvals',0),@('--is-retail-build',3))) {
    $probe=Start-Process -FilePath $exe -ArgumentList $case[0] -Wait -PassThru
    if ($probe.ExitCode -ne $case[1]) {throw "Unexpected EXE gate for $($case[0])"}
}
if ((Get-AuthenticodeSignature -LiteralPath $exe).Status -ne 'NotSigned') {
    throw 'Technical preview unexpectedly signed'
}
$stage=Start-Process -FilePath $exe -ArgumentList '--stage-oss-files' -Wait -PassThru
if ($stage.ExitCode -ne 0) {throw "Native admin-only ERP stage failed: $($stage.ExitCode)"}
if ((Get-FileHash -LiteralPath (Join-Path $stageDir 'bundle-manifest.json') -Algorithm SHA256).Hash -cne $manifest) {
    throw 'Staged ERP manifest differs from approved original'
}
& (Join-Path $bundle 'tools/verify-esterium-bundle.ps1') -BundleDir $stageDir | Out-Null
if (Test-Path -LiteralPath (Join-Path $stageDir 'vendor')) {throw 'Vendor binary incorrectly copied to ERP stage'}
$acl=Get-Acl -LiteralPath $stageDir
if (-not $acl.AreAccessRulesProtected) {throw 'Open-source ERP stage inherited unauthorized ACLs'}
$access=@($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]))
if (@($access | Where-Object { $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544') }).Count -ne 0) {
    throw 'Open-source ERP staging is writable by an unapproved principal'
}
if (@($access | Where-Object { $_.IdentityReference.Value -eq 'S-1-5-18' }).Count -eq 0 -or
    @($access | Where-Object { $_.IdentityReference.Value -eq 'S-1-5-32-544' }).Count -eq 0) {
    throw 'Native staging missing required SYSTEM/Administrators ACL'
}
$repeated=Start-Process -FilePath $exe -ArgumentList '--stage-oss-files' -Wait -PassThru
if ($repeated.ExitCode -ne 5) {throw 'Reinstallation must be blocked'}
& (Join-Path $bundle 'tools/verify-esterium-bundle.ps1') -BundleDir $stageDir | Out-Null
$services=@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue)
if ($services.Count -ne 0) {throw 'ERP stage unexpectedly registered services'}
Write-Output 'PASS: unsigned native EXE staged verified ERP into admin-only Program Files.'
Write-Output 'PASS: second run rejected without altering staging; vendor, SQL and services not installed.'


# PHASE 2A: stage real, independently pinned EDB/Garnet/NSSM files in
# a separate Program Files directory. NEVER run vendors or register SCM.
$vendorStage=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage'
if (Test-Path -LiteralPath $vendorStage) { throw 'Disposable runner has preexisting vendor staging' }
$prepared=Start-Process -FilePath $exe -ArgumentList '--stage-oss-vendors' -Wait -PassThru
if ($prepared.ExitCode -ne 0) { throw "Native vendor preparation failed: $($prepared.ExitCode)" }
$items=@(
    @{name='postgresql.exe';hash=$edbHash},
    @{name='garnet.zip';hash=$garnetHash},
    @{name='nssm.exe';hash=$nssmHash}
)
$found=@(Get-ChildItem -LiteralPath $vendorStage -File -Force)
if ($found.Count -ne $items.Count) {throw 'Vendor stage has unexpected file count'}
foreach ($item in $items) {
    $p=Join-Path $vendorStage $item.name
    if (-not (Test-Path -LiteralPath $p -PathType Leaf) -or
        (Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash -cne $item.hash) {
        throw "Vendor staged hash mismatch: $($item.name)"
    }
}
$auth=Get-AuthenticodeSignature -LiteralPath (Join-Path $vendorStage 'postgresql.exe')
if ($auth.Status -ne [Management.Automation.SignatureStatus]::Valid -or
    $null -eq $auth.SignerCertificate -or
    $auth.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $edbCert) {
    throw 'Staged EnterpriseDB executable publisher signature invalid'
}
$vendorAcl=Get-Acl -LiteralPath $vendorStage
if (-not $vendorAcl.AreAccessRulesProtected) {throw 'Vendor stage has inherited ACL'}
$entries=@($vendorAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]))
if (@($entries | Where-Object { $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544') }).Count -ne 0) {
    throw 'Vendor stage ACL unexpectedly grants outside principals'
}
$retry=Start-Process -FilePath $exe -ArgumentList '--stage-oss-vendors' -Wait -PassThru
if ($retry.ExitCode -ne 5) {throw 'Vendor stage unexpectedly allowed second execution'}
$services=@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue)
if ($services.Count -ne 0) {throw 'Vendor preparation created an unexpected service'}
Write-Output 'PASS: unsigned native EXE staged genuine EDB/Garnet/NSSM under protected Program Files.'
Write-Output 'PASS: EDB trusted Authenticode signer, pinned hashes and admin-only ACL.'
Write-Output 'PASS: vendor-stage reinstallation blocked; database/credentials/services unchanged.'
