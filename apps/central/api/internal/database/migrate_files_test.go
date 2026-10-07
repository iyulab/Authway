package database

import (
	"strings"
	"testing"
	"testing/fstest"
)

// The embedded set is what ships: if it cannot be read unambiguously the API
// refuses to start, so the suite must refuse first.
func TestEmbeddedMigrationFilesAreUnambiguous(t *testing.T) {
	migrations, err := getMigrationFiles()
	if err != nil {
		t.Fatalf("embedded migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no embedded migrations")
	}
	if migrations[0].Version != "000" {
		t.Fatalf("first migration = %s, want 000", migrations[0].Version)
	}
}

func TestReadMigrationFilesOrdersByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"002_b.sql": {Data: []byte("SELECT 2;")},
		"000_a.sql": {Data: []byte("SELECT 0;")},
		"001_c.sql": {Data: []byte("SELECT 1;")},
		"README.md": {Data: []byte("not a migration")},
		"notes.txt": {Data: []byte("not a migration")},
	}

	migrations, err := readMigrationFiles(fsys)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, m := range migrations {
		got = append(got, m.Version+"_"+m.Name)
	}
	if strings.Join(got, ",") != "000_a,001_c,002_b" {
		t.Fatalf("order = %v", got)
	}
	if len(migrations[0].Checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(migrations[0].Checksum))
	}
}

func TestReadMigrationFilesRefusesSharedVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"008_add_feature.sql":          {Data: []byte("ALTER TABLE t ADD COLUMN c int;")},
		"008_add_feature_rollback.sql": {Data: []byte("ALTER TABLE t DROP COLUMN c;")},
	}

	_, err := readMigrationFiles(fsys)
	if err == nil || !strings.Contains(err.Error(), "share version 008") {
		t.Fatalf("err = %v, want shared-version refusal", err)
	}
}

func TestReadMigrationFilesRefusesUnversionedSQL(t *testing.T) {
	fsys := fstest.MapFS{
		"000_a.sql":      {Data: []byte("SELECT 0;")},
		"add_column.sql": {Data: []byte("ALTER TABLE t ADD COLUMN c int;")},
	}

	_, err := readMigrationFiles(fsys)
	if err == nil || !strings.Contains(err.Error(), "add_column.sql") {
		t.Fatalf("err = %v, want refusal naming the file", err)
	}
}
