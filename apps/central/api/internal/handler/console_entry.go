package handler

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/console"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ConsoleEntryHandler answers the console's first question: which client does
// it sign console accounts in with?
type ConsoleEntryHandler struct {
	clients    client.Service
	console    *console.Service
	consoleURL string
	logger     *zap.Logger
}

// NewConsoleEntryHandler builds the handler. consoleURL is the console's
// public address; empty, console accounts have no sign-in.
func NewConsoleEntryHandler(clients client.Service, consoleService *console.Service, consoleURL string, logger *zap.Logger) *ConsoleEntryHandler {
	return &ConsoleEntryHandler{clients: clients, console: consoleService, consoleURL: consoleURL, logger: logger}
}

// Get answers with the console's client in the system tenant — registered the
// first time the console asks, and brought to the configured address after.
// Public: everything in the answer is what a browser sees anyway. 404
// console_sign_in_unavailable while the deployment names no console address or
// no system tenant: the console then signs in with the admin password only.
// GET /api/v1/console/entry
func (h *ConsoleEntryHandler) Get(c *fiber.Ctx) error {
	if h.consoleURL == "" {
		return unavailable(c, "This deployment names no console address (AUTHWAY_APP_CONSOLE_URL).")
	}
	systemTenant, err := h.console.SystemTenantID(c.UserContext())
	if errors.Is(err, console.ErrNoSystemTenant) {
		return unavailable(c, "This deployment has no system tenant: console accounts are users of it.")
	}
	if err != nil {
		h.logger.Error("Failed to read the system tenant", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to read the system tenant")
	}
	app, err := h.clients.EnsureConsoleClient(systemTenant, h.consoleURL)
	if err != nil {
		h.logger.Error("Failed to get the console's client", zap.String("tenant_id", systemTenant.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadGateway, "console_sign_in_unavailable", "Console sign-in is not available right now. Try again shortly.")
	}
	redirectURI := ""
	if len(app.RedirectURIs) > 0 {
		redirectURI = app.RedirectURIs[0]
	}
	return c.JSON(fiber.Map{
		"client_id":    app.ClientID,
		"redirect_uri": redirectURI,
		"scope":        "openid email profile",
		"audience":     client.ConsoleAudience,
	})
}

func unavailable(c *fiber.Ctx, detail string) error {
	return apierror.RefuseWith(c, fiber.StatusNotFound, "console_sign_in_unavailable", detail,
		fiber.Map{"hint": "Sign in with the admin password, or configure the console address and the system tenant."})
}
