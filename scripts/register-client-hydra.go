package main

import (
	"authway/src/server/internal/hydra"
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run register-client-hydra.go <admin_url> <client_id> <client_secret>")
		fmt.Println("Example: go run register-client-hydra.go http://localhost:4445 authway_yW9iQXu8Ho4ISf6plaHl_w secret123")
		os.Exit(1)
	}

	adminURL := os.Args[1]
	clientID := os.Args[2]
	clientSecret := os.Args[3]

	log.Printf("Registering client in Hydra...")
	log.Printf("Admin URL: %s", adminURL)
	log.Printf("Client ID: %s", clientID)

	hydraClient := hydra.NewClient(adminURL)

	oauth2Client := &hydra.OAuth2Client{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              "ASP.NET Sample",
		RedirectUris:            []string{"https://localhost:5001/signin-oidc", "http://localhost:5000/signin-oidc"},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		Scope:                   "openid profile email",
		TokenEndpointAuthMethod: "client_secret_post",
	}

	createdClient, err := hydraClient.CreateOAuth2Client(oauth2Client)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	log.Printf("✅ Client registered successfully!")
	log.Printf("Client ID: %s", createdClient.ClientID)
	log.Printf("Client Name: %s", createdClient.ClientName)
}
