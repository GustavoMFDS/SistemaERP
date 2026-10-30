using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Security.AccessControl;
using System.Web.Script.Serialization;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

// Phase 5, MIT preview ONLY: offline first-install secret material.
// Never installs a service, generates a production certificate, trusts a
// certificate, runs vendor binaries, creates a user or touches a database.
// If interrupted, leave protected partial state for manual diagnosis and
// REFUSE rerun; never "clean" secrets belonging to a possible installation.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        private static string NewOfflineSecret()
        {
            byte[] random = new byte[48];
            using (var rng = RandomNumberGenerator.Create())
                rng.GetBytes(random);
            try
            {
                // Windows local account rules + 384 bits of CSPRNG entropy.
                return "Q9_z" + Convert.ToBase64String(random)
                    .Replace('+', '-').Replace('/', '_').TrimEnd('=');
            }
            finally { Array.Clear(random, 0, random.Length); }
        }

        private static void VerifySecretAcl(string path)
        {
            if (!File.Exists(path) ||
                (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Arquivo de credencial ausente ou vinculado.");
            var security = File.GetAccessControl(path);
            var rules = security.GetAccessRules(true, true, typeof(SecurityIdentifier));
            foreach (FileSystemAccessRule rule in rules)
            {
                if (rule.AccessControlType != AccessControlType.Allow) continue;
                string id = ((SecurityIdentifier)rule.IdentityReference).Value;
                if (id != "S-1-5-18" && id != "S-1-5-32-544")
                    throw new InvalidOperationException("Credencial acessivel por outra conta Windows.");
            }
        }

        // READ ONLY. This reports the shape of native staging, never a
        // recovery decision or a claim that a real installation is safe.
        // No filesystem writes, password reads, user/service modification,
        // vendor execution or network access are allowed in this method.
        private static void InspectProtectedEntry(string path, bool directory)
        {
            if ((directory && !Directory.Exists(path)) ||
                (!directory && !File.Exists(path)) ||
                (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Estado nativo parcial ou inseguro; revisao manual obrigatoria.");
            FileSystemSecurity acl = directory ?
                (FileSystemSecurity)Directory.GetAccessControl(path) :
                File.GetAccessControl(path);
            foreach (FileSystemAccessRule rule in acl.GetAccessRules(true, true, typeof(SecurityIdentifier)))
            {
                if (rule.AccessControlType != AccessControlType.Allow) continue;
                string sid = ((SecurityIdentifier)rule.IdentityReference).Value;
                if (sid != "S-1-5-18" && sid != "S-1-5-32-544")
                    throw new InvalidOperationException("ACL de estado nativo exige revisao manual.");
            }
        }

        internal static string InspectOpenSourceStageReadOnly()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview)
                throw new InvalidOperationException("Inspecao de fase limitada a previa MIT ZIP.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent())
                .IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("Inspecao das pastas protegidas exige Administrador.");
            string root = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "Esterium");
            InspectProtectedEntry(root, true);
            if (!Directory.GetAccessControl(root).AreAccessRulesProtected)
                throw new InvalidOperationException("ProgramData sem ACL protegida.");
            string[] folders = { "PostgresData", "GarnetData", "Secrets", "Backups" };
            string[] entries = Directory.GetFileSystemEntries(root);
            if (entries.Length != 5)
                throw new InvalidOperationException("Estado nativo possui entradas desconhecidas.");
            foreach (string entry in entries)
            {
                string name = Path.GetFileName(entry);
                if (name != "oss-data-layout.json" && Array.IndexOf(folders, name) < 0)
                    throw new InvalidOperationException("Estado nativo contem arquivos inesperados.");
            }
            foreach (string folder in folders)
                InspectProtectedEntry(Path.Combine(root, folder), true);
            string journal = Path.Combine(root, "oss-data-layout.json");
            InspectProtectedEntry(journal, false);
            Dictionary<string, object> state;
            try
            {
                state = new JavaScriptSerializer()
                    .Deserialize<Dictionary<string, object>>(File.ReadAllText(journal, Encoding.UTF8));
            }
            catch
            {
                throw new InvalidOperationException("Diario ilegivel; revisao manual obrigatoria.");
            }
            if (state == null || !state.ContainsKey("schemaVersion") ||
                !(state["schemaVersion"] is int) || (int)state["schemaVersion"] != 1 ||
                !state.ContainsKey("phase") || !(state["phase"] is string) ||
                !state.ContainsKey("databaseInitialized") ||
                !Object.Equals(state["databaseInitialized"], false) ||
                !state.ContainsKey("servicesInstalled") ||
                !Object.Equals(state["servicesInstalled"], false) ||
                !state.ContainsKey("backupsConfigured") ||
                !Object.Equals(state["backupsConfigured"], false))
                throw new InvalidOperationException("Diario incompativel ou incompleto.");
            foreach (string name in new [] { "EsteriumPostgreSQL16", "EsteriumGarnet", "EsteriumERP" })
            {
                using (var service = new System.ServiceProcess.ServiceController(name))
                {
                    try
                    {
                        var unused = service.Status;
                        throw new InvalidOperationException("Servico ja existe; revisao manual obrigatoria.");
                    }
                    catch (InvalidOperationException ex)
                    {
                        var native = ex.InnerException as Win32Exception;
                        if (native == null || native.NativeErrorCode != 1060) throw;
                    }
                }
            }
            string phase = (string)state["phase"];
            if (phase == "DATA_DIRECTORIES_ONLY")
            {
                // The earlier phase must have completely empty directories.
                // That verifier also rejects a fake/altered stage marker.
                VerifyProtectedDataLayout(root);
                return "DATA_DIRECTORIES_ONLY";
            }
            if (phase != "OFFLINE_SECRETS_ONLY" ||
                !state.ContainsKey("secretsGenerated") ||
                !Object.Equals(state["secretsGenerated"], true))
                throw new InvalidOperationException("Fase parcialmente publicada ou desconhecida.");
            foreach (string name in new [] { "PostgresData", "GarnetData", "Backups" })
            {
                if (Directory.GetFileSystemEntries(Path.Combine(root, name)).Length != 0)
                    throw new InvalidOperationException("Dados adicionais presentes; nao retomar automaticamente.");
            }
            string secrets = Path.Combine(root, "Secrets");
            string[] known = { "pg-super.secret", "pg-service.secret", "pg-app.secret",
                "garnet-password.secret", "jwt.secret", "metrics.secret" };
            if (Directory.GetFileSystemEntries(secrets).Length != known.Length)
                throw new InvalidOperationException("Numero de credenciais inesperado; revisao manual.");
            foreach (string name in known)
            {
                string path = Path.Combine(secrets, name);
                InspectProtectedEntry(path, false);
                // Structural inspection only; do not load password values.
                // This is NOT a cryptographic validation of the secrets.
                long size = new FileInfo(path).Length;
                if (size < 68 || size > 72)
                    throw new InvalidOperationException("Credencial parcial; revisao manual obrigatoria.");
            }
            return "OFFLINE_SECRETS_ONLY";
        }

        internal static void StageOpenSourceSecrets()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview)
                throw new InvalidOperationException("Credenciais exigem a previa MIT integrada e validada.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent())
                    .IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("Criacao offline exige UAC de Administrador.");

            // Re-verify the pinned ZIP before advancing a protected data stage.
            AuditOpenSourcePostgresBinaries();
            string root = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "Esterium");
            VerifyProtectedDataLayout(root); // requires empty directories + DATA_DIRECTORIES_ONLY marker
            string secrets = Path.Combine(root, "Secrets");
            string journal = Path.Combine(root, "oss-data-layout.json");

            foreach (string name in new [] { "EsteriumPostgreSQL16", "EsteriumGarnet", "EsteriumERP" })
            {
                using (var service = new System.ServiceProcess.ServiceController(name))
                {
                    try
                    {
                        var unused = service.Status;
                        throw new InvalidOperationException("Servico existente; credenciais nao geradas: " + name);
                    }
                    catch (InvalidOperationException ex)
                    {
                        var native = ex.InnerException as Win32Exception;
                        if (native == null || native.NativeErrorCode != 1060) throw;
                    }
                }
            }
            var acl = Directory.GetAccessControl(secrets);
            foreach (FileSystemAccessRule rule in acl.GetAccessRules(true, true, typeof(SecurityIdentifier)))
            {
                if (rule.AccessControlType != AccessControlType.Allow) continue;
                string sid = ((SecurityIdentifier)rule.IdentityReference).Value;
                if (sid != "S-1-5-18" && sid != "S-1-5-32-544")
                    throw new InvalidOperationException("Diretorio Secrets com permissao nao autorizada.");
            }

            string[] names = { "pg-super.secret", "pg-service.secret", "pg-app.secret",
                "garnet-password.secret", "jwt.secret", "metrics.secret" };
            var fingerprints = new HashSet<string>(StringComparer.Ordinal);
            foreach (string name in names)
            {
                // A fresh, independent secret per purpose. Never log a value.
                string secret = NewOfflineSecret();
                if (!fingerprints.Add(secret))
                    throw new InvalidOperationException("Sorteio repetido; estado parcial protegido.");
                string path = Path.Combine(secrets, name);
                byte[] bytes = new UTF8Encoding(false).GetBytes(secret + Environment.NewLine);
                try
                {
                    using (var file = new FileStream(path, FileMode.CreateNew,
                        FileAccess.Write, FileShare.None))
                    {
                        file.Write(bytes, 0, bytes.Length);
                        file.Flush(true);
                    }
                    VerifySecretAcl(path);
                }
                finally { Array.Clear(bytes, 0, bytes.Length); }
            }
            // Marker transition occurs only after ALL six secrets were
            // physically written and their ACL checked. Atomic replacement
            // prevents a partially written or false success journal.
            const string next = "{\r\n" +
                "  \"schemaVersion\": 1,\r\n" +
                "  \"phase\": \"OFFLINE_SECRETS_ONLY\",\r\n" +
                "  \"secretsGenerated\": true,\r\n" +
                "  \"databaseInitialized\": false,\r\n" +
                "  \"servicesInstalled\": false,\r\n" +
                "  \"backupsConfigured\": false\r\n" +
                "}\r\n";
            string draft = Path.Combine(root, ".oss-data-layout-" + Guid.NewGuid().ToString("N") + ".json");
            using (var output = new FileStream(draft, FileMode.CreateNew,
                FileAccess.Write, FileShare.None))
            {
                byte[] data = new UTF8Encoding(false).GetBytes(next);
                try
                {
                    output.Write(data, 0, data.Length);
                    output.Flush(true);
                }
                finally { Array.Clear(data, 0, data.Length); }
            }
            File.Replace(draft, journal, null);
            OrdinaryTree(root);
            if (Directory.GetFiles(secrets).Length != names.Length)
                throw new InvalidOperationException("Numero incorreto de credenciais apos publicacao.");
            foreach (string name in names)
                VerifySecretAcl(Path.Combine(secrets, name));
            // DO NOT run initdb, create Windows users/services, or change
            // trust stores. The customer Install button remains disabled.
        }

        async Task RunStageSecrets()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview || working)
                return;
            working = true;
            check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled =
                stageVendors.Enabled = preflight.Enabled = stagePostgres.Enabled =
                stageData.Enabled = stageSecrets.Enabled = false;
            try
            {
                if (MessageBox.Show(
                    "PREPARAR SENHAS OFFLINE - NAO INSTALA O ERP.\n\n" +
                    "Exige pastas vazias em ProgramData criadas anteriormente. " +
                    "Gera 6 credenciais independentes com gerador criptografico " +
                    "e as grava SOMENTE em Secrets protegido.\n\n" +
                    "NAO instala PostgreSQL, Garnet, servicos, TLS nem backup. " +
                    "Se interrompido, preserva o estado e bloqueia repeticao.\n\nProsseguir?",
                    "Esterium MIT - etapa 5", MessageBoxButtons.YesNo,
                    MessageBoxIcon.Warning) != DialogResult.Yes)
                    return;
                var start = new ProcessStartInfo(Application.ExecutablePath, "--stage-oss-secrets")
                { UseShellExecute = true, Verb = "runas" };
                int code = await Task.Run(() => {
                    using (var process = Process.Start(start))
                    { process.WaitForExit(); return process.ExitCode; }
                });
                Log(code == 0 ?
                    "PASS: credenciais offline em Secrets protegido. Servicos e banco NAO instalados." :
                    "BLOQUEADO: credenciais nao preparadas (codigo " + code +
                    "). Inspecionar estado protegido antes de repetir.");
            }
            catch (Win32Exception ex) { Log("UAC cancelado: " + ex.Message); }
            catch (Exception ex) { Log("BLOQUEADO: " + ex.Message); }
            finally
            {
                working = false;
                check.Enabled = true;
                plan.Enabled = !Program.PreviewBuild || (Program.HasPayload && !IsPostgresZipPreview);
                stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = Program.HasPayload;
                stagePostgres.Enabled = stageData.Enabled = stageSecrets.Enabled =
                    Program.HasPayload && IsPostgresZipPreview;
                install.Enabled = false;
            }
        }
    }
}
