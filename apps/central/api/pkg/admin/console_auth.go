package admin

import (
	"context"
	"errors"
	"slices"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/console"
	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Introspector validates an access token at the authorization server.
type Introspector interface {
	IntrospectToken(token string) (*hydra.IntrospectResponse, error)
}

// ConsoleAccounts is what the admin middleware reads to admit a console
// account.
type ConsoleAccounts interface {
	SystemTenantID(ctx context.Context) (uuid.UUID, error)
	MembershipsOf(ctx context.Context, userID uuid.UUID) ([]console.Membership, error)
}

// UseConsoleAccounts lets the administration API take a console account's
// access token besides the admin key and the console session.
func (h *Handler) UseConsoleAccounts(introspector Introspector, accounts ConsoleAccounts) {
	h.introspector = introspector
	h.accounts = accounts
}

// enterScope records what the caller administers and refuses a request that
// names a tenant outside it — before any handler reads the tenant, so one
// that reads the header itself cannot be pointed elsewhere either.
func enterScope(c *fiber.Ctx, s tenantscope.Scope) *apierror.Refusal {
	tenantscope.SetScope(c, s)
	if _, _, err := tenantscope.Named(c); err != nil {
		var r *apierror.Refusal
		if errors.As(err, &r) {
			return r
		}
		return apierror.Reject(fiber.StatusBadRequest, "invalid_request", "the tenant id is not a valid id")
	}
	return nil
}

// consoleAccount admits the holder of a console account's access token: a
// user's token (not a client's own, not one a client holds for the user)
// issued for the console's audience, to an account of the system tenant
// that holds a membership. handled is false when the token is not such a
// token at all, so the caller answers as for any token it does not know.
func (h *Handler) consoleAccount(c *fiber.Ctx, resp *hydra.IntrospectResponse) (handled bool, refusal *apierror.Refusal) {
	if h.accounts == nil || resp == nil || !resp.Active || resp.ClientID == resp.Subject {
		return false, nil
	}
	if !slices.Contains(resp.Audience, console.Audience) {
		return false, nil
	}
	userID, err := uuid.Parse(resp.Subject)
	if err != nil {
		return false, nil
	}
	if _, delegated := resp.Ext["act"]; delegated {
		return true, apierror.Reject(fiber.StatusForbidden, "delegated_token", "A token a client holds to act for the user cannot administer.")
	}
	system, err := h.accounts.SystemTenantID(c.Context())
	if err != nil {
		if !errors.Is(err, console.ErrNoSystemTenant) {
			h.logger.Error("Failed to read the system tenant", zap.Error(err))
		}
		return false, nil
	}
	if tid, _ := resp.Ext["tenant_id"].(string); tid != system.String() {
		return false, nil
	}
	ms, err := h.accounts.MembershipsOf(c.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to read console memberships", zap.Error(err))
		return true, apierror.Reject(fiber.StatusInternalServerError, "internal_server_error", "Failed to validate session")
	}
	if len(ms) == 0 {
		return true, apierror.Reject(fiber.StatusForbidden, "no_console_access", "This account administers nothing in this Authway.")
	}
	c.Locals("admin_authenticated", true)
	c.Locals("is_admin_console", true)
	c.Locals("auth_method", "console_token")
	c.Locals("actor_type", "console_user")
	c.Locals("actor_id", userID)
	c.Locals(console.AccountLocal, userID)
	h.setTenantIDLocal(c)
	return true, enterScope(c, console.ScopeOf(ms))
}

// introspect asks the authorization server about token; nil when there is
// no introspector or the call failed.
func (h *Handler) introspect(introspector Introspector, token string) *hydra.IntrospectResponse {
	if introspector == nil {
		return nil
	}
	resp, err := introspector.IntrospectToken(token)
	if err != nil {
		return nil
	}
	return resp
}
