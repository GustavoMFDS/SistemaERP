using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Security.Cryptography;
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
        internal static bool HasPayload
        {
            get { using (var s = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.Payload")) return s != null; }
        }
        [STAThread]
        static void Main(string[] args)
        {
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
            if (args.Length > 0 && args[0] == "--is-retail-build") { Environment.Exit(PreviewBuild ? 3 : 0); return; }
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
            Application.Run(new Installer());
        }
    }

    sealed partial class Installer : Form
    {
        readonly Dictionary<string, TextBox> inputs = new Dictionary<string, TextBox>();
        readonly TextBox output = new TextBox();
        readonly Button check = new Button(), plan = new Button(), install = new Button(), stageFiles = new Button(), stageVendors = new Button(), preflight = new Button();
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
            var table = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 12,
                AutoScroll = true, Padding = new Padding(14) };
            table.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 300));
            table.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            Controls.Add(table);
            var title = new Label { AutoSize = true, Text = "Instalação local do Esterium ERP",
                Font = new Font("Segoe UI", 16, FontStyle.Bold) };
            table.Controls.Add(title, 0, 0); table.SetColumnSpan(title, 2);
            var detail = new Label { AutoSize = true, MaximumSize = new Size(760, 0),
                Text = Program.PreviewBuild ?
                       "CÓDIGO ABERTO / SEM ASSINATURA: hashes de referência carregados do build. " +
                       "Confira o SHA-256 do EXE por canal independente. Só verificar e planejar; NUNCA instalar." :
                       "Instalação técnica protegida. Informe aprovações obtidas fora do próprio pacote. " +
                       "O ERP só poderá entrar em operação após testes e homologação." };
            table.Controls.Add(detail, 0, 1); table.SetColumnSpan(detail, 2);
            Add(table, 2, "Endereço HTTPS autorizado", "origin", "https://erp.exemplo.com");
            Add(table, 3, "SHA-256 do manifesto ERP", "manifest", "");
            Add(table, 4, "Certificado do editor (40 caracteres)", "editor", "");
            Add(table, 5, "SHA-256 PostgreSQL", "postgres", "");
            Add(table, 6, "Certificado do PostgreSQL", "pgSigner", "");
            Add(table, 7, "SHA-256 Garnet", "garnet", "");
            Add(table, 8, "SHA-256 NSSM", "nssm", "");
            LoadPreviewApprovals();
            var actions = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill, WrapContents = true };
            check.Text = "Verificar arquivos"; check.AutoSize = true;
            plan.Text = "Ver plano"; plan.AutoSize = true;
            plan.Enabled = !Program.PreviewBuild || Program.HasPayload;
            install.Text = Program.PreviewBuild ? "Instalação bloqueada (prévia)" : "Instalar";
            install.AutoSize = true;
            install.Enabled = !Program.PreviewBuild;
            stageFiles.Text = "Preparar arquivos (não instala)"; stageFiles.AutoSize = true;
            stageFiles.Enabled = Program.PreviewBuild && Program.HasPayload;
            stageVendors.Text = "Preparar dependências (não instala)"; stageVendors.AutoSize = true;
            stageVendors.Enabled = Program.PreviewBuild && Program.HasPayload;
            actions.Controls.Add(check); actions.Controls.Add(plan); actions.Controls.Add(stageFiles);
            preflight.Text = "Pré-checar Windows (não instala)"; preflight.AutoSize = true;
            preflight.Enabled = Program.PreviewBuild && Program.HasPayload;
            actions.Controls.Add(stageVendors); actions.Controls.Add(preflight); actions.Controls.Add(install);
            table.Controls.Add(actions, 0, 9); table.SetColumnSpan(actions, 2);
            var warning = new Label { AutoSize = true, MaximumSize = new Size(760, 0),
                Text = Program.PreviewBuild ?
                       "Preparar arquivos copia o ERP e Preparar dependências copia os binários de fornecedores " +
                       "para pastas protegidas, com hashes e assinatura EDB. " +
                       "NÃO instala banco, serviços ou HTTPS. Nunca repita uma etapa que falhou sem diagnóstico." :
                       "A instalação altera serviços Windows. Em caso de falha parcial, NÃO repita a instalação " +
                       "nem apague banco ou dados. O instalador nunca libera automaticamente vendas." };
            table.Controls.Add(warning, 0, 10); table.SetColumnSpan(warning, 2);
            output.Dock = DockStyle.Fill; output.Multiline = true; output.ScrollBars = ScrollBars.Vertical;
            output.ReadOnly = true; output.MinimumSize = new Size(200, 145);
            table.Controls.Add(output, 0, 11); table.SetColumnSpan(output, 2);
            for (int i = 0; i < 11; i++) table.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            table.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
            check.Click += async (s, e) => await Run(false, false);
            plan.Click += async (s, e) => await Run(true, false);
            install.Click += async (s, e) => await Run(true, true);
            stageFiles.Click += async (s, e) => await RunStageFiles();
            stageVendors.Click += async (s, e) => await RunStageVendors();
            preflight.Click += async (s, e) => await RunUnsignedPreflight();
            Log(Program.PreviewBuild ?
                "Versão sem assinatura: somente verificação e PLANO de instalação sem alterações. Instalar continua bloqueado." :
                "Edição assinada: aplique somente após aprovação independente e homologação.");
            FormClosing += (s, e) => { if (working) { e.Cancel = true; Log("Aguarde a conclusão da operação em andamento."); } };
            FormClosed += (s, e) => CleanTemporaryFiles();
        }

        void LoadPreviewApprovals()
        {
            if (!Program.PreviewBuild) return;
            using (var resource = Assembly.GetExecutingAssembly().GetManifestResourceStream("Esterium.PreviewApprovals"))
            {
                if (resource == null) return;
                using (var reader = new StreamReader(resource, Encoding.UTF8))
                {
                    var approvals = new JavaScriptSerializer().Deserialize<Dictionary<string,string>>(reader.ReadToEnd());
                    foreach (var entry in approvals)
                        if (inputs.ContainsKey(entry.Key) && entry.Key != "editor" && entry.Key != "origin")
                            inputs[entry.Key].Text = entry.Value;
                }
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
            string[] names = { "postgresql.exe", "garnet.zip", "nssm.exe" };
            string[] keys = { "postgres", "garnet", "nssm" };
            for (int i = 0; i < names.Length; i++)
            {
                string file = Path.Combine(staging, "vendor", names[i]);
                if (!File.Exists(file) || (File.GetAttributes(file) & FileAttributes.ReparsePoint) != 0 ||
                    Digest(file) != Hex(names[i], values[keys[i]], 64))
                    throw new InvalidOperationException("Fornecedor ausente ou SHA-256 divergente: " + names[i]);
            }
            if (!Program.PreviewBuild) Hex("editor", values["editor"], 40);
            // PostgreSQL is always signed by its real upstream publisher, even in unsigned preview.
            Hex("certificado PostgreSQL", values["pgSigner"], 40);
            Origin(values["origin"].Trim());
            return erp;
        }

        static void CheckSignedLauncher(string publisher)
        {
            // The signed EXE itself must be authenticated before elevation.
            // Keep paths and thumbprints in environment variables, never interpolate into PowerShell.
            const string check = "$s=Get-AuthenticodeSignature -LiteralPath $env:ESTERIUM_SETUP_EXE; " +
                "if($s.Status -ne 'Valid' -or $null -eq $s.SignerCertificate -or " +
                "$s.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $env:ESTERIUM_SETUP_PUBLISHER){exit 1}; exit 0";
            string encoded = Convert.ToBase64String(Encoding.Unicode.GetBytes(check));
            var psi = new ProcessStartInfo("powershell.exe",
                "-NoLogo -NoProfile -NonInteractive -EncodedCommand " + encoded) {
                UseShellExecute = false, CreateNoWindow = true
            };
            psi.EnvironmentVariables["ESTERIUM_SETUP_EXE"] = Assembly.GetExecutingAssembly().Location;
            psi.EnvironmentVariables["ESTERIUM_SETUP_PUBLISHER"] = publisher;
            using (var p = Process.Start(psi))
            {
                p.WaitForExit();
                if (p.ExitCode != 0)
                    throw new InvalidOperationException("EXE sem assinatura Authenticode válida do editor aprovado.");
            }
        }
        static string Quote(string arg)
        {
            if (arg.Contains("\"") || arg.Contains("\r") || arg.Contains("\n"))
                throw new InvalidOperationException("Parâmetro inválido.");
            return "\"" + arg + "\"";
        }
        ProcessStartInfo Build(string erp, bool apply)
        {
            string[] args = {
                "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy",
                Program.PreviewBuild ? "RemoteSigned" : "AllSigned", "-File",
                Path.Combine(erp, "tools", "install-esterium-local-complete.ps1"),
                "-SourceBundleDir", erp, "-AllowedOrigin", Origin(inputs["origin"].Text.Trim()),
                "-PostgresInstaller", Path.Combine(staging, "vendor", "postgresql.exe"),
                "-ExpectedPostgresSHA256", inputs["postgres"].Text.Trim(),
                "-ExpectedPostgresPublisherThumbprint", inputs["pgSigner"].Text.Trim(),
                "-GarnetPackage", Path.Combine(staging, "vendor", "garnet.zip"),
                "-ExpectedGarnetSHA256", inputs["garnet"].Text.Trim(),
                "-ServiceWrapperExe", Path.Combine(staging, "vendor", "nssm.exe"),
                "-ExpectedServiceWrapperSHA256", inputs["nssm"].Text.Trim(),
                "-ApprovedManifestSHA256", inputs["manifest"].Text.Trim(),
                "-TrustedPublisherThumbprint", inputs["editor"].Text.Trim()
            };
            var list = new List<string>(args);
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
            if (apply && Program.PreviewBuild) { Log("BLOQUEADO: prévia técnica nunca executa instalação."); return; }
            if (invoke && Program.PreviewBuild && !Program.HasPayload) { Log("BLOQUEADO: prévia sem payload."); return; }
            working = true; check.Enabled = plan.Enabled = install.Enabled = stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = false;
            try
            {
                // Capture UI state on its owning thread; hashing and extraction run off-thread.
                var values = new Dictionary<string,string>();
                foreach (var field in inputs) values[field.Key] = field.Value.Text;
                Log("Verificando e conferindo arquivos do pacote...");
                string erp = await Task.Run(() => Prepare(values));
                Log("PASS: integridade dos arquivos ERP e fornecedores conferida com os hashes informados.");
                if (!invoke) return;
                if (apply) CheckSignedLauncher(Hex("editor", inputs["editor"].Text, 40));
                if (apply && MessageBox.Show(
                    "O processo irá alterar o Windows e criar serviços. Confirma a instalação técnica de um pacote comercial assinado e aprovado?",
                    "Confirmar", MessageBoxButtons.YesNo, MessageBoxIcon.Warning) != DialogResult.Yes) return;
                ProcessStartInfo psi = Build(erp, apply);
                Log(apply ? "Solicitando UAC. Serviços continuarão desativados para vendas." :
                    (Program.PreviewBuild ? "Executando APENAS o plano técnico não elevado, sem Apply." :
                     "Verificando plano de instalação com PowerShell AllSigned."));
                string message = "";
                int exit = await Task.Run(() => {
                    using (var p = Process.Start(psi))
                    {
                        if (!apply)
                        {
                            Task<string> o = p.StandardOutput.ReadToEndAsync();
                            Task<string> e = p.StandardError.ReadToEndAsync();
                            p.WaitForExit();
                            message = o.Result + Environment.NewLine + e.Result;
                        }
                        else p.WaitForExit();
                        return p.ExitCode;
                    }
                });
                if (!apply && message.Length != 0) Log(message.Length > 12000 ? message.Substring(0, 12000) : message);
                Log(exit == 0 ? (apply ? "Procedimento técnico finalizado. Ainda NÃO liberado para vendas." : "Plano concluído sem alterações.") :
                    "FALHA (código " + exit + "). Não repita uma instalação parcial sem diagnóstico.");
            }
            catch (Exception error) { Log("BLOQUEADO: " + error.Message); }
            finally { working = false; check.Enabled = true; plan.Enabled = !Program.PreviewBuild || Program.HasPayload;
                      stageFiles.Enabled = stageVendors.Enabled = preflight.Enabled = Program.PreviewBuild && Program.HasPayload;
                      install.Enabled = !Program.PreviewBuild; }
        }
    }
}
