package handler

import (
	"authway/apps/central/api/pkg/apierror"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/terms"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
)

// nextTerms asks the sign-in screen for the user's acceptance of the
// tenant's documents, quoting terms_challenge.
const nextTerms = "terms"

// TermsLedger knows which of a tenant's documents a user still owes, and
// records an acceptance (terms.Service).
type TermsLedger interface {
	Owed(ctx context.Context, tenantID, userID uuid.UUID) ([]terms.Document, error)
	Accept(ctx context.Context, tenantID, userID uuid.UUID, accepted []terms.KindVersion, clientID string) error
}

// pendingTerms is a sign-in whose user has proved who they are and owes the
// tenant's documents: what the authorization server is told once they accept.
type pendingTerms struct {
	HydraChallenge string
	UserID         uuid.UUID
	Audit          string   // signInMethod.audit
	AMR            []string // signInMethod.amr
	Provider       string
	KeepSignedIn   bool
	// Reused marks a sign-in that reuses the browser's session: accepting it
	// neither opens nor lengthens a session.
	Reused bool
}

func (p *pendingTerms) method() signInMethod {
	return signInMethod{audit: p.Audit, amr: p.AMR, provider: p.Provider}
}

// termsChallengeTTL bounds how long a sign-in waits for its documents to be
// accepted — the window the second factor gets.
const termsChallengeTTL = mfaChallengeTTL

// TermsChallengeStore holds sign-ins that owe the tenant's documents, keyed by
// an opaque terms_challenge (MFAChallengeStore has the same shape, without the
// attempt counter: there is nothing to guess here).
type TermsChallengeStore struct {
	redis  redis.UniversalClient
	prefix string
}

func NewTermsChallengeStore(redisClient redis.UniversalClient) *TermsChallengeStore {
	return &TermsChallengeStore{redis: redisClient, prefix: "terms_challenge:"}
}

func (s *TermsChallengeStore) Set(challenge string, p *pendingTerms) error {
	ctx := context.Background()
	key := s.prefix + challenge
	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"hydra_challenge": p.HydraChallenge,
		"user_id":         p.UserID.String(),
		"audit":           p.Audit,
		"amr":             strings.Join(p.AMR, " "),
		"provider":        p.Provider,
		"keep_signed_in":  p.KeepSignedIn,
		"reused":          p.Reused,
	})
	pipe.Expire(ctx, key, termsChallengeTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// Get returns the sign-in parked under challenge for flow.
func (s *TermsChallengeStore) Get(challenge, flow string) (*pendingTerms, bool) {
	if challenge == "" {
		return nil, false
	}
	vals, err := s.redis.HGetAll(context.Background(), s.prefix+challenge).Result()
	if err != nil || len(vals) == 0 || vals["hydra_challenge"] != flow {
		return nil, false
	}
	userID, err := uuid.Parse(vals["user_id"])
	if err != nil {
		return nil, false
	}
	return &pendingTerms{
		HydraChallenge: vals["hydra_challenge"],
		UserID:         userID,
		Audit:          vals["audit"],
		AMR:            strings.Fields(vals["amr"]),
		Provider:       vals["provider"],
		KeepSignedIn:   vals["keep_signed_in"] == "1",
		Reused:         vals["reused"] == "1",
	}, true
}

func (s *TermsChallengeStore) Delete(challenge string) {
	s.redis.Del(context.Background(), s.prefix+challenge)
}

// errTermsStep is a failure to read which documents a user owes, or to park
// the sign-in until they accept them. The sign-in does not go ahead without
// the answer.
var errTermsStep = errors.New("could not run the terms step")

// owed returns the documents u still owes; none when the deployment asks for
// none.
func (s *signIn) owed(u *user.User) ([]terms.Document, error) {
	if s.terms == nil {
		return nil, nil
	}
	due, err := s.terms.Owed(context.Background(), u.TenantID, u.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errTermsStep, err)
	}
	return due, nil
}

// proved continues a flow whose user has proved who they are, every factor
// included: to the documents they owe, or signed in.
func (s *signIn) proved(flow string, u *user.User, method signInMethod, keepSignedIn bool) (signInStep, error) {
	due, err := s.owed(u)
	if err != nil {
		return signInStep{}, err
	}
	if len(due) > 0 {
		return s.park(&pendingTerms{
			HydraChallenge: flow, UserID: u.ID, Audit: method.audit, AMR: method.amr,
			Provider: method.provider, KeepSignedIn: keepSignedIn,
		}, due)
	}
	redirectTo, err := s.accept(flow, u, method, keepSignedIn)
	return signInStep{RedirectTo: redirectTo}, err
}

func (s *signIn) park(p *pendingTerms, due []terms.Document) (signInStep, error) {
	challenge, err := tokenhash.Generate()
	if err == nil {
		err = s.owing.Set(challenge, p)
	}
	if err != nil {
		return signInStep{}, fmt.Errorf("%w: %w", errTermsStep, err)
	}
	return signInStep{TermsChallenge: challenge, Terms: due}, nil
}

// reuse signs sessionUser in to flow on the browser's session. A reused
// session was authenticated earlier: its tokens say how it was then, not that
// anything was proved now, and reusing it does not lengthen it — how long it
// lasts was decided when the user signed in (the authorization server ignores
// a lifetime given here).
func (s *signIn) reuse(flow string, sessionUser *user.User, amr []string) (string, error) {
	// Suspending an account ends its sessions; one that outlived that (the
	// authorization server was unreachable) still does not sign in.
	if !sessionUser.Active {
		return "", errAccountSuspended
	}
	resp, err := s.hydra.AcceptLoginRequest(flow, &hydra.AcceptLoginRequest{
		Subject: sessionUser.ID.String(),
		AMR:     amr,
		Context: map[string]any{
			"email":     sessionUser.Email,
			"name":      sessionUser.Name,
			"tenant_id": sessionUser.TenantID.String(),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.RedirectTo, nil
}

// termsStep is the screen's answer for a sign-in parked for its documents.
func termsStep(flow string, step signInStep) fiber.Map {
	shown := make([]terms.DocumentView, 0, len(step.Terms))
	for _, d := range step.Terms {
		shown = append(shown, d.Shown())
	}
	return fiber.Map{
		"next":            nextTerms,
		"flow":            flow,
		"terms_challenge": step.TermsChallenge,
		"terms":           shown,
	}
}

func respondInvalidTermsChallenge(c *fiber.Ctx) error {
	return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_terms_challenge", "This sign-in step has expired. Start again from the application.")
}

// GetOwedTerms serves GET /api/v1/login-flows/:flow/terms?terms_challenge=…
// to a screen that arrived at the terms step by a page navigation (a sign-in
// through another identity provider) and has only the flow and the challenge.
func (h *AuthHandler) GetOwedTerms(c *fiber.Ctx) error {
	flow := flowParam(c)
	challenge := c.Query("terms_challenge")
	pending, ok := h.flows.owing.Get(challenge, flow)
	if !ok {
		return respondInvalidTermsChallenge(c)
	}
	u, err := h.userService.GetByID(pending.UserID)
	if err != nil || u == nil {
		return respondInvalidTermsChallenge(c)
	}
	due, err := h.flows.owed(u)
	if err != nil {
		h.logger.Error("Failed to read the documents a user owes", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "Failed to read the terms")
	}
	return c.JSON(termsStep(flow, signInStep{TermsChallenge: challenge, Terms: due}))
}

// AcceptTermsRequest is the body of POST /api/v1/login-flows/:flow/terms.
type AcceptTermsRequest struct {
	TermsChallenge string              `json:"terms_challenge"`
	Accepted       []terms.KindVersion `json:"accepted"`
}

// AcceptTerms serves POST /api/v1/login-flows/:flow/terms: the user accepts
// the documents shown, the acceptance is recorded and the sign-in completes.
func (h *AuthHandler) AcceptTerms(c *fiber.Ctx) error {
	flow := flowParam(c)
	var req AcceptTermsRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	pending, ok := h.flows.owing.Get(req.TermsChallenge, flow)
	if !ok {
		return respondInvalidTermsChallenge(c)
	}
	u, err := h.userService.GetByID(pending.UserID)
	if err != nil || u == nil {
		return respondInvalidTermsChallenge(c)
	}
	loginReq, err := h.hydraClient.GetLoginRequest(flow)
	if err != nil {
		return respondFlowLookupError(c, err)
	}
	clientID := ""
	if loginReq.Client != nil {
		clientID = loginReq.Client.ClientID
	}

	err = h.flows.terms.Accept(c.Context(), u.TenantID, u.ID, req.Accepted, clientID)
	var mismatch *terms.MismatchError
	switch {
	case errors.As(err, &mismatch):
		body := termsStep(flow, signInStep{Terms: mismatch.Owed})
		return apierror.RefuseWith(c, fiber.StatusBadRequest, "terms_mismatch", "The terms changed while they were shown. Review them again.", fiber.Map{"terms": body["terms"]})
	case err != nil:
		h.logger.Error("Failed to record an acceptance", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "Failed to record the acceptance")
	}
	h.flows.owing.Delete(req.TermsChallenge)
	h.logUserAudit(c, u, audit.ActionUserTermsAccepted, map[string]any{
		"challenge": flow, "client_id": clientID, "accepted": req.Accepted,
	})

	method := pending.method()
	var redirectTo string
	if pending.Reused {
		redirectTo, err = h.flows.reuse(flow, u, pending.AMR)
	} else {
		redirectTo, err = h.flows.accept(flow, u, method, pending.KeepSignedIn)
	}
	if err != nil {
		h.logger.Error("Failed to accept login request", zap.Error(err))
		return respondFlowLookupError(c, err)
	}
	if pending.Reused {
		return c.JSON(fiber.Map{"next": nextRedirect, "redirect_to": redirectTo})
	}
	return h.signedIn(c, flow, u, method, pending.KeepSignedIn, redirectTo)
}

// DeclineTermsRequest is the body of POST /api/v1/login-flows/:flow/terms/decline.
type DeclineTermsRequest struct {
	TermsChallenge string `json:"terms_challenge"`
}

// DeclineTerms serves POST /api/v1/login-flows/:flow/terms/decline: the user
// does not accept, and the application is told so. Nothing is recorded and
// the account is left as it is; the next sign-in asks again.
func (h *AuthHandler) DeclineTerms(c *fiber.Ctx) error {
	flow := flowParam(c)
	var req DeclineTermsRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	if _, ok := h.flows.owing.Get(req.TermsChallenge, flow); !ok {
		return respondInvalidTermsChallenge(c)
	}
	h.flows.owing.Delete(req.TermsChallenge)
	resp, err := h.hydraClient.RejectLoginRequest(flow, "access_denied", "The user did not accept the terms.")
	if err != nil {
		return respondFlowLookupError(c, err)
	}
	return c.JSON(fiber.Map{"next": nextRedirect, "redirect_to": resp.RedirectTo})
}

// sessionOwesTerms settles a reused session whose user owes documents: the
// terms step for the screen, or — when the application asked for no prompt —
// interaction_required (OpenID Connect Core 3.1.2.6).
func (h *AuthHandler) sessionOwesTerms(flow string, loginReq *hydra.LoginRequest, sessionUser *user.User, amr []string, due []terms.Document) (fiber.Map, bool, error) {
	if asksForNoPrompt(loginReq.RequestURL) {
		resp, err := h.hydraClient.RejectLoginRequest(flow, "interaction_required", "The user has terms to accept.")
		if err != nil {
			return nil, false, err
		}
		return fiber.Map{"next": nextRedirect, "redirect_to": resp.RedirectTo}, true, nil
	}
	step, err := h.flows.park(&pendingTerms{HydraChallenge: flow, UserID: sessionUser.ID, AMR: amr, Reused: true}, due)
	if err != nil {
		return nil, false, err
	}
	return termsStep(flow, step), true, nil
}
