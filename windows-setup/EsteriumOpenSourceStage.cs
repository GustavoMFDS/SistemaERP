using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Web.Script.Serialization;

// MIT open-source installation PHASE 1: stage verified ERP files only.
// It never runs a vendor EXE, PowerShell, SQL, service registration, or network calls.
// Keep it separate from the existing Authenticode-gated commercial installation.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        internal static void StageOpenSourceFiles()
        {
            if (!Program.PreviewBuild || !Program.HasPayload)
                throw new InvalidOperationException("Apenas a previa MIT integrada pode preparar arquivos.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("Preparacao exige UAC e permissao de Administrador.");

            Dictionary<string, string> pins;
            using (var input = System.Reflection.Assembly.GetExecutingAssembly()
                .GetManifestResourceStream("Esterium.PreviewApprovals"))
            {
                if (input == null) throw new InvalidOperationException("Faltam os hashes incorporados.");
                using (var reader = new StreamReader(input, Encoding.UTF8))
                    pins = new JavaScriptSerializer().Deserialize<Dictionary<string, string>>(reader.ReadToEnd());
            }
            if (pins == null) throw new InvalidOperationException("Hashes incorporados invalidos.");
            string manifestHash = Hex("manifesto", pins["manifest"], 64);
            var filenames = new[] { "postgresql.exe", "garnet.zip", "nssm.exe" };
            var keys = new[] { "postgres", "garnet", "nssm" };

            string programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            if (String.IsNullOrWhiteSpace(programFiles) || !Path.IsPathRooted(programFiles))
                throw new InvalidOperationException("Program Files do Windows indisponivel.");
            string destination = Path.Combine(programFiles, "EsteriumOpenSourceStage");
            if (Directory.Exists(destination) || File.Exists(destination))
                throw new InvalidOperationException("Ja existe uma preparacao; nao sobrescreva arquivos anteriores.");

            string extracted = null;
            string partial = null;
            bool committed = false;
            try
            {
                extracted = Unpack();
                OrdinaryTree(extracted);
                string erp = Path.Combine(extracted, "erp");
                Bundle(erp, manifestHash);
                for (int i = 0; i < filenames.Length; i++)
                {
                    string vendor = Path.Combine(extracted, "vendor", filenames[i]);
                    if (!File.Exists(vendor) || (File.GetAttributes(vendor) & FileAttributes.ReparsePoint) != 0 ||
                        Digest(vendor) != Hex(filenames[i], pins[keys[i]], 64))
                        throw new InvalidOperationException("Hash divergente do fornecedor " + filenames[i]);
                }

                // Create a unique, admin-owned sibling under Program Files; never overwrite.
                // Protect parent before writing any file, then validate copied bytes.
                partial = Path.Combine(programFiles, ".esterium-open-stage-" + Guid.NewGuid().ToString("N"));
                Directory.CreateDirectory(partial);
                var acl = new DirectorySecurity();
                acl.SetAccessRuleProtection(true, false);
                const InheritanceFlags inheritance = InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit;
                acl.AddAccessRule(new FileSystemAccessRule(
                    new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                    FileSystemRights.FullControl, inheritance, PropagationFlags.None, AccessControlType.Allow));
                acl.AddAccessRule(new FileSystemAccessRule(
                    new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                    FileSystemRights.FullControl, inheritance, PropagationFlags.None, AccessControlType.Allow));
                Directory.SetAccessControl(partial, acl);

                CopyVerifiedTree(erp, partial);
                OrdinaryTree(partial);
                Bundle(partial, manifestHash);
                if (Directory.Exists(destination) || File.Exists(destination))
                    throw new InvalidOperationException("Destino criado por outra instalacao; abortado.");
                Directory.Move(partial, destination);
                committed = true;
                // Dest is now protected by administrator-only ACLs.
                Bundle(destination, manifestHash);
                // NEVER initiate any follow-up step in this build.
            }
            finally
            {
                if (extracted != null && Directory.Exists(extracted))
                {
                    try { OrdinaryTree(extracted); Directory.Delete(extracted, true); }
                    catch { /* Keep unusual trees for manual inspection; never follow links. */ }
                }
                if (!committed && partial != null && Directory.Exists(partial))
                {
                    try { OrdinaryTree(partial); Directory.Delete(partial, true); }
                    catch { /* Explicit recovery; never retry or overwrite a partial stage. */ }
                }
            }
        }

        static void CopyVerifiedTree(string source, string destination)
        {
            OrdinaryTree(source);
            var queue = new Queue<Tuple<string, string>>();
            queue.Enqueue(Tuple.Create(source, destination));
            while (queue.Count > 0)
            {
                var current = queue.Dequeue();
                foreach (var dir in Directory.GetDirectories(current.Item1))
                {
                    var dest = Path.Combine(current.Item2, Path.GetFileName(dir));
                    if ((File.GetAttributes(dir) & FileAttributes.ReparsePoint) != 0)
                        throw new InvalidOperationException("Diretorio simbolico recusado.");
                    Directory.CreateDirectory(dest);
                    queue.Enqueue(Tuple.Create(dir, dest));
                }
                foreach (var file in Directory.GetFiles(current.Item1))
                {
                    if ((File.GetAttributes(file) & FileAttributes.ReparsePoint) != 0)
                        throw new InvalidOperationException("Arquivo simbolico recusado.");
                    File.Copy(file, Path.Combine(current.Item2, Path.GetFileName(file)), false);
                }
            }
        }

        async Task RunStageFiles()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || working) return;
            working = true;
            check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled = stageVendors.Enabled = false;
            try
            {
                var values = new Dictionary<string, string>();
                foreach (var item in inputs) values[item.Key] = item.Value.Text;
                Log("Conferindo hashes do ERP e fornecedores antes de solicitar UAC...");
                await Task.Run(() => Prepare(values));
                // The elevated EXE independently extracts and hashes the embedded bytes.
                // Do not leave a second 400+ MB extraction resident during UAC.
                string inspected = staging;
                await Task.Run(() => CleanTemporaryFiles());
                if (inspected != null && Directory.Exists(inspected))
                    throw new InvalidOperationException("Nao foi possivel limpar os arquivos temporarios.");
                staging = null;
                ownsStaging = false;
                var result = MessageBox.Show(
                    "PREPARAR ARQUIVOS, NAO INSTALAR.\n\nSera criada a pasta protegida C:\\Program Files\\EsteriumOpenSourceStage.\n" +
                    "O banco, os servicos, a rede e as vendas NAO serao alterados.\n" +
                    "Nao repita esta operacao em caso de falha. Prosseguir?",
                    "Esterium MIT - etapa 1", MessageBoxButtons.YesNo, MessageBoxIcon.Warning);
                if (result != DialogResult.Yes) { Log("Preparacao cancelada."); return; }

                var start = new ProcessStartInfo(Application.ExecutablePath, "--stage-oss-files")
                { UseShellExecute = true, Verb = "runas" };
                Log("Solicitando autorizacao UAC para copiar os arquivos protegidos...");
                int code = await Task.Run(() =>
                {
                    using (var p = Process.Start(start))
                    {
                        p.WaitForExit();
                        return p.ExitCode;
                    }
                });
                Log(code == 0 ?
                    "PASS: ERP verificado preparado em Program Files; NENHUM SERVICO OU BANCO instalado." :
                    "BLOQUEADO: preparacao nao concluida (codigo " + code + "). Nao repetir sem diagnostico.");
            }
            catch (Win32Exception ex) { Log("UAC cancelado ou falhou: " + ex.Message); }
            catch (Exception ex) { Log("BLOQUEADO: " + ex.Message); }
            finally
            {
                working = false;
                check.Enabled = true;
                plan.Enabled = Program.HasPayload;
                stageFiles.Enabled = stageVendors.Enabled = Program.HasPayload;
                install.Enabled = false;
            }
        }
    }
}
