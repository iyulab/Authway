package database

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migration represents a database migration
type Migration struct {
	ID            int       `gorm:"primaryKey"`
	MigrationFile string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	AppliedAt     time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	Checksum      string    `gorm:"type:varchar(64)"`
}

// TableName overrides the table name
func (Migration) TableName() string {
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
		applied, err := isMigrationApplied(db, migration.Name)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", migration.Name, err)
		}

		if applied {
			logger.Debug("Skipping already applied migration", zap.String("file", migration.Name))
			skippedCount++
			continue
		}

		logger.Info("Applying migration", zap.String("file", migration.Name))

		if err := applyMigration(db, migration, logger); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}

		appliedCount++
		logger.Info("Successfully applied migration", zap.String("file", migration.Name))
	}

	logger.Info("Migration summary",
		zap.Int("applied", appliedCount),
		zap.Int("skipped", skippedCount),
		zap.Int("total", len(migrations)))

	return nil
}

// migrationFile represents a migration file
type migrationFile struct {
	Name     string
	Content  string
	Checksum string
}

// createMigrationsTable creates the schema_migrations tracking table
func createMigrationsTable(db *gorm.DB) error {
	// Use raw SQL to avoid GORM AutoMigrate issues
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		id SERIAL PRIMARY KEY,
		migration_file VARCHAR(255) UNIQUE NOT NULL,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		checksum VARCHAR(64)
	);
	`
	return db.Exec(createTableSQL).Error
}

// getMigrationFiles reads all migration files from embedded filesystem
func getMigrationFiles() ([]migrationFile, error) {
	var migrations []migrationFile

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

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
			Name:     entry.Name(),
			Content:  string(content),
			Checksum: checksum,
		})
	}

	// Sort by filename (ensures 000_, 001_, etc. order)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Name < migrations[j].Name
	})

	return migrations, nil
}

// isMigrationApplied checks if a migration has already been applied
func isMigrationApplied(db *gorm.DB, filename string) (bool, error) {
	var count int64
	err := db.Model(&Migration{}).Where("migration_file = ?", filename).Count(&count).Error
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

	// Start transaction
	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	if err := executeMigrationSQL(tx, migration.Content, logger); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration in tracking table
	recordSQL := `INSERT INTO schema_migrations (migration_file, checksum, applied_at)
	              VALUES ($1, $2, $3)`
	_, err = tx.Exec(recordSQL, migration.Name, migration.Checksum, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

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
