using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

// Phase 3 of the MIT unsigned technical preview:
// Extract independently SHA-256 pinned upstream PostgreSQL ZIP binaries into
// a fresh, access-controlled Program Files directory. NO initdb, no service,
// no user/account, no SQL and no network. Never touches an existing install.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        internal static void StageOpenSourcePostgresBinaries()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview)
                throw new InvalidOperationException("Somente previa MIT integrada com ZIP oficial PostgreSQL.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent())
                    .IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("UAC e Administrador obrigatorios.");

            // Preflight verifies the *previous* ERP/vendor stages, embedded pins,
            // PostgreSQL upstream ZIP SHA+entry safety and fresh Windows state.
            AssertUnsignedFreshHost();
            string pf = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            if (String.IsNullOrEmpty(pf) || !Path.IsPathRooted(pf))
                throw new InvalidOperationException("Program Files invalido.");
            string destination = Path.Combine(pf, "EsteriumOpenSourcePostgresBin");
            if (Directory.Exists(destination) || File.Exists(destination))
                throw new InvalidOperationException("Binarios PostgreSQL ja preparados. Nao sobrescrever.");

            string vendor = Path.Combine(pf, "EsteriumOpenSourceVendorStage");
            OrdinaryTree(vendor);
            string file = Path.Combine(vendor, EmbeddedPostgresName);
            if (!File.Exists(file) ||
                (File.GetAttributes(file) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("ZIP PostgreSQL preparado ausente ou vinculado.");
            AssertOfficialPostgresZip(file); // Independent fixed upstream SHA-256.
            string partial = null;
            bool committed = false;
            try
            {
                partial = Path.Combine(pf, ".esterium-oss-pg-bin-" + Guid.NewGuid().ToString("N"));
                Directory.CreateDirectory(partial);
                var access = new DirectorySecurity();
                access.SetAccessRuleProtection(true, false);
                var inheritance = InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit;
                access.AddAccessRule(new FileSystemAccessRule(
                    new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                    FileSystemRights.FullControl, inheritance, PropagationFlags.None, AccessControlType.Allow));
                access.AddAccessRule(new FileSystemAccessRule(
                    new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                    FileSystemRights.FullControl, inheritance, PropagationFlags.None, AccessControlType.Allow));
                Directory.SetAccessControl(partial, access);

                var hashes = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
                long total = 0;
                int count = 0;
                string rootPrefix = Path.GetFullPath(partial).TrimEnd(Path.DirectorySeparatorChar) +
                                    Path.DirectorySeparatorChar;
                // Reopen under the protected source tree, after checking the
                // official ZIP SHA. Archives are never executed or mounted.
                using (var zip = ZipFile.OpenRead(file))
                {
                    foreach (var entry in zip.Entries)
                    {
                        if (++count > 30000 || entry.Length > 1300000000L ||
                            total + entry.Length > 1300000000L)
                            throw new InvalidOperationException("ZIP PostgreSQL excede limites.");
                        string name = entry.FullName;
                        if (!name.StartsWith("pgsql/", StringComparison.Ordinal) ||
                            name.Contains("\\") || name.Contains(":") ||
                            !name.Equals(name.Trim(), StringComparison.Ordinal))
                            throw new InvalidOperationException("Entrada PostgreSQL invalida.");
                        string[] segments = name.TrimEnd('/').Split('/');
                        foreach (string segment in segments)
                            if (segment.Length == 0 || segment == "." || segment == ".." ||
                                segment.EndsWith(".") || segment.EndsWith(" "))
                                throw new InvalidOperationException("Caminho ZIP inseguro.");
                        if (((entry.ExternalAttributes >> 16) & 0xf000) == 0xa000)
                            throw new InvalidOperationException("Link simbolico ZIP recusado.");
                        string target = Path.GetFullPath(Path.Combine(partial,
                            name.Replace('/', Path.DirectorySeparatorChar)));
                        if (!target.StartsWith(rootPrefix, StringComparison.OrdinalIgnoreCase))
                            throw new InvalidOperationException("ZIP tenta sair do diretorio protegido.");
                        if (name.EndsWith("/"))
                        {
                            Directory.CreateDirectory(target);
                            continue;
                        }
                        if (!hashes.ContainsKey(name))
                        {
                            Directory.CreateDirectory(Path.GetDirectoryName(target));
                            long length = 0;
                            using (var stream = entry.Open())
                            using (var output = new FileStream(target, FileMode.CreateNew,
                                FileAccess.Write, FileShare.None))
                            using (var sha = SHA256.Create())
                            {
                                byte[] buffer = new byte[65536];
                                int read;
                                while ((read = stream.Read(buffer, 0, buffer.Length)) > 0)
                                {
                                    length += read;
                                    if (length > entry.Length || total + length > 1300000000L)
                                        throw new InvalidOperationException("ZIP excede tamanho declarado.");
                                    output.Write(buffer, 0, read);
                                    sha.TransformBlock(buffer, 0, read, buffer, 0);
                                }
                                sha.TransformFinalBlock(new byte[0], 0, 0);
                                output.Flush(true);
                                if (length != entry.Length)
                                    throw new InvalidOperationException("Entrada ZIP truncada.");
                                hashes.Add(name, BitConverter.ToString(sha.Hash).Replace("-", ""));
                            }
                            total += length;
                        }
                        else throw new InvalidOperationException("ZIP possui caminho duplicado.");
                    }
                }

                foreach (string required in new [] { "pgsql/bin/postgres.exe",
                    "pgsql/bin/initdb.exe", "pgsql/bin/pg_ctl.exe", "pgsql/bin/psql.exe",
                    "pgsql/bin/pg_isready.exe" })
                    if (!hashes.ContainsKey(required))
                        throw new InvalidOperationException("Binario PostgreSQL obrigatorio ausente.");

                OrdinaryTree(partial);
                if (Directory.Exists(destination) || File.Exists(destination))
                    throw new InvalidOperationException("Outro instalador criou destino PostgreSQL.");
                Directory.Move(partial, destination);
                committed = true;
                // Verify every extracted byte in its actual final location
                // against the hashes computed from the fixed upstream ZIP.
                foreach (var item in hashes)
                {
                    string installed = Path.Combine(destination,
                        item.Key.Replace('/', Path.DirectorySeparatorChar));
                    if (!File.Exists(installed) || Digest(installed) != item.Value)
                        throw new InvalidOperationException("Binario extraido alterado: " + item.Key);
                }
                OrdinaryTree(destination);
                // Deliberately stop HERE. No process execution, initdb, SCM
                // registration, service accounts, SQL or customer data.
            }
            finally
            {
                if (!committed && partial != null && Directory.Exists(partial))
                {
                    try { OrdinaryTree(partial); Directory.Delete(partial, true); }
                    catch { /* Fail closed: keep suspicious temporary paths for manual review. */ }
                }
            }
        }

        async Task RunStagePostgresBinaries()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview || working)
                return;
            working = true;
            check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled =
                stageVendors.Enabled = preflight.Enabled = stagePostgres.Enabled = false;
            try
            {
                var values = new Dictionary<string,string>();
                foreach (var item in inputs) values[item.Key] = item.Value.Text;
                Log("Conferindo ZIP PostgreSQL oficial antes da preparacao dos binarios...");
                await Task.Run(() => Prepare(values));
                string temp = staging;
                await Task.Run(() => CleanTemporaryFiles());
                if (temp != null && Directory.Exists(temp))
                    throw new InvalidOperationException("Area temporaria nao foi limpa.");
                staging = null;
                ownsStaging = false;
                if (MessageBox.Show(
                    "PREPARAR BINARIOS POSTGRESQL, NAO INSTALAR.\n\n" +
                    "Requer ERP e fornecedores preparados e preflight de Windows limpo.\n" +
                    "Copia binarios para Program Files protegidos. NAO cria cluster, " +
                    "conta, servico, banco ou conexao de rede.\n\nProsseguir?",
                    "Esterium MIT - etapa 3", MessageBoxButtons.YesNo,
                    MessageBoxIcon.Warning) != DialogResult.Yes)
                    return;
                var start = new ProcessStartInfo(Application.ExecutablePath, "--stage-oss-postgres")
                { UseShellExecute = true, Verb = "runas" };
                int code = await Task.Run(() => {
                    using (var p = Process.Start(start)) { p.WaitForExit(); return p.ExitCode; }
                });
                Log(code == 0 ?
                    "PASS: binarios PostgreSQL preparados. Cluster e servicos NAO instalados." :
                    "BLOQUEADO: falha na preparacao PostgreSQL (codigo " + code + "). Nao repetir sem diagnostico.");
            }
            catch (Win32Exception ex) { Log("UAC cancelado/falhou: " + ex.Message); }
            catch (Exception ex) { Log("BLOQUEADO: " + ex.Message); }
            finally
            {
                working = false;
                check.Enabled = true;
                plan.Enabled = !Program.PreviewBuild || (Program.HasPayload && !IsPostgresZipPreview);
                stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = Program.HasPayload;
                stagePostgres.Enabled = Program.HasPayload && IsPostgresZipPreview;
                install.Enabled = false;
            }
        }
    }
}
