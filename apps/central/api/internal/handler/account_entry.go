package handler

import (
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/client"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// AccountEntryHandler answers the account screen's first question: an
// application linked a user to it — which client does the screen sign the
// user in with, and where does the user go back to?
type AccountEntryHandler struct {
	clients client.Service
	loginUI string
	logger  *zap.Logger
}

// NewAccountEntryHandler builds the handler. loginUI is the login screens'
// public address, where the account screen lives.
func NewAccountEntryHandler(clients client.Service, loginUI string, logger *zap.Logger) *AccountEntryHandler {
	return &AccountEntryHandler{clients: clients, loginUI: loginUI, logger: logger}
}

// Get answers with the tenant's account screen client — registered the first
// time a user of the tenant gets here — and the application to go back to.
// Public: everything in the answer is what a browser sees anyway.
// GET /api/v1/account-entry?client_id=<the application's client_id>
func (h *AccountEntryHandler) Get(c *fiber.Ctx) error {
	clientID := c.Query("client_id")
	if clientID == "" {
		return apierror.RefuseWith(c, fiber.StatusBadRequest, "invalid_request", "client_id is required", fiber.Map{"field": "client_id"})
	}
	app, err := h.clients.GetByClientID(clientID)
	if err != nil || !app.Active {
		return apierror.Refuse(c, fiber.StatusNotFound, "unknown_client", "No application is registered with that client_id.")
	}
	account, err := h.clients.EnsureAccountClient(app.TenantID, h.loginUI)
	if err != nil {
		h.logger.Error("Failed to get the account screen client", zap.String("tenant_id", app.TenantID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadGateway, "account_unavailable", "The account screen is not available right now. Try again shortly.")
	}
	redirectURI := ""
	if len(account.RedirectURIs) > 0 {
		redirectURI = account.RedirectURIs[0]
	}
	return c.JSON(fiber.Map{
		"client_id":    account.ClientID,
		"redirect_uri": redirectURI,
		"scope":        "openid email profile",
		"app": fiber.Map{
			"name":       app.Name,
			"client_uri": app.ClientURI,
		},
	})
}
