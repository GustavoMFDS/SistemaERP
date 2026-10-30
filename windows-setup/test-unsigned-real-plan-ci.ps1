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
    # A GitHub Actions cache is UNTRUSTED: reuse only independently SHA-256
    # approved vendor bytes, without allowing cache contents to bypass checks.
    $cache=''
    if($PostgresBinaryZip -and
       $expected -ceq '43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'){
        $cache=Join-Path $root '.vendor-cache\postgresql-16.15-5-windows-x64-binaries.zip'
    }
    if($cache -and (Test-Path -LiteralPath $cache -PathType Leaf)){
        if((Get-FileHash -LiteralPath $cache -Algorithm SHA256).Hash -cne $expected){
            throw 'Untrusted CI cache has an incorrect PostgreSQL vendor digest.'
        }
        Copy-Item -LiteralPath $cache -Destination $file -ErrorAction Stop
        Write-Output 'PASS: SHA-pinned official PostgreSQL ZIP reused from disposable CI cache.'
    }else{
        & curl.exe -fLsS --retry 2 --connect-timeout 25 --max-time 480 -o $file $url
        if ($LASTEXITCODE -ne 0) {throw "Pinned upstream download unavailable: $url"}
    }
    if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -cne $expected) {
        throw "Pinned vendor hash mismatch: $file"
    }
    if($cache -and -not(Test-Path -LiteralPath $cache)){
        [IO.Directory]::CreateDirectory((Split-Path -Parent $cache)) | Out-Null
        Copy-Item -LiteralPath $file -Destination $cache -ErrorAction Stop
        if((Get-FileHash -LiteralPath $cache -Algorithm SHA256).Hash -cne $expected){
            throw 'Failed to preserve independently approved ZIP in CI cache.'
        }
    }
}
# Intentionally tamper the CI runner's OWN native journal; restore byte-for-byte.
# This must NEVER be used on a customer machine or unknown ProgramData files.
function Test-CIOnlyStrictJournalRejection([string]$Exe,[string]$Journal,[string]$ExpectedPhase) {
    if(-not $PostgresBinaryZip -or
       $env:ESTERIUM_NATIVE_DATA_DISPOSABLE_CI -ne '1' -or
       $env:GITHUB_ACTIONS -ne 'true' -or
       $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
       $Journal -cne (Join-Path $env:ProgramData 'Esterium\oss-data-layout.json') -or
       -not(Test-Path -LiteralPath $Journal -PathType Leaf) -or
       ((Get-Item -LiteralPath $Journal -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){
        throw 'Strict JSON journal injection permitted ONLY for the known disposable native Windows CI file.'
    }
    $bytes=[IO.File]::ReadAllBytes($Journal)
    $original=[Text.UTF8Encoding]::new($false).GetString($bytes)
    $baseline=(Get-FileHash -LiteralPath $Journal -Algorithm SHA256).Hash
    if(-not $original.Contains('"phase": "'+$ExpectedPhase+'"') -or
       -not $original.Contains('"databaseInitialized": false') -or
       -not $original.Contains('"backupsConfigured": false')){
        throw 'Strict journal mutation requires intact known CI stage.'
    }
    $other=if($ExpectedPhase -ceq 'DATA_DIRECTORIES_ONLY'){'OFFLINE_SECRETS_ONLY'}else{'DATA_DIRECTORIES_ONLY'}
    $cases=@(
        @{Name='extra-field'; Text=$original.Replace('"schemaVersion": 1','"schemaVersion": 1, "untrustedExtra": true')},
        @{Name='duplicate-phase'; Text=$original.Replace('"phase": "'+$ExpectedPhase+'"','"phase": "'+$ExpectedPhase+'", "phase": "'+$ExpectedPhase+'"')},
        @{Name='contradictory-bool'; Text=$original.Replace('"databaseInitialized": false','"databaseInitialized": true')},
        @{Name='string-not-bool'; Text=$original.Replace('"databaseInitialized": false','"databaseInitialized": "false"')},
        @{Name='missing-required'; Text=$original.Replace('"backupsConfigured": false','"backupConfigOmitted": false')},
        @{Name='wrong-phase'; Text=$original.Replace('"phase": "'+$ExpectedPhase+'"','"phase": "'+$other+'"')},
        @{Name='escaped-key-alias'; Text=$original.Replace('"phase": "'+$ExpectedPhase+'"','"ph\u0061se": "'+$ExpectedPhase+'"')},
        @{Name='whitespace-reformat'; Text=$original.Replace('"schemaVersion": 1','"schemaVersion" : 1')},
        @{Name='key-order-rewrite'; Text=$original.Replace(
            '  "databaseInitialized": false,'+[Environment]::NewLine+'  "servicesInstalled": false,',
            '  "servicesInstalled": false,'+[Environment]::NewLine+'  "databaseInitialized": false,')}
    )
    # BOM and invalid UTF-8 may be silently tolerated by a permissive decoder.
    $bom=New-Object byte[] ($bytes.Length+3)
    $bom[0]=0xef;$bom[1]=0xbb;$bom[2]=0xbf
    [Array]::Copy($bytes,0,$bom,3,$bytes.Length)
    $invalidBytes=[byte[]]$bytes.Clone()
    $newlineOffset=[Array]::IndexOf($invalidBytes,[byte]13)
    if($newlineOffset -lt 0){throw 'Canonical CRLF missing from CI journal fixture.'}
    $invalidBytes[$newlineOffset]=0xff
    $cases+=@{Name='utf8-bom'; Bytes=$bom}
    $cases+=@{Name='invalid-utf8'; Bytes=$invalidBytes}
    if($ExpectedPhase -ceq 'OFFLINE_SECRETS_ONLY'){
        $cases+=@{Name='offline-secret-flag-false'; Text=$original.Replace('"secretsGenerated": true','"secretsGenerated": false')}
    }
    try{
        foreach($case in $cases){
            $mutatedHash=''
            try{
                if($case.ContainsKey('Bytes')){
                    if([Convert]::ToBase64String([byte[]]$case.Bytes) -ceq [Convert]::ToBase64String($bytes)){
                        throw ('Ineffective byte tamper fixture: '+$case.Name)
                    }
                    [IO.File]::WriteAllBytes($Journal,[byte[]]$case.Bytes)
                }else{
                    if($case.Text -ceq $original){throw ('Ineffective JSON tamper fixture: '+$case.Name)}
                    [IO.File]::WriteAllText($Journal,$case.Text,[Text.UTF8Encoding]::new($false))
                }
                $mutatedHash=(Get-FileHash -LiteralPath $Journal -Algorithm SHA256).Hash
                if($mutatedHash -ceq $baseline){throw 'Tampered journal kept original SHA-256.'}
                $inspector=Start-Process -FilePath $Exe -ArgumentList '--inspect-oss-state' -Wait -PassThru -ErrorAction Stop
                if($null -eq $inspector){throw 'Native journal inspector process missing.'}
                $exitCode=$inspector.ExitCode
                $inspector.Dispose()
                if($exitCode -ne 5){throw ('Native inspector accepted tampered journal case: '+$case.Name)}
                if($ExpectedPhase -ceq 'DATA_DIRECTORIES_ONLY' -and $case.Name -ceq 'extra-field'){
                    $stage=Start-Process -FilePath $Exe -ArgumentList '--stage-oss-secrets' -Wait -PassThru -ErrorAction Stop
                    if($null -eq $stage){throw 'Native secret stage did not start for invalid journal.'}
                    $stageCode=$stage.ExitCode
                    $stage.Dispose()
                    if($stageCode -ne 5){throw 'Offline secret stage accepted unknown journal field.'}
                    if(@(Get-ChildItem -LiteralPath (Join-Path $env:ProgramData 'Esterium\Secrets') -Force).Count){
                        throw 'Tampered journal caused native secret creation.'
                    }
                }
                if((Get-FileHash -LiteralPath $Journal -Algorithm SHA256).Hash -cne $mutatedHash){
                    throw 'Read-only inspector changed tampered native journal.'
                }
            }finally{
                # Do not overwrite a replaced/linked/unknown journal.
                if($mutatedHash -and
                   (Test-Path -LiteralPath $Journal -PathType Leaf) -and
                   -not((Get-Item -LiteralPath $Journal -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -and
                   (Get-FileHash -LiteralPath $Journal -Algorithm SHA256).Hash -ceq $mutatedHash){
                    [IO.File]::WriteAllBytes($Journal,$bytes)
                }
            }
            if((Get-FileHash -LiteralPath $Journal -Algorithm SHA256).Hash -cne $baseline){
                throw ('Original protected CI journal was not restored after '+$case.Name)
            }
        }
    }finally{[Array]::Clear($bytes,0,$bytes.Length)}
    Write-Output ('PASS: strict native journal schema refused '+$cases.Count+' synthetic JSON tamper cases for '+$ExpectedPhase+'; original journal restored byte-for-byte.')
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
    # Fresh-host preflight is a one-way gate; after extraction a repeat
    # of initial-install preflight must not claim a clean host.
    $notFresh=Start-Process -FilePath $exe -ArgumentList '--preflight-oss' -Wait -PassThru
    if($notFresh.ExitCode -ne 5){
        throw 'Initial clean-host preflight accepted a previously staged PostgreSQL installation.'
    }
    Write-Output 'PASS: fresh-host preflight is rejected after native PostgreSQL staging.'
    $again=Start-Process -FilePath $exe -ArgumentList '--stage-oss-postgres' -Wait -PassThru
    if($again.ExitCode -ne 5){throw 'PostgreSQL binary stage accepted a second execution'}
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'PostgreSQL binary extraction created an unexpected Windows service'
    }
    # A future privileged install must RE-AUTHENTICATE installed bytes,
    # not trust the earlier one-time ZIP extraction or the staging directory.
    $audit=Start-Process -FilePath $exe -ArgumentList '--audit-oss-postgres' -Wait -PassThru
    if($audit.ExitCode -ne 0){throw 'PostgreSQL native integrity audit refused the intact pinned vendor stage.'}
    $extra=Join-Path $pgStage 'ci-audit-injected-extra-file.marker'
    if(Test-Path -LiteralPath $extra){throw 'Fresh CI PostgreSQL binary stage had an unexpected marker.'}
    try{
        [IO.File]::WriteAllText($extra,'CI-only extra file that must be rejected')
        $rejected=Start-Process -FilePath $exe -ArgumentList '--audit-oss-postgres' -Wait -PassThru
        if($rejected.ExitCode -ne 5){
            throw 'Native PostgreSQL audit accepted an unexpected file in protected stage.'
        }
    }finally{
        if(Test-Path -LiteralPath $extra){Remove-Item -LiteralPath $extra -Force -ErrorAction Stop}
    }
    $againAudit=Start-Process -FilePath $exe -ArgumentList '--audit-oss-postgres' -Wait -PassThru
    if($againAudit.ExitCode -ne 0){throw 'PostgreSQL audit did not recover after CI-only extra file removal.'}
    Write-Output 'PASS: PostgreSQL native integrity audit confirmed every upstream file, rejected an extra file and approved intact stage.'
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
        -VerifiedPgRoot (Join-Path $pgStage 'pgsql') -FromIntegratedStage -ProgramDataLayout
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'Integrated CI cluster test altered a production-named service.'
    }
    Write-Output 'PASS: PostgreSQL launched from the native EXE stage as non-admin on ephemeral Windows; SQL and backup/restore passed.'
    # PHASE 4 of the real unsigned MIT EXE: atomically create protected
    # EMPTY ProgramData folders. No DB, credentials, account or service is
    # permitted to be installed by this command. Fail on preexisting folders.
    $dataRoot=Join-Path $env:ProgramData 'Esterium'
    if(Test-Path -LiteralPath $dataRoot){
        throw 'Expected disposable Windows runner with no preexisting Esterium ProgramData.'
    }
    $prepare=Start-Process -FilePath $exe -ArgumentList '--stage-oss-data' -Wait -PassThru
    if($prepare.ExitCode -ne 0){
        throw ('Native protected ProgramData layout refused the pinned ZIP stage: '+$prepare.ExitCode)
    }
    $expectedFolders=@('PostgresData','GarnetData','Secrets','Backups')
    foreach($name in $expectedFolders){
        $dir=Join-Path $dataRoot $name
        if(-not(Test-Path -LiteralPath $dir -PathType Container) -or
            @(Get-ChildItem -LiteralPath $dir -Force).Count -ne 0){
            throw ('ProgramData stage must contain only empty folders: '+$name)
        }
    }
    $acl=Get-Acl -LiteralPath $dataRoot
    if(-not $acl.AreAccessRulesProtected){throw 'ProgramData layout inherited unsafe ACL.'}
    $rules=@($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]))
    if(@($rules | Where-Object { $_.AccessControlType -eq 'Allow' -and
            $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544')
        }).Count -ne 0){
        throw 'Native ProgramData layout ACL grants access outside SYSTEM/Administrators.'
    }
    $marker=Join-Path $dataRoot 'oss-data-layout.json'
    if(-not(Test-Path -LiteralPath $marker -PathType Leaf)){throw 'Missing data staging journal.'}
    $state=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json -ErrorAction Stop
    if($state.phase -cne 'DATA_DIRECTORIES_ONLY' -or $state.schemaVersion -ne 1 -or
       $state.databaseInitialized -ne $false -or $state.servicesInstalled -ne $false -or
       $state.backupsConfigured -ne $false){
        throw 'Staging marker falsely claims a configured PostgreSQL instance.'
    }
    $markerHash=(Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash
    Test-CIOnlyStrictJournalRejection -Exe $exe -Journal $marker -ExpectedPhase 'DATA_DIRECTORIES_ONLY'
    $inspectedData=Start-Process -FilePath $exe -ArgumentList '--inspect-oss-state' -Wait -PassThru -ErrorAction Stop
    if($null -eq $inspectedData){throw 'Read-only stage inspection did not start.'}
    $inspectionCode=$inspectedData.ExitCode
    $inspectedData.Dispose()
    if($inspectionCode -ne 0 -or (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $markerHash){
        throw 'Read-only inspector refused or modified intact DATA_DIRECTORIES_ONLY stage.'
    }
    Write-Output 'PASS: native read-only inspection recognized intact protected data-only stage.'
    $repeat=Start-Process -FilePath $exe -ArgumentList '--stage-oss-data' -Wait -PassThru
    if($repeat.ExitCode -ne 5 -or
       (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $markerHash){
        throw 'Native ProgramData stage rerun replaced or modified already staged data.'
    }
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'Data folder preparation unexpectedly installed a product Windows service.'
    }
    Write-Output 'PASS: native EXE prepared empty protected ProgramData\\Esterium directories with honest journal and refused rerun.'
    Write-Output 'NOT INSTALLED: no PostgreSQL cluster, Garnet data, service, credential or backup configured.'
    # A second, disposable CI-only cycle uses the REAL four folders from
    # the EXE's protected ProgramData layout. Prove service, authenticated
    # SQL, backup/restore and cleanup WITHOUT installing any customer service.
    if($env:ESTERIUM_NATIVE_DATA_DISPOSABLE_CI -ne '1'){
        throw 'Real native ProgramData cluster fixture requires explicit disposable Windows CI gate.'
    }
    & (Join-Path $root 'windows-setup/test-oss-native-data-initdb-ci.ps1') -IntegratedExe $exe
    Write-Output 'PASS: protected native ProgramData layout supported isolated non-admin PostgreSQL initdb, SQL and recovery.'
    # Simulate an interrupted phase-5 write BEFORE the journal advances.
    # The only simulated file is CI-owned and deliberately NOT a credential.
    # A fresh --stage-oss-secrets MUST refuse to adopt or replace it.
    $interruptedDir=Join-Path $dataRoot 'Secrets'
    $interruptedFile=Join-Path $interruptedDir 'pg-super.secret'
    if(@(Get-ChildItem -LiteralPath $interruptedDir -Force).Count -ne 0){
        throw 'Interrupt injection requires an empty native Secrets folder.'
    }
    $beforeInterrupted=(Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash
    $injectedBytes=[Text.UTF8Encoding]::new($false).GetBytes('DISPOSABLE-CI-PARTIAL-'+[Guid]::NewGuid().ToString('N'))
    $injectedDigest=''
    $interruptedOwned=$false
    try {
        $handle=[IO.FileStream]::new($interruptedFile,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        $interruptedOwned=$true
        try{$handle.Write($injectedBytes,0,$injectedBytes.Length);$handle.Flush($true)}finally{$handle.Dispose()}
        $injectedDigest=(Get-FileHash -LiteralPath $interruptedFile -Algorithm SHA256).Hash
        $injectedAcl=Get-Acl -LiteralPath $interruptedFile
        if(@($injectedAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]) |
             Where-Object { $_.AccessControlType -eq 'Allow' -and
                 $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544') }).Count){
            throw 'Partial CI secret fixture unexpectedly has an open ACL.'
        }
        $partialInspection=Start-Process -FilePath $exe -ArgumentList '--inspect-oss-state' -Wait -PassThru -ErrorAction Stop
        if($null -eq $partialInspection){throw 'Partial-stage inspector did not run.'}
        $partialCode=$partialInspection.ExitCode
        $partialInspection.Dispose()
        if($partialCode -ne 5){throw 'Read-only inspector accepted a partial secret stage.'}
        $interruptedAttempt=Start-Process -FilePath $exe -ArgumentList '--stage-oss-secrets' -Wait -PassThru -ErrorAction Stop
        if($null -eq $interruptedAttempt){throw 'Interrupted native-stage probe did not run.'}
        $interruptedCode=$interruptedAttempt.ExitCode
        $interruptedAttempt.Dispose()
        if($interruptedCode -ne 5 -or
           (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $beforeInterrupted -or
           -not(Test-Path -LiteralPath $interruptedFile -PathType Leaf) -or
           ((Get-Item -LiteralPath $interruptedFile -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -or
           (Get-FileHash -LiteralPath $interruptedFile -Algorithm SHA256).Hash -cne $injectedDigest -or
           @(Get-ChildItem -LiteralPath $interruptedDir -Force).Count -ne 1){
            throw 'Interrupted native secret stage was adopted, regenerated or altered.'
        }
        $phaseAfterInterrupted=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json -ErrorAction Stop
        if($phaseAfterInterrupted.phase -cne 'DATA_DIRECTORIES_ONLY' -or
           $phaseAfterInterrupted.databaseInitialized -ne $false -or
           $phaseAfterInterrupted.servicesInstalled -ne $false){
            throw 'Interrupted secret stage falsely advanced installer phase.'
        }
        Write-Output 'PASS: native EXE refused simulated partial secret stage; protected first file and journal remained unchanged.'
    }finally{
        [Array]::Clear($injectedBytes,0,$injectedBytes.Length)
        # Only erase the exact synthetic CI-owned file after verifying it
        # was neither swapped nor modified by the native installer.
        if($interruptedOwned -and
           (Test-Path -LiteralPath $interruptedFile -PathType Leaf) -and
           -not((Get-Item -LiteralPath $interruptedFile -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -and
           (Get-FileHash -LiteralPath $interruptedFile -Algorithm SHA256).Hash -ceq $injectedDigest){
            Remove-Item -LiteralPath $interruptedFile -Force -ErrorAction Stop
        }
    }
    if(@(Get-ChildItem -LiteralPath $interruptedDir -Force).Count -ne 0 -or
       (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $beforeInterrupted){
        throw 'Disposable interruption fixture did not restore empty pre-secret state.'
    }
    # Simulate a later interruption: all six synthetic CI-only files exist,
    # while an incomplete atomic journal draft was never committed.
    # The real native EXE must NOT adopt, erase or complete either side.
    $pendingNames=@('pg-super.secret','pg-service.secret','pg-app.secret',
        'garnet-password.secret','jwt.secret','metrics.secret')
    $pendingJournalDraft=Join-Path $dataRoot ('.oss-data-layout-'+[Guid]::NewGuid().ToString('N')+'.json')
    $pendingCreated=New-Object 'System.Collections.Generic.List[string]'
    $pendingDigests=@{}
    $beforeJournalCrash=(Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash
    try{
        if(@(Get-ChildItem -LiteralPath $interruptedDir -Force).Count -ne 0 -or
           @(Get-ChildItem -LiteralPath $dataRoot -Force).Count -ne 5){
            throw 'Journal interruption injection requires fresh CI-only native stage.'
        }
        foreach($secretName in $pendingNames){
            $syntheticPath=Join-Path $interruptedDir $secretName
            $syntheticBytes=[Text.UTF8Encoding]::new($false).GetBytes(
                'DISPOSABLE-CI-UNUSABLE-JOURNAL-PENDING-'+[Guid]::NewGuid().ToString('N'))
            try{
                $stream=[IO.FileStream]::new($syntheticPath,[IO.FileMode]::CreateNew,
                    [IO.FileAccess]::Write,[IO.FileShare]::None)
                $pendingCreated.Add($syntheticPath)
                try{$stream.Write($syntheticBytes,0,$syntheticBytes.Length);$stream.Flush($true)}
                finally{$stream.Dispose()}
            }finally{[Array]::Clear($syntheticBytes,0,$syntheticBytes.Length)}
            $pendingDigests[$syntheticPath]=(Get-FileHash -LiteralPath $syntheticPath -Algorithm SHA256).Hash
        }
        # A deliberately incomplete *synthetic* replacement draft. The
        # original journal must remain at DATA_DIRECTORIES_ONLY.
        $draftBytes=[Text.UTF8Encoding]::new($false).GetBytes('{"schemaVersion":1,"phase":"OFFLINE_')
        try{
            $draftStream=[IO.FileStream]::new($pendingJournalDraft,[IO.FileMode]::CreateNew,
                [IO.FileAccess]::Write,[IO.FileShare]::None)
            $pendingCreated.Add($pendingJournalDraft)
            try{$draftStream.Write($draftBytes,0,$draftBytes.Length);$draftStream.Flush($true)}
            finally{$draftStream.Dispose()}
        }finally{[Array]::Clear($draftBytes,0,$draftBytes.Length)}
        $pendingDigests[$pendingJournalDraft]=(Get-FileHash -LiteralPath $pendingJournalDraft -Algorithm SHA256).Hash
        foreach($syntheticPath in $pendingCreated){
            $syntheticAcl=Get-Acl -LiteralPath $syntheticPath
            if(@($syntheticAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]) |
                Where-Object { $_.AccessControlType -eq 'Allow' -and
                    $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544') }).Count){
                throw 'Synthetic pending journal or secret has an unauthorized Windows ACL.'
            }
        }
        $draftInspection=Start-Process -FilePath $exe -ArgumentList '--inspect-oss-state' -Wait -PassThru -ErrorAction Stop
        if($null -eq $draftInspection){throw 'Pending-journal inspector did not run.'}
        $draftCode=$draftInspection.ExitCode
        $draftInspection.Dispose()
        if($draftCode -ne 5){throw 'Read-only inspector accepted an uncommitted journal draft.'}
        $pendingProbe=Start-Process -FilePath $exe -ArgumentList '--stage-oss-secrets' -Wait -PassThru -ErrorAction Stop
        if($null -eq $pendingProbe){throw 'Native journal interruption probe did not start.'}
        $pendingCode=$pendingProbe.ExitCode
        $pendingProbe.Dispose()
        if($pendingCode -ne 5 -or
           (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $beforeJournalCrash -or
           @(Get-ChildItem -LiteralPath $interruptedDir -Force).Count -ne 6 -or
           @(Get-ChildItem -LiteralPath $dataRoot -Force).Count -ne 6){
            throw 'Native EXE adopted or altered a pending offline journal transition.'
        }
        foreach($syntheticPath in $pendingCreated){
            if(-not(Test-Path -LiteralPath $syntheticPath -PathType Leaf) -or
               ((Get-Item -LiteralPath $syntheticPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -or
               (Get-FileHash -LiteralPath $syntheticPath -Algorithm SHA256).Hash -cne $pendingDigests[$syntheticPath]){
                throw 'Native EXE replaced or changed a synthetic pending journal/secret.'
            }
        }
        $pendingOriginal=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json -ErrorAction Stop
        if($pendingOriginal.phase -cne 'DATA_DIRECTORIES_ONLY' -or
           $pendingOriginal.databaseInitialized -ne $false -or
           $pendingOriginal.servicesInstalled -ne $false){
            throw 'Native interrupted journal unexpectedly claims a completed installation.'
        }
        Write-Output 'PASS: native EXE refused six synthetic pending secrets plus incomplete journal draft; original phase and all files unchanged.'
    }finally{
        # Only restore files created here and still matching their own CI
        # fingerprints. Never delete a swapped or unknown object.
        for($index=$pendingCreated.Count-1;$index -ge 0;$index--){
            $syntheticPath=$pendingCreated[$index]
            if($pendingDigests.ContainsKey($syntheticPath) -and
               (Test-Path -LiteralPath $syntheticPath -PathType Leaf) -and
               -not((Get-Item -LiteralPath $syntheticPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -and
               (Get-FileHash -LiteralPath $syntheticPath -Algorithm SHA256).Hash -ceq $pendingDigests[$syntheticPath]){
                Remove-Item -LiteralPath $syntheticPath -Force -ErrorAction Stop
            }
        }
    }
    if(@(Get-ChildItem -LiteralPath $interruptedDir -Force).Count -ne 0 -or
       @(Get-ChildItem -LiteralPath $dataRoot -Force).Count -ne 5 -or
       (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $beforeJournalCrash){
        throw 'CI pending-journal interruption did not restore the original native layout.'
    }
    # PHASE 5 native MIT preview: generate real OFFLINE first-install
    # secret material with CSPRNG, no process argv/logging or installed service.
    # The preceding fixture returned ProgramData to the empty staged state.
    $secretStage=Start-Process -FilePath $exe -ArgumentList '--stage-oss-secrets' -Wait -PassThru
    if($null -eq $secretStage -or $secretStage.ExitCode -ne 0){
        throw 'Protected offline secret generation failed after intact native ProgramData test.'
    }
    $secretFolder=Join-Path $dataRoot 'Secrets'
    $secretNames=@('pg-super.secret','pg-service.secret','pg-app.secret',
        'garnet-password.secret','jwt.secret','metrics.secret')
    $found=@(Get-ChildItem -LiteralPath $secretFolder -File -Force)
    if($found.Count -ne $secretNames.Count){
        throw 'Native offline secrets stage generated unexpected number of files.'
    }
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach($name in $secretNames){
        $path=Join-Path $secretFolder $name
        if(-not(Test-Path -LiteralPath $path -PathType Leaf) -or
           ((Get-Item -LiteralPath $path -Force).Attributes -band
             [IO.FileAttributes]::ReparsePoint)){
            throw ('Missing protected offline credential: '+$name)
        }
        $value=[IO.File]::ReadAllText($path).TrimEnd([char]13,[char]10)
        if($value -cnotmatch '^Q9_z[A-Za-z0-9_-]{64}$' -or
           -not $seen.Add($value)){
            throw 'Native secret has invalid format, inadequate length or duplicate contents.'
        }
        $secretAcl=Get-Acl -LiteralPath $path
        $grants=@($secretAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]))
        if(@($grants | Where-Object {
            $_.AccessControlType -eq 'Allow' -and
            $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544')
        }).Count -ne 0){
            throw 'Offline secret file exposes data to an unapproved Windows account.'
        }
        $value=$null
    }
    $secretMarker=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json -ErrorAction Stop
    if($secretMarker.phase -cne 'OFFLINE_SECRETS_ONLY' -or
       $secretMarker.secretsGenerated -ne $true -or
       $secretMarker.databaseInitialized -ne $false -or
       $secretMarker.servicesInstalled -ne $false -or
       $secretMarker.backupsConfigured -ne $false){
        throw 'Offline secret journal has incorrect installation state.'
    }
    # Reject synthetic schema corruption while preserving real offline secrets.
    Test-CIOnlyStrictJournalRejection -Exe $exe -Journal $marker -ExpectedPhase 'OFFLINE_SECRETS_ONLY'
    # Inspect the published offline phase without reading any secret values.
    $offlineInspect=Start-Process -FilePath $exe -ArgumentList '--inspect-oss-state' -Wait -PassThru -ErrorAction Stop
    if($null -eq $offlineInspect){throw 'Offline read-only inspection did not start.'}
    $offlineCode=$offlineInspect.ExitCode
    $offlineInspect.Dispose()
    if($offlineCode -ne 0){throw 'Read-only inspector rejected published offline secret stage.'}
    Write-Output 'PASS: native read-only inspector approved only structurally intact offline secret stage and rejected both synthetic interrupted states.'
    # Prove refusal preserves both the journal AND all six secret bytes.
    # Only digests are retained in memory, never printed to CI logs.
    $stable=(Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash
    $secretDigests=@{}
    foreach($name in $secretNames){
        $secretDigests[$name]=(Get-FileHash -LiteralPath (Join-Path $secretFolder $name) -Algorithm SHA256).Hash
    }
    $repeat=Start-Process -FilePath $exe -ArgumentList '--stage-oss-secrets' -Wait -PassThru
    if($repeat.ExitCode -ne 5 -or
       (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $stable -or
       @(Get-ChildItem -LiteralPath $secretFolder -File -Force).Count -ne $secretNames.Count){
        throw 'Offline secret regeneration changed state without explicit operator review.'
    }
    foreach($name in $secretNames){
        if((Get-FileHash -LiteralPath (Join-Path $secretFolder $name) -Algorithm SHA256).Hash -cne $secretDigests[$name]){
            throw 'Offline secret regeneration changed existing protected credentials.'
        }
    }
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'Offline secret preparation registered an unexpected service.'
    }
    Write-Output 'PASS: native MIT EXE created six distinct CSPRNG offline secrets under protected ProgramData; no values logged.'
    Write-Output 'PASS: journal changed atomically to OFFLINE_SECRETS_ONLY; regeneration and customer service installation blocked.'
    # Check that the real protected pg-service credential can authenticate a
    # unique disposable NON-ADMIN Windows identity, with full local-user cleanup.
    & (Join-Path $root 'windows-setup/test-oss-offline-service-identity-ci.ps1') -IntegratedExe $exe
    if(@(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count){
        throw 'Offline Windows identity proof unexpectedly registered a customer service.'
    }



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
