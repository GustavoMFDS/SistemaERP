#requires -Version 5.1
<#
  CI-only LSA grant/revoke for a unique ephemeral epg... account,
  or explicit offline-secret eos... identity with extra CI safeguards.
  Modify exactly SeServiceLogonRight, not security templates or global GPOs.
  Never execute this on customer/admin workstations.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidateSet('Grant','Revoke','VerifyGranted','VerifyRevoked')][string]$Action,
    [Parameter(Mandatory=$true)][string]$Sid,
    [switch]$OfflineSecretAccount
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1') {
    throw 'Local policy changes permitted ONLY in disposable GitHub Windows CI.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'LSA account-right test requires Administrator.'
}
if ($Sid -cnotmatch '^S-1-5-21-(\d+-){3}\d+$') {
    throw 'Only a local machine account SID may be used.'
}
$account=@(Get-LocalUser | Where-Object { [string]$_.SID.Value -ceq $Sid })
if ($account.Count -ne 1) {throw 'LSA SID must belong to exactly one CI local user.'}
if ($OfflineSecretAccount) {
    if ($env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1' -or
        $env:ESTERIUM_NATIVE_DATA_DISPOSABLE_CI -ne '1' -or
        $account[0].Name -cnotmatch '^eos[0-9a-f]{12}$' -or
        @(Get-Service -Name @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP') -ErrorAction SilentlyContinue).Count){
        throw 'Offline service-logon rights require a disposable EOS account and no customer services.'
    }
} elseif ($Action -notin @('Grant','Revoke') -or $account[0].Name -cnotmatch '^epg[0-9a-f]{12}$') {
    throw 'Original PostgreSQL LSA path is restricted to ephemeral epg accounts.'
}
$source=@'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;

public static class EsteriumDisposableLsa
{
    [StructLayout(LayoutKind.Sequential)]
    private struct LsaObjectAttributes {
        public uint Length;
        public IntPtr RootDirectory;
        public IntPtr ObjectName;
        public uint Attributes;
        public IntPtr SecurityDescriptor;
        public IntPtr SecurityQualityOfService;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct LsaUnicodeString {
        public ushort Length;
        public ushort MaximumLength;
        public IntPtr Buffer;
    }
    [DllImport("advapi32.dll")]
    private static extern uint LsaOpenPolicy(IntPtr systemName,
        ref LsaObjectAttributes objectAttributes, uint desiredAccess,
        out IntPtr policyHandle);
    [DllImport("advapi32.dll")]
    private static extern uint LsaAddAccountRights(IntPtr policyHandle,
        IntPtr accountSid, [In] LsaUnicodeString[] rights, uint count);
    [DllImport("advapi32.dll")]
    private static extern uint LsaRemoveAccountRights(IntPtr policyHandle,
        IntPtr accountSid, [MarshalAs(UnmanagedType.U1)] bool allRights,
        [In] LsaUnicodeString[] rights, uint count);
    [DllImport("advapi32.dll")]
    private static extern uint LsaClose(IntPtr handle);
    [DllImport("advapi32.dll")]
    private static extern uint LsaEnumerateAccountRights(IntPtr policyHandle,
        IntPtr accountSid, out IntPtr userRights, out uint countOfRights);
    [DllImport("advapi32.dll")]
    private static extern uint LsaFreeMemory(IntPtr buffer);
    [DllImport("advapi32.dll")]
    private static extern uint LsaNtStatusToWinError(uint status);

    // Independently verify the applied account right through LSA.
    public static bool HasServiceLogonRight(string sidValue)
    {
        var sid=new SecurityIdentifier(sidValue);
        var bytes=new byte[sid.BinaryLength];
        sid.GetBinaryForm(bytes,0);
        IntPtr sidPtr=IntPtr.Zero;
        IntPtr policy=IntPtr.Zero;
        IntPtr rights=IntPtr.Zero;
        try {
            sidPtr=Marshal.AllocHGlobal(bytes.Length);
            Marshal.Copy(bytes,0,sidPtr,bytes.Length);
            LsaObjectAttributes attributes=new LsaObjectAttributes();
            attributes.Length=(uint)Marshal.SizeOf(typeof(LsaObjectAttributes));
            uint status=LsaOpenPolicy(IntPtr.Zero,ref attributes,0x800,out policy);
            if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status));
            uint count=0;
            status=LsaEnumerateAccountRights(policy,sidPtr,out rights,out count);
            if(status==0xC0000034) return false; // no rights assigned
            if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status));
            int size=Marshal.SizeOf(typeof(LsaUnicodeString));
            for(uint i=0;i<count;i++) {
                var entry=(LsaUnicodeString)Marshal.PtrToStructure(
                    IntPtr.Add(rights,checked((int)i*size)),typeof(LsaUnicodeString));
                if(String.Equals(Marshal.PtrToStringUni(entry.Buffer,entry.Length/2),
                    "SeServiceLogonRight",StringComparison.OrdinalIgnoreCase)) return true;
            }
            return false;
        } finally {
            if(rights!=IntPtr.Zero) LsaFreeMemory(rights);
            if(policy!=IntPtr.Zero) LsaClose(policy);
            if(sidPtr!=IntPtr.Zero) Marshal.FreeHGlobal(sidPtr);
            Array.Clear(bytes,0,bytes.Length);
        }
    }

    public static void Change(string sidValue, bool grant)
    {
        var sid = new SecurityIdentifier(sidValue);
        var bytes = new byte[sid.BinaryLength];
        sid.GetBinaryForm(bytes,0);
        IntPtr sidPtr = IntPtr.Zero;
        IntPtr strPtr = IntPtr.Zero;
        IntPtr policy = IntPtr.Zero;
        try {
            sidPtr = Marshal.AllocHGlobal(bytes.Length);
            Marshal.Copy(bytes,0,sidPtr,bytes.Length);
            const string right = "SeServiceLogonRight";
            strPtr = Marshal.StringToHGlobalUni(right);
            LsaUnicodeString[] rights = new LsaUnicodeString[] {
                new LsaUnicodeString {
                    Length=(ushort)(right.Length*2),
                    MaximumLength=(ushort)((right.Length+1)*2),
                    Buffer=strPtr
                }
            };
            LsaObjectAttributes attributes = new LsaObjectAttributes();
            attributes.Length=(uint)Marshal.SizeOf(typeof(LsaObjectAttributes));
            // POLICY_LOOKUP_NAMES | POLICY_CREATE_ACCOUNT. Never open with ALL_ACCESS.
            uint status = LsaOpenPolicy(IntPtr.Zero, ref attributes, 0x810, out policy);
            if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status));
            status = grant ? LsaAddAccountRights(policy,sidPtr,rights,1) :
                LsaRemoveAccountRights(policy,sidPtr,false,rights,1);
            if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status));
        } finally {
            if(policy!=IntPtr.Zero) LsaClose(policy);
            if(strPtr!=IntPtr.Zero) Marshal.FreeHGlobal(strPtr);
            if(sidPtr!=IntPtr.Zero) Marshal.FreeHGlobal(sidPtr);
            Array.Clear(bytes,0,bytes.Length);
        }
    }
}
'@
if ($null -eq ([Management.Automation.PSTypeName]'EsteriumDisposableLsa').Type) {
    Add-Type -TypeDefinition $source -ErrorAction Stop
}
if($Action -in @('Grant','Revoke')){
    [EsteriumDisposableLsa]::Change($Sid,($Action -eq 'Grant'))
    Write-Output ('PASS: CI-only SeServiceLogonRight '+$Action+' for an ephemeral Windows account.')
}else{
    $present=[EsteriumDisposableLsa]::HasServiceLogonRight($Sid)
    if(($Action -eq 'VerifyGranted' -and -not $present) -or
       ($Action -eq 'VerifyRevoked' -and $present)){
        throw ('Actual Windows LSA right does not match '+$Action+'.')
    }
    Write-Output ('PASS: Windows LSA confirmed '+$Action+' for disposable identity.')
}
