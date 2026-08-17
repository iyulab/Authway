package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"authway/apps/branding/auth-api/internal/config"
	"go.uber.org/zap"
)

func newTestGoogleService(tokenURL, userInfoURL string) *GoogleService {
	cfg := &config.GoogleOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost/callback",
	}
	return NewGoogleServiceForTesting(cfg, zap.NewNop(), "https://accounts.example.com/auth", tokenURL, userInfoURL)
}

func TestGetAuthURL(t *testing.T) {
	svc := newTestGoogleService("", "")
	got := svc.GetAuthURL("state-abc")

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("GetAuthURL() returned an unparseable URL: %v", err)
	}
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != "https://accounts.example.com/auth" {
		t.Errorf("GetAuthURL() base = %q, want the configured auth endpoint", got)
	}
	q := parsed.Query()
	if q.Get("state") != "state-abc" {
		t.Errorf("state param = %q, want state-abc", q.Get("state"))
	}
	if q.Get("client_id") != "test-client-id" {
		t.Errorf("client_id param = %q, want test-client-id", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "http://localhost/callback" {
		t.Errorf("redirect_uri param = %q, want http://localhost/callback", q.Get("redirect_uri"))
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type param = %q, want code", q.Get("response_type"))
	}
}

func TestExchangeCode_Success(t *testing.T) {
	var gotMethod, gotContentType string
	var gotForm url.Values

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"tok123","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	svc := newTestGoogleService(tokenSrv.URL, "")
	resp, err := svc.ExchangeCode(context.Background(), "auth-code-1")
	if err != nil {
		t.Fatalf("ExchangeCode() error: %v", err)
	}
	if resp.AccessToken != "tok123" {
		t.Errorf("AccessToken = %q, want tok123", resp.AccessToken)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("request method = %q, want POST", gotMethod)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotContentType)
	}
	if gotForm.Get("code") != "auth-code-1" {
		t.Errorf("form code = %q, want auth-code-1", gotForm.Get("code"))
	}
	if gotForm.Get("grant_type") != "authorization_code" {
		t.Errorf("form grant_type = %q, want authorization_code", gotForm.Get("grant_type"))
	}
	if gotForm.Get("client_id") != "test-client-id" {
		t.Errorf("form client_id = %q, want test-client-id", gotForm.Get("client_id"))
	}
}

func TestExchangeCode_ErrorStatusIsReported(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenSrv.Close()

	svc := newTestGoogleService(tokenSrv.URL, "")
	if _, err := svc.ExchangeCode(context.Background(), "bad-code"); err == nil {
		t.Error("ExchangeCode() expected an error for a non-200 response, got nil")
	} else if !strings.Contains(err.Error(), "400") {
		t.Errorf("ExchangeCode() error = %q, want it to mention the status code", err)
	}
}

func TestGetUserInfo_Success(t *testing.T) {
	var gotAuthHeader string

	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"g1","email":"user@example.com","name":"Test User"}`))
	}))
	defer userSrv.Close()

	svc := newTestGoogleService("", userSrv.URL)
	info, err := svc.GetUserInfo(context.Background(), "tok123")
	if err != nil {
		t.Fatalf("GetUserInfo() error: %v", err)
	}
	if info.Email != "user@example.com" || info.ID != "g1" || info.Name != "Test User" {
		t.Errorf("GetUserInfo() = %+v, want id=g1 email=user@example.com name=\"Test User\"", info)
	}
	if gotAuthHeader != "Bearer tok123" {
		t.Errorf("Authorization header = %q, want %q", gotAuthHeader, "Bearer tok123")
	}
}

func TestGetUserInfo_ErrorStatusIsReported(t *testing.T) {
	userSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer userSrv.Close()

	svc := newTestGoogleService("", userSrv.URL)
	if _, err := svc.GetUserInfo(context.Background(), "bad-token"); err == nil {
		t.Error("GetUserInfo() expected an error for a non-200 response, got nil")
	} else if !strings.Contains(err.Error(), "401") {
		t.Errorf("GetUserInfo() error = %q, want it to mention the status code", err)
	}
}
