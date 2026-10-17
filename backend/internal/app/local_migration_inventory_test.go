package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalMigrationHeadValidatesVersionedPairs(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- synthetic\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("0001_base.up.sql")
	write("0001_base.down.sql")
	write("0002_stock.up.sql")
	write("0002_stock.down.sql")
	if head, err := localMigrationHead(dir); err != nil || head != 2 {
		t.Fatalf("paired migration head = %d, error=%v", head, err)
	}
	write("0003_partial.up.sql")
	if _, err := localMigrationHead(dir); err == nil {
		t.Fatal("accepted migration without down pair")
	}
	write("0003_partial.down.sql")
	if head, err := localMigrationHead(dir); err != nil || head != 3 {
		t.Fatalf("complete migration 3 was rejected: %d, %v", head, err)
	}
	if err := os.Remove(filepath.Join(dir, "0002_stock.up.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err := localMigrationHead(dir); err == nil {
		t.Fatal("accepted missing intermediate migration")
	}
}

func TestLocalMigrationHeadRejectsMissingOrInvalidFolder(t *testing.T) {
	if _, err := localMigrationHead(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted nonexistent migration folder")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "credentials.secret"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := localMigrationHead(dir); err == nil {
		t.Fatal("accepted unexpected files in migration folder")
	}
}

func TestVerifyLocalSchemaVersionRejectsOutdatedDatabase(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"0001_initial.up.sql", "0001_initial.down.sql",
		"0002_next.up.sql", "0002_next.down.sql",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- synthetic\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyLocalSchemaVersion(2, false, dir); err != nil {
		t.Fatalf("rejected equal clean schema: %v", err)
	}
	if err := verifyLocalSchemaVersion(1, false, dir); err == nil {
		t.Fatal("accepted database version older than bundle")
	}
	if err := verifyLocalSchemaVersion(3, false, dir); err == nil {
		t.Fatal("accepted database version newer than bundle")
	}
	if err := verifyLocalSchemaVersion(2, true, dir); err == nil {
		t.Fatal("accepted dirty migration state")
	}
}
