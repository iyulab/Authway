package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// Simple test server to validate Hydra integration
func main() {
	// Login handler
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// Display login form
			challenge := r.URL.Query().Get("login_challenge")
			if challenge == "" {
				http.Error(w, "Missing login_challenge", http.StatusBadRequest)
				return
			}

			html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Login - Authway</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; max-width: 400px; margin: 100px auto; padding: 20px; }
        input { width: 100%%; padding: 10px; margin: 10px 0; box-sizing: border-box; }
        button { background: #007bff; color: white; padding: 10px 20px; border: none; cursor: pointer; }
    </style>
</head>
<body>
    <h2>Authway Login</h2>
    <form method="post">
        <input type="hidden" name="challenge" value="%s">
        <input type="email" name="email" placeholder="Email" required>
        <input type="password" name="password" placeholder="Password" required>
        <label><input type="checkbox" name="remember"> Remember me</label><br><br>
        <button type="submit">Login</button>
    </form>
    <p><small>Test credentials: admin@test.com / password</small></p>
</body>
</html>`, challenge)

			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(html))
			return
		}

		if r.Method == "POST" {
			// Process login
			challenge := r.FormValue("challenge")
			email := r.FormValue("email")
			password := r.FormValue("password")
			remember := r.FormValue("remember") == "on"

			log.Printf("Login attempt: %s", email)

			// Simple credential check
			if email == "admin@test.com" && password == "password" {
				// Accept login request
				acceptData := map[string]interface{}{
					"subject":     email,
					"remember":    remember,
					"remember_for": 3600,
				}

				jsonData, _ := json.Marshal(acceptData)

				req, _ := http.NewRequest("PUT",
					fmt.Sprintf("http://localhost:4445/admin/oauth2/auth/requests/login/accept?login_challenge=%s", challenge),
					bytes.NewReader(jsonData))
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("Error accepting login: %v", err)
					http.Error(w, "Login failed", http.StatusInternalServerError)
					return
				}
				defer resp.Body.Close()

				var result map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&result)

				if redirectTo, ok := result["redirect_to"].(string); ok {
					http.Redirect(w, r, redirectTo, http.StatusFound)
					return
				}
			}

			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		}
	})

	// Consent handler
	http.HandleFunc("/consent", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			challenge := r.URL.Query().Get("consent_challenge")
			if challenge == "" {
				http.Error(w, "Missing consent_challenge", http.StatusBadRequest)
				return
			}

			// Get consent request info
			resp, err := http.Get(fmt.Sprintf("http://localhost:4445/admin/oauth2/auth/requests/consent?consent_challenge=%s", challenge))
			if err != nil {
				log.Printf("Error getting consent info: %v", err)
				http.Error(w, "Error getting consent info", http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			var consentReq map[string]interface{}
			json.Unmarshal(body, &consentReq)

			client := consentReq["client"].(map[string]interface{})
			requestedScopes := consentReq["requested_scope"].([]interface{})

			html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Consent - Authway</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 100px auto; padding: 20px; }
        .scope { margin: 10px 0; }
        button { padding: 10px 20px; margin: 10px 5px; cursor: pointer; }
        .accept { background: #28a745; color: white; border: none; }
        .deny { background: #dc3545; color: white; border: none; }
    </style>
</head>
<body>
    <h2>Grant Access</h2>
    <p><strong>%s</strong> is requesting access to:</p>
    <form method="post">
        <input type="hidden" name="challenge" value="%s">`,
				client["client_name"], challenge)

			for _, scope := range requestedScopes {
				scopeStr := scope.(string)
				html += fmt.Sprintf(`
        <div class="scope">
            <label><input type="checkbox" name="scopes" value="%s" checked> %s</label>
        </div>`, scopeStr, scopeStr)
			}

			html += `
        <br>
        <label><input type="checkbox" name="remember"> Remember this choice</label><br><br>
        <button type="submit" name="action" value="accept" class="accept">Allow</button>
        <button type="submit" name="action" value="deny" class="deny">Deny</button>
    </form>
</body>
</html>`

			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(html))
			return
		}

		if r.Method == "POST" {
			challenge := r.FormValue("challenge")
			action := r.FormValue("action")

			if action == "accept" {
				scopes := r.Form["scopes"]
				remember := r.FormValue("remember") == "on"

				acceptData := map[string]interface{}{
					"grant_scope":   scopes,
					"remember":      remember,
					"remember_for":  3600,
				}

				jsonData, _ := json.Marshal(acceptData)

				req, _ := http.NewRequest("PUT",
					fmt.Sprintf("http://localhost:4445/admin/oauth2/auth/requests/consent/accept?consent_challenge=%s", challenge),
					bytes.NewReader(jsonData))
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("Error accepting consent: %v", err)
					http.Error(w, "Consent failed", http.StatusInternalServerError)
					return
				}
				defer resp.Body.Close()

				var result map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&result)

				if redirectTo, ok := result["redirect_to"].(string); ok {
					http.Redirect(w, r, redirectTo, http.StatusFound)
					return
				}
			} else {
				// Deny consent
				req, _ := http.NewRequest("PUT",
					fmt.Sprintf("http://localhost:4445/admin/oauth2/auth/requests/consent/reject?consent_challenge=%s", challenge),
					strings.NewReader(`{"error":"access_denied","error_description":"User denied access"}`))
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("Error rejecting consent: %v", err)
					http.Error(w, "Error processing request", http.StatusInternalServerError)
					return
				}
				defer resp.Body.Close()

				var result map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&result)

				if redirectTo, ok := result["redirect_to"].(string); ok {
					http.Redirect(w, r, redirectTo, http.StatusFound)
					return
				}
			}
		}
	})

	// Simple callback handler for testing
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head><title>OAuth Callback</title></head>
<body>
    <h2>OAuth Authorization Complete</h2>
    <p><strong>Authorization Code:</strong> %s</p>
    <p><strong>State:</strong> %s</p>
    <p>✅ OAuth2 flow completed successfully!</p>
</body>
</html>`, code, state)

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})

	// Health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Println("Test server starting on :8080")
	log.Println("Hydra should redirect to:")
	log.Println("  Login: http://localhost:8080/login")
	log.Println("  Consent: http://localhost:8080/consent")
	log.Println("  Callback: http://localhost:8080/callback")
	log.Fatal(http.ListenAndServe(":8080", nil))
}