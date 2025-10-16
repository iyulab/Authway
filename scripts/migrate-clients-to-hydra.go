package main

import (
	"authway/src/server/internal/config"
	"authway/src/server/internal/database"
	"authway/src/server/internal/hydra"
	"authway/src/server/pkg/client"
	"fmt"
	"log"
	"strings"

	"github.com/lib/pq"
)

func main() {
	log.Println("🔄 Starting client migration to Hydra...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load configuration: %v", err)
	}

	// Connect to database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// Initialize Hydra client
	hydraClient := hydra.NewClient(cfg.Hydra.AdminURL)
	log.Printf("📡 Hydra Admin URL: %s", cfg.Hydra.AdminURL)

	// Query all active clients from database
	var clients []client.Client
	if err := db.Where("active = ?", true).Find(&clients).Error; err != nil {
		log.Fatalf("❌ Failed to query clients: %v", err)
	}

	log.Printf("📊 Found %d active clients in database", len(clients))

	successCount := 0
	skipCount := 0
	failCount := 0

	for i, c := range clients {
		log.Printf("\n[%d/%d] Processing client: %s (ID: %s)", i+1, len(clients), c.Name, c.ClientID)

		// Check if client already exists in Hydra
		existing, err := hydraClient.GetOAuth2Client(c.ClientID)
		if err == nil && existing != nil {
			log.Printf("  ⏭️  Client already exists in Hydra, skipping...")
			skipCount++
			continue
		}

		// Prepare OAuth2Client for Hydra
		oauth2Client := &hydra.OAuth2Client{
			ClientID:                c.ClientID,
			ClientSecret:            c.ClientSecret,
			ClientName:              c.Name,
			RedirectUris:            stringArrayToSlice(c.RedirectURIs),
			GrantTypes:              stringArrayToSlice(c.GrantTypes),
			ResponseTypes:           []string{"code"},
			Scope:                   strings.Join(c.Scopes, " "),
			TokenEndpointAuthMethod: "client_secret_post",
		}

		// Register client in Hydra
		_, err = hydraClient.CreateOAuth2Client(oauth2Client)
		if err != nil {
			log.Printf("  ❌ Failed to register in Hydra: %v", err)
			failCount++
			continue
		}

		log.Printf("  ✅ Successfully registered in Hydra")
		successCount++
	}

	// Print summary
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("📋 Migration Summary:")
	log.Printf("   Total clients:     %d", len(clients))
	log.Printf("   ✅ Migrated:        %d", successCount)
	log.Printf("   ⏭️  Already existed: %d", skipCount)
	log.Printf("   ❌ Failed:          %d", failCount)
	log.Println(strings.Repeat("=", 60))

	if failCount > 0 {
		log.Println("\n⚠️  Some clients failed to migrate. Please check the errors above.")
	} else {
		log.Println("\n🎉 All clients successfully processed!")
	}
}

// stringArrayToSlice converts pq.StringArray to []string
func stringArrayToSlice(arr pq.StringArray) []string {
	if arr == nil {
		return []string{}
	}
	result := make([]string, len(arr))
	copy(result, arr)
	return result
}
