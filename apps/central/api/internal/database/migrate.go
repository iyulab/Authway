package database

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationFile represents a migration file
type migrationFile struct {
	Filename string
	Version  string
	Name     string
	Content  string
	Checksum string
}

// RunMigrations executes all pending SQL migrations.
// All migrations run in a single transaction protected by pg_advisory_xact_lock(999999),
// ensuring only one instance migrates at a time and providing atomic all-or-nothing semantics.
func RunMigrations(db *gorm.DB, logger *zap.Logger) error {
	logger.Info("Starting database migrations")

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying DB: %w", err)
	}

	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start migration transaction: %w", err)
	}
	defer tx.Rollback()

	// Acquire exclusive advisory lock — released automatically when tx ends.
	// Uses the same lock ID as the PowerShell migration helper for cross-tool safety.
	if _, err = tx.Exec("SELECT pg_advisory_xact_lock(999999)"); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	if err := createMigrationsTableTx(tx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	migrations, err := getMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	if len(migrations) == 0 {
		logger.Info("No migration files found")
		return tx.Commit()
	}

	logger.Info("Found migration files", zap.Int("count", len(migrations)))

	appliedCount := 0
	skippedCount := 0

	for _, migration := range migrations {
		applied, err := isMigrationAppliedTx(tx, migration.Version)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", migration.Name, err)
		}

		if applied {
			logger.Debug("Skipping already applied migration",
				zap.String("version", migration.Version),
				zap.String("name", migration.Name))
			skippedCount++
			continue
		}

		logger.Info("Applying migration",
			zap.String("version", migration.Version),
			zap.String("name", migration.Name))

		if err := applyMigrationTx(tx, migration, logger); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}

		appliedCount++
		logger.Info("Successfully applied migration",
			zap.String("version", migration.Version),
			zap.String("name", migration.Name))
	}

	logger.Info("Migration summary",
		zap.Int("applied", appliedCount),
		zap.Int("skipped", skippedCount),
		zap.Int("total", len(migrations)))

	return tx.Commit()
}

// getMigrationFiles reads all migration files from embedded filesystem
func getMigrationFiles() ([]migrationFile, error) {
	var migrations []migrationFile

	filenameRegex := regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		matches := filenameRegex.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		version := matches[1]
		name := matches[2]

		// Use forward slash for embed.FS (not filepath.Join which uses OS-specific separator)
		filePath := "migrations/" + entry.Name()
		content, err := fs.ReadFile(migrationFiles, filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", entry.Name(), err)
		}

		hash := sha256.Sum256(content)
		checksum := fmt.Sprintf("%x", hash)[:64]

		migrations = append(migrations, migrationFile{
			Filename: entry.Name(),
			Version:  version,
			Name:     name,
			Content:  string(content),
			Checksum: checksum,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// createMigrationsTableTx creates the schema_migrations tracking table within tx.
func createMigrationsTableTx(tx *sql.Tx) error {
	_, err := tx.Exec(`
	CREATE TABLE IF NOT EXISTS schema_migrations (
		id SERIAL PRIMARY KEY,
		version VARCHAR(255) NOT NULL UNIQUE,
		name VARCHAR(255) NOT NULL,
		executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		execution_time_ms INTEGER,
		checksum VARCHAR(64),
		success BOOLEAN NOT NULL DEFAULT TRUE,
		error_message TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
	CREATE INDEX IF NOT EXISTS idx_schema_migrations_executed_at ON schema_migrations(executed_at DESC);

	INSERT INTO schema_migrations (version, name, execution_time_ms, success)
	VALUES ('000', 'init_migration_system', 0, true)
	ON CONFLICT (version) DO NOTHING;
	`)
	return err
}

// isMigrationAppliedTx checks if a migration has already been applied successfully.
func isMigrationAppliedTx(tx *sql.Tx, version string) (bool, error) {
	var count int
	err := tx.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = $1 AND success = true",
		version,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// applyMigrationTx executes a migration and records it within the shared transaction.
// Because all migrations share one transaction, a failure rolls back the entire run —
// providing all-or-nothing semantics without partial state.
func applyMigrationTx(tx *sql.Tx, migration migrationFile, logger *zap.Logger) error {
	startTime := time.Now()

	logger.Debug("Executing migration SQL", zap.String("version", migration.Version))
	if _, err := tx.Exec(migration.Content); err != nil {
		return fmt.Errorf("migration execution failed: %w", err)
	}

	executionTime := int(time.Since(startTime).Milliseconds())

	_, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, checksum, success, execution_time_ms)
		VALUES ($1, $2, $3, true, $4)
		ON CONFLICT (version) DO UPDATE
		SET success = true, execution_time_ms = $4, executed_at = CURRENT_TIMESTAMP, error_message = NULL
	`, migration.Version, migration.Name, migration.Checksum, executionTime)

	return err
}
