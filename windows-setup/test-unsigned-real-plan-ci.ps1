#requires -Version 5.1
<#
  Disposable hosted Windows CI ONLY. Downloads exact upstream bytes, builds
  the real ERP bundle, and runs the unsigned READ-ONLY two-phase plan.
  NEVER use -Apply or execute this test on a user's device.
#>
[CmdletBinding()]
param([switch]$PostgresBinaryZip)
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
$edbHash=if($PostgresBinaryZip){'43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'}else{'DE926FEFAD00E313E212CD438C0F04BF033E200099AD56C012724EFCEBED79F2'}
$garnetHash='5CD130686AB415B13C59D36D048A6FADED81E707FFE6DA5953F98BEEB79F9F16'
$nssmArchiveHash='99F5045FFFBFFB745D67FE3A065A953C4A3D9C253B868892D9B685B0EE7D07B8'
$nssmHash='EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'
$edbCert='7BEDD1269FCCF7A5D95F18274750B79893C06C70'
$pg=Join-Path $dest $(if($PostgresBinaryZip){'postgresql-binaries.zip'}else{'postgresql.exe'})
$gn=Join-Path $dest 'garnet.zip'
$nupkg=Join-Path $dest 'nssm.nupkg'
$nz=Join-Path $dest 'nssm.zip'
$nssm=Join-Path $dest 'nssm.exe'
$pgURL=if($PostgresBinaryZip){'https://get.enterprisedb.com/postgresql/postgresql-16.15-5-windows-x64-binaries.zip'}else{'https://get.enterprisedb.com/postgresql/postgresql-16.15-1-windows-x64.exe'}
DownloadPinned $pgURL $pg $edbHash
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
if (-not $PostgresBinaryZip){
    $sig=Get-AuthenticodeSignature -LiteralPath $pg
    if ($sig.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $sig.SignerCertificate -or
        $sig.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $edbCert){
        throw 'EDB EXE publisher identity is invalid'
    }
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
if(-not $PostgresBinaryZip){
    & (Join-Path $bundle 'tools/install-esterium-local-complete.ps1') @args
}else{
    Write-Output 'ZIP preview: skip legacy coordinator that requires EDB .exe.'
}
$after=@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue)
if ($after.Count -ne 0) {throw 'Read-only plan unexpectedly modified services'}
if($PostgresBinaryZip){
    Write-Output 'PASS: native MIT ZIP preview deliberately skips unsupported legacy EDB installer plan.'
}else{ Write-Output 'PASS: unsigned real ERP + EDB/Garnet/NSSM plan on disposable Windows.' }
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
if($PostgresBinaryZip){
    $build.Remove('ExpectedPostgresPublisherThumbprint')
    $build.TechnicalPreviewPostgresBinaryZip=$true
}
& (Join-Path $root 'windows-setup/build-esterium-setup-exe.ps1') @build | Out-Null
$exe=Join-Path $exeDir 'EsteriumERP-Setup-TECHNICAL-PREVIEW.exe'
if (-not (Test-Path $exe -PathType Leaf)) {throw 'Missing integrated unsigned EXE'}
# Inspect hashes INSIDE the complete EXE, including official ZIP format.
& (Join-Path $root 'windows-setup/test-embedded-preview.ps1') -Exe $exe -ExpectedManifestSHA256 $manifest -ExpectedPostgresSHA256 $edbHash -ExpectedGarnetSHA256 $garnetHash -ExpectedNssmSHA256 $nssmHash
if($PostgresBinaryZip){
    $bad=$build.Clone()
    $bad.OutputDir=Join-Path $dest 'reject-bad-zip-hash'
    $bad.ExpectedPostgresSHA256=('A'*64)
    $refused=$false
    try{ & (Join-Path $root 'windows-setup/build-esterium-setup-exe.ps1') @bad | Out-Null }catch{ $refused=$true }
    if(-not $refused -or (Test-Path -LiteralPath $bad.OutputDir)){
        throw 'Builder must reject an incorrect independently pinned PostgreSQL ZIP digest before creating any output.'
    }
    Write-Output 'PASS: unsigned ZIP preview refuses an invalid EDB digest without writing an EXE.'
}
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
    @{name=$(if($PostgresBinaryZip){'postgresql-binaries.zip'}else{'postgresql.exe'});hash=$edbHash},
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
if (-not $PostgresBinaryZip){
    $auth=Get-AuthenticodeSignature -LiteralPath (Join-Path $vendorStage 'postgresql.exe')
    if ($auth.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $auth.SignerCertificate -or
        $auth.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $edbCert) {
        throw 'Staged EnterpriseDB executable publisher signature invalid'
    }
}else{
    if((Get-FileHash -LiteralPath (Join-Path $vendorStage 'postgresql-binaries.zip') -Algorithm SHA256).Hash -cne $edbHash){
        throw 'Official ZIP publisher-pinned digest mismatch after staging'
    }
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
if($PostgresBinaryZip){Write-Output 'PASS: unsigned EXE staged official PostgreSQL binaries ZIP, Garnet and NSSM.'}
else{Write-Output 'PASS: unsigned native EXE staged genuine EDB/Garnet/NSSM under protected Program Files.'}
if($PostgresBinaryZip){Write-Output 'PASS: Official EDB binary ZIP independently pinned SHA-256 and admin-only ACL.'}
else{Write-Output 'PASS: EDB trusted Authenticode signer, pinned hashes and admin-only ACL.'}
Write-Output 'PASS: vendor-stage reinstallation blocked; database/credentials/services unchanged.'
$preflight=Start-Process -FilePath $exe -ArgumentList '--preflight-oss' -Wait -PassThru
if ($preflight.ExitCode -ne 0) { throw 'Integrated unsigned native Windows preflight rejected a fresh runner.' }
Write-Output 'PASS: unsigned native fresh-host preflight approved clean staged vendor and ERP files.'
# PHASE 3: extract actual independently pinned PostgreSQL ZIP bytes under
# protected Program Files. No initdb, no service/account, no network.
if($PostgresBinaryZip){
    $pgStage=Join-Path $env:ProgramFiles 'EsteriumOpenSourcePostgresBin'
    if(Test-Path -LiteralPath $pgStage) {throw 'Fresh CI runner has a preexisting PostgreSQL binaries stage'}
    $pgPrep=Start-Process -FilePath $exe -ArgumentList '--stage-oss-postgres' -Wait -PassThru
    if($pgPrep.ExitCode -ne 0){throw ('Native ZIP PostgreSQL stage failed: '+$pgPrep.ExitCode)}
    foreach($relative in @('pgsql\bin\postgres.exe','pgsql\bin\initdb.exe','pgsql\bin\pg_ctl.exe',
        'pgsql\bin\psql.exe','pgsql\bin\pg_isready.exe')){
        $item=Join-Path $pgStage $relative
        if(-not(Test-Path -LiteralPath $item -PathType Leaf)){
            throw 'Missing extracted PostgreSQL vendor binary: '+$relative
        }
        $output=@(& $item '--version' 2>&1)
        if($LASTEXITCODE -ne 0 -or (($output -join ' ') -notmatch '16\.15')){
            throw 'Extracted PostgreSQL binary reported incorrect version: '+$relative
        }
    }
    $pgAcl=Get-Acl -LiteralPath $pgStage
    if(-not $pgAcl.AreAccessRulesProtected){throw 'PostgreSQL stage inherited broad Windows permissions'}
    $pgRules=@($pgAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]))
    if(@($pgRules | Where-Object {
        $_.AccessControlType -eq 'Allow' -and $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544')
    }).Count -ne 0){throw 'PostgreSQL stage grants access to unexpected principal'}
    $again=Start-Process -FilePath $exe -ArgumentList '--stage-oss-postgres' -Wait -PassThru
    if($again.ExitCode -ne 5){throw 'PostgreSQL binary stage accepted a second execution'}
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'PostgreSQL binary extraction created an unexpected Windows service'
    }
    Write-Output 'PASS: native MIT EXE extracted PostgreSQL 16.15 into protected Program Files, verified binaries and refused retry.'
    Write-Output 'PASS: ZIP extraction installed no PostgreSQL cluster, Windows service, user or credential.'
    # E2E integration proof: use the exact binaries created by THIS native
    # EXE, not a second download or unrelated manually extracted archive.
    # Create a throwaway non-admin CI-only cluster under RUNNER_TEMP, exercise
    # Windows SCM, authenticated SQL and backup/restore, then remove the
    # service, rights and temporary account. Never runs on a shop machine.
    if($env:GITHUB_ACTIONS -ne 'true' -or
       $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
       $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1'){
        throw 'Native ZIP integration execution is allowed ONLY on disposable GitHub-hosted Windows.'
    }
    & (Join-Path $root 'windows-setup/test-postgres-oss-initdb-ci.ps1') `
        -VerifiedPgRoot (Join-Path $pgStage 'pgsql') -FromIntegratedStage
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'Integrated CI cluster test altered a production-named service.'
    }
    Write-Output 'PASS: PostgreSQL launched from the native EXE stage as non-admin on ephemeral Windows; SQL and backup/restore passed.'
}
# Continue only on a GitHub-hosted ephemeral Windows machine. Unlike the
# preceding steps, this final smoke truly installs EDB on CI, never on a shop PC.
if(-not $PostgresBinaryZip){
    & (Join-Path $root 'windows-setup/test-real-edb-install-disposable-ci.ps1') `
        -PostgresInstaller (Join-Path $vendorStage 'postgresql.exe') `
        -ExpectedSHA256 $edbHash -ExpectedSignerThumbprint $edbCert
}
# EDB now has a real disabled/stopped service and a data directory. Refuse
# any second bootstrap on this previously-used host even if hashes still match.
if(-not $PostgresBinaryZip){
    $afterEdb=Start-Process -FilePath $exe -ArgumentList '--preflight-oss' -Wait -PassThru
    if ($afterEdb.ExitCode -ne 5) {throw 'Unsigned native preflight accepted a previously-installed PostgreSQL instance.'}
    Write-Output 'PASS: unsigned native preflight rejects reinstallation after real EDB setup.'
}else{
    Write-Output 'PASS: integrated unsigned ZIP-based preview staged ERP and vendors and preflight; no server or service installed.'
}
