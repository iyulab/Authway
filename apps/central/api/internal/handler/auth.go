package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/mfa"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userService   user.Service
	clientService client.Service
	claimsService claims.Service
	mfaService    mfa.Service
	hydraClient   *hydra.Client
	logger        *zap.Logger
	auditService  audit.Service
	mfaStore      *MFAChallengeStore
}

func NewAuthHandler(userService user.Service, clientService client.Service, claimsService claims.Service, mfaService mfa.Service, hydraClient *hydra.Client, logger *zap.Logger, auditService audit.Service, redisClient *redis.Client) *AuthHandler {
	return &AuthHandler{
		userService:   userService,
		clientService: clientService,
		claimsService: claimsService,
		mfaService:    mfaService,
		hydraClient:   hydraClient,
		logger:        logger,
		auditService:  auditService,
		mfaStore:      NewMFAChallengeStore(redisClient),
	}
}

// logUserAudit emits a success-path audit entry with the resolved user as actor.
func (h *AuthHandler) logUserAudit(c *fiber.Ctx, u *user.User, action audit.AuditAction, extra map[string]any) {
	if h.auditService == nil || u == nil {
		return
	}
	entry := audit.EntryFromFiber(c, u.TenantID, action, "user", u.ID.String())
	entry.ActorID = &u.ID
	entry.ActorEmail = u.Email
	entry.ActorType = "user"
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// logAuthFailure emits a sync audit entry for login failures. Sync so buffer
// overflow cannot swallow security events.
func (h *AuthHandler) logAuthFailure(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, attemptedEmail, reason string, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	details := map[string]any{
		"reason": reason,
	}
	if attemptedEmail != "" {
		details["attempted_email"] = attemptedEmail
	}
	for k, v := range extra {
		details[k] = v
	}
	entry := &audit.AuditEntry{
		TenantID:     tenantID,
		ActorType:    "anonymous",
		Action:       action,
		Severity:     audit.SeverityWarning,
		ResourceType: "user",
		IPAddress:    c.IP(),
		UserAgent:    c.Get("User-Agent"),
		Details:      details,
		Success:      false,
		ErrorMsg:     reason,
	}
	if err := h.auditService.Log(context.Background(), entry); err != nil {
		h.logger.Warn("Failed to record auth-failure audit", zap.Error(err), zap.String("action", string(action)))
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LoginPageRequest for POST request body
type LoginPageRequest struct {
	LoginChallenge string `json:"login_challenge"`
}

// Login flow handler - supports both GET and POST
func (h *AuthHandler) LoginPage(c *fiber.Ctx) error {
	// Try to get challenge from query parameter first (GET)
	challenge := c.Query("login_challenge")

	// If not in query, try POST body
	if challenge == "" && c.Method() == "POST" {
		var req LoginPageRequest
		if err := c.BodyParser(&req); err == nil {
			challenge = req.LoginChallenge
		}
	}

	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "login_challenge parameter is required",
			"hint":  "The login_challenge parameter must be included in the URL query string or POST body. This parameter is provided by Ory Hydra in the OAuth 2.0 authorization flow.",
			"docs":  "https://www.ory.sh/docs/hydra/guides/login",
		})
	}

	// Get login request from Hydra
	h.logger.Info("Getting login request from Hydra", zap.String("challenge", challenge))
	loginReq, err := h.hydraClient.GetLoginRequest(challenge)
	if err != nil {
		h.logger.Error("Failed to get login request from Hydra",
			zap.String("challenge", challenge),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get login request from Hydra",
			"details": apierror.Message(err, "unable to reach Hydra"),
			"hint":    "Verify that Ory Hydra is running and accessible. Check the HYDRA_ADMIN_URL environment variable.",
			"debug": fiber.Map{
				"hydra_admin_url": h.hydraClient.AdminURL,
				"challenge":       challenge[:min(50, len(challenge))] + "...",
			},
		})
	}

	// Get client information to check tenant
	h.logger.Info("Looking for client", zap.String("client_id", loginReq.Client.ClientID))
	requestedClient, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.logger.Error("Failed to get client information",
			zap.String("client_id", loginReq.Client.ClientID),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "OAuth client not registered in Authway",
			"details": apierror.Message(err, "client lookup failed"),
			"hint":    "Register this OAuth client in Authway Admin Console before using it. Each client must be associated with a tenant.",
			"solution": fiber.Map{
				"step_1": "Go to Admin Console: http://localhost:3000",
				"step_2": "Navigate to Clients section",
				"step_3": "Register client with client_id: " + loginReq.Client.ClientID,
			},
			"client_id": loginReq.Client.ClientID,
		})
	}

	// SSO Check: If user is already authenticated, verify tenant match
	if loginReq.Skip && loginReq.Subject != "" {
		userID, err := uuid.Parse(loginReq.Subject)
		if err != nil {
			// Invalid user ID format - revoke sessions and force fresh login
			h.logger.Warn("Invalid user ID in skip request, revoking sessions",
				zap.String("subject", loginReq.Subject),
				zap.Error(err))
			// Revoke all sessions for this subject
			if revokeErr := h.hydraClient.RevokeUserSessions(loginReq.Subject); revokeErr != nil {
				h.logger.Error("Failed to revoke user sessions", zap.Error(revokeErr))
			}
			// Reject with login_required to show login form without propagating error to OAuth client
			resp, rejectErr := h.hydraClient.RejectLoginRequest(challenge, "login_required", "Please login again")
			if rejectErr != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to reject login request",
				})
			}
			// Return JSON response with redirect_to for frontend to handle
			return c.JSON(fiber.Map{
				"redirect_to":     resp.RedirectTo,
				"session_cleared": true,
			})
		}

		// Get user to check tenant
		authenticatedUser, err := h.userService.GetByID(userID)
		if err != nil {
			// User not found - revoke sessions and force fresh login
			h.logger.Warn("User not found in skip request, revoking sessions",
				zap.String("user_id", userID.String()),
				zap.Error(err))
			// Revoke all sessions for this subject
			if revokeErr := h.hydraClient.RevokeUserSessions(userID.String()); revokeErr != nil {
				h.logger.Error("Failed to revoke user sessions", zap.Error(revokeErr))
			}
			// Reject with login_required to show login form without propagating error to OAuth client
			resp, rejectErr := h.hydraClient.RejectLoginRequest(challenge, "login_required", "Please login again")
			if rejectErr != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to reject login request",
				})
			}
			// Return JSON response with redirect_to for frontend to handle
			return c.JSON(fiber.Map{
				"redirect_to":     resp.RedirectTo,
				"session_cleared": true,
			})
		}

		// Compare tenant_id for SSO eligibility
		if authenticatedUser.TenantID == requestedClient.TenantID {
			// Same tenant → SSO automatic approval
			h.logger.Info("SSO approved - same tenant",
				zap.String("user_id", authenticatedUser.ID.String()),
				zap.String("tenant_id", authenticatedUser.TenantID.String()))

			// Get and store claims for SSO login
			userClaims, err := h.claimsService.GetClaimsForLogin(c.Context(), authenticatedUser.ID, authenticatedUser.TenantID, challenge)
			if err != nil {
				h.logger.Warn("Failed to get claims for SSO login",
					zap.String("user_id", authenticatedUser.ID.String()),
					zap.Error(err))
				// Continue without claims
				userClaims = nil
			}

			acceptBody := &hydra.AcceptLoginRequest{
				Subject:     loginReq.Subject,
				Remember:    true,
				RememberFor: 3600,
				Context: map[string]any{
					"email":     authenticatedUser.Email,
					"name":      authenticatedUser.Name,
					"tenant_id": authenticatedUser.TenantID.String(),
					"sso":       true, // Mark this as SSO auto-login for consent detection
				},
			}

			h.logger.Info("SSO login with claims",
				zap.String("user_id", authenticatedUser.ID.String()),
				zap.Int("claims_count", len(userClaims)))

			resp, err := h.hydraClient.AcceptLoginRequest(challenge, acceptBody)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to accept login request",
				})
			}

			// Return JSON response for SSO auto-login
			return c.JSON(fiber.Map{
				"redirect_to": resp.RedirectTo,
				"sso":         true,
			})
		}
		// Different tenant → Force re-authentication by showing login form
		h.logger.Info("Different tenant - forcing re-authentication",
			zap.String("user_tenant_id", authenticatedUser.TenantID.String()),
			zap.String("client_tenant_id", requestedClient.TenantID.String()))
	}

	// Render login form with challenge
	return c.JSON(fiber.Map{
		"challenge":       challenge,
		"client_name":     loginReq.Client.ClientName,
		"requested_scope": loginReq.RequestedScope,
		"tenant_id":       requestedClient.TenantID.String(),
		"client": fiber.Map{
			"client_id": loginReq.Client.ClientID,
		},
	})
}

type LoginRequest struct {
	Challenge string `json:"challenge"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Remember  bool   `json:"remember"`
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get login request from Hydra
	loginReq, err := h.hydraClient.GetLoginRequest(req.Challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get login request",
		})
	}

	// Resolve the requesting client's tenant first — GetByEmail (deprecated)
	// matches globally in undefined order, but the schema allows the same
	// email to exist in more than one tenant (idx_users_tenant_email). The
	// sibling LoginPage handler already does this same lookup for its SSO
	// tenant check; Login needs it too since it is the one that actually
	// verifies the password.
	requestedClient, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.logger.Error("Failed to resolve OAuth client for login",
			zap.String("client_id", loginReq.Client.ClientID), zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": "Failed to resolve OAuth client"})
	}

	// Authenticate user
	user, err := h.userService.GetByEmailAndTenant(requestedClient.TenantID, req.Email)
	if err != nil {
		h.logAuthFailure(c, uuid.Nil, audit.ActionUserLoginFailed, req.Email, "user_not_found", nil)
		middleware.IncrementRateLimitOnFailure(c)
		// Reject login request
		resp, rejectErr := h.hydraClient.RejectLoginRequest(req.Challenge, "invalid_credentials", "Invalid email or password")
		if rejectErr != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to reject login request"})
		}
		return c.JSON(fiber.Map{
			"error":       "Invalid email or password",
			"redirect_to": resp.RedirectTo,
		})
	}

	// Verify password
	if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		h.logAuthFailure(c, user.TenantID, audit.ActionUserLoginFailed, req.Email, "invalid_password", map[string]any{
			"user_id": user.ID.String(),
		})
		middleware.IncrementRateLimitOnFailure(c)
		// Reject login request
		resp, rejectErr := h.hydraClient.RejectLoginRequest(req.Challenge, "invalid_credentials", "Invalid email or password")
		if rejectErr != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to reject login request"})
		}
		return c.JSON(fiber.Map{
			"error":       "Invalid email or password",
			"redirect_to": resp.RedirectTo,
		})
	}

	rememberFor := 0
	if req.Remember {
		rememberFor = 3600 // 1 hour
	}

	// A password alone is not enough for a TOTP-enabled user — park the
	// verified-but-not-yet-accepted login and hand the client a fresh
	// mfa_challenge instead of touching Hydra. Verify()/VerifyRecoveryCode()
	// complete the accept once the second factor checks out.
	if user.TOTPEnabled {
		challenge, err := tokenhash.Generate()
		if err != nil {
			h.logger.Error("Failed to generate mfa_challenge", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to start MFA challenge"})
		}
		h.mfaStore.Set(challenge, &pendingMFALogin{
			HydraChallenge: req.Challenge,
			UserID:         user.ID,
			Remember:       req.Remember,
			RememberFor:    rememberFor,
			CreatedAt:      time.Now(),
		})
		h.logger.Info("Password verified, MFA required", zap.String("user_id", user.ID.String()))
		return c.JSON(fiber.Map{
			"mfa_required":  true,
			"mfa_challenge": challenge,
		})
	}

	return h.completeLogin(c, req.Challenge, user, req.Remember, rememberFor, "password")
}

// completeLogin accepts the Hydra login request for an already-authenticated
// user (password alone, or password + a verified second factor) and records
// the login audit entry. Shared by Login and the two MFA-verify handlers so
// none of them duplicate the accept/audit/response tail.
func (h *AuthHandler) completeLogin(c *fiber.Ctx, challenge string, u *user.User, remember bool, rememberFor int, method string) error {
	userClaims, err := h.claimsService.GetClaimsForLogin(c.Context(), u.ID, u.TenantID, challenge)
	if err != nil {
		h.logger.Warn("Failed to get claims for login",
			zap.String("user_id", u.ID.String()),
			zap.Error(err))
		// Continue without claims
		userClaims = nil
	}

	acceptBody := &hydra.AcceptLoginRequest{
		Subject:     u.ID.String(),
		Remember:    remember,
		RememberFor: rememberFor,
		Context: map[string]any{
			"email":     u.Email,
			"name":      u.Name,
			"tenant_id": u.TenantID.String(),
		},
	}

	h.logger.Info("Accepting login request",
		zap.String("user_id", u.ID.String()),
		zap.Int("claims_count", len(userClaims)))

	resp, err := h.hydraClient.AcceptLoginRequest(challenge, acceptBody)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to accept login request",
		})
	}

	middleware.ResetRateLimitOnSuccess(c)
	h.logUserAudit(c, u, audit.ActionUserLogin, map[string]any{
		"challenge": challenge,
		"remember":  remember,
		"method":    method,
	})

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

// VerifyMFALoginRequest is the body for the two login-time MFA endpoints
// below. Code holds either a 6-digit TOTP code or a recovery code depending
// on which endpoint is called.
type VerifyMFALoginRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

// VerifyMFALogin completes a login that Login() parked pending TOTP.
// POST /mfa/verify — unauthenticated: the mfa_challenge itself is the bearer
// credential for this one-shot exchange, same trust model as a Hydra
// login_challenge.
func (h *AuthHandler) VerifyMFALogin(c *fiber.Ctx) error {
	var req VerifyMFALoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	pending, ok := h.mfaStore.Get(req.Challenge)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "invalid or expired mfa challenge"})
	}

	u, err := h.userService.GetByID(pending.UserID)
	if err != nil || u == nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid or expired mfa challenge"})
	}

	valid, err := h.mfaService.Verify(pending.UserID, req.Code)
	if err != nil || !valid {
		h.logMFALoginFailure(c, u, "totp")
		middleware.IncrementRateLimitOnFailure(c)
		if locked := h.mfaStore.RecordFailure(req.Challenge); locked {
			return c.Status(401).JSON(fiber.Map{"error": "too many failed attempts — please sign in again"})
		}
		return c.Status(401).JSON(fiber.Map{"error": "invalid verification code"})
	}

	h.mfaStore.Delete(req.Challenge)
	return h.completeLogin(c, pending.HydraChallenge, u, pending.Remember, pending.RememberFor, "password+totp")
}

// VerifyMFARecoveryLogin is VerifyMFALogin's recovery-code counterpart.
// POST /mfa/recovery
func (h *AuthHandler) VerifyMFARecoveryLogin(c *fiber.Ctx) error {
	var req VerifyMFALoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	pending, ok := h.mfaStore.Get(req.Challenge)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "invalid or expired mfa challenge"})
	}

	u, err := h.userService.GetByID(pending.UserID)
	if err != nil || u == nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid or expired mfa challenge"})
	}

	valid, err := h.mfaService.VerifyRecoveryCode(pending.UserID, req.Code)
	if err != nil || !valid {
		h.logMFALoginFailure(c, u, "recovery_code")
		middleware.IncrementRateLimitOnFailure(c)
		if locked := h.mfaStore.RecordFailure(req.Challenge); locked {
			return c.Status(401).JSON(fiber.Map{"error": "too many failed attempts — please sign in again"})
		}
		return c.Status(401).JSON(fiber.Map{"error": "invalid recovery code"})
	}

	h.mfaStore.Delete(req.Challenge)
	return h.completeLogin(c, pending.HydraChallenge, u, pending.Remember, pending.RememberFor, "password+recovery_code")
}

// logMFALoginFailure emits a sync audit entry for a failed login-time MFA
// attempt — mirrors MFAHandler.logMFAFailure (internal/handler/mfa.go) for
// the self-service MFA API, kept separate because AuthHandler has no
// tenantForUser lookup helper and already has the user record in hand here.
func (h *AuthHandler) logMFALoginFailure(c *fiber.Ctx, u *user.User, phase string) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, u.TenantID, audit.ActionUserMFAFailed, "user_mfa", u.ID.String())
	entry.Severity = audit.SeverityWarning
	entry.Success = false
	entry.ErrorMsg = "invalid code"
	entry.Details["phase"] = phase
	if err := h.auditService.Log(c.UserContext(), entry); err != nil {
		h.logger.Error("Failed to write MFA login-failure audit log", zap.Error(err), zap.String("user_id", u.ID.String()))
	}
}

// ConsentPageRequest for POST request body
type ConsentPageRequest struct {
	ConsentChallenge string `json:"consent_challenge" form:"consent_challenge"`
}

// Consent flow handler - supports both GET and POST
func (h *AuthHandler) ConsentPage(c *fiber.Ctx) error {
	// Try to get challenge from query parameter first (GET)
	challenge := c.Query("consent_challenge")

	// If not in query, try POST body (supports both JSON and form-urlencoded)
	if challenge == "" && c.Method() == "POST" {
		var req ConsentPageRequest
		// BodyParser supports both JSON and form-urlencoded automatically
		if err := c.BodyParser(&req); err == nil {
			challenge = req.ConsentChallenge
		}
	}

	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "consent_challenge parameter is required",
			"hint":  "The consent_challenge parameter must be included in the URL query string or POST body. This parameter is provided by Ory Hydra after successful login.",
			"docs":  "https://www.ory.sh/docs/hydra/guides/consent",
		})
	}

	// Get consent request from Hydra
	consentReq, err := h.hydraClient.GetConsentRequest(challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get consent request from Hydra",
			"details": apierror.Message(err, "unable to reach Hydra"),
			"hint":    "Verify that Ory Hydra is running and the consent_challenge is valid. The challenge may have expired or been used already.",
			"debug": fiber.Map{
				"hydra_admin_url": h.hydraClient.AdminURL,
				"challenge":       challenge[:min(50, len(challenge))] + "...",
			},
		})
	}

	// Get user information first
	userID, err := uuid.Parse(consentReq.Subject)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Invalid user ID"})
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "User not found"})
	}

	// Get claims for this consent flow
	loginChallenge := consentReq.LoginChallenge
	userName := ""
	if user.Name != nil {
		userName = *user.Name
	}
	userInfo := &claims.UserInfo{
		Email:    user.Email,
		Name:     userName,
		TenantID: user.TenantID,
	}
	userClaims, err := h.claimsService.GetClaimsForConsent(c.Context(), loginChallenge, userID, user.TenantID, userInfo)
	if err != nil {
		h.logger.Warn("Failed to get claims for consent", zap.Error(err))
		userClaims = make(claims.ClaimMap)
	}

	// Check source of claims to determine if this is a claims update scenario
	claimsSource, _ := userClaims["_source"].(string)
	isClaimsUpdate := claimsSource == "pending" || claimsSource == "login_challenge"

	// Remove internal _source field before sending to Hydra
	delete(userClaims, "_source")

	// Check if this is a silent (prompt=none) authentication
	// Hydra may not set Skip=true for prompt=none, but we should still auto-accept
	isSilentAuth := false
	if consentReq.Context != nil {
		if prompt, exists := consentReq.Context["prompt"]; exists && prompt == "none" {
			isSilentAuth = true
		}
	}

	// Check if this consent request came from SSO auto-login
	// SSO auto-login should not require user consent again
	isSSOLogin := false
	if consentReq.Context != nil {
		if sso, exists := consentReq.Context["sso"]; exists {
			if ssoVal, ok := sso.(bool); ok && ssoVal {
				isSSOLogin = true
			}
		}
	}

	// Auto-accept consent if:
	// 1. Client has skip_consent=true (from Hydra)
	// 2. Claims update scenario (workspace switch, etc.)
	// 3. Silent authentication (prompt=none)
	// 4. SSO auto-login (user already authenticated, should not show consent again)
	shouldAutoAccept := consentReq.Skip || isClaimsUpdate || isSilentAuth || isSSOLogin

	if shouldAutoAccept {
		h.logger.Info("Auto-accepting consent",
			zap.String("client_id", consentReq.Client.ClientID),
			zap.String("claims_source", claimsSource),
			zap.Bool("skip_consent", consentReq.Skip),
			zap.Bool("is_claims_update", isClaimsUpdate),
			zap.Bool("is_silent_auth", isSilentAuth),
			zap.Bool("is_sso_login", isSSOLogin),
			zap.String("challenge", challenge[:min(50, len(challenge))]+"..."),
			zap.Any("updated_claims", userClaims))

		// Auto-accept consent
		acceptRequest := &hydra.AcceptConsentRequest{
			GrantScope:               consentReq.RequestedScope,
			GrantAccessTokenAudience: consentReq.RequestedAudience,
			Remember:                 true,
			RememberFor:              3600,
			Session: &hydra.ConsentSession{
				AccessToken: userClaims,
				IDToken:     userClaims,
			},
		}

		redirectTo, err := h.hydraClient.AcceptConsentRequest(challenge, acceptRequest)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to auto-accept consent"})
		}

		// Return redirect_to as JSON for frontend to handle
		return c.JSON(fiber.Map{
			"redirect_to":   redirectTo.RedirectTo,
			"auto_accepted": true,
		})
	}

	// Show consent page (only if client doesn't have skip_consent=true)
	h.logger.Info("Showing consent page",
		zap.String("client_id", consentReq.Client.ClientID),
		zap.String("claims_source", claimsSource),
		zap.Bool("skip", consentReq.Skip))

	return c.JSON(fiber.Map{
		"challenge":       challenge,
		"client_name":     consentReq.Client.ClientName,
		"requested_scope": consentReq.RequestedScope,
		"user": fiber.Map{
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

type ConsentRequest struct {
	Challenge   string   `json:"challenge"`
	GrantScope  []string `json:"grant_scope"`
	Remember    bool     `json:"remember"`
	RememberFor int      `json:"remember_for"`
}

func (h *AuthHandler) Consent(c *fiber.Ctx) error {
	var req ConsentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get consent request from Hydra
	consentReq, err := h.hydraClient.GetConsentRequest(req.Challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get consent request",
		})
	}

	// Get user information for session
	userID, err := uuid.Parse(consentReq.Subject)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get user information",
		})
	}

	// Get claims for this consent flow (with fallback to pending/db claims if login_challenge mismatch)
	loginChallenge := consentReq.LoginChallenge
	userName := ""
	if user.Name != nil {
		userName = *user.Name
	}
	userInfoForClaims := &claims.UserInfo{
		Email:    user.Email,
		Name:     userName,
		TenantID: user.TenantID,
	}
	userClaims, err := h.claimsService.GetClaimsForConsent(c.Context(), loginChallenge, userID, user.TenantID, userInfoForClaims)
	if err != nil {
		h.logger.Warn("Failed to get claims for consent",
			zap.String("login_challenge", loginChallenge),
			zap.String("user_id", userID.String()),
			zap.Error(err))
		// Continue without additional claims
		userClaims = make(claims.ClaimMap)
	}

	// Build session data with base claims + user claims
	accessTokenClaims := map[string]any{
		"email":     user.Email,
		"name":      user.Name,
		"tenant_id": user.TenantID.String(),
	}

	idTokenClaims := map[string]any{
		"email":          user.Email,
		"name":           user.Name,
		"email_verified": user.EmailVerified,
		"tenant_id":      user.TenantID.String(),
	}

	// Merge user claims into both access token and ID token
	for key, value := range userClaims {
		accessTokenClaims[key] = value
		idTokenClaims[key] = value
	}

	// Accept consent request
	acceptBody := &hydra.AcceptConsentRequest{
		GrantScope:               req.GrantScope,
		GrantAccessTokenAudience: consentReq.RequestedAudience,
		Remember:                 req.Remember,
		RememberFor:              req.RememberFor,
		Session: &hydra.ConsentSession{
			AccessToken: accessTokenClaims,
			IDToken:     idTokenClaims,
		},
	}

	// Log detailed consent request data
	h.logger.Info("Sending consent accept to Hydra",
		zap.String("challenge", req.Challenge),
		zap.Strings("grant_scope", req.GrantScope),
		zap.Strings("grant_access_token_audience", consentReq.RequestedAudience),
		zap.Bool("remember", req.Remember),
		zap.Int("remember_for", req.RememberFor),
		zap.String("user_id", user.ID.String()),
		zap.String("tenant_id", user.TenantID.String()))

	resp, err := h.hydraClient.AcceptConsentRequest(req.Challenge, acceptBody)
	if err != nil {
		h.logger.Error("Failed to accept consent request",
			zap.Error(err),
			zap.String("challenge", req.Challenge))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to accept consent request",
		})
	}

	h.logger.Info("Consent accepted, redirecting",
		zap.String("redirect_to", resp.RedirectTo),
		zap.String("user_id", user.ID.String()))

	h.logUserAudit(c, user, audit.ActionConsentGranted, map[string]any{
		"challenge":   req.Challenge,
		"grant_scope": req.GrantScope,
		"audience":    consentReq.RequestedAudience,
		"client_id":   consentReq.Client.ClientID,
		"remember":    req.Remember,
	})

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

// RejectConsentRequest for POST request body
type RejectConsentRequest struct {
	ConsentChallenge string `json:"consent_challenge" form:"consent_challenge"`
}

func (h *AuthHandler) RejectConsent(c *fiber.Ctx) error {
	// Try to get challenge from query parameter first (GET)
	challenge := c.Query("consent_challenge")

	// If not in query, try POST body (supports both JSON and form-urlencoded)
	if challenge == "" && c.Method() == "POST" {
		var req RejectConsentRequest
		if err := c.BodyParser(&req); err == nil {
			challenge = req.ConsentChallenge
		}
	}

	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "consent_challenge parameter is required",
			"hint":  "The consent_challenge parameter must be included in the URL query string or POST body",
		})
	}

	// Resolve subject for audit actor before rejecting — best effort, ignore errors.
	var rejectingUser *user.User
	if consentReq, err := h.hydraClient.GetConsentRequest(challenge); err == nil && consentReq != nil && consentReq.Subject != "" {
		if uid, parseErr := uuid.Parse(consentReq.Subject); parseErr == nil {
			rejectingUser, _ = h.userService.GetByID(uid)
		}
	}

	resp, err := h.hydraClient.RejectConsentRequest(challenge, "access_denied", "User denied consent")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to reject consent request",
		})
	}

	if rejectingUser != nil {
		h.logUserAudit(c, rejectingUser, audit.ActionConsentRevoked, map[string]any{
			"challenge": challenge,
			"reason":    "user_denied",
		})
	}

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

// LogoutPage handles logout flow - auto-accept since skip_logout_consent is true
func (h *AuthHandler) LogoutPage(c *fiber.Ctx) error {
	challenge := c.Query("logout_challenge")

	if challenge == "" {
		h.logger.Warn("Logout request without challenge")
		return c.Status(400).JSON(fiber.Map{
			"error": "logout_challenge parameter is required",
			"guide": "This endpoint should be called by Hydra with a logout_challenge parameter",
		})
	}

	// Get logout request from Hydra
	logoutReq, err := h.hydraClient.GetLogoutRequest(challenge)
	if err != nil {
		h.logger.Error("Failed to get logout request from Hydra",
			zap.String("challenge", challenge),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get logout request",
			"guide":   "Hydra might be unavailable or the challenge may have expired. Try logging out again.",
			"details": apierror.Message(err, "unable to reach Hydra"),
		})
	}

	h.logger.Info("Processing logout request",
		zap.String("subject", logoutReq.Subject),
		zap.String("challenge", challenge))

	// Auto-accept logout (skip_logout_consent is true)
	resp, err := h.hydraClient.AcceptLogoutRequest(challenge)
	if err != nil {
		h.logger.Error("Failed to accept logout request",
			zap.String("challenge", challenge),
			zap.String("subject", logoutReq.Subject),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to accept logout request",
			"guide":   "Hydra logout acceptance failed. Try again or contact support.",
			"details": apierror.Message(err, "unable to reach Hydra"),
		})
	}

	h.logger.Info("Logout accepted - redirecting",
		zap.String("subject", logoutReq.Subject),
		zap.String("redirect_to", resp.RedirectTo))

	if subjectUUID, parseErr := uuid.Parse(logoutReq.Subject); parseErr == nil {
		if logoutUser, getErr := h.userService.GetByID(subjectUUID); getErr == nil {
			h.logUserAudit(c, logoutUser, audit.ActionUserLogout, map[string]any{
				"flow": "oidc",
			})
		}
	}

	// Redirect to Hydra's logout endpoint (which will then redirect to post_logout_redirect_uri)
	return c.Redirect(resp.RedirectTo)
}

// LogoutRequest for direct logout API.
// NOTE: the subject is NEVER taken from the body — it is derived from the
// validated bearer token by the jwtAuth middleware. Only non-security-sensitive
// redirect parameters are accepted here.
type LogoutRequest struct {
	PostLogoutRedirectURI string `json:"post_logout_redirect_uri"` // Optional: Where to redirect after logout
	State                 string `json:"state"`                    // Optional: State parameter for redirect
}

// LogoutResponse for logout API response
type LogoutResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	RedirectURL string `json:"redirect_url,omitempty"` // URL to redirect to (if post_logout_redirect_uri provided)
}

// Logout revokes all OAuth2 sessions for the AUTHENTICATED user (direct
// kill-switch via Hydra Admin API).
//
// SECURITY: the subject is derived solely from the validated bearer token
// (jwtAuth middleware → Hydra introspection), never from the request body. A
// caller can therefore revoke only their own sessions. The previous version
// trusted a body `subject`/`id_token` (the latter parsed WITHOUT signature
// verification), which let anyone force-logout any user by subject UUID.
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	// Authenticated subject, established by jwtAuth.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "Unauthorized",
			"guide":   "Send a valid 'Authorization: Bearer <access_token>' header.",
		})
	}
	subject := userID.String()

	// Body is optional and carries only non-security-sensitive redirect params.
	var req LogoutRequest
	_ = c.BodyParser(&req)

	// Direct session revocation via Hydra Admin API — revokes every OAuth2
	// session for the authenticated subject only.
	if err := h.hydraClient.RevokeUserSessions(subject); err != nil {
		h.logger.Error("Failed to revoke user sessions",
			zap.String("subject", subject),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to revoke sessions",
			"guide":   "This could be a temporary Hydra connection issue. Try again or use OIDC logout flow.",
			"details": apierror.Message(err, "unable to reach Hydra"),
		})
	}

	h.logger.Info("User sessions revoked successfully",
		zap.String("subject", subject))

	if logoutUser, getErr := h.userService.GetByID(userID); getErr == nil {
		h.logUserAudit(c, logoutUser, audit.ActionUserLogout, nil)
	}

	response := LogoutResponse{
		Success: true,
		Message: "Logout successful - all sessions revoked",
	}

	// If post_logout_redirect_uri is provided, include it in response
	if req.PostLogoutRedirectURI != "" {
		redirectURL := req.PostLogoutRedirectURI
		if req.State != "" {
			redirectURL += "?state=" + req.State
		}
		response.RedirectURL = redirectURL
	}

	return c.JSON(response)
}

// NOTE: The public self-registration endpoint (RegisterRequest + Register) was
// removed — onboarding is invitation-only. Users are created via the invitation
// accept flow (pkg/invitation) or by an admin. See decision D-a/B.

// User profile endpoint
func (h *AuthHandler) Profile(c *fiber.Ctx) error {
	userID := c.Params("id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid user ID format",
		})
	}
	user, err := h.userService.GetByID(userUUID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"email_verified": user.EmailVerified,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	})
}

// ProfileMe - Get current authenticated user's profile
// Requires JWT middleware to be applied
func (h *AuthHandler) ProfileMe(c *fiber.Ctx) error {
	// Get user ID from JWT context (set by JWT middleware)
	userIDValue := c.Locals("user_id")
	if userIDValue == nil {
		h.logger.Error("user_id not found in context - JWT middleware not applied?")
		return c.Status(500).JSON(fiber.Map{
			"error": "User ID not found in context",
		})
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		h.logger.Error("user_id in context is not a UUID")
		return c.Status(500).JSON(fiber.Map{
			"error": "Invalid user ID format in context",
		})
	}

	// Get user from database
	user, err := h.userService.GetByID(userID)
	if err != nil {
		h.logger.Error("Failed to get user by ID",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"email_verified": user.EmailVerified,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	})
}

// PopupCallback - OAuth callback handler for popup mode
// Returns HTML page that sends postMessage to parent window and closes popup
// Used by @authway/client and @authway/react SDK popup login flows
func (h *AuthHandler) PopupCallback(c *fiber.Ctx) error {
	// Extract OAuth callback parameters
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")
	errorDesc := c.Query("error_description")

	h.logger.Info("Popup callback received",
		zap.Bool("hasCode", code != ""),
		zap.Bool("hasState", state != ""),
		zap.Bool("hasError", errorParam != ""),
	)

	// Helper function to safely convert string to JSON string literal
	toJSONString := func(s string) string {
		if s == "" {
			return "null"
		}
		// Escape special characters for JSON
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, "\"", "\\\"")
		s = strings.ReplaceAll(s, "\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\r")
		s = strings.ReplaceAll(s, "\t", "\t")
		return fmt.Sprintf("\"%s\"", s)
	}

	// Generate HTML with embedded JavaScript that sends postMessage
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authentication Complete</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
        }
        .container {
            text-align: center;
            padding: 2rem;
        }
        .spinner {
            border: 4px solid rgba(255, 255, 255, 0.3);
            border-radius: 50%%;
            border-top: 4px solid white;
            width: 40px;
            height: 40px;
            animation: spin 1s linear infinite;
            margin: 0 auto 1rem;
        }
        @keyframes spin {
            0%% { transform: rotate(0deg); }
            100%% { transform: rotate(360deg); }
        }
        .message {
            font-size: 1.25rem;
            font-weight: bold;
            margin-bottom: 0.5rem;
        }
        .sub-message {
            font-size: 0.875rem;
            opacity: 0.9;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="spinner"></div>
        <div class="message">Authentication Successful</div>
        <div class="sub-message">This window will close automatically...</div>
    </div>

    <script>
        (function() {
            console.log('[Authway PopupCallback] Starting callback handling');

            // Check if running in popup
            if (!window.opener) {
                console.error('[Authway PopupCallback] Not running in popup - window.opener is null');
                document.querySelector('.message').textContent = 'Error: Not in Popup';
                document.querySelector('.sub-message').textContent = 'This page must be opened in a popup window';
                return;
            }

            // Prepare message for parent window
            var message = {
                type: 'authway-callback',
                code: %s,
                state: %s,
                error: %s,
                error_description: %s
            };

            console.log('[Authway PopupCallback] Sending message to opener:', {
                type: message.type,
                hasCode: message.code !== null,
                hasState: message.state !== null,
                hasError: message.error !== null,
                origin: window.opener.origin
            });

            // Send message to parent window
            // Security: window.opener.origin ensures message only goes to parent
            window.opener.postMessage(message, window.opener.origin);

            console.log('[Authway PopupCallback] Message sent, closing popup in 500ms');

            // Close popup after small delay to ensure message delivery
            setTimeout(function() {
                window.close();
            }, 500);
        })();
    </script>
</body>
</html>`,
		toJSONString(code),
		toJSONString(state),
		toJSONString(errorParam),
		toJSONString(errorDesc),
	)

	// Set content type and return HTML
	c.Set("Content-Type", "text/html; charset=utf-8")
	// Prevent caching
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	return c.SendString(html)
}
