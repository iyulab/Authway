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

// SchemaMigration represents a database migration in the schema_migrations table
type SchemaMigration struct {
	ID                int       `gorm:"primaryKey"`
	Version           string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	Name              string    `gorm:"type:varchar(255);not null"`
	ExecutedAt        time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	ExecutionTimeMs   int       `gorm:"type:int"`
	Checksum          string    `gorm:"type:varchar(64)"`
	Success           bool      `gorm:"type:boolean;default:true"`
	ErrorMessage      string    `gorm:"type:text"`
}

// TableName overrides the table name
func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

// RunMigrations executes all pending SQL migrations
func RunMigrations(db *gorm.DB, logger *zap.Logger) error {
	logger.Info("Starting database migrations")

	// Create migrations tracking table
	if err := createMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get all migration files
	migrations, err := getMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	if len(migrations) == 0 {
		logger.Info("No migration files found")
		return nil
	}

	logger.Info("Found migration files", zap.Int("count", len(migrations)))

	// Execute migrations
	appliedCount := 0
	skippedCount := 0

	for _, migration := range migrations {
		applied, err := isMigrationApplied(db, migration.Version)
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

		if err := applyMigration(db, migration, logger); err != nil {
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

	return nil
}

// migrationFile represents a migration file
type migrationFile struct {
	Filename string
	Version  string
	Name     string
	Content  string
	Checksum string
}

// createMigrationsTable creates the schema_migrations tracking table
func createMigrationsTable(db *gorm.DB) error {
	// Use raw SQL to create idempotent tracking table
	createTableSQL := `
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

	-- Record tracking table creation if not exists
	INSERT INTO schema_migrations (version, name, execution_time_ms, success)
	VALUES ('000', 'init_migration_system', 0, true)
	ON CONFLICT (version) DO NOTHING;
	`
	return db.Exec(createTableSQL).Error
}

// getMigrationFiles reads all migration files from embedded filesystem
func getMigrationFiles() ([]migrationFile, error) {
	var migrations []migrationFile

	// Regex to extract version and name from filename (e.g., 001_add_user_claims.sql)
	filenameRegex := regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Extract version and name from filename
		matches := filenameRegex.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			// Skip files that don't match version pattern
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

		// Calculate checksum
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

	// Sort by version number
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// isMigrationApplied checks if a migration has already been applied successfully
func isMigrationApplied(db *gorm.DB, version string) (bool, error) {
	var count int64
	err := db.Model(&SchemaMigration{}).
		Where("version = ? AND success = ?", version, true).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// applyMigration executes a migration and records it
func applyMigration(db *gorm.DB, migration migrationFile, logger *zap.Logger) error {
	// Use raw SQL connection for migration execution
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying DB: %w", err)
	}

	startTime := time.Now()

	// Start transaction
	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Record migration start (with success=false)
	recordStartSQL := `
		INSERT INTO schema_migrations (version, name, checksum, success)
		VALUES ($1, $2, $3, false)
		ON CONFLICT (version) DO UPDATE
		SET success = false, executed_at = CURRENT_TIMESTAMP
	`
	_, err = tx.Exec(recordStartSQL, migration.Version, migration.Name, migration.Checksum)
	if err != nil {
		return fmt.Errorf("failed to record migration start: %w", err)
	}

	// Execute migration SQL
	migrationErr := executeMigrationSQL(tx, migration.Content, logger)

	executionTime := int(time.Since(startTime).Milliseconds())

	if migrationErr != nil {
		// Record migration failure
		recordFailSQL := `
			UPDATE schema_migrations
			SET success = false,
			    error_message = $1,
			    execution_time_ms = $2,
			    executed_at = CURRENT_TIMESTAMP
			WHERE version = $3
		`
		_, _ = tx.Exec(recordFailSQL, migrationErr.Error(), executionTime, migration.Version)

		// Rollback will happen in defer
		return fmt.Errorf("failed to execute migration SQL: %w", migrationErr)
	}

	// Record migration success
	recordSuccessSQL := `
		UPDATE schema_migrations
		SET success = true,
		    execution_time_ms = $1,
		    executed_at = CURRENT_TIMESTAMP,
		    error_message = NULL
		WHERE version = $2
	`
	_, err = tx.Exec(recordSuccessSQL, executionTime, migration.Version)
	if err != nil {
		return fmt.Errorf("failed to record migration success: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	logger.Info("Migration completed",
		zap.String("version", migration.Version),
		zap.Int("execution_time_ms", executionTime))

	return nil
}

// executeMigrationSQL executes the SQL content of a migration
func executeMigrationSQL(tx *sql.Tx, content string, logger *zap.Logger) error {
	// Execute entire migration file as a single statement
	// PostgreSQL can handle multiple statements separated by semicolons
	// This is safer than trying to parse SQL ourselves
	logger.Debug("Executing migration SQL")

	if _, err := tx.Exec(content); err != nil {
		return fmt.Errorf("migration execution failed: %w", err)
	}

	return nil
}
