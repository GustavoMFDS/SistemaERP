#requires -Version 5.1
<#
  CI-only LSA grant/revoke for one newly created epg... local account.
  Modify exactly SeServiceLogonRight, not security templates or global GPOs.
  Never execute this on customer/admin workstations.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidateSet('Grant','Revoke')][string]$Action,
    [Parameter(Mandatory=$true)][string]$Sid
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
if ($account.Count -ne 1 -or $account[0].Name -cnotmatch '^epg[0-9a-f]{12}$') {
    throw 'Cannot change logon rights outside the unique test PostgreSQL account.'
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
    private static extern uint LsaNtStatusToWinError(uint status);

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
[EsteriumDisposableLsa]::Change($Sid,($Action -eq 'Grant'))
Write-Output ('PASS: CI-only PostgreSQL service logon right '+$Action.ToLowerInvariant()+'ed for ephemeral account.')
