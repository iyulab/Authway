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
	serviceName = "Banana Service"
	servicePort = "9002"
	serviceColor = "#FFD93D" // Yellow/Banana color
)

var (
	oauthConfig *shared.OAuthConfig
	sessions    = make(map[string]*shared.Session) // Simple in-memory session storage
	templates   *template.Template
)

func main() {
	// Initialize OAuth configuration (using Ory Hydra + Authway)
	oauthConfig = &shared.OAuthConfig{
		ClientID:     "banana-service-client",  // Fixed client ID
		ClientSecret: "banana-service-secret",   // Fixed client secret
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
	log.Printf("🍌 %s starting on http://localhost%s\n", serviceName, addr)
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

	// Store state in cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to authorization URL
	authURL := oauthConfig.GetAuthURL(state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Error(w, "State cookie not found", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state != stateCookie.Value {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

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
