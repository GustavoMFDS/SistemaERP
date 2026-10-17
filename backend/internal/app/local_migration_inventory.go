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
	head := 0
	for _, entry := range entries {
		if entry.IsDir() {
			return 0, fmt.Errorf("unexpected directory in migration bundle")
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
