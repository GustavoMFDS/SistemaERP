using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Web.Script.Serialization;

// Phase 4: prepare the FUTURE data directories with protected ACLs.
// No PostgreSQL cluster, SQL, accounts, services, credentials, network,
// backups or migrations are created by this operation.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        [StructLayout(LayoutKind.Sequential)]
        private struct WinSecurityAttributes
        {
            public int nLength;
            public IntPtr lpSecurityDescriptor;
            [MarshalAs(UnmanagedType.Bool)] public bool bInheritHandle;
        }

        [DllImport("kernel32.dll", EntryPoint = "CreateDirectoryW",
            CharSet = CharSet.Unicode, SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        private static extern bool CreateProtectedDirectory(
            string path, ref WinSecurityAttributes attributes);

        private static Dictionary<string, string> PreviewPins()
        {
            using (var resource = Assembly.GetExecutingAssembly()
                .GetManifestResourceStream("Esterium.PreviewApprovals"))
            {
                if (resource == null)
                    throw new InvalidOperationException("Hashes embutidos ausentes.");
                using (var reader = new StreamReader(resource, Encoding.UTF8))
                {
                    var data = new JavaScriptSerializer().Deserialize<Dictionary<string,string>>(
                        reader.ReadToEnd());
                    if (data == null || !data.ContainsKey("manifest") ||
                        !data.ContainsKey("garnet") || !data.ContainsKey("nssm"))
                        throw new InvalidOperationException("Aprovacoes de arquivos incompletas.");
                    return data;
                }
            }
        }

        internal static void StageOpenSourceDataFolders()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview)
                throw new InvalidOperationException("Requer a previa MIT com ZIP PostgreSQL aprovado.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent())
                    .IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("UAC de Administrador obrigatorio.");

            // Reauthenticate actual installed vendor bytes BEFORE any write.
            // Unlike first-install preflight, the PostgreSQL binary directory
            // intentionally already exists and is not considered a fresh host.
            AuditOpenSourcePostgresBinaries();

            string pf = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            string pd = Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData);
            if (String.IsNullOrWhiteSpace(pd) || !Path.IsPathRooted(pd) ||
                String.IsNullOrWhiteSpace(pf) || !Path.IsPathRooted(pf))
                throw new InvalidOperationException("Program Files ou ProgramData invalidos.");

            // Ensure no existing ERP installation or account/service is ever
            // overwritten. A failure leaves a conspicuous protected footprint.
            string productDir = Path.Combine(pd, "Esterium");
            if (Directory.Exists(productDir) || File.Exists(productDir))
                throw new InvalidOperationException(
                    "Dados Esterium existentes; nao sobrescrever nem remover automaticamente.");
            foreach (string name in new [] { "EsteriumPostgreSQL16", "EsteriumGarnet", "EsteriumERP" })
            {
                using (var svc = new System.ServiceProcess.ServiceController(name))
                {
                    try
                    {
                        var unused = svc.Status;
                        throw new InvalidOperationException("Servico ja existe: " + name);
                    }
                    catch (InvalidOperationException e)
                    {
                        var win32 = e.InnerException as Win32Exception;
                        if (win32 == null || win32.NativeErrorCode != 1060) throw;
                    }
                }
            }

            // Initial ERP/vendor staging is still protected and intact.
            string erp = Path.Combine(pf, "EsteriumOpenSourceStage");
            string vendor = Path.Combine(pf, "EsteriumOpenSourceVendorStage");
            OrdinaryTree(erp);
            OrdinaryTree(vendor);
            var approval = PreviewPins();
            Bundle(erp, Hex("manifesto", approval["manifest"], 64));
            foreach (var kv in new Dictionary<string, string> {
                { "garnet.zip", "garnet" }, { "nssm.exe", "nssm" } })
            {
                string path = Path.Combine(vendor, kv.Key);
                if (!File.Exists(path) ||
                    (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0 ||
                    Digest(path) != Hex(kv.Key, approval[kv.Value], 64))
                    throw new InvalidOperationException("Fornecedor alterado: " + kv.Key);
            }
            var root = new DirectorySecurity();
            root.SetAccessRuleProtection(true, false);
            var inheritance = InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit;
            var sys = new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);
            var admins = new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null);
            root.AddAccessRule(new FileSystemAccessRule(sys, FileSystemRights.FullControl,
                inheritance, PropagationFlags.None, AccessControlType.Allow));
            root.AddAccessRule(new FileSystemAccessRule(admins, FileSystemRights.FullControl,
                inheritance, PropagationFlags.None, AccessControlType.Allow));
            byte[] descriptor = root.GetSecurityDescriptorBinaryForm();
            IntPtr nativeDescriptor = IntPtr.Zero;
            bool created = false;
            try
            {
                // CreateDirectoryW with an explicit DACL is atomic. If another
                // process precreated the path, Windows returns 183 and we
                // refuse it rather than adopting an attacker-controlled folder.
                nativeDescriptor = Marshal.AllocHGlobal(descriptor.Length);
                Marshal.Copy(descriptor, 0, nativeDescriptor, descriptor.Length);
                var sa = new WinSecurityAttributes {
                    nLength = Marshal.SizeOf(typeof(WinSecurityAttributes)),
                    lpSecurityDescriptor = nativeDescriptor,
                    bInheritHandle = false
                };
                if (!CreateProtectedDirectory(productDir, ref sa))
                    throw new Win32Exception(Marshal.GetLastWin32Error(),
                        "Nao foi possivel criar ProgramData protegido sem sobrescrever.");
                created = true;
            }
            finally
            {
                if (nativeDescriptor != IntPtr.Zero) Marshal.FreeHGlobal(nativeDescriptor);
                Array.Clear(descriptor, 0, descriptor.Length);
            }
            if (!created) throw new InvalidOperationException("Pasta nao preparada.");

            // The protected root is never deleted automatically. A partially
            // written stage requires explicit operator review; it must not be
            // mistaken for an empty Windows host on the next invocation.
            foreach (string leaf in new [] { "PostgresData", "GarnetData", "Secrets", "Backups" })
                Directory.CreateDirectory(Path.Combine(productDir, leaf));

            string marker = Path.Combine(productDir, "oss-data-layout.json");
            const string state = "{\r\n" +
                "  \"schemaVersion\": 1,\r\n" +
                "  \"phase\": \"DATA_DIRECTORIES_ONLY\",\r\n" +
                "  \"databaseInitialized\": false,\r\n" +
                "  \"servicesInstalled\": false,\r\n" +
                "  \"backupsConfigured\": false\r\n" +
                "}\r\n";
            using (var output = new FileStream(marker, FileMode.CreateNew,
                FileAccess.Write, FileShare.None))
            {
                byte[] buffer = new UTF8Encoding(false).GetBytes(state);
                output.Write(buffer, 0, buffer.Length);
                output.Flush(true);
            }
            OrdinaryTree(productDir);
            VerifyProtectedDataLayout(productDir);
            // Never place a password, account, DB cluster or fiscal data here.
        }

        internal static void VerifyProtectedDataLayout(string productDir)
        {
            if (!Directory.Exists(productDir) ||
                (File.GetAttributes(productDir) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Pasta de dados nao encontrada ou vinculada.");
            var acl = Directory.GetAccessControl(productDir);
            if (!acl.AreAccessRulesProtected)
                throw new InvalidOperationException("ProgramData herdou ACL aberta.");
            foreach (FileSystemAccessRule rule in acl.GetAccessRules(true, true,
                typeof(SecurityIdentifier)))
            {
                if (rule.AccessControlType != AccessControlType.Allow) continue;
                string sid = ((SecurityIdentifier)rule.IdentityReference).Value;
                if (sid != "S-1-5-18" && sid != "S-1-5-32-544")
                    throw new InvalidOperationException("Permissao nao aprovada em ProgramData.");
            }
            var children = new [] { "PostgresData", "GarnetData", "Secrets", "Backups" };
            foreach (string leaf in children)
            {
                string path = Path.Combine(productDir, leaf);
                if (!Directory.Exists(path) ||
                    (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0 ||
                    Directory.GetFileSystemEntries(path).Length != 0)
                    throw new InvalidOperationException("Diretorio de dados inicial inseguro: " + leaf);
            }
            var entries = Directory.GetFileSystemEntries(productDir);
            if (entries.Length != 5)
                throw new InvalidOperationException("ProgramData contem entradas inesperadas.");
            foreach (string entry in entries)
            {
                string name = Path.GetFileName(entry);
                if (name != "oss-data-layout.json" &&
                    Array.IndexOf(children, name) < 0)
                    throw new InvalidOperationException("Entrada nao reconhecida em ProgramData.");
            }
            string layout = File.ReadAllText(Path.Combine(productDir, "oss-data-layout.json"),
                Encoding.UTF8);
            if (!layout.Contains("\"phase\": \"DATA_DIRECTORIES_ONLY\"") ||
                !layout.Contains("\"servicesInstalled\": false") ||
                !layout.Contains("\"databaseInitialized\": false"))
                throw new InvalidOperationException("Marcador de fase inseguro ou modificado.");
        }

        async Task RunStageDataFolders()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview || working)
                return;
            working = true;
            check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled =
                stageVendors.Enabled = preflight.Enabled = stagePostgres.Enabled =
                stageData.Enabled = false;
            try
            {
                if (MessageBox.Show(
                    "PREPARAR PASTAS DE DADOS - NAO INSTALA O ERP.\n\n" +
                    "Cria ProgramData\\Esterium e diretorios vazios com ACL " +
                    "restrita a Administradores/SYSTEM. NAO cria banco, conta, " +
                    "senha, servico, backup ou conexao de rede.\n" +
                    "Nao apaga ou altera instalacoes existentes.\n\nProsseguir?",
                    "Esterium MIT - etapa 4", MessageBoxButtons.YesNo,
                    MessageBoxIcon.Warning) != DialogResult.Yes)
                    return;
                var start = new ProcessStartInfo(Application.ExecutablePath, "--stage-oss-data")
                    { UseShellExecute = true, Verb = "runas" };
                int code = await Task.Run(() => {
                    using (var p = Process.Start(start)) { p.WaitForExit(); return p.ExitCode; }
                });
                Log(code == 0 ?
                    "PASS: ProgramData protegido, apenas pastas vazias. Banco NAO instalado." :
                    "BLOQUEADO: ProgramData nao preparado (codigo " + code + "). Inspecione antes de repetir.");
            }
            catch (Win32Exception ex) { Log("UAC cancelado: " + ex.Message); }
            catch (Exception ex) { Log("BLOQUEADO: " + ex.Message); }
            finally
            {
                working = false;
                check.Enabled = true;
                plan.Enabled = !Program.PreviewBuild || (Program.HasPayload && !IsPostgresZipPreview);
                stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = Program.HasPayload;
                stagePostgres.Enabled = stageData.Enabled =
                    Program.HasPayload && IsPostgresZipPreview;
                install.Enabled = false;
            }
        }
    }
}
