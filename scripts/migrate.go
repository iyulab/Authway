package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	// Database connection string
	connStr := "host=localhost port=5432 user=authway password=authway dbname=authway sslmode=disable"

	// Connect to database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	fmt.Println("✓ Connected to database")

	// Read migration file
	sqlFile := "migrations/000_v0_clean_slate.sql"
	sqlBytes, err := os.ReadFile(sqlFile)
	if err != nil {
		log.Fatal("Failed to read migration file:", err)
	}

	fmt.Println("✓ Read migration file:", sqlFile)

	// Execute migration
	sqlContent := string(sqlBytes)
	_, err = db.Exec(sqlContent)
	if err != nil {
		log.Fatal("Failed to execute migration:", err)
	}

	fmt.Println("✓ Migration executed successfully")

	// Verify tables
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&tableCount)
	if err != nil {
		log.Fatal("Failed to count tables:", err)
	}

	fmt.Printf("✓ Database has %d tables\n", tableCount)

	// List tables
	rows, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name")
	if err != nil {
		log.Fatal("Failed to list tables:", err)
	}
	defer rows.Close()

	fmt.Println("\n📋 Tables created:")
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			log.Fatal("Failed to scan table name:", err)
		}
		fmt.Println("  -", tableName)
	}

	// Check default tenant
	var tenantCount int
	err = db.QueryRow("SELECT COUNT(*) FROM tenants").Scan(&tenantCount)
	if err != nil {
		log.Fatal("Failed to count tenants:", err)
	}

	fmt.Printf("\n✓ Default tenant created: %d tenant(s) in database\n", tenantCount)

	fmt.Println("\n🎉 Migration completed successfully!")
}
