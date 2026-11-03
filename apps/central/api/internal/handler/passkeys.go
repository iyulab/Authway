package handler

import (
	"encoding/json"
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/passkey"
	"authway/apps/central/api/pkg/user"
)

// PasskeyHandler serves a user's own passkeys: listing them, registering one,
// removing one.
//
// Every call takes a token issued to the deployment's account application.
// Registering a passkey adds a way to sign in to the account, and the server
// cannot tell a real device from a program that writes the answer itself — a
// browser enforces that the page asking is the login screen's, a program holding
// the user's token does not. So an application that holds a token for the user
// cannot register one in their place, as it cannot decide an approval.
type PasskeyHandler struct {
	users      user.Service
	passkeys   user.PasskeyService
	clients    client.Service
	ceremonies *passkey.Ceremonies
	tenantName func(uuid.UUID) string
	audit      audit.Service
	logger     *zap.Logger
}

// NewPasskeyHandler returns a PasskeyHandler. ceremonies is nil where passkeys
// are not set up; then every call answers 404 passkeys_unavailable. tenantName
// and auditService may be nil.
func NewPasskeyHandler(users user.Service, passkeys user.PasskeyService, clients client.Service, ceremonies *passkey.Ceremonies, tenantName func(uuid.UUID) string, auditService audit.Service, logger *zap.Logger) *PasskeyHandler {
	return &PasskeyHandler{users: users, passkeys: passkeys, clients: clients, ceremonies: ceremonies, tenantName: tenantName, audit: auditService, logger: logger}
}

// passkeyView is a passkey as the account screen sees it. The credential id and
// the key are not shown: nothing on the screen needs them.
func passkeyView(p user.Passkey) fiber.Map {
	return fiber.Map{
		"id":           p.ID,
		"name":         p.Name,
		"created_at":   p.CreatedAt,
		"last_used_at": p.LastUsedAt,
		// Whether the device syncs the passkey to the user's other devices, as
		// it reported at its last use.
		"synced": p.Flags&int16(protocol.FlagBackupState) != 0,
	}
}

// account returns the token's user, once the call is known to be the account
// application's and passkeys are set up. It writes the refusal itself, and
// then returns nil.
func (h *PasskeyHandler) account(c *fiber.Ctx, needRecent bool, doing string) *user.User {
	if h.ceremonies == nil {
		_ = apierror.Refuse(c, fiber.StatusNotFound, "passkeys_unavailable", "Passkeys are not set up on this server.")
		return nil
	}
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		_ = apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to "+doing+".")
		return nil
	}
	clientID, _ := c.Locals("client_id").(string)
	cl, err := h.clients.GetByClientID(clientID)
	if err != nil || !cl.IsAccountScreen() {
		_ = apierror.Refuse(c, fiber.StatusForbidden, "account_application_required",
			"Manage passkeys on the account screen: a token another application holds for you cannot.")
		return nil
	}
	u, err := h.users.GetByID(userID)
	if err != nil || !u.Active || u.TenantID != cl.TenantID {
		_ = apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Account not found.")
		return nil
	}
	if needRecent && !signedInRecently(c) {
		_ = refuseStaleSignIn(c, "Sign in again to "+doing+".")
		return nil
	}
	return u
}

// List handles GET /api/v1/users/passkeys — the user's passkeys, oldest first.
func (h *PasskeyHandler) List(c *fiber.Ctx) error {
	u := h.account(c, false, "see your passkeys")
	if u == nil {
		return nil
	}
	rows, err := h.passkeys.List(u.ID)
	if err != nil {
		h.logger.Error("Failed to list passkeys", zap.String("user_id", u.ID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "Your passkeys could not be read. Try again shortly.")
	}
	views := make([]fiber.Map, 0, len(rows))
	for _, row := range rows {
		views = append(views, passkeyView(row))
	}
	return c.JSON(fiber.Map{"passkeys": views})
}

// BeginRegistration handles POST /api/v1/users/passkeys/register/begin — the
// options the browser gives navigator.credentials.create, and the ceremony id
// to name in finish. It needs a recent sign-in, as the other changes to how an
// account signs in do.
func (h *PasskeyHandler) BeginRegistration(c *fiber.Ctx) error {
	u := h.account(c, true, "add a passkey")
	if u == nil {
		return nil
	}
	label := u.Email
	if h.tenantName != nil {
		if name := h.tenantName(u.TenantID); name != "" {
			label = u.Email + " · " + name
		}
	}
	reg, err := h.ceremonies.BeginRegistration(c.UserContext(), u, label)
	if err != nil {
		h.logger.Error("Failed to begin a passkey registration", zap.String("user_id", u.ID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "A passkey could not be started. Try again shortly.")
	}
	return c.JSON(fiber.Map{"ceremony_id": reg.CeremonyID, "options": reg.Options})
}

type finishRegistrationRequest struct {
	CeremonyID string `json:"ceremony_id"`
	// Name is what the user calls the passkey; optional.
	Name string `json:"name"`
	// Credential is the browser's answer: the PublicKeyCredential as its
	// toJSON() writes it.
	Credential json.RawMessage `json:"credential"`
}

// FinishRegistration handles POST /api/v1/users/passkeys/register/finish.
func (h *PasskeyHandler) FinishRegistration(c *fiber.Ctx) error {
	u := h.account(c, true, "add a passkey")
	if u == nil {
		return nil
	}
	var req finishRegistrationRequest
	if err := c.BodyParser(&req); err != nil || req.CeremonyID == "" || len(req.Credential) == 0 {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "ceremony_id and credential are required")
	}
	p, err := h.ceremonies.FinishRegistration(c.UserContext(), u, req.CeremonyID, req.Credential, req.Name)
	switch {
	case errors.Is(err, passkey.ErrCeremonyNotFound):
		middleware.IncrementRateLimitOnFailure(c)
		return apierror.Refuse(c, fiber.StatusBadRequest, "ceremony_expired", "The passkey was not saved in time. Start again.")
	case errors.Is(err, passkey.ErrRejected):
		h.logger.Warn("A passkey registration was refused", zap.String("user_id", u.ID.String()), zap.Error(err))
		middleware.IncrementRateLimitOnFailure(c)
		return apierror.Refuse(c, fiber.StatusBadRequest, "passkey_rejected", "The passkey could not be verified. Try again.")
	case errors.Is(err, user.ErrPasskeyTaken):
		return apierror.Refuse(c, fiber.StatusConflict, "passkey_already_registered", "This passkey is already registered.")
	case err != nil:
		h.logger.Error("Failed to finish a passkey registration", zap.String("user_id", u.ID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The passkey could not be saved. Try again shortly.")
	}
	middleware.ResetRateLimitOnSuccess(c)
	h.record(c, u, audit.ActionUserPasskeyAdded, p.ID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"passkey": passkeyView(*p)})
}

// Remove handles DELETE /api/v1/users/passkeys/:id. It needs a recent sign-in,
// and it does not take away the account's only way to sign in.
func (h *PasskeyHandler) Remove(c *fiber.Ctx) error {
	u := h.account(c, true, "remove a passkey")
	if u == nil {
		return nil
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Passkey not found.")
	}
	switch err := h.passkeys.Remove(u.ID, id); {
	case errors.Is(err, user.ErrPasskeyNotFound):
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Passkey not found.")
	case errors.Is(err, user.ErrLastSignInMethod):
		return apierror.Refuse(c, fiber.StatusConflict, "last_sign_in_method", "This is the only way the account signs in. Set a password first.")
	case err != nil:
		h.logger.Error("Failed to remove a passkey", zap.String("user_id", u.ID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The passkey could not be removed. Try again shortly.")
	}
	h.record(c, u, audit.ActionUserPasskeyRemoved, id)
	return c.JSON(fiber.Map{"message": "Passkey removed"})
}

// record writes the audit entry of a change to u's passkeys. The passkey's id is
// recorded; its credential id and key are not.
func (h *PasskeyHandler) record(c *fiber.Ctx, u *user.User, action audit.AuditAction, passkeyID uuid.UUID) {
	if h.audit == nil {
		return
	}
	entry := audit.EntryFromFiber(c, u.TenantID, action, "user", u.ID.String())
	entry.ActorID = &u.ID
	entry.ActorEmail = u.Email
	entry.ActorType = "user"
	entry.Details["passkey_id"] = passkeyID.String()
	h.audit.LogAsync(entry)
}
