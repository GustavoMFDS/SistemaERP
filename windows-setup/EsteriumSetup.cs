using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Web.Script.Serialization;

// Graphical wrapper of the existing fail-closed, signed PowerShell coordinator.
// The EXE cannot turn on selling, enable SEFAZ, approve its own signature or install unsigned software.
namespace Esterium.WindowsSetup
{
    static class Program
    {
#if ESTERIUM_PREVIEW
        internal static readonly bool PreviewBuild = true;
#else
        internal static readonly bool PreviewBuild = false;
#endif
#if ESTERIUM_VM_LAB
        internal static readonly bool VmLabBuild = true;
#else
        internal static readonly bool VmLabBuild = false;
#endif
        internal static bool HasPayload
        {
            get { using (var s = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.Payload")) return s != null; }
        }
        internal static Dictionary<string, string> ReleaseApprovals
        {
            get
            {
                try
                {
                    using (var resource = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.ReleaseApprovals"))
                    {
                        if (resource == null) return null;
                        using (var reader = new StreamReader(resource, Encoding.UTF8))
                        {
                            var values = new JavaScriptSerializer().Deserialize<Dictionary<string, string>>(reader.ReadToEnd());
                            string pgFormat;
                            string pgSigner;
                            if (values == null || !HexValue(values, "manifest", 64) || !HexValue(values, "editor", 40) ||
                                !HexValue(values, "postgres", 64) || !values.TryGetValue("pgFormat", out pgFormat) ||
                                !values.TryGetValue("pgSigner", out pgSigner) ||
                                (pgFormat == "edb" && !Regex.IsMatch(pgSigner ?? "", "^[A-Fa-f0-9]{40}$")) ||
                                (pgFormat == "zip" && (!String.IsNullOrEmpty(pgSigner) ||
                                    !String.Equals(values["postgres"], Installer.OfficialPostgresZipSHA256,
                                        StringComparison.OrdinalIgnoreCase))) ||
                                (pgFormat != "edb" && pgFormat != "zip") ||
                                !HexValue(values, "garnet", 64) || !HexValue(values, "winsw", 64)) return null;
                            foreach (string key in new[] { "manifest", "editor", "postgres", "pgSigner", "garnet", "winsw" })
                                values[key] = values[key].ToUpperInvariant();
                            return values;
                        }
                    }
                }
                catch { return null; }
            }
        }
        static bool HexValue(Dictionary<string, string> values, string key, int length)
        {
            string value;
            return values.TryGetValue(key, out value) && Regex.IsMatch(value ?? "", "^[A-Fa-f0-9]{" + length + "}$");
        }
        internal static bool HasReleaseApprovals { get { return ReleaseApprovals != null; } }
        internal static bool CanApply { get { return !PreviewBuild && HasPayload && HasReleaseApprovals; } }
        internal static bool IsInstallableBuild
        {
            get
            {
                var approvals = ReleaseApprovals;
                return CanApply && approvals != null && Installer.IsLauncherAuthenticodeValid(approvals["editor"]);
            }
        }
        internal static bool IsRetailBuild { get { return !VmLabBuild && IsInstallableBuild; } }
        [STAThread]
        static void Main(string[] args)
        {
            if (args.Length > 0 && args[0] == "--audit-oss-postgres")
            {
                try { Installer.AuditOpenSourcePostgresBinaries(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--inspect-oss-state")
            {
                try
                {
                    string phase = Installer.InspectOpenSourceStageReadOnly();
                    Console.WriteLine("STAGE_INSPECTION: " + phase);
                    Environment.Exit(0);
                }
                catch (Exception)
                {
                    // No filesystem paths, metadata or secret data on error.
                    Console.Error.WriteLine("STAGE_INSPECTION: BLOQUEADO; revisar estado nativo manualmente.");
                    Environment.Exit(5);
                }
                return;
            }
            if (args.Length > 0 && args[0] == "--stage-oss-secrets")
            {
                try { Installer.StageOpenSourceSecrets(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--stage-oss-data")
            {
                try { Installer.StageOpenSourceDataFolders(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--stage-oss-postgres")
            {
                try { Installer.StageOpenSourcePostgresBinaries(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--preflight-oss")
            {
                try { Installer.AssertUnsignedFreshHost(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--stage-oss-vendors")
            {
                try { Installer.StageOpenSourceVendors(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--stage-oss-files")
            {
                try { Installer.StageOpenSourceFiles(); Environment.Exit(0); }
                catch (Exception ex) { Console.Error.WriteLine(ex.Message); Environment.Exit(5); }
                return;
            }
            if (args.Length > 0 && args[0] == "--self-test") { Environment.Exit(0); return; }
            if (args.Length > 0 && args[0] == "--is-retail-build") { Environment.Exit(IsRetailBuild ? 0 : 3); return; }
            if (args.Length > 0 && args[0] == "--has-release-approvals") { Environment.Exit(HasReleaseApprovals ? 0 : 2); return; }
            if (args.Length > 0 && args[0] == "--has-preview-approvals")
            {
                using (var meta = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.PreviewApprovals"))
                    Environment.Exit(meta == null ? 2 : 0);
                return;
            }
            if (args.Length > 0 && args[0] == "--has-payload")
            {
                using (var payload = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.Payload"))
                    Environment.Exit(payload == null ? 2 : 0);
                return;
            }
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            if (VmLabBuild && !IsVirtualBoxGuest())
            {
                MessageBox.Show("Esta build serve somente para homologação em uma VM VirtualBox. Nenhuma instalação foi iniciada.",
                    "Esterium ERP - laboratório", MessageBoxButtons.OK, MessageBoxIcon.Warning);
                Environment.Exit(8);
                return;
            }
            Application.Run(new Installer());
        }

        static bool IsVirtualBoxGuest()
        {
            try
            {
                using (var searcher = new System.Management.ManagementObjectSearcher(
                    "SELECT Manufacturer, Model FROM Win32_ComputerSystem"))
                using (var rows = searcher.Get())
                {
                    foreach (System.Management.ManagementObject row in rows)
                    {
                        string identity = Convert.ToString(row["Manufacturer"]) + " " + Convert.ToString(row["Model"]);
                        if (identity.IndexOf("virtualbox", StringComparison.OrdinalIgnoreCase) >= 0 ||
                            identity.IndexOf("innotek", StringComparison.OrdinalIgnoreCase) >= 0 ||
                            identity.IndexOf("oracle vm", StringComparison.OrdinalIgnoreCase) >= 0)
                            return true;
                    }
                }
            }
            catch { }
            return false;
        }
    }

    sealed partial class Installer : Form
    {
        readonly Dictionary<string, TextBox> inputs = new Dictionary<string, TextBox>();
        readonly TextBox output = new TextBox();
        readonly Button check = new Button(), plan = new Button(), install = new Button(), stageFiles = new Button(), stageVendors = new Button(), preflight = new Button(), stagePostgres = new Button(), stageData = new Button(), stageSecrets = new Button(), inspect = new Button();
        string staging;
        bool working;
        bool ownsStaging;

        public Installer()
        {
            Text = "Esterium ERP - Instalador Windows";
            StartPosition = FormStartPosition.CenterScreen;
            Size = new Size(840, 735);
            MinimumSize = new Size(710, 620);
            Font = new Font("Segoe UI", 9);
            var table = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 15,
                AutoScroll = true, Padding = new Padding(14) };
            table.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 300));
            table.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            Controls.Add(table);
            var title = new Label { AutoSize = true, Text = "Instalação local do Esterium ERP",
                Font = new Font("Segoe UI", 16, FontStyle.Bold) };
            table.Controls.Add(title, 0, 0); table.SetColumnSpan(title, 2);
            var detail = new Label { AutoSize = true, MaximumSize = new Size(760, 0),
                Text = Program.VmLabBuild ?
                       "MODO DE HOMOLOGAÇÃO VM: assinatura temporária, sem valor comercial. Use apenas nesta VM VirtualBox isolada; nunca em cliente." :
                       (Program.PreviewBuild ?
                       "CÓDIGO ABERTO / SEM ASSINATURA: hashes de referência carregados do build. " +
                       "Confira o SHA-256 do EXE por canal independente. Só verificar e planejar; NUNCA instalar." :
                       "Instalação técnica protegida. Hashes e publicador aprovado vêm fixados no pacote. " +
                       "O ERP só poderá entrar em operação após testes e homologação.") };
            table.Controls.Add(detail, 0, 1); table.SetColumnSpan(detail, 2);
            Add(table, 2, "Endereço HTTPS autorizado", "origin", "https://erp.exemplo.com:8443");
            Add(table, 3, "SHA-256 do manifesto ERP", "manifest", "");
            Add(table, 4, "Certificado do editor (40 caracteres)", "editor", "");
            Add(table, 5, "SHA-256 PostgreSQL", "postgres", "");
            Add(table, 6, IsPostgresZipBuild ? "ZIP oficial: SHA-256 fixado, sem certificado EDB" : "Certificado do PostgreSQL", "pgSigner", "");
            Add(table, 7, "SHA-256 Garnet", "garnet", "");
            Add(table, 8, "SHA-256 WinSW", "winsw", "");
            Add(table, 9, "Certificado HTTPS da loja (PEM local)", "tlsCert", "");
            Add(table, 10, "Chave HTTPS privada (PEM local)", "tlsKey", "");
            Add(table, 11, "CIDR privado permitido (ex. 192.168.1.42/32)", "trustedRemote", "");
            LoadBuildApprovals();
            var actions = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill, WrapContents = true };
            check.Text = "Verificar arquivos"; check.AutoSize = true;
            plan.Text = "Ver plano"; plan.AutoSize = true;
            plan.Enabled = !Program.PreviewBuild || Program.HasPayload;
            install.Text = Program.PreviewBuild ? "Instalação bloqueada (prévia)" : "Instalar";
            install.AutoSize = true;
            install.Enabled = Program.CanApply;
            stageFiles.Text = "Preparar arquivos (não instala)"; stageFiles.AutoSize = true;
            stageFiles.Enabled = Program.PreviewBuild && Program.HasPayload;
            stageVendors.Text = "Preparar dependências (não instala)"; stageVendors.AutoSize = true;
            stageVendors.Enabled = Program.PreviewBuild && Program.HasPayload;
            actions.Controls.Add(check); actions.Controls.Add(plan); actions.Controls.Add(stageFiles);
            preflight.Text = "Pré-checar Windows (não instala)"; preflight.AutoSize = true;
            preflight.Enabled = Program.PreviewBuild && Program.HasPayload;
            stagePostgres.Text = "Preparar binários PostgreSQL (não instala)";
            stagePostgres.AutoSize = true;
            stagePostgres.Enabled = Program.PreviewBuild && Program.HasPayload && IsPostgresZipPreview;
            actions.Controls.Add(stageVendors); actions.Controls.Add(preflight);
            stageData.Text = "Preparar pastas de dados (não instala)";
            stageData.AutoSize = true;
            stageData.Enabled = Program.PreviewBuild && Program.HasPayload && IsPostgresZipPreview;
            stageSecrets.Text = "Preparar credenciais offline (não instala)";
            stageSecrets.AutoSize = true;
            stageSecrets.Enabled = Program.PreviewBuild && Program.HasPayload && IsPostgresZipPreview;
            inspect.Text = "Inspecionar estado (somente leitura)";
            inspect.AutoSize = true;
            inspect.Enabled = Program.PreviewBuild && Program.HasPayload && IsPostgresZipPreview;
            actions.Controls.Add(stagePostgres); actions.Controls.Add(stageData);
            actions.Controls.Add(stageSecrets); actions.Controls.Add(inspect); actions.Controls.Add(install);
            table.Controls.Add(actions, 0, 12); table.SetColumnSpan(actions, 2);
            var warning = new Label { AutoSize = true, MaximumSize = new Size(760, 0),
                Text = Program.VmLabBuild ?
                       "A build de laboratório só abre em VirtualBox. Apply instala serviços e grava dados de teste nesta VM; informe certificado e chave HTTPS válidos e limite o firewall ao CIDR privado de um único cliente." :
                       (Program.PreviewBuild ?
                       "Preparar arquivos copia o ERP e Preparar dependências copia os binários de fornecedores " +
                       "para pastas protegidas. O ZIP oficial usa SHA-256 fixado; o EXE EDB usa assinatura. " +
                       "A etapa ZIP PostgreSQL extrai somente os binários. NÃO cria banco, serviços ou HTTPS. " +
                       "Inspecionar estado apenas verifica a estrutura; não repara nem retoma a instalação. " +
                       "Nunca repita uma etapa que falhou sem diagnóstico." :
                       "A instalação altera serviços Windows. Em caso de falha parcial, NÃO repita a instalação " +
                       "nem apague banco ou dados. O instalador nunca libera automaticamente vendas.") };
            table.Controls.Add(warning, 0, 13); table.SetColumnSpan(warning, 2);
            output.Dock = DockStyle.Fill; output.Multiline = true; output.ScrollBars = ScrollBars.Vertical;
            output.ReadOnly = true; output.MinimumSize = new Size(200, 145);
            table.Controls.Add(output, 0, 14); table.SetColumnSpan(output, 2);
            for (int i = 0; i < 14; i++) table.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            table.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
            check.Click += async (s, e) => await Run(false, false);
            plan.Click += async (s, e) => await Run(true, false);
            install.Click += async (s, e) => await Run(true, true);
            stageFiles.Click += async (s, e) => await RunStageFiles();
            stageVendors.Click += async (s, e) => await RunStageVendors();
            preflight.Click += async (s, e) => await RunUnsignedPreflight();
            stagePostgres.Click += async (s, e) => await RunStagePostgresBinaries();
            stageData.Click += async (s, e) => await RunStageDataFolders();
            stageSecrets.Click += async (s, e) => await RunStageSecrets();
            inspect.Click += async (s, e) => await RunInspectState();
            Log(Program.PreviewBuild ?
                (IsPostgresZipPreview ? "Prévia ZIP: valida pacotes e apresenta o plano; não instala. Instalar bloqueado." :
                 "Versão sem assinatura: somente verificação e PLANO de instalação sem alterações. Instalar continua bloqueado.") :
                (Program.CanApply ? "Pacote completo: assinatura e integridade serão conferidas antes de Apply." :
                 "Instalação bloqueada: faltam payload ou aprovações de release válidas."));
            FormClosing += (s, e) => { if (working) { e.Cancel = true; Log("Aguarde a conclusão da operação em andamento."); } };
            FormClosed += (s, e) => CleanTemporaryFiles();
        }

        void LoadBuildApprovals()
        {
            Dictionary<string, string> approvals = Program.PreviewBuild ? null : Program.ReleaseApprovals;
            if (Program.PreviewBuild)
            {
                using (var resource = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.PreviewApprovals"))
                {
                    if (resource != null)
                    using (var reader = new StreamReader(resource, Encoding.UTF8))
                        approvals = new JavaScriptSerializer().Deserialize<Dictionary<string, string>>(reader.ReadToEnd());
                }
            }
            if (approvals == null) return;
            foreach (var entry in approvals)
                if (inputs.ContainsKey(entry.Key) && entry.Key != "origin")
                {
                    inputs[entry.Key].Text = entry.Value;
                    inputs[entry.Key].ReadOnly = !Program.PreviewBuild;
                }
        }
        void Add(TableLayoutPanel t, int row, string label, string key, string value)
        {
            t.Controls.Add(new Label { Text = label, AutoSize = true, Margin = new Padding(0, 6, 2, 4) }, 0, row);
            var box = new TextBox { Text = value, Dock = DockStyle.Top };
            inputs[key] = box;
            t.Controls.Add(box, 1, row);
        }
        void Log(string message) { output.AppendText(message + Environment.NewLine); }
        static string Hex(string label, string data, int n)
        {
            if (!Regex.IsMatch(data.Trim(), "^[A-Fa-f0-9]{" + n + "}$"))
                throw new InvalidOperationException("O campo " + label + " exige valor aprovado independentemente.");
            return data.Trim().ToUpperInvariant();
        }
        static string Digest(string file)
        {
            using (var s = File.OpenRead(file)) using (var h = SHA256.Create())
                return BitConverter.ToString(h.ComputeHash(s)).Replace("-", "");
        }
        static void OrdinaryTree(string dir)
        {
            if (!Directory.Exists(dir) || (File.GetAttributes(dir) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Diretório de pacote ausente ou vinculado.");
            var folders = new Queue<string>();
            folders.Enqueue(dir);
            while (folders.Count > 0)
            {
                foreach (string p in Directory.GetFileSystemEntries(folders.Dequeue()))
                {
                    FileAttributes attributes = File.GetAttributes(p);
                    if ((attributes & FileAttributes.ReparsePoint) != 0)
                        throw new InvalidOperationException("Links simbólicos/junctions não são permitidos no pacote.");
                    if ((attributes & FileAttributes.Directory) != 0) folders.Enqueue(p);
                }
            }
        }
        static string Origin(string value)
        {
            Uri u;
            if (!Uri.TryCreate(value, UriKind.Absolute, out u) || u.Scheme != "https" || u.UserInfo.Length != 0 ||
                u.AbsolutePath != "/" || u.Query.Length != 0 || u.Fragment.Length != 0 ||
                u.Host == "localhost" || u.Host == "127.0.0.1" || u.Host == "::1" ||
                !Regex.IsMatch(u.Host, "^[A-Za-z0-9.-]+$"))
                throw new InvalidOperationException("Informe uma origem HTTPS válida e previamente aprovada.");
            return u.AbsoluteUri.TrimEnd('/');
        }
        static void Bundle(string dir, string approved)
        {
            OrdinaryTree(dir);
            string manifest = Path.Combine(dir, "bundle-manifest.json");
            if (!File.Exists(manifest) || Digest(manifest) != approved)
                throw new InvalidOperationException("Manifesto ERP diferente do SHA-256 aprovado.");
            var json = new JavaScriptSerializer { MaxJsonLength = 50000000 }
                .DeserializeObject(File.ReadAllText(manifest)) as Dictionary<string, object>;
            if (json == null || Convert.ToInt32(json["schema"]) != 1)
                throw new InvalidOperationException("Manifesto do ERP inválido.");
            var records = json["files"] as object[];
            if (records == null || records.Length < 3 || records.Length > 10000)
                throw new InvalidOperationException("Manifesto ERP incompleto.");
            var listed = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            foreach (object value in records)
            {
                var item = value as Dictionary<string, object>;
                if (item == null) throw new InvalidOperationException("Entrada inválida.");
                string rel = (string)item["path"], hash = (string)item["sha256"];
                if (String.IsNullOrEmpty(rel) || rel.Contains("\\") || rel.Contains(":") || rel.StartsWith("/") ||
                    !Regex.IsMatch(hash, "^[A-Fa-f0-9]{64}$") || !listed.Add(rel))
                    throw new InvalidOperationException("Caminho inválido ou repetido no manifesto.");
                foreach (string part in rel.Split('/'))
                    if (part == "" || part == "." || part == "..")
                        throw new InvalidOperationException("Caminho relativo inválido.");
                if (rel != "esterium-server.exe" && rel != "README_LOCAL.md" &&
                    !rel.StartsWith("tools/") && !rel.StartsWith("web/") && !rel.StartsWith("migrations/"))
                    throw new InvalidOperationException("Arquivo inesperado no ERP.");
                if (Regex.IsMatch(rel, @"(^|/)(\.env(\.[^/]*)?|[^/]*\.(key|pem|pfx|p12|secret|backup|db|sqlite|dump))($|/)", RegexOptions.IgnoreCase))
                    throw new InvalidOperationException("Material potencialmente secreto no ERP.");
                string file = Path.Combine(dir, rel.Replace('/', Path.DirectorySeparatorChar));
                if (!File.Exists(file) || Digest(file) != hash.ToUpperInvariant())
                    throw new InvalidOperationException("Arquivo adulterado ou ausente: " + rel);
            }
            if (!listed.Contains("esterium-server.exe") || !listed.Contains("web/index.html") ||
                !listed.Contains("tools/install-esterium-local-complete.ps1") ||
                Directory.GetFiles(dir, "*", SearchOption.AllDirectories).Length != listed.Count + 1)
                throw new InvalidOperationException("Bundle incompleto ou com arquivos extras.");
        }

        static string Unpack()
        {
            using (var resource = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.Payload"))
            {
                if (resource == null)
                    throw new InvalidOperationException("Este EXE não contém o pacote completo. Nenhuma instalação é permitida.");
                string root = Path.Combine(Path.GetTempPath(), "EsteriumSetup-" + Guid.NewGuid().ToString("N"));
                Directory.CreateDirectory(root);
                try
                {
                using (var zip = new ZipArchive(resource, ZipArchiveMode.Read))
                {
                    if (zip.Entries.Count < 4 || zip.Entries.Count > 15000)
                        throw new InvalidOperationException("Arquivo de distribuição vazio ou grande demais.");
                    var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
                    long expanded = 0;
                    foreach (var entry in zip.Entries)
                    {
                        string name = entry.FullName.Replace('\\', '/');
                        string[] parts = name.TrimEnd('/').Split('/');
                        if (name.StartsWith("/") || name.Contains(":") || parts.Length < 2 ||
                            (parts[0] != "erp" && parts[0] != "vendor") ||
                            ((entry.ExternalAttributes >> 16) & 0xf000) == 0xa000 ||
                            !seen.Add(name.TrimEnd('/')))
                            throw new InvalidOperationException("ZIP contém entrada insegura.");
                        foreach (string part in parts)
                            if (part == "" || part == "." || part == ".." || part.EndsWith(".") ||
                                part.EndsWith(" ") ||
                                Regex.IsMatch(part, @"^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\..*)?$", RegexOptions.IgnoreCase) ||
                                Regex.IsMatch(part, @"[<>?*|\x00-\x1F]"))
                                throw new InvalidOperationException("ZIP contém nome incompatível ou reservado do Windows.");
                        expanded += entry.Length;
                        if (expanded > 4L * 1024 * 1024 * 1024 || entry.Length > 2L * 1024 * 1024 * 1024)
                            throw new InvalidOperationException("ZIP excede os limites permitidos.");
                        string target = Path.Combine(root, name.Replace('/', Path.DirectorySeparatorChar));
                        if (name.EndsWith("/")) { Directory.CreateDirectory(target); continue; }
                        Directory.CreateDirectory(Path.GetDirectoryName(target));
                        using (var input = entry.Open())
                        using (var result = new FileStream(target, FileMode.CreateNew))
                            input.CopyTo(result);
                    }
                }
                return root;
                }
                catch
                {
                    try { OrdinaryTree(root); Directory.Delete(root, true); }
                    catch { /* Leave unexpected paths for manual review. */ }
                    throw;
                }
            }
        }
        string Prepare(Dictionary<string,string> values)
        {
            if (staging == null) { staging = Unpack(); ownsStaging = true; }
            string erp = Path.Combine(staging, "erp");
            Bundle(erp, Hex("manifesto", values["manifest"], 64));
            string[] names = { EmbeddedPostgresName, "garnet.zip", "winsw.exe" };
            string[] keys = { "postgres", "garnet", "winsw" };
            for (int i = 0; i < names.Length; i++)
            {
                string file = Path.Combine(staging, "vendor", names[i]);
                if (!File.Exists(file) || (File.GetAttributes(file) & FileAttributes.ReparsePoint) != 0 ||
                    Digest(file) != Hex(names[i], values[keys[i]], 64))
                    throw new InvalidOperationException("Fornecedor ausente ou SHA-256 divergente: " + names[i]);
            }
            if (!Program.PreviewBuild) Hex("editor", values["editor"], 40);
            // EDB installer requires Authenticode. Official binary ZIP uses an
            // independently vendor-published SHA instead and is preview-only.
            if (IsPostgresZipBuild) AssertOfficialPostgresZip(Path.Combine(staging, "vendor", EmbeddedPostgresName));
            else Hex("certificado PostgreSQL", values["pgSigner"], 40);
            Origin(values["origin"].Trim());
            return erp;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        struct WinTrustFileInfo
        {
            public uint cbStruct;
            public IntPtr pcwszFilePath;
            public IntPtr hFile;
            public IntPtr pgKnownSubject;
        }
        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        struct WinTrustData
        {
            public uint cbStruct;
            public IntPtr pPolicyCallbackData;
            public IntPtr pSIPClientData;
            public uint dwUIChoice;
            public uint fdwRevocationChecks;
            public uint dwUnionChoice;
            public IntPtr pFile;
            public uint dwStateAction;
            public IntPtr hWVTStateData;
            public IntPtr pwszURLReference;
            public uint dwProvFlags;
            public uint dwUIContext;
            public IntPtr pSignatureSettings;
        }
        [DllImport("wintrust.dll", ExactSpelling = true, SetLastError = true)]
        static extern int WinVerifyTrust(IntPtr hwnd, ref Guid action, ref WinTrustData data);
        internal static bool IsLauncherAuthenticodeValid(string publisher)
        {
            // Verify the full bundled EXE through Windows trust APIs. A subprocess
            // can stall while PowerShell re-hashes this large file; keep the GUI responsive.
            string file = Assembly.GetExecutingAssembly().Location;
            IntPtr path = IntPtr.Zero;
            IntPtr fileInfoPtr = IntPtr.Zero;
            try
            {
                path = Marshal.StringToCoTaskMemUni(file);
                var fileInfo = new WinTrustFileInfo {
                    cbStruct = (uint)Marshal.SizeOf(typeof(WinTrustFileInfo)),
                    pcwszFilePath = path
                };
                fileInfoPtr = Marshal.AllocHGlobal(Marshal.SizeOf(typeof(WinTrustFileInfo)));
                Marshal.StructureToPtr(fileInfo, fileInfoPtr, false);
                var data = new WinTrustData {
                    cbStruct = (uint)Marshal.SizeOf(typeof(WinTrustData)),
                    dwUIChoice = 2, // WTD_UI_NONE
                    fdwRevocationChecks = 0, // WTD_REVOKE_NONE
                    dwUnionChoice = 1, // WTD_CHOICE_FILE
                    pFile = fileInfoPtr,
                    dwStateAction = 0, // WTD_STATEACTION_IGNORE
                    dwProvFlags = 0x00001000 // WTD_CACHE_ONLY_URL_RETRIEVAL
                };
                var action = new Guid("00AAC56B-CD44-11D0-8CC2-00C04FC295EE");
                if (WinVerifyTrust(IntPtr.Zero, ref action, ref data) != 0) return false;
                using (var raw = X509Certificate.CreateFromSignedFile(file))
                using (var certificate = new X509Certificate2(raw))
                {
                    return String.Equals(certificate.Thumbprint, publisher,
                        StringComparison.OrdinalIgnoreCase);
                }
            }
            catch { return false; }
            finally
            {
                if (fileInfoPtr != IntPtr.Zero) Marshal.FreeHGlobal(fileInfoPtr);
                if (path != IntPtr.Zero) Marshal.FreeCoTaskMem(path);
            }
        }
        static void CheckSignedLauncher(string publisher)
        {
            if (!IsLauncherAuthenticodeValid(publisher))
                throw new InvalidOperationException("EXE sem assinatura Authenticode válida do editor aprovado.");
        }
        static string Quote(string arg)
        {
            if (arg.Contains("\"") || arg.Contains("\r") || arg.Contains("\n"))
                throw new InvalidOperationException("Parâmetro inválido.");
            return "\"" + arg + "\"";
        }
        static string LocalFileInput(string label, string value)
        {
            string path = (value ?? "").Trim();
            if (!Path.IsPathRooted(path) || path.StartsWith("\\\\", StringComparison.Ordinal) ||
                !File.Exists(path) || (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Informe um arquivo local existente para " + label + ".");
            return Path.GetFullPath(path);
        }
        ProcessStartInfo Build(string erp, bool apply, string tlsCert, string tlsKey, string trustedRemote)
        {
            string allowedOrigin = Origin(inputs["origin"].Text.Trim());
            int httpsPort = new Uri(allowedOrigin, UriKind.Absolute).Port;
            string[] args = {
                "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy",
                Program.PreviewBuild ? "RemoteSigned" : "AllSigned", "-File",
                Path.Combine(erp, "tools", "install-esterium-local-complete.ps1"),
                "-SourceBundleDir", erp, "-AllowedOrigin", allowedOrigin,
                "-ListenAddress", "0.0.0.0:" + httpsPort.ToString(System.Globalization.CultureInfo.InvariantCulture),
                "-PostgresInstaller", Path.Combine(staging, "vendor", EmbeddedPostgresName),
                "-ExpectedPostgresSHA256", inputs["postgres"].Text.Trim(),
                "-GarnetPackage", Path.Combine(staging, "vendor", "garnet.zip"),
                "-ExpectedGarnetSHA256", inputs["garnet"].Text.Trim(),
                "-ServiceWrapperExe", Path.Combine(staging, "vendor", "winsw.exe"),
                "-ExpectedServiceWrapperSHA256", inputs["winsw"].Text.Trim(),
                "-ApprovedManifestSHA256", inputs["manifest"].Text.Trim(),
                "-TrustedPublisherThumbprint", inputs["editor"].Text.Trim()
            };
            var list = new List<string>(args);
            if (IsPostgresZipBuild) list.Add("-PostgresBinaryZip");
            else list.AddRange(new[] { "-ExpectedPostgresPublisherThumbprint", inputs["pgSigner"].Text.Trim() });
            if (apply) list.AddRange(new[] { "-TLSCertFile", tlsCert, "-TLSKeyFile", tlsKey,
                "-TrustedRemoteAddress", trustedRemote });
            if (apply) list.Add("-Apply");
            return new ProcessStartInfo("powershell.exe", String.Join(" ", list.ConvertAll(Quote))) {
                UseShellExecute = apply, Verb = apply ? "runas" : "",
                RedirectStandardOutput = !apply, RedirectStandardError = !apply, CreateNoWindow = !apply
            };
        }
        void CleanTemporaryFiles()
        {
            if (!ownsStaging || staging == null) return;
            string prefix = Path.GetFullPath(Path.GetTempPath()).TrimEnd(Path.DirectorySeparatorChar) +
                            Path.DirectorySeparatorChar + "EsteriumSetup-";
            try
            {
                string path = Path.GetFullPath(staging);
                if (!path.StartsWith(prefix, StringComparison.OrdinalIgnoreCase)) return;
                OrdinaryTree(path);
                Directory.Delete(path, true);
            }
            catch { /* Never delete an unexpected/reparse directory. */ }
        }
        async Task Run(bool invoke, bool apply)
        {
            if (working) return;
            if (apply && !Program.CanApply) { Log("BLOQUEADO: pacote sem payload ou aprovações de release válidas."); return; }
            if (invoke && Program.PreviewBuild && !Program.HasPayload) { Log("BLOQUEADO: prévia sem payload."); return; }
            working = true; check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = stagePostgres.Enabled = stageData.Enabled = stageSecrets.Enabled = false;
            string phase = "verificação dos arquivos";
            try
            {
                if (apply)
                {
                    phase = "autenticação do instalador";
                    Log("Validando a assinatura digital do instalador...");
                    await Task.Run(() => CheckSignedLauncher(Program.ReleaseApprovals["editor"]));
                }
                // Capture UI state on its owning thread; hashing and extraction run off-thread.
                var values = new Dictionary<string,string>();
                foreach (var field in inputs) values[field.Key] = field.Value.Text;
                Log("Verificando e conferindo arquivos do pacote...");
                string erp = await Task.Run(() => Prepare(values));
                Log("PASS: integridade dos arquivos ERP e fornecedores conferida com os hashes informados.");
                if (!invoke) return;
                phase = apply ? "validação do plano de instalação" : "plano de instalação";
                string tlsCert = "", tlsKey = "", trustedRemote = "";
                if (apply)
                {
                    phase = "validação dos caminhos HTTPS e do CIDR privado";
                    tlsCert = LocalFileInput("certificado HTTPS PEM", inputs["tlsCert"].Text);
                    tlsKey = LocalFileInput("chave HTTPS PEM", inputs["tlsKey"].Text);
                    trustedRemote = inputs["trustedRemote"].Text.Trim();
                    if (trustedRemote.Length == 0 || trustedRemote.Length > 64 ||
                        trustedRemote.IndexOfAny(new[] { '\r', '\n', '"' }) >= 0)
                        throw new InvalidOperationException("Informe um CIDR IPv4 privado explícito.");
                }
                phase = apply ? "validação do plano de instalação" : "plano de instalação";
                if (apply && MessageBox.Show(
                    Program.VmLabBuild ?
                    "Este pacote usa assinatura temporária de homologação e altera serviços e dados somente nesta VM VirtualBox. Não use em cliente. Confirma a instalação de teste?" :
                    "O processo irá alterar o Windows e criar serviços. Confirma a instalação técnica de um pacote comercial assinado e aprovado?",
                    "Confirmar", MessageBoxButtons.YesNo, MessageBoxIcon.Warning) != DialogResult.Yes) return;
                ProcessStartInfo psi = Build(erp, apply, tlsCert, tlsKey, trustedRemote);
                Log(apply ? "Solicitando UAC. Serviços continuarão desativados para vendas." :
                    (Program.PreviewBuild ? "Executando APENAS o plano técnico não elevado, sem Apply." :
                     "Verificando plano de instalação com PowerShell AllSigned."));
                int exit = await Task.Run(() => {
                    using (var p = Process.Start(psi))
                    {
                        if (!apply)
                        {
                            Task<string> stdout = p.StandardOutput.ReadToEndAsync();
                            Task<string> stderr = p.StandardError.ReadToEndAsync();
                            p.WaitForExit();
                            Task.WaitAll(stdout, stderr);
                        }
                        else p.WaitForExit();
                        return p.ExitCode;
                    }
                });
                if (!apply)
                {
                    if (exit == 0)
                    {
                        Log("Plano concluído sem alterações.");
                        Log("PLANO: PostgreSQL 16, Garnet e ERP; TLS e banco novo; vendas permanecem desativadas.");
                    }
                    else Log("FALHA (código " + exit + ") na etapa do plano. Nenhuma alteração foi aplicada.");
                }
                else Log(exit == 0 ? "Procedimento técnico finalizado. Ainda NÃO liberado para vendas." :
                    "FALHA (código " + exit + "). Preserve evidências e não repita automaticamente.");
            }
            catch (Exception)
            {
                Log("BLOQUEADO: falha na etapa de " + phase + ".");
                Log(apply ? "Preserve evidências e não repita automaticamente." : "Nenhuma alteração foi aplicada.");
            }
            finally { working = false; check.Enabled = true;
                      plan.Enabled = !Program.PreviewBuild || Program.HasPayload;
                      stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = Program.PreviewBuild && Program.HasPayload;
                      stagePostgres.Enabled = stageData.Enabled = stageSecrets.Enabled = Program.PreviewBuild && Program.HasPayload && IsPostgresZipPreview;
                      install.Enabled = Program.CanApply; }
        }
    }
}
