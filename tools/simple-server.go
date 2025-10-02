package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// CORS 미들웨어
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// 응답 구조체
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

func main() {
	// Health check endpoint
	http.HandleFunc("/health", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		response := Response{
			Status:  "success",
			Message: "Authway API is running",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))

	// Login endpoint
	http.HandleFunc("/api/auth/login", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var loginReq LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			response := Response{
				Status:  "error",
				Message: "Invalid JSON format",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Mock authentication (demo purposes)
		if loginReq.Email == "admin@authway.com" && loginReq.Password == "password123" {
			loginResp := LoginResponse{
				Token: "mock-jwt-token-" + fmt.Sprintf("%d", time.Now().Unix()),
				User: User{
					ID:     1,
					Email:  loginReq.Email,
					Name:   "Admin User",
					Role:   "admin",
					Active: true,
				},
			}

			response := Response{
				Status:  "success",
				Message: "Login successful",
				Data:    loginResp,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			response := Response{
				Status:  "error",
				Message: "Invalid email or password",
			}
			json.NewEncoder(w).Encode(response)
		}
	}))

	// Users endpoint (for admin dashboard)
	http.HandleFunc("/api/users", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		// Mock user data
		users := []User{
			{ID: 1, Email: "admin@authway.com", Name: "Admin User", Role: "admin", Active: true},
			{ID: 2, Email: "user1@example.com", Name: "John Doe", Role: "user", Active: true},
			{ID: 3, Email: "user2@example.com", Name: "Jane Smith", Role: "user", Active: false},
		}

		response := Response{
			Status:  "success",
			Message: "Users retrieved successfully",
			Data:    users,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))

	// Clients endpoint (OAuth clients)
	http.HandleFunc("/api/clients", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		// Mock client data
		clients := []map[string]interface{}{
			{
				"id":           1,
				"client_id":    "example-app",
				"name":         "Example Application",
				"description":  "Demo OAuth client",
				"redirect_uris": []string{"http://localhost:3000/callback"},
				"active":       true,
			},
		}

		response := Response{
			Status:  "success",
			Message: "OAuth clients retrieved successfully",
			Data:    clients,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))

	// Consent endpoint
	http.HandleFunc("/api/consent", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Mock consent request
			consentData := map[string]interface{}{
				"client": map[string]string{
					"client_id": "example-app",
					"name":      "Example Application",
					"logo_uri":  "",
				},
				"requested_scope": []string{"openid", "profile", "email"},
				"user": map[string]string{
					"email": "user@example.com",
					"name":  "Test User",
				},
			}

			response := Response{
				Status:  "success",
				Message: "Consent data retrieved",
				Data:    consentData,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else if r.Method == http.MethodPost {
			// Handle consent submission
			response := Response{
				Status:  "success",
				Message: "Consent processed successfully",
				Data: map[string]string{
					"redirect_to": "http://localhost:3000/callback?code=mock-auth-code",
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))

	// Stats endpoint (for dashboard)
	http.HandleFunc("/api/stats", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		stats := map[string]interface{}{
			"total_users":    150,
			"active_users":   142,
			"total_clients":  12,
			"active_clients": 10,
			"daily_logins":   45,
			"monthly_growth": 12.5,
		}

		response := Response{
			Status:  "success",
			Message: "Statistics retrieved successfully",
			Data:    stats,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))

	fmt.Println("🚀 Simple Authway API Server starting on port 8080")
	fmt.Println("📊 Health check: http://localhost:8080/health")
	fmt.Println("🔐 Login endpoint: http://localhost:8080/api/auth/login")
	fmt.Println("👥 Users endpoint: http://localhost:8080/api/users")
	fmt.Println("📱 Clients endpoint: http://localhost:8080/api/clients")
	fmt.Println("")
	fmt.Println("Demo credentials:")
	fmt.Println("  Email: admin@authway.com")
	fmt.Println("  Password: password123")
	fmt.Println("")

	log.Fatal(http.ListenAndServe(":8080", nil))
}