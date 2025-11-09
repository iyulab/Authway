package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
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

	// Claims testing routes
	http.HandleFunc("/api/claims/update", handleUpdateClaims)
	http.HandleFunc("/api/claims", handleGetClaims)
	http.HandleFunc("/test-workspace", handleTestWorkspace)

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

	// Check if this is a re-authentication (user already has a session)
	sessionCookie, err := r.Cookie("session_id")
	isReauth := err == nil && sessions[sessionCookie.Value] != nil

	// Verify state exists in server memory (skip for re-authentication)
	createdAt, exists := oauthStates[state]
	if !exists && !isReauth {
		log.Printf("❌ State not found in server storage: %s", state)
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	if exists {
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
	} else {
		// Re-authentication flow
		log.Printf("🔄 Re-authentication detected - skipping state validation")
		log.Printf("   Existing session: %s", sessionCookie.Value)
	}

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

// handleUpdateClaims demonstrates Claims API usage
func handleUpdateClaims(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get session
	sessionID, err := r.Cookie("session_id")
	if err != nil || sessions[sessionID.Value] == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Not authenticated",
		})
		return
	}

	session := sessions[sessionID.Value]

	// Parse request body
	var reqBody struct {
		WorkspaceID string `json:"workspace_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Invalid request body",
		})
		return
	}

	// Call Authway Claims API
	claimsReq := map[string]interface{}{
		"claims": map[string]interface{}{
			"workspace_id": reqBody.WorkspaceID,
			"app":          "apple-service",
		},
		"permanent":    false,
		"client_id":    oauthConfig.ClientID,
		"redirect_uri": oauthConfig.RedirectURL,
	}

	reqJSON, _ := json.Marshal(claimsReq)
	req, err := http.NewRequest("POST", "http://localhost:8080/api/v1/claims/update",
		strings.NewReader(string(reqJSON)))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Failed to create request",
		})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Failed to call Claims API: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Forward response
	var claimsResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&claimsResp)

	w.WriteHeader(resp.StatusCode)
	json.NewEncoder(w).Encode(claimsResp)
}

// handleGetClaims retrieves current claims
func handleGetClaims(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get session
	sessionID, err := r.Cookie("session_id")
	if err != nil || sessions[sessionID.Value] == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Not authenticated",
		})
		return
	}

	session := sessions[sessionID.Value]

	// Call Authway Claims API
	req, err := http.NewRequest("GET", "http://localhost:8080/api/v1/claims", nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Failed to create request",
		})
		return
	}

	req.Header.Set("Authorization", "Bearer "+session.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Failed to call Claims API: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Forward response
	var claimsResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&claimsResp)

	w.WriteHeader(resp.StatusCode)
	json.NewEncoder(w).Encode(claimsResp)
}

// handleTestWorkspace shows a workspace selection test page
func handleTestWorkspace(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("session_id")
	if err != nil || sessions[sessionID.Value] == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	session := sessions[sessionID.Value]

	// Simple HTML page for testing workspace selection
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Workspace Test - Apple Service</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 { color: #FF6B6B; }
        .workspace-btn {
            display: block;
            width: 100%;
            padding: 15px;
            margin: 10px 0;
            background: #FF6B6B;
            color: white;
            border: none;
            border-radius: 4px;
            font-size: 16px;
            cursor: pointer;
        }
        .workspace-btn:hover {
            background: #ff5252;
        }
        #result {
            margin-top: 20px;
            padding: 15px;
            background: #f9f9f9;
            border-radius: 4px;
            display: none;
        }
        .claims-info {
            background: #e3f2fd;
            padding: 15px;
            border-radius: 4px;
            margin-top: 20px;
        }
        pre {
            background: #263238;
            color: #aed581;
            padding: 15px;
            border-radius: 4px;
            overflow-x: auto;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🍎 Workspace Selection Test</h1>
        <p><strong>User:</strong> ` + session.UserInfo.Email + `</p>

        <h2>Select a Workspace:</h2>
        <button class="workspace-btn" onclick="selectWorkspace('ws-apple-team')">
            🍎 Apple Team Workspace
        </button>
        <button class="workspace-btn" onclick="selectWorkspace('ws-development')">
            💻 Development Workspace
        </button>
        <button class="workspace-btn" onclick="selectWorkspace('ws-production')">
            🚀 Production Workspace
        </button>

        <div id="result"></div>

        <div class="claims-info">
            <h3>Current Claims:</h3>
            <button onclick="loadClaims()" style="padding: 10px 20px; cursor: pointer;">
                🔄 Load Current Claims
            </button>
            <pre id="claims-display">Click "Load Current Claims" to see...</pre>
        </div>

        <p style="margin-top: 20px;">
            <a href="/">← Back to Home</a>
        </p>
    </div>

    <script>
        async function selectWorkspace(workspaceId) {
            const resultDiv = document.getElementById('result');
            resultDiv.style.display = 'block';
            resultDiv.innerHTML = '⏳ Updating claims...';

            try {
                const response = await fetch('/api/claims/update', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ workspace_id: workspaceId })
                });

                const data = await response.json();

                if (response.ok && data.auth_url) {
                    resultDiv.innerHTML = '✅ Claims updated! Re-authenticating...<br><pre>' +
                        JSON.stringify(data, null, 2) + '</pre>';

                    // Auto re-authenticate with prompt=none
                    setTimeout(() => {
                        const iframe = document.createElement('iframe');
                        iframe.style.display = 'none';
                        iframe.src = data.auth_url;
                        iframe.onload = () => {
                            resultDiv.innerHTML += '<br>🎉 Re-authentication complete! Token refreshed with new claims.';
                            setTimeout(() => loadClaims(), 1000);
                        };
                        document.body.appendChild(iframe);
                    }, 1000);
                } else {
                    resultDiv.innerHTML = '❌ Error: <pre>' + JSON.stringify(data, null, 2) + '</pre>';
                }
            } catch (error) {
                resultDiv.innerHTML = '❌ Error: ' + error.message;
            }
        }

        async function loadClaims() {
            const display = document.getElementById('claims-display');
            display.textContent = 'Loading...';

            try {
                const response = await fetch('/api/claims');
                const data = await response.json();

                if (response.ok) {
                    display.textContent = JSON.stringify(data, null, 2);
                } else {
                    display.textContent = 'Error: ' + JSON.stringify(data, null, 2);
                }
            } catch (error) {
                display.textContent = 'Error: ' + error.message;
            }
        }

        // Auto-load claims on page load
        window.onload = () => loadClaims();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
