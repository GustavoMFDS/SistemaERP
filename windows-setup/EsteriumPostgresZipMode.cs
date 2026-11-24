using System;
using System.Collections.Generic;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Text;
using System.Text.RegularExpressions;
using System.Web.Script.Serialization;

// The fixed official PostgreSQL binary archive can be used by both the
// unsigned staging preview and the signed retail installer. Its exact
// upstream SHA-256 replaces the EDB installer Authenticode check.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        internal const string OfficialPostgresZipSHA256 =
            "43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281";
        internal static bool IsPostgresZipBuild
        {
            get
            {
                if (!Program.HasPayload) return false;
                string resourceName = Program.PreviewBuild
                    ? "Esterium.PreviewApprovals" : "Esterium.ReleaseApprovals";
                using (var s = Assembly.GetExecutingAssembly()
                    .GetManifestResourceStream(resourceName))
                {
                    if (s == null) return false;
                    using (var reader = new StreamReader(s, Encoding.UTF8))
                    {
                        var meta = new JavaScriptSerializer()
                            .Deserialize<Dictionary<string,string>>(reader.ReadToEnd());
                        if (meta == null) throw new InvalidOperationException("Metadados invalidos.");
                        string format;
                        if (!meta.TryGetValue("pgFormat", out format) || format == "edb") return false;
                        if (format != "zip" || !meta.ContainsKey("postgres") ||
                            !String.Equals(meta["postgres"], OfficialPostgresZipSHA256,
                                StringComparison.OrdinalIgnoreCase) ||
                            (meta.ContainsKey("pgSigner") && !String.IsNullOrWhiteSpace(meta["pgSigner"])))
                            throw new InvalidOperationException("PostgreSQL ZIP nao corresponde ao pacote EDB oficial fixado.");
                        return true;
                    }
                }
            }
        }
        internal static bool IsPostgresZipPreview
        {
            get { return Program.PreviewBuild && Program.HasPayload && IsPostgresZipBuild; }
        }
        internal static string EmbeddedPostgresName
        {
            get { return IsPostgresZipBuild ? "postgresql-binaries.zip" : "postgresql.exe"; }
        }
        internal static void AssertOfficialPostgresZip(string file)
        {
            if (!IsPostgresZipBuild || Digest(file) != OfficialPostgresZipSHA256)
                throw new InvalidOperationException("ZIP oficial PostgreSQL nao corresponde ao SHA-256 fixado.");
            var mustHave = new HashSet<string>(StringComparer.Ordinal) {
                "pgsql/bin/postgres.exe", "pgsql/bin/initdb.exe",
                "pgsql/bin/pg_ctl.exe", "pgsql/bin/psql.exe", "pgsql/bin/pg_isready.exe"
            };
            using (var stream = File.OpenRead(file))
            using (var zip = new ZipArchive(stream, ZipArchiveMode.Read))
            {
                if (zip.Entries.Count < 5 || zip.Entries.Count > 30000)
                    throw new InvalidOperationException("Quantidade de entradas ZIP PostgreSQL invalida.");
                var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
                long total = 0;
                foreach (var entry in zip.Entries)
                {
                    var name = entry.FullName.Replace('\\', '/');
                    var parts = name.TrimEnd('/').Split('/');
                    if (!name.StartsWith("pgsql/", StringComparison.Ordinal) ||
                        name.Contains(":") || name.StartsWith("/") || name.Length > 200 ||
                        parts.Length < 2 || !seen.Add(name.TrimEnd('/')) ||
                        ((entry.ExternalAttributes >> 16) & 0xf000) == 0xa000)
                        throw new InvalidOperationException("PostgreSQL ZIP possui caminho incorreto.");
                    foreach (var part in parts)
                        if (part == "" || part == "." || part == ".." ||
                            part.EndsWith(".", StringComparison.Ordinal) ||
                            part.EndsWith(" ", StringComparison.Ordinal) ||
                            Regex.IsMatch(part, "^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\\..*)?$"))
                            throw new InvalidOperationException("PostgreSQL ZIP possui entrada insegura.");
                    if (!name.EndsWith("/")) mustHave.Remove(name);
                    total += entry.Length;
                    if (total > 1300000000L)
                        throw new InvalidOperationException("ZIP PostgreSQL excede limite esperado.");
                }
            }
            if (mustHave.Count != 0)
                throw new InvalidOperationException("ZIP PostgreSQL nao possui todos os binarios essenciais.");
        }
    }
}
