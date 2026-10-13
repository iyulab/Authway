package client

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestWritesDoNotNameTheRetiredColumn: allow_email_signup is mapped read-only
// until a migration drops it. A write that named it would fail on the
// instances still running while that migration is applied.
func TestWritesDoNotNameTheRetiredColumn(t *testing.T) {
	db := setupPostgres(t)
	c := &Client{ID: uuid.New(), TenantID: uuid.New(), ClientID: "retired-column-probe", Name: "probe"}

	for name, stmt := range map[string]*gorm.Statement{
		"insert": db.Session(&gorm.Session{DryRun: true}).Create(c).Statement,
		"save":   db.Session(&gorm.Session{DryRun: true}).Save(c).Statement,
	} {
		sql := stmt.SQL.String()
		if !strings.Contains(sql, "client_id") {
			t.Fatalf("not the %s this test means to read: %s", name, sql)
		}
		if strings.Contains(sql, "allow_email_signup") {
			t.Fatalf("the %s names allow_email_signup: %s", name, sql)
		}
	}
}
