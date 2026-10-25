using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Reflection;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Web.Script.Serialization;

// MIT unsigned flow, dependency PREPARATION ONLY.
// Zero vendor execution, no secrets, no Windows service/account/SQL changes.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        internal static void StageOpenSourceVendors()
        {
            if (!Program.PreviewBuild || !Program.HasPayload)
                throw new InvalidOperationException("Arquivo tecnico integrado obrigatorio.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent())
                    .IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("A etapa exige autorizacao de Administrador.");
            Dictionary<string,string> pins;
            using (var stream = Assembly.GetExecutingAssembly()
                .GetManifestResourceStream("Esterium.PreviewApprovals"))
            {
                if (stream == null) throw new InvalidOperationException("Hashes nao incorporados.");
                using (var reader = new StreamReader(stream, Encoding.UTF8))
                    pins = new JavaScriptSerializer()
                        .Deserialize<Dictionary<string,string>>(reader.ReadToEnd());
            }
            if (pins == null) throw new InvalidOperationException("Metadados de versao invalidos.");
            var manifest = Hex("manifesto ERP", pins["manifest"], 64);
            string postgres = Hex("PostgreSQL", pins["postgres"], 64);
            string garnet = Hex("Garnet", pins["garnet"], 64);
            string nssm = Hex("NSSM", pins["nssm"], 64);
            string publisher = Hex("Editor PostgreSQL", pins["pgSigner"], 40);

            string programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            if (String.IsNullOrWhiteSpace(programFiles) || !Path.IsPathRooted(programFiles))
                throw new InvalidOperationException("Pasta Program Files ausente.");
            string erpStage = Path.Combine(programFiles, "EsteriumOpenSourceStage");
            string vendorStage = Path.Combine(programFiles, "EsteriumOpenSourceVendorStage");
            // The ERP MUST be staged and intact before preparing dependencies.
            if (!Directory.Exists(erpStage))
                throw new InvalidOperationException("Prepare primeiro os arquivos ERP.");
            OrdinaryTree(erpStage);
            Bundle(erpStage, manifest);
            if (Directory.Exists(vendorStage) || File.Exists(vendorStage))
                throw new InvalidOperationException("Dependencias ja preparadas; nao sobrescrever.");

            string extracted = null, partial = null;
            bool committed = false;
            var files = new [] { "postgresql.exe", "garnet.zip", "nssm.exe" };
            var hashes = new [] { postgres, garnet, nssm };
            try
            {
                extracted = Unpack();
                string erp = Path.Combine(extracted, "erp");
                Bundle(erp, manifest);
                string vendors = Path.Combine(extracted, "vendor");
                OrdinaryTree(vendors);
                if (Directory.GetFiles(vendors, "*", SearchOption.AllDirectories).Length != files.Length ||
                    Directory.GetDirectories(vendors, "*", SearchOption.AllDirectories).Length != 0)
                    throw new InvalidOperationException("Pacote possui fornecedor extra ou caminho invalido.");
                for (int i = 0; i < files.Length; i++)
                {
                    string file = Path.Combine(vendors, files[i]);
                    if (!File.Exists(file) || Digest(file) != hashes[i])
                        throw new InvalidOperationException("SHA-256 incorreto: " + files[i]);
                }
                AssertEdBAuthenticode(Path.Combine(vendors, "postgresql.exe"), publisher);

                // All bytes copied into admin-only sibling, then atomically moved.
                partial = Path.Combine(programFiles, ".esterium-oss-vendor-" +
                    Guid.NewGuid().ToString("N"));
                Directory.CreateDirectory(partial);
                var acl = new DirectorySecurity();
                acl.SetAccessRuleProtection(true, false);
                var inherit = InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit;
                acl.AddAccessRule(new FileSystemAccessRule(
                    new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                    FileSystemRights.FullControl, inherit, PropagationFlags.None, AccessControlType.Allow));
                acl.AddAccessRule(new FileSystemAccessRule(
                    new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                    FileSystemRights.FullControl, inherit, PropagationFlags.None, AccessControlType.Allow));
                Directory.SetAccessControl(partial, acl);
                for (int i = 0; i < files.Length; i++)
                {
                    string dest = Path.Combine(partial, files[i]);
                    File.Copy(Path.Combine(vendors, files[i]), dest, false);
                    if (Digest(dest) != hashes[i])
                        throw new InvalidOperationException("Arquivo copiado divergente: " + files[i]);
                }
                AssertEdBAuthenticode(Path.Combine(partial, "postgresql.exe"), publisher);
                OrdinaryTree(partial);
                Bundle(erpStage, manifest);
                if (Directory.Exists(vendorStage) || File.Exists(vendorStage))
                    throw new InvalidOperationException("Outra instalacao criou a pasta de fornecedores.");
                Directory.Move(partial, vendorStage);
                committed = true;
                for (int i = 0; i < files.Length; i++)
                    if (Digest(Path.Combine(vendorStage, files[i])) != hashes[i])
                        throw new InvalidOperationException("Fornecedor alterado apos preparacao.");
                AssertEdBAuthenticode(Path.Combine(vendorStage, "postgresql.exe"), publisher);
                // STOP: no vendor processes, services, SQL, or credential setup.
            }
            finally
            {
                if (extracted != null && Directory.Exists(extracted))
                    try { OrdinaryTree(extracted); Directory.Delete(extracted, true); }
                    catch { /* Never follow unexpected reparse trees. */ }
                if (!committed && partial != null && Directory.Exists(partial))
                    try { OrdinaryTree(partial); Directory.Delete(partial, true); }
                    catch { /* Manual diagnostics if temporary cleanup unsafe. */ }
            }
        }

        static void AssertEdBAuthenticode(string path, string approvedThumbprint)
        {
            // Actual WinVerifyTrust verdict via Windows PowerShell,
            // not just presence of a matching embedded signing certificate.
            const string check =
                "$s=Get-AuthenticodeSignature -LiteralPath $env:ESTERIUM_EDB_FILE; " +
                "if ($s.Status -ne 'Valid' -or $null -eq $s.SignerCertificate -or " +
                "$s.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $env:ESTERIUM_EDB_SIGNER) " +
                "{exit 3}; exit 0";
            var command = Convert.ToBase64String(Encoding.Unicode.GetBytes(check));
            var process = new ProcessStartInfo("powershell.exe",
                "-NoLogo -NoProfile -NonInteractive -EncodedCommand " + command)
                { UseShellExecute = false, CreateNoWindow = true };
            process.EnvironmentVariables["ESTERIUM_EDB_FILE"] = path;
            process.EnvironmentVariables["ESTERIUM_EDB_SIGNER"] = approvedThumbprint;
            using (var p = Process.Start(process))
            {
                if (!p.WaitForExit(120000))
                {
                    try { p.Kill(); } catch { }
                    throw new InvalidOperationException("Verificacao EDB excedeu limite de tempo.");
                }
                if (p.ExitCode != 0)
                    throw new InvalidOperationException("Assinatura Authenticode do PostgreSQL EDB invalida.");
            }
        }

        async Task RunStageVendors()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || working) return;
            working = true;
            check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled = stageVendors.Enabled = false;
            try
            {
                var values = new Dictionary<string,string>();
                foreach (var entry in inputs) values[entry.Key] = entry.Value.Text;
                Log("Verificando pacotes e hashes antes da preparacao de dependencias...");
                await Task.Run(() => Prepare(values));
                var inspected = staging;
                await Task.Run(() => CleanTemporaryFiles());
                if (inspected != null && Directory.Exists(inspected))
                    throw new InvalidOperationException("Nao foi possivel limpar a area temporaria.");
                staging = null;
                ownsStaging = false;
                if (MessageBox.Show(
                    "PREPARAR DEPENDENCIAS, NAO INSTALAR.\n\n" +
                    "Exige ERP ja preparado. Copia PostgreSQL, Garnet e NSSM " +
                    "para pasta protegida, confere hashes e assinatura EDB.\n" +
                    "NAO executa instaladores, cria contas, configura SQL nem registra servicos.\n" +
                    "Continuar?",
                    "Esterium MIT - etapa 2", MessageBoxButtons.YesNo,
                    MessageBoxIcon.Warning) != DialogResult.Yes) return;
                var start = new ProcessStartInfo(Application.ExecutablePath, "--stage-oss-vendors")
                { UseShellExecute = true, Verb = "runas" };
                int code = await Task.Run(() =>
                {
                    using (var process = Process.Start(start))
                    {
                        process.WaitForExit();
                        return process.ExitCode;
                    }
                });
                Log(code == 0 ?
                    "PASS: dependencias preparadas e verificadas. Instalar servicos ainda BLOQUEADO." :
                    "BLOQUEADO: falha na preparacao de fornecedores (codigo " + code + ").");
            }
            catch (Win32Exception ex) { Log("UAC cancelado ou falhou: " + ex.Message); }
            catch (Exception ex) { Log("BLOQUEADO: " + ex.Message); }
            finally
            {
                working = false; check.Enabled = true;
                plan.Enabled = stageFiles.Enabled = stageVendors.Enabled = Program.HasPayload;
                install.Enabled = false;
            }
        }
    }
}
