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

// PasswordLoginRequest is the body of POST /api/v1/login-flows/:flow/password.
type PasswordLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// SubmitPassword signs a user in to a login flow with email and password.
// It answers {"next":"redirect","redirect_to":…} once the flow is accepted,
// or {"next":"mfa","mfa_challenge":…} when the user has a second factor.
func (h *AuthHandler) SubmitPassword(c *fiber.Ctx) error {
	flow := flowParam(c)
	var req PasswordLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
			"code":  "invalid_request",
		})
	}

	loginReq, err := h.hydraClient.GetLoginRequest(flow)
	if err != nil {
		return respondFlowLookupError(c, err)
	}

	// The password is checked against the requesting client's tenant: the
	// same email may exist in more than one tenant (idx_users_tenant_email).
	requestedClient, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.logger.Error("OAuth client is not registered in Authway",
			zap.String("client_id", loginReq.Client.ClientID), zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "This application is not registered with Authway.",
			"code":  "client_not_registered",
		})
	}
	if !requestedClient.AllowsSignInMethod("email") {
		return respondSignInMethodNotAllowed(c)
	}

	user, err := h.userService.GetByEmailAndTenant(requestedClient.TenantID, req.Email)
	if err != nil {
		h.logAuthFailure(c, uuid.Nil, audit.ActionUserLoginFailed, req.Email, "user_not_found", nil)
		middleware.IncrementRateLimitOnFailure(c)
		return respondInvalidCredentials(c)
	}

	if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		h.logAuthFailure(c, user.TenantID, audit.ActionUserLoginFailed, req.Email, "invalid_password", map[string]any{
			"user_id": user.ID.String(),
		})
		middleware.IncrementRateLimitOnFailure(c)
		return respondInvalidCredentials(c)
	}

	rememberFor := 0
	if req.Remember {
		rememberFor = 3600 // 1 hour
	}

	// A password alone is not enough for a TOTP-enabled user — park the
	// verified-but-not-yet-accepted login and hand the screen an
	// mfa_challenge instead of touching Hydra. The mfa_challenge proves the
	// password step passed; the flow id alone must not open the second step.
	if user.TOTPEnabled {
		challenge, err := tokenhash.Generate()
		if err != nil {
			h.logger.Error("Failed to generate mfa_challenge", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to start MFA challenge"})
		}
		h.mfaStore.Set(challenge, &pendingMFALogin{
			HydraChallenge: flow,
			UserID:         user.ID,
			Remember:       req.Remember,
			RememberFor:    rememberFor,
			CreatedAt:      time.Now(),
		})
		h.logger.Info("Password verified, MFA required", zap.String("user_id", user.ID.String()))
		return c.JSON(fiber.Map{
			"next":          nextMFA,
			"mfa_challenge": challenge,
		})
	}

	return h.completeLogin(c, flow, user, req.Remember, rememberFor, "password")
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
		h.logger.Error("Failed to accept login request", zap.Error(err))
		return respondFlowLookupError(c, err)
	}

	middleware.ResetRateLimitOnSuccess(c)
	h.logUserAudit(c, u, audit.ActionUserLogin, map[string]any{
		"challenge": challenge,
		"remember":  remember,
		"method":    method,
	})

	return c.JSON(fiber.Map{
		"next":        nextRedirect,
		"redirect_to": resp.RedirectTo,
	})
}

// VerifyMFALoginRequest is the body of the two login-time second-factor
// endpoints. Code holds a 6-digit TOTP code or a recovery code depending on
// the endpoint.
type VerifyMFALoginRequest struct {
	MFAChallenge string `json:"mfa_challenge"`
	Code         string `json:"code"`
}

// VerifyMFALogin completes a login that SubmitPassword parked pending TOTP.
// POST /api/v1/login-flows/:flow/mfa — the mfa_challenge is the bearer
// credential for this one-shot exchange and must belong to the flow.
func (h *AuthHandler) VerifyMFALogin(c *fiber.Ctx) error {
	return h.verifySecondFactor(c, "totp", h.mfaService.Verify, "invalid verification code")
}

// VerifyMFARecoveryLogin is VerifyMFALogin's recovery-code counterpart.
// POST /api/v1/login-flows/:flow/mfa/recovery
func (h *AuthHandler) VerifyMFARecoveryLogin(c *fiber.Ctx) error {
	return h.verifySecondFactor(c, "recovery_code", h.mfaService.VerifyRecoveryCode, "invalid recovery code")
}

func (h *AuthHandler) verifySecondFactor(c *fiber.Ctx, phase string, verify func(uuid.UUID, string) (bool, error), invalidMessage string) error {
	flow := flowParam(c)
	var req VerifyMFALoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body", "code": "invalid_request"})
	}

	pending, ok := h.mfaStore.Get(req.MFAChallenge)
	if !ok || pending.HydraChallenge != flow {
		return respondInvalidMFAChallenge(c)
	}

	u, err := h.userService.GetByID(pending.UserID)
	if err != nil || u == nil {
		return respondInvalidMFAChallenge(c)
	}

	valid, err := verify(pending.UserID, req.Code)
	if err != nil || !valid {
		h.logMFALoginFailure(c, u, phase)
		middleware.IncrementRateLimitOnFailure(c)
		if locked := h.mfaStore.RecordFailure(req.MFAChallenge); locked {
			return c.Status(401).JSON(fiber.Map{"error": "too many failed attempts — please sign in again", "code": "mfa_locked"})
		}
		return c.Status(401).JSON(fiber.Map{"error": invalidMessage, "code": "invalid_code"})
	}

	h.mfaStore.Delete(req.MFAChallenge)
	method := "password+totp"
	if phase == "recovery_code" {
		method = "password+recovery_code"
	}
	return h.completeLogin(c, pending.HydraChallenge, u, pending.Remember, pending.RememberFor, method)
}

func respondInvalidMFAChallenge(c *fiber.Ctx) error {
	return c.Status(400).JSON(fiber.Map{"error": "invalid or expired mfa challenge", "code": "invalid_mfa_challenge"})
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

// GetConsentFlow tells the consent screen what to do with a consent flow:
// {"next":"redirect","redirect_to":…} when no question needs asking (the
// client skips consent, a silent or single-sign-on login, a claims update),
// otherwise {"next":"form",…} with what to ask.
// GET /api/v1/consent-flows/:flow
func (h *AuthHandler) GetConsentFlow(c *fiber.Ctx) error {
	challenge := flowParam(c)

	// Get consent request from Hydra
	consentReq, err := h.hydraClient.GetConsentRequest(challenge)
	if err != nil {
		return respondFlowLookupError(c, err)
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
			h.logger.Error("Failed to auto-accept consent", zap.Error(err))
			return respondFlowLookupError(c, err)
		}

		return c.JSON(fiber.Map{
			"next":        nextRedirect,
			"redirect_to": redirectTo.RedirectTo,
		})
	}

	// Show consent page (only if client doesn't have skip_consent=true)
	h.logger.Info("Showing consent page",
		zap.String("client_id", consentReq.Client.ClientID),
		zap.String("claims_source", claimsSource),
		zap.Bool("skip", consentReq.Skip))

	return c.JSON(fiber.Map{
		"next":            nextForm,
		"flow":            challenge,
		"client_name":     consentReq.Client.ClientName,
		"requested_scope": consentReq.RequestedScope,
		"user": fiber.Map{
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

// AcceptConsentRequest is the body of POST /api/v1/consent-flows/:flow/accept.
type AcceptConsentRequest struct {
	GrantScope  []string `json:"grant_scope"`
	Remember    bool     `json:"remember"`
	RememberFor int      `json:"remember_for"`
}

// AcceptConsent grants the scopes the user approved on the consent screen.
// POST /api/v1/consent-flows/:flow/accept
func (h *AuthHandler) AcceptConsent(c *fiber.Ctx) error {
	challenge := flowParam(c)
	var req AcceptConsentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
			"code":  "invalid_request",
		})
	}

	consentReq, err := h.hydraClient.GetConsentRequest(challenge)
	if err != nil {
		return respondFlowLookupError(c, err)
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
		zap.String("challenge", challenge),
		zap.Strings("grant_scope", req.GrantScope),
		zap.Strings("grant_access_token_audience", consentReq.RequestedAudience),
		zap.Bool("remember", req.Remember),
		zap.Int("remember_for", req.RememberFor),
		zap.String("user_id", user.ID.String()),
		zap.String("tenant_id", user.TenantID.String()))

	resp, err := h.hydraClient.AcceptConsentRequest(challenge, acceptBody)
	if err != nil {
		h.logger.Error("Failed to accept consent request",
			zap.Error(err),
			zap.String("challenge", challenge))
		return respondFlowLookupError(c, err)
	}

	h.logger.Info("Consent accepted, redirecting",
		zap.String("redirect_to", resp.RedirectTo),
		zap.String("user_id", user.ID.String()))

	h.logUserAudit(c, user, audit.ActionConsentGranted, map[string]any{
		"challenge":   challenge,
		"grant_scope": req.GrantScope,
		"audience":    consentReq.RequestedAudience,
		"client_id":   consentReq.Client.ClientID,
		"remember":    req.Remember,
	})

	return c.JSON(fiber.Map{
		"next":        nextRedirect,
		"redirect_to": resp.RedirectTo,
	})
}

// RejectConsent records that the user declined; the application receives
// access_denied. POST /api/v1/consent-flows/:flow/reject
func (h *AuthHandler) RejectConsent(c *fiber.Ctx) error {
	challenge := flowParam(c)

	// Resolve subject for audit actor before rejecting — best effort, ignore errors.
	var rejectingUser *user.User
	if consentReq, err := h.hydraClient.GetConsentRequest(challenge); err == nil && consentReq != nil && consentReq.Subject != "" {
		if uid, parseErr := uuid.Parse(consentReq.Subject); parseErr == nil {
			rejectingUser, _ = h.userService.GetByID(uid)
		}
	}

	resp, err := h.hydraClient.RejectConsentRequest(challenge, "access_denied", "User denied consent")
	if err != nil {
		h.logger.Error("Failed to reject consent request", zap.Error(err))
		return respondFlowLookupError(c, err)
	}

	if rejectingUser != nil {
		h.logUserAudit(c, rejectingUser, audit.ActionConsentRevoked, map[string]any{
			"challenge": challenge,
			"reason":    "user_denied",
		})
	}

	return c.JSON(fiber.Map{
		"next":        nextRedirect,
		"redirect_to": resp.RedirectTo,
	})
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

// respondInvalidCredentials answers a failed password check without ending
// the login flow. Rejecting the Hydra login request here would send the user
// back to the application with an OAuth error after a single typo; the
// challenge stays valid, so the login screen can show the error and let them
// retry (attempts are bounded by the login rate limit). Unknown email and
// wrong password produce the same answer so the response does not reveal
// which accounts exist.
func respondInvalidCredentials(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "Invalid email or password",
		"code":  "invalid_credentials",
	})
}

// respondSignInMethodNotAllowed answers a sign-in attempt with a method the
// client has not enabled. The screen hides such methods; this is the server's
// own check.
func respondSignInMethodNotAllowed(c *fiber.Ctx) error {
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"error": "This sign-in method is not available for this application.",
		"code":  "sign_in_method_not_allowed",
	})
}
