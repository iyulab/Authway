package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"authway-samples/shared"
)

const (
	serviceName = "Apple Service"
	servicePort = "9001"
	serviceColor = "#FF6B6B" // Red/Apple color
)

var (
	oauthConfig *shared.OAuthConfig
	sessions    = make(map[string]*shared.Session) // Simple in-memory session storage
	templates   *template.Template
	// OAuth state storage - maps state value to creation time
	oauthStates = make(map[string]time.Time)
)

func main() {
	// Initialize OAuth configuration (using Ory Hydra + Authway)
	oauthConfig = &shared.OAuthConfig{
		ClientID:     "apple-service-client",  // Fixed client ID
		ClientSecret: "apple-service-secret",   // Fixed client secret
		RedirectURL:  fmt.Sprintf("http://localhost:%s/callback", servicePort),
		AuthURL:      "http://localhost:4444/oauth2/auth", // Hydra OAuth endpoints
		TokenURL:     "http://localhost:4444/oauth2/token",
		UserInfoURL:  "http://localhost:4444/userinfo",
		Scopes:       []string{"openid", "profile", "email"},
	}

	// Load templates
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	// Routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/callback", handleCallback)
	http.HandleFunc("/profile", handleProfile)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/api/session", handleAPISession)

	// Start server
	addr := ":" + servicePort
	log.Printf("🍎 %s starting on http://localhost%s\n", serviceName, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("session_id")
	var session *shared.Session
	if err == nil {
		session = sessions[sessionID.Value]
	}

	data := map[string]interface{}{
		"ServiceName":  serviceName,
		"ServiceColor": serviceColor,
		"LoggedIn":     session != nil,
		"User":         session,
	}

	templates.ExecuteTemplate(w, "index.html", data)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate state for CSRF protection
	state, err := shared.GenerateState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	log.Printf("🔑 Generated OAuth state: %s (len=%d)", state, len(state))

	// Store state in server memory (not cookie, to avoid SameSite issues in local development)
	// In production, use Redis or similar distributed storage
	oauthStates[state] = time.Now()

	// Clean up expired states (older than 5 minutes)
	cleanExpiredStates()

	// Redirect to authorization URL
	authURL := oauthConfig.GetAuthURL(state)
	log.Printf("🔗 Redirecting to: %s", authURL)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// cleanExpiredStates removes OAuth states older than 5 minutes
func cleanExpiredStates() {
	now := time.Now()
	for state, createdAt := range oauthStates {
		if now.Sub(createdAt) > 5*time.Minute {
			delete(oauthStates, state)
		}
	}
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	// Get state from URL
	state := r.URL.Query().Get("state")
	if state == "" {
		log.Printf("❌ State parameter missing")
		http.Error(w, "State parameter missing", http.StatusBadRequest)
		return
	}

	// Verify state exists in server memory
	createdAt, exists := oauthStates[state]
	if !exists {
		log.Printf("❌ State not found in server storage: %s", state)
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Check if state has expired (5 minutes)
	if time.Since(createdAt) > 5*time.Minute {
		log.Printf("❌ State expired: %s (age: %v)", state, time.Since(createdAt))
		delete(oauthStates, state)
		http.Error(w, "State parameter expired", http.StatusBadRequest)
		return
	}

	// Debug logging
	log.Printf("🔍 OAuth Callback State Validation:")
	log.Printf("   URL state:     %s (len=%d)", state, len(state))
	log.Printf("   Server state:  exists=%v, age=%v", exists, time.Since(createdAt))
	log.Printf("   ✅ State validated successfully")

	// Remove used state (one-time use)
	delete(oauthStates, state)

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No authorization code", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens
	ctx := context.Background()
	token, err := oauthConfig.ExchangeCode(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to exchange code: %v", err), http.StatusInternalServerError)
		return
	}

	// Get user info
	userInfo, err := oauthConfig.GetUserInfo(ctx, token.AccessToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user info: %v", err), http.StatusInternalServerError)
		return
	}

	// Create session
	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	sessions[sessionID] = &shared.Session{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
		UserInfo:     *userInfo,
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   3600, // 1 hour
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to home
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("session_id")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	session := sessions[sessionID.Value]
	if session == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := map[string]interface{}{
		"ServiceName":  serviceName,
		"ServiceColor": serviceColor,
		"User":         session,
	}

	templates.ExecuteTemplate(w, "profile.html", data)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("session_id")
	if err == nil {
		delete(sessions, sessionID.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func handleAPISession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sessionID, err := r.Cookie("session_id")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logged_in": false,
		})
		return
	}

	session := sessions[sessionID.Value]
	if session == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logged_in": false,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"logged_in": true,
		"user":      session.UserInfo,
		"expires_at": session.ExpiresAt,
	})
}
