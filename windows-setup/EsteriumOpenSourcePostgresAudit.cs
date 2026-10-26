using System;
using System.Collections.Generic;
using System.IO;
using System.IO.Compression;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;

// Read-only native integrity audit of PostgreSQL files prepared by the MIT
// technical preview. Do NOT execute, restore or modify any PostgreSQL bytes.
// This is a prerequisite to any future elevated bootstrap, NOT an installer.
namespace Esterium.WindowsSetup
{
    sealed partial class Installer
    {
        internal static void AuditOpenSourcePostgresBinaries()
        {
            if (!Program.PreviewBuild || !Program.HasPayload || !IsPostgresZipPreview)
                throw new InvalidOperationException("Auditoria exige a previa MIT integrada com ZIP oficial.");
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent())
                    .IsInRole(WindowsBuiltInRole.Administrator))
                throw new InvalidOperationException("Auditoria de Program Files exige Administrador.");
            string pf = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            string source = Path.Combine(pf, "EsteriumOpenSourceVendorStage",
                "postgresql-binaries.zip");
            string destination = Path.Combine(pf, "EsteriumOpenSourcePostgresBin");
            if (!File.Exists(source) || !Directory.Exists(destination) ||
                (File.GetAttributes(source) & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Arquivos PostgreSQL preparados nao encontrados.");
            OrdinaryTree(Path.GetDirectoryName(source));
            OrdinaryTree(destination);
            var rootAcl = Directory.GetAccessControl(destination);
            if (!rootAcl.AreAccessRulesProtected)
                throw new InvalidOperationException("Diretorio PostgreSQL perdeu ACL protegida.");
            // This audit runs before an account is configured. Reject broad
            // access before an executable could be loaded by any future service.
            foreach (FileSystemAccessRule acl in rootAcl.GetAccessRules(true, true,
                typeof(SecurityIdentifier)))
            {
                if (acl.AccessControlType != AccessControlType.Allow) continue;
                var sid = ((SecurityIdentifier)acl.IdentityReference).Value;
                if (sid != "S-1-5-18" && sid != "S-1-5-32-544")
                    throw new InvalidOperationException("Diretorio PostgreSQL possui ACL nao aprovada.");
            }

            string prefix = Path.GetFullPath(destination).TrimEnd(Path.DirectorySeparatorChar) +
                            Path.DirectorySeparatorChar;
            var expected = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            long expanded = 0;
            int entries = 0;
            using (var stream = new FileStream(source, FileMode.Open, FileAccess.Read, FileShare.Read))
            {
                using (var sha = SHA256.Create())
                {
                    string vendorHash = BitConverter.ToString(sha.ComputeHash(stream)).Replace("-", "");
                    if (!String.Equals(vendorHash, OfficialPostgresZipSHA256,
                        StringComparison.Ordinal))
                        throw new InvalidOperationException("O ZIP PostgreSQL aprovado foi alterado.");
                }
                stream.Position = 0;
                using (var archive = new ZipArchive(stream, ZipArchiveMode.Read, true))
                {
                    foreach (var entry in archive.Entries)
                    {
                        if (++entries > 30000 || entry.Length > 1300000000L ||
                            expanded + entry.Length > 1300000000L)
                            throw new InvalidOperationException("ZIP excede os limites de auditoria.");
                        string name = entry.FullName;
                        if (!name.StartsWith("pgsql/", StringComparison.Ordinal) ||
                            name.Contains("\\") || name.Contains(":"))
                            throw new InvalidOperationException("Entrada ZIP PostgreSQL invalida.");
                        foreach (var part in name.TrimEnd('/').Split('/'))
                            if (part.Length == 0 || part == "." || part == ".." ||
                                part.EndsWith(".") || part.EndsWith(" "))
                                throw new InvalidOperationException("Caminho ZIP inseguro.");
                        if (((entry.ExternalAttributes >> 16) & 0xf000) == 0xa000)
                            throw new InvalidOperationException("ZIP contem simbolos ou links.");
                        if (name.EndsWith("/")) continue;
                        if (!expected.Add(name))
                            throw new InvalidOperationException("ZIP duplicou caminho de arquivo.");
                        string path = Path.GetFullPath(Path.Combine(destination,
                            name.Replace('/', Path.DirectorySeparatorChar)));
                        if (!path.StartsWith(prefix, StringComparison.OrdinalIgnoreCase) ||
                            !File.Exists(path) ||
                            (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
                            throw new InvalidOperationException("Arquivo PostgreSQL ausente ou vinculado.");
                        string reference;
                        using (var referenceStream = entry.Open())
                        using (var checksum = SHA256.Create())
                            reference = BitConverter.ToString(
                                checksum.ComputeHash(referenceStream)).Replace("-", "");
                        expanded += entry.Length;
                        if (!String.Equals(reference, Digest(path), StringComparison.Ordinal))
                            throw new InvalidOperationException("Arquivo PostgreSQL divergiu da origem: " + name);
                    }
                }
            }
            int actual = 0;
            foreach (string path in Directory.GetFiles(destination, "*", SearchOption.AllDirectories))
            {
                actual++;
                string relative = path.Substring(prefix.Length)
                    .Replace(Path.DirectorySeparatorChar, '/');
                if (!expected.Contains(relative))
                    throw new InvalidOperationException("Arquivo nao aprovado no diretorio PostgreSQL: " + relative);
            }
            if (actual != expected.Count ||
                !expected.Contains("pgsql/bin/postgres.exe") ||
                !expected.Contains("pgsql/bin/initdb.exe") ||
                !expected.Contains("pgsql/bin/pg_ctl.exe"))
                throw new InvalidOperationException("Auditoria PostgreSQL incompleta.");
            // Read-only success. No new secrets, data, services or listeners.
        }
    }
}
