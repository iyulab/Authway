package main

import (
	"database/sql"
	"fmt"
	"log"

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

	fmt.Println("🔍 Verifying Database Schema...\n")

	// Check tenants table
	fmt.Println("📊 Tenants Table:")
	rows, err := db.Query("SELECT id, name, slug, active FROM tenants")
	if err != nil {
		log.Fatal("Failed to query tenants:", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, slug string
		var active bool
		if err := rows.Scan(&id, &name, &slug, &active); err != nil {
			log.Fatal("Failed to scan tenant:", err)
		}
		activeStatus := "✓"
		if !active {
			activeStatus = "✗"
		}
		fmt.Printf("  %s %s (slug: %s)\n", activeStatus, name, slug)
	}

	// Check table columns
	tables := []string{"tenants", "users", "clients", "sessions", "email_verifications", "password_resets", "admin_sessions"}

	fmt.Println("\n📋 Table Structure Verification:")
	for _, table := range tables {
		var count int
		err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = '%s'", table)).Scan(&count)
		if err != nil {
			log.Fatal("Failed to count columns:", err)
		}
		fmt.Printf("  ✓ %s: %d columns\n", table, count)
	}

	// Check foreign keys
	fmt.Println("\n🔗 Foreign Key Constraints:")
	constraintRows, err := db.Query(`
		SELECT
			tc.table_name,
			tc.constraint_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		AND tc.table_schema = 'public'
		ORDER BY tc.table_name, tc.constraint_name
	`)
	if err != nil {
		log.Fatal("Failed to query constraints:", err)
	}
	defer constraintRows.Close()

	for constraintRows.Next() {
		var tableName, constraintName, columnName, foreignTable, foreignColumn string
		if err := constraintRows.Scan(&tableName, &constraintName, &columnName, &foreignTable, &foreignColumn); err != nil {
			log.Fatal("Failed to scan constraint:", err)
		}
		fmt.Printf("  ✓ %s.%s → %s.%s\n", tableName, columnName, foreignTable, foreignColumn)
	}

	// Check indexes
	fmt.Println("\n📇 Indexes:")
	indexRows, err := db.Query(`
		SELECT
			tablename,
			indexname
		FROM pg_indexes
		WHERE schemaname = 'public'
		ORDER BY tablename, indexname
	`)
	if err != nil {
		log.Fatal("Failed to query indexes:", err)
	}
	defer indexRows.Close()

	currentTable := ""
	for indexRows.Next() {
		var tableName, indexName string
		if err := indexRows.Scan(&tableName, &indexName); err != nil {
			log.Fatal("Failed to scan index:", err)
		}
		if currentTable != tableName {
			currentTable = tableName
			fmt.Printf("\n  %s:\n", tableName)
		}
		fmt.Printf("    - %s\n", indexName)
	}

	fmt.Println("\n✅ Database schema verification complete!")
}
