package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var migrationSQLName = regexp.MustCompile(`^([0-9]{4})_[a-z0-9_]+\.(up|down)\.sql$`)

// localMigrationHead checks release files without executing any SQL or
// touching a database. Paired, contiguous migrations are required.
func localMigrationHead(dir string) (int, error) {
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		return 0, fmt.Errorf("cannot inspect bundled migrations: %w", err)
	}
	up := make(map[string]bool)
	down := make(map[string]bool)
	versions := make(map[int]bool)
	upStemsByVersion := make(map[int]string)
	head := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("unexpected directory or symlink in migration bundle")
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			return 0, fmt.Errorf("unexpected file in migration bundle")
		}
		match := migrationSQLName.FindStringSubmatch(name)
		if match == nil {
			return 0, fmt.Errorf("invalid migration filename")
		}
		version, parseErr := strconv.Atoi(match[1])
		if parseErr != nil || version <= 0 {
			return 0, fmt.Errorf("invalid migration version")
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(name, ".up.sql"), ".down.sql")
		if match[2] == "up" {
			if existing, ok := upStemsByVersion[version]; ok && existing != stem {
				return 0, fmt.Errorf("duplicate migration version %04d", version)
			}
			upStemsByVersion[version] = stem
			up[stem] = true
			versions[version] = true
		} else {
			down[stem] = true
		}
		if version > head {
			head = version
		}
	}
	if head == 0 || len(up) != len(down) {
		return 0, fmt.Errorf("migration bundle is empty or missing pairs")
	}
	for stem := range up {
		if !down[stem] {
			return 0, fmt.Errorf("missing rollback SQL for a bundled migration")
		}
	}
	for version := 1; version <= head; version++ {
		if !versions[version] {
			return 0, fmt.Errorf("migration bundle has a version gap")
		}
	}
	return head, nil
}

func verifyLocalSchemaVersion(databaseVersion int, dirty bool, migrationDir string) error {
	if databaseVersion <= 0 || dirty {
		return fmt.Errorf("database schema migrations are incomplete or dirty")
	}
	head, err := localMigrationHead(migrationDir)
	if err != nil {
		return fmt.Errorf("installed SQL migration bundle is invalid: %w", err)
	}
	if databaseVersion != head {
		return fmt.Errorf("database schema version %d differs from installed bundle version %d", databaseVersion, head)
	}
	return nil
}
