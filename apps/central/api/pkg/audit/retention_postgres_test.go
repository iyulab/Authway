package audit_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tenantscope"
)

func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres audit tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestPurgeExpired_KeepsEachTenantsEntriesAsLongAsItSays: an entry goes once
// it is older than its tenant keeps entries — the tenant's
// audit_retention_days, or the default for a tenant that has not said and
// for an entry of no tenant.
func TestPurgeExpired_KeepsEachTenantsEntriesAsLongAsItSays(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	newTenant := func(settings tenant.TenantSettings) uuid.UUID {
		t.Helper()
		suffix := uuid.NewString()[:8]
		tn, err := ts.CreateTenant(tenant.CreateTenantRequest{Name: "audit-retention-" + suffix, Slug: "audit-retention-" + suffix, Settings: settings})
		if err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
		return tn.ID
	}
	short := newTenant(tenant.TenantSettings{AuditRetentionDays: 30})
	unsaid := newTenant(tenant.TenantSettings{})

	entry := func(tenantID *uuid.UUID, ageDays int) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := db.Exec(`INSERT INTO audit_logs (id, tenant_id, action, resource_type, success, created_at)
			VALUES (?, ?, 'user.login', 'user', true, NOW() - make_interval(days => ?))`, id, tenantID, ageDays).Error; err != nil {
			t.Fatalf("seed entry: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM audit_logs WHERE id = ?`, id) })
		return id
	}
	gone := []uuid.UUID{entry(&short, 40), entry(&unsaid, 400), entry(nil, 400)}
	kept := []uuid.UUID{entry(&short, 10), entry(&unsaid, 40), entry(nil, 40)}

	if _, err := audit.NewService(db, zap.NewNop()).PurgeExpired(tenant.DefaultAuditRetentionDays); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	count := func(id uuid.UUID) (n int64) {
		db.Raw(`SELECT count(*) FROM audit_logs WHERE id = ?`, id).Scan(&n)
		return n
	}
	for i, id := range gone {
		if count(id) != 0 {
			t.Errorf("entry %d is past its retention and should be gone", i)
		}
	}
	for i, id := range kept {
		if count(id) != 1 {
			t.Errorf("entry %d is within its retention and should stay", i)
		}
	}
}

// TestPurge_WithoutRetentionDaysKeepsWhatTheTenantKeeps: an owner's purge that
// names no retention_days removes what the daily clean-up would — entries
// past the tenant's own retention — not entries past some shorter default.
func TestPurge_WithoutRetentionDaysKeepsWhatTheTenantKeeps(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	newTenant := func(settings tenant.TenantSettings) uuid.UUID {
		t.Helper()
		suffix := uuid.NewString()[:8]
		tn, err := ts.CreateTenant(tenant.CreateTenantRequest{Name: "audit-purge-" + suffix, Slug: "audit-purge-" + suffix, Settings: settings})
		if err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
		return tn.ID
	}
	entry := func(tenantID uuid.UUID, ageDays int) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := db.Exec(`INSERT INTO audit_logs (id, tenant_id, action, resource_type, success, created_at)
			VALUES (?, ?, 'user.login', 'user', true, NOW() - make_interval(days => ?))`, id, tenantID, ageDays).Error; err != nil {
			t.Fatalf("seed entry: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM audit_logs WHERE id = ?`, id) })
		return id
	}
	count := func(id uuid.UUID) (n int64) {
		db.Raw(`SELECT count(*) FROM audit_logs WHERE id = ?`, id).Scan(&n)
		return n
	}

	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}})
	// The admin middleware puts the named tenant in this local.
	app.Use(func(c *fiber.Ctx) error { c.Locals(tenantscope.LocalKey, c.Get(tenantscope.Header)); return c.Next() })
	app.Delete("/purge", audit.NewHandler(audit.NewService(db, zap.NewNop()), zap.NewNop(), tenant.DefaultAuditRetentionDays).PurgeOldLogs)
	purge := func(tenantID uuid.UUID) (answer struct {
		Deleted       int64 `json:"deleted"`
		RetentionDays int   `json:"retention_days"`
	}) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodDelete, "/purge", nil)
		req.Header.Set(tenantscope.Header, tenantID.String())
		res, err := app.Test(req)
		if err != nil || res.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("purge: %v %d %s", err, res.StatusCode, body)
		}
		if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return answer
	}

	for _, tc := range []struct {
		name     string
		settings tenant.TenantSettings
		days     int
	}{
		{"its own retention", tenant.TenantSettings{AuditRetentionDays: 200}, 200},
		{"the default when it has not said", tenant.TenantSettings{}, tenant.DefaultAuditRetentionDays},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := newTenant(tc.settings)
			kept := entry(id, 100) // past the old 90-day default
			gone := entry(id, tc.days+10)
			got := purge(id)
			if got.RetentionDays != tc.days || got.Deleted != 1 {
				t.Errorf("answered %+v, want retention_days %d and 1 deleted", got, tc.days)
			}
			if count(kept) != 1 {
				t.Error("an entry the tenant still keeps was purged")
			}
			if count(gone) != 0 {
				t.Error("an entry past the tenant's retention is still there")
			}
		})
	}
}
