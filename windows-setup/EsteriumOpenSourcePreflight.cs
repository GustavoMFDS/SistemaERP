using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Net.NetworkInformation;
using System.Reflection;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Web.Script.Serialization;

// Phase 2B: read-only fresh Windows dependency preflight for MIT unsigned build.
// This code never installs or starts any vendor software.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        internal static void AssertUnsignedFreshHost()
        {
            if (!Program.PreviewBuild || !Program.HasPayload)
                throw new InvalidOperationException("Pacote integrado sem assinatura obrigatorio.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("Preflight necessita Administrador somente para conferir pastas protegidas.");
            Dictionary<string, string> pins;
            using (var input = Assembly.GetExecutingAssembly()
                .GetManifestResourceStream("Esterium.PreviewApprovals"))
            {
                if (input == null) throw new InvalidOperationException("Metadados de hashes ausentes.");
                using (var reader = new StreamReader(input, Encoding.UTF8))
                    pins = new JavaScriptSerializer().Deserialize<Dictionary<string,string>>(reader.ReadToEnd());
            }
            if (pins == null) throw new InvalidOperationException("Metadados invalidos.");
            string programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            string programData = Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData);
            string erp = Path.Combine(programFiles, "EsteriumOpenSourceStage");
            string vendor = Path.Combine(programFiles, "EsteriumOpenSourceVendorStage");
            if (!Directory.Exists(erp) || !Directory.Exists(vendor))
                throw new InvalidOperationException("Prepare ERP e fornecedores antes do preflight.");
            OrdinaryTree(erp);
            OrdinaryTree(vendor);
            Bundle(erp, Hex("manifesto", pins["manifest"], 64));
            string[] files = { EmbeddedPostgresName, "garnet.zip", "nssm.exe" };
            string[] keys = { "postgres", "garnet", "nssm" };
            if (Directory.GetFiles(vendor, "*", SearchOption.AllDirectories).Length != 3 ||
                Directory.GetDirectories(vendor, "*", SearchOption.AllDirectories).Length != 0)
                throw new InvalidOperationException("Pasta de fornecedores possui arquivo extra.");
            for (int i = 0; i < files.Length; i++)
            {
                string path = Path.Combine(vendor, files[i]);
                if (!File.Exists(path) ||
                    (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0 ||
                    Digest(path) != Hex(files[i], pins[keys[i]], 64))
                    throw new InvalidOperationException("Fornecedor incorreto: " + files[i]);
            }
            if (IsPostgresZipPreview) AssertOfficialPostgresZip(Path.Combine(vendor, EmbeddedPostgresName));
            else AssertEdBAuthenticode(Path.Combine(vendor, "postgresql.exe"),
                Hex("certificado EDB", pins["pgSigner"], 40));
            // Detect conflicts BEFORE a future installer has a chance to create accounts,
            // secrets, DB clusters, Windows services, or any network listener.
            var paths = new[] {
                Path.Combine(programFiles, "PostgreSQL", "16"),
                Path.Combine(programFiles, "EsteriumGarnet"),
                Path.Combine(programFiles, "EsteriumERP"),
                Path.Combine(programFiles, "EsteriumApprovedSetup"),
                Path.Combine(programData, "Esterium", "PostgresData"),
                Path.Combine(programData, "Esterium", "PostgresInstallOptions"),
                Path.Combine(programData, "Esterium", "PostgresTlsPolicy"),
                Path.Combine(programData, "Esterium", "DependencyBootstrap"),
                Path.Combine(programData, "Esterium", "GarnetConfig"),
                Path.Combine(programData, "Esterium", "PostgresApplication"),
                Path.Combine(programData, "Esterium", "Secrets")
            };
            foreach (string path in paths)
                if (Directory.Exists(path) || File.Exists(path))
                    throw new InvalidOperationException("Pasta de instalacao ja existente: " + path +
                        ". Nunca sobrescrever ou apagar dados automaticamente.");
            var services = new[] { "EsteriumPostgreSQL16", "EsteriumGarnet", "EsteriumERP" };
            foreach (string name in services)
                using (var service = new System.ServiceProcess.ServiceController(name))
                {
                    try { var status = service.Status; throw new InvalidOperationException(
                        "Servico ja registrado: " + name); }
                    catch (InvalidOperationException e)
                    {
                        // ServiceController throws Win32Exception 1060 for a missing service,
                        // not InvalidOperationException. Rethrow real application failures.
                        if (e.InnerException == null) throw;
                        var win32 = e.InnerException as Win32Exception;
                        if (win32 == null || win32.NativeErrorCode != 1060) throw;
                    }
                }
            var listeners = IPGlobalProperties.GetIPGlobalProperties().GetActiveTcpListeners();
            foreach (int port in new[] { 5432, 6379, 8080 })
                if (Array.Exists(listeners, endpoint => endpoint.Port == port))
                    throw new InvalidOperationException("Porta ja usada: " + port);
            // Inspect the local dedicated account without changing it.
            using (var query = new System.Management.ManagementObjectSearcher(
                "SELECT Name FROM Win32_UserAccount WHERE LocalAccount = TRUE AND Name = 'esterium_pgsvc'"))
            {
                if (query.Get().Count != 0)
                    throw new InvalidOperationException("A conta PostgreSQL ja existe. Nao alterar senhas.");
            }
        }

        async Task RunUnsignedPreflight()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || working) return;
            working = true;
            check.Enabled = plan.Enabled = install.Enabled =
                stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = stagePostgres.Enabled = false;
            try
            {
                if (MessageBox.Show(
                    "Este teste LE SOMENTE dados do Windows e os arquivos preparados.\n" +
                    "Ele nao instala, inicia nem interrompe qualquer servico.\n" +
                    "Executar com permissao de Administrador?",
                    "Preflight sem instalacao", MessageBoxButtons.YesNo, MessageBoxIcon.Information)
                    != DialogResult.Yes) return;
                var psi = new ProcessStartInfo(Application.ExecutablePath, "--preflight-oss")
                { UseShellExecute = true, Verb = "runas" };
                int code = await Task.Run(() => {
                    using (var p = Process.Start(psi)) { p.WaitForExit(); return p.ExitCode; }
                });
                Log(code == 0 ?
                    "PASS: arquivos e Windows limpo para futura instalacao. Nenhum servico instalado." :
                    "BLOQUEADO: algum requisito nao passou. Preserve pastas, dados e servicos existentes.");
            }
            catch (Win32Exception ex) { Log("UAC cancelado/falhou: " + ex.Message); }
            catch (Exception ex) { Log("BLOQUEADO: " + ex.Message); }
            finally
            {
                working = false; check.Enabled = true;
                plan.Enabled = stageFiles.Enabled = stageVendors.Enabled =
                    preflight.Enabled = Program.HasPayload;
                stagePostgres.Enabled = Program.HasPayload && IsPostgresZipPreview;
                install.Enabled = false;
            }
        }
    }
}
