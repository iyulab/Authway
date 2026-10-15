package handler

import (
	"context"
	"errors"
	"slices"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/mfa"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userService     user.Service
	clientService   client.Service
	claimsService   claims.Service
	mfaService      mfa.Service
	hydraClient     *hydra.Client
	logger          *zap.Logger
	auditService    audit.Service
	mfaStore        *MFAChallengeStore
	authentications *AuthenticationStore
	signInMethods   SignInMethodSource
	flows           *signIn
}

// SignInMethodSource says how a client's users can sign in on this
// deployment (see SocialHandler.SignInMethodsFor).
type SignInMethodSource interface {
	SignInMethodsFor(cl *client.Client) []string
}

func NewAuthHandler(userService user.Service, clientService client.Service, claimsService claims.Service, mfaService mfa.Service, hydraClient *hydra.Client, logger *zap.Logger, auditService audit.Service, redisClient redis.UniversalClient, signInMethods SignInMethodSource, policy SignInPolicy) *AuthHandler {
	flows := newSignIn(hydraClient, userService, logger, redisClient, policy)
	return &AuthHandler{
		flows:           flows,
		signInMethods:   signInMethods,
		userService:     userService,
		clientService:   clientService,
		claimsService:   claimsService,
		mfaService:      mfaService,
		hydraClient:     hydraClient,
		logger:          logger,
		auditService:    auditService,
		mfaStore:        flows.pending,
		authentications: flows.authentications,
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

// RecordSignOut records that the user named by subject (the authorization
// server's subject, the user's id) signed out: the user.logout audit entry,
// from which the webhook event follows. Unknown subjects record nothing.
func (h *AuthHandler) RecordSignOut(c *fiber.Ctx, subject string) {
	id, err := uuid.Parse(subject)
	if err != nil {
		return
	}
	if u, err := h.userService.GetByID(id); err == nil {
		h.logUserAudit(c, u, audit.ActionUserLogout, nil)
	}
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

// PasswordLoginRequest is the body of POST /api/v1/login-flows/:flow/password.
// Remember is the user's choice to stay signed in on this browser.
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
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
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
		return apierror.Refuse(c, fiber.StatusBadRequest, "client_not_registered", "This application is not registered with Authway.")
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

	return h.firstFactorPassed(c, flow, user, byPassword, req.Remember)
}

// firstFactorPassed answers the sign-in screen for a user who has just proved
// who they are by first: the second factor step when they have one, signed in
// otherwise. The step names its flow, which a screen that came in by an
// emailed link does not otherwise know.
func (h *AuthHandler) firstFactorPassed(c *fiber.Ctx, flow string, u *user.User, first signInMethod, keepSignedIn bool) error {
	step, err := h.flows.afterFirstFactor(flow, u, first, keepSignedIn)
	if errors.Is(err, errAccountSuspended) {
		h.logAuthFailure(c, u.TenantID, audit.ActionUserLoginFailed, u.Email, "account_suspended", map[string]any{"user_id": u.ID.String()})
		return respondFlowLookupError(c, err)
	}
	if errors.Is(err, errSecondFactorNotStarted) || errors.Is(err, errTermsStep) {
		h.logger.Error("Failed to start the next sign-in step", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "Failed to continue the sign-in")
	}
	if err != nil {
		h.logger.Error("Failed to accept login request", zap.Error(err))
		return respondFlowLookupError(c, err)
	}
	if step.TermsChallenge != "" {
		return c.JSON(termsStep(flow, step))
	}
	if step.MFAChallenge != "" {
		h.logger.Info("First factor verified, second factor required",
			zap.String("user_id", u.ID.String()), zap.String("method", first.audit))
		return c.JSON(fiber.Map{
			"next":          nextMFA,
			"mfa_challenge": step.MFAChallenge,
			"flow":          flow,
		})
	}
	return h.signedIn(c, flow, u, first, keepSignedIn, step.RedirectTo)
}

// signedIn records a completed sign-in and sends the screen on.
func (h *AuthHandler) signedIn(c *fiber.Ctx, flow string, u *user.User, method signInMethod, keepSignedIn bool, redirectTo string) error {
	middleware.ResetRateLimitOnSuccess(c)
	h.logUserAudit(c, u, audit.ActionUserLogin, method.details(flow, keepSignedIn))

	return c.JSON(fiber.Map{
		"next":        nextRedirect,
		"redirect_to": redirectTo,
	})
}

// VerifyMFALoginRequest is the body of the two login-time second-factor
// endpoints. Code holds a 6-digit TOTP code or a recovery code depending on
// the endpoint.
type VerifyMFALoginRequest struct {
	MFAChallenge string `json:"mfa_challenge"`
	Code         string `json:"code"`
}

// VerifyMFALogin completes a login parked pending TOTP after its first factor.
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
		return apierror.Refuse(c, 400, "invalid_request", "Invalid request body")
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
			return apierror.Refuse(c, 401, "mfa_locked", "too many failed attempts — please sign in again")
		}
		return apierror.Refuse(c, 401, "invalid_code", invalidMessage)
	}

	h.mfaStore.Delete(req.MFAChallenge)
	method := firstFactor(pending.First, pending.Provider).withTOTP()
	if phase == "recovery_code" {
		method = firstFactor(pending.First, pending.Provider).withRecoveryCode()
	}
	step, err := h.flows.proved(pending.HydraChallenge, u, method, pending.KeepSignedIn)
	if errors.Is(err, errTermsStep) {
		h.logger.Error("Failed to read the documents a user owes", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "Failed to continue the sign-in")
	}
	if err != nil {
		h.logger.Error("Failed to accept login request", zap.Error(err))
		return respondFlowLookupError(c, err)
	}
	if step.TermsChallenge != "" {
		return c.JSON(termsStep(pending.HydraChallenge, step))
	}
	return h.signedIn(c, pending.HydraChallenge, u, method, pending.KeepSignedIn, step.RedirectTo)
}

func respondInvalidMFAChallenge(c *fiber.Ctx) error {
	return apierror.Refuse(c, 400, "invalid_mfa_challenge", "invalid or expired mfa challenge")
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

// issuedFor is what Authway asserts about u in the tokens it issues; those
// claims go on top of any stored ones, on every consent path.
func issuedFor(u *user.User) claims.Issued {
	return claims.Issued{
		TenantID:      u.TenantID,
		Email:         u.Email,
		Name:          u.Name,
		EmailVerified: u.EmailVerified,
	}
}

// grantedAudience is the audience of the access token a consent grants: the
// resource servers the client is registered for. A request that names some of
// them gets those only. The authorization server takes whatever audience the
// consent grants, so nothing the client is not registered for is granted
// here.
func grantedAudience(consentReq *hydra.ConsentRequest) []string {
	if consentReq.Client == nil {
		return nil
	}
	registered := consentReq.Client.Audience
	if len(consentReq.RequestedAudience) == 0 {
		return registered
	}
	var granted []string
	for _, requested := range consentReq.RequestedAudience {
		if slices.Contains(registered, requested) {
			granted = append(granted, requested)
		}
	}
	return granted
}

// storedClaims returns the claims stored for u, for the tokens about to be
// issued. A failure to read them does not fail the consent: the tokens then
// carry the issuer's claims only.
func (h *AuthHandler) storedClaims(c *fiber.Ctx, u *user.User) claims.ClaimMap {
	stored, err := h.claimsService.List(c.Context(), u.ID, u.TenantID)
	if err != nil {
		h.logger.Warn("Failed to get stored claims for consent", zap.String("user_id", u.ID.String()), zap.Error(err))
		return claims.ClaimMap{}
	}
	return stored
}

// GetConsentFlow tells the consent screen what to do with a consent flow:
// {"next":"redirect","redirect_to":…} when no question needs asking (the
// client skips consent, a silent or single-sign-on login),
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
		return apierror.Refuse(c, 500, apierror.CodeForStatus(500), "Invalid user ID")
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		return apierror.Refuse(c, 500, apierror.CodeForStatus(500), "User not found")
	}

	if redirectTo, settled, err := h.consentWithoutQuestion(c, challenge, consentReq, user); err != nil {
		h.logger.Error("Failed to auto-accept consent", zap.Error(err))
		return respondFlowLookupError(c, err)
	} else if settled {
		return c.JSON(fiber.Map{
			"next":        nextRedirect,
			"redirect_to": redirectTo,
		})
	}

	h.logger.Info("Showing consent page",
		zap.String("client_id", consentReq.Client.ClientID),
		zap.Bool("skip", consentReq.Skip))

	return c.JSON(fiber.Map{
		"next":            nextForm,
		"flow":            challenge,
		"client_name":     consentReq.Client.ClientName,
		"client_logo_uri": consentReq.Client.LogoURI,
		"requested_scope": consentReq.RequestedScope,
		"user": fiber.Map{
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

// ConsentWithoutScreen settles a consent flow at the moment the authorization
// server sends the browser over, when there is no question to put to the user
// (FlowSettler). Anything unexpected leaves it to the screen, which reports it.
func (h *AuthHandler) ConsentWithoutScreen(c *fiber.Ctx, flow string) (string, bool) {
	consentReq, err := h.hydraClient.GetConsentRequest(flow)
	if err != nil {
		return "", false
	}
	userID, err := uuid.Parse(consentReq.Subject)
	if err != nil {
		return "", false
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		return "", false
	}
	redirectTo, settled, err := h.consentWithoutQuestion(c, flow, consentReq, user)
	if err != nil {
		h.logger.Error("Failed to auto-accept consent", zap.Error(err))
		return "", false
	}
	return redirectTo, settled
}

// consentWithoutQuestion accepts a consent flow whose user is not to be
// asked, and reports settled=false when they are.
//
// The user is not asked when their earlier answer is remembered, or when the
// client is registered to skip the question (its own application of the
// tenant). Nothing else decides this: not what is stored about the user, and
// not that they arrived on an existing session — a session says who the user
// is, not what they allowed this client.
//
// The authorization server reports the client's registration only as
// client.skip_consent; `skip` is true for a remembered consent alone.
func (h *AuthHandler) consentWithoutQuestion(c *fiber.Ctx, challenge string, consentReq *hydra.ConsentRequest, user *user.User) (string, bool, error) {
	skipsQuestion := consentReq.Client != nil && consentReq.Client.SkipConsent
	if !consentReq.Skip && !skipsQuestion {
		return "", false, nil
	}
	h.logger.Info("Consent continues without a question",
		zap.String("client_id", consentReq.Client.ClientID),
		zap.Bool("remembered", consentReq.Skip),
		zap.Bool("client_skips_consent", skipsQuestion),
		zap.String("challenge", challenge[:min(50, len(challenge))]+"..."))

	userClaims := h.storedClaims(c, user)
	issued := issuedFor(user)
	resp, err := h.hydraClient.AcceptConsentRequest(challenge, &hydra.AcceptConsentRequest{
		GrantScope:               consentReq.RequestedScope,
		GrantAccessTokenAudience: grantedAudience(consentReq),
		Remember:                 true,
		RememberFor:              int(autoConsentRememberedFor.Seconds()),
		Session: &hydra.ConsentSession{
			AccessToken: withSession(issued.AccessToken(userClaims), h.authentications, consentReq.LoginSessionID),
			IDToken:     issued.IDToken(userClaims),
		},
	})
	if err != nil {
		return "", false, err
	}
	return resp.RedirectTo, true, nil
}

// AcceptConsentRequest is the body of POST /api/v1/consent-flows/:flow/accept.
// Remember is the user's choice not to be asked again by this client; the
// answer is then kept until the user's consents are revoked.
type AcceptConsentRequest struct {
	GrantScope []string `json:"grant_scope"`
	Remember   bool     `json:"remember"`
}

// AcceptConsent grants the scopes the user approved on the consent screen.
// POST /api/v1/consent-flows/:flow/accept
func (h *AuthHandler) AcceptConsent(c *fiber.Ctx) error {
	challenge := flowParam(c)
	var req AcceptConsentRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, 400, "invalid_request", "Invalid request body")
	}

	consentReq, err := h.hydraClient.GetConsentRequest(challenge)
	if err != nil {
		return respondFlowLookupError(c, err)
	}

	// Get user information for session
	userID, err := uuid.Parse(consentReq.Subject)
	if err != nil {
		return apierror.Refuse(c, 500, apierror.CodeForStatus(500), "Invalid user ID")
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		return apierror.Refuse(c, 500, apierror.CodeForStatus(500), "Failed to get user information")
	}

	userClaims := h.storedClaims(c, user)
	issued := issuedFor(user)

	// Accept consent request
	audience := grantedAudience(consentReq)
	acceptBody := &hydra.AcceptConsentRequest{
		GrantScope:               req.GrantScope,
		GrantAccessTokenAudience: audience,
		Remember:                 req.Remember,
		RememberFor:              0, // until revoked
		Session: &hydra.ConsentSession{
			AccessToken: withSession(issued.AccessToken(userClaims), h.authentications, consentReq.LoginSessionID),
			IDToken:     issued.IDToken(userClaims),
		},
	}

	// Log detailed consent request data
	h.logger.Info("Sending consent accept to Hydra",
		zap.String("challenge", challenge),
		zap.Strings("grant_scope", req.GrantScope),
		zap.Strings("grant_access_token_audience", audience),
		zap.Bool("remember", req.Remember),
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
		"audience":    audience,
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

// LogoutResponse for logout API response. Where the application goes next is
// its own to decide: an address taken from the request and answered back
// unchecked would be one a caller could point anywhere.
type LogoutResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
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
		return apierror.RefuseWith(c, fiber.StatusUnauthorized, apierror.CodeForStatus(fiber.StatusUnauthorized), "Unauthorized", fiber.Map{"hint": "Send a valid 'Authorization: Bearer <access_token>' header."})
	}
	subject := userID.String()

	// Direct session revocation via Hydra Admin API — revokes every OAuth2
	// session for the authenticated subject only.
	err := h.hydraClient.RevokeUserSessions(subject)
	if errors.Is(err, hydra.ErrApplicationsNotNotified) {
		h.logger.Warn("Sessions revoked, but not every application was notified", zap.String("subject", subject), zap.Error(err))
		err = nil
	}
	if err != nil {
		h.logger.Error("Failed to revoke user sessions",
			zap.String("subject", subject),
			zap.Error(err))
		return apierror.RefuseWith(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to revoke sessions", fiber.Map{"hint": "The authorization server may be briefly unreachable. Try again, or sign out through the OpenID Connect logout flow."})
	}

	h.logger.Info("User sessions revoked successfully",
		zap.String("subject", subject))

	h.RecordSignOut(c, userID.String())

	return c.JSON(LogoutResponse{
		Success: true,
		Message: "Logout successful - all sessions revoked",
	})
}

// NOTE: The public self-registration endpoint (RegisterRequest + Register) was
// removed — onboarding is invitation-only. Users are created via the invitation
// accept flow (pkg/invitation) or by an admin. See decision D-a/B.

// ProfileMe - Get current authenticated user's profile
// Requires JWT middleware to be applied
func (h *AuthHandler) ProfileMe(c *fiber.Ctx) error {
	// Get user ID from JWT context (set by JWT middleware)
	userIDValue := c.Locals("user_id")
	if userIDValue == nil {
		h.logger.Error("user_id not found in context - JWT middleware not applied?")
		return apierror.Refuse(c, 500, apierror.CodeForStatus(500), "User ID not found in context")
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		h.logger.Error("user_id in context is not a UUID")
		return apierror.Refuse(c, 500, apierror.CodeForStatus(500), "Invalid user ID format in context")
	}

	// Get user from database
	user, err := h.userService.GetByID(userID)
	if err != nil {
		h.logger.Error("Failed to get user by ID",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return apierror.Refuse(c, 404, apierror.CodeForStatus(404), "User not found")
	}

	return c.JSON(fiber.Map{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"email_verified": user.EmailVerified,
		// Whether the account has a password — the account screen offers to
		// change it, or to set one.
		"has_password": user.PasswordHash != "",
		"created_at":   user.CreatedAt,
		"updated_at":   user.UpdatedAt,
	})
}

// respondInvalidCredentials answers a failed password check without ending
// the login flow. Rejecting the Hydra login request here would send the user
// back to the application with an OAuth error after a single typo; the
// challenge stays valid, so the login screen can show the error and let them
// retry (attempts are bounded by the login rate limit). Unknown email and
// wrong password produce the same answer so the response does not reveal
// which accounts exist.
func respondInvalidCredentials(c *fiber.Ctx) error {
	return apierror.Refuse(c, fiber.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
}

// respondSignInMethodNotAllowed answers a sign-in attempt with a method the
// client has not enabled. The screen hides such methods; this is the server's
// own check.
func respondSignInMethodNotAllowed(c *fiber.Ctx) error {
	return apierror.Refuse(c, fiber.StatusForbidden, "sign_in_method_not_allowed", "This sign-in method is not available for this application.")
}
