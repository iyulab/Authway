package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
)

// UserCodeAccepter confirms the code a user typed for a device sign-in.
type UserCodeAccepter interface {
	AcceptUserCodeRequest(challenge, userCode string) (*hydra.LoginResponse, error)
}

// DeviceFlowHandler is the backend of the login UI's device page: a user
// signing in a TV, a console or a command-line tool (RFC 8628) types the
// code the device shows, and the sign-in goes on from there exactly like any
// other — the login and consent screens show which application it is.
type DeviceFlowHandler struct {
	hydra  UserCodeAccepter
	logger *zap.Logger
}

func NewDeviceFlowHandler(h UserCodeAccepter, logger *zap.Logger) *DeviceFlowHandler {
	return &DeviceFlowHandler{hydra: h, logger: logger}
}

// DeviceCodeSubmission is the body of POST /api/v1/device-flows/:flow.
type DeviceCodeSubmission struct {
	UserCode string `json:"user_code"`
}

// SubmitUserCode serves POST /api/v1/device-flows/:flow.
func (h *DeviceFlowHandler) SubmitUserCode(c *fiber.Ctx) error {
	flow := flowParam(c)
	var body DeviceCodeSubmission
	if err := c.BodyParser(&body); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid request body")
	}
	// The deployment issues codes of capital letters (the shared
	// authorization-server settings), so a code is read without regard to
	// case, as RFC 8628 §6.1 advises for a code copied from one screen to
	// another; spaces and dashes are how people group it while copying.
	code := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(body.UserCode)))
	if code == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "user_code_required", "Enter the code shown on your device.")
	}
	accepted, err := h.hydra.AcceptUserCodeRequest(flow, code)
	if err != nil {
		if errors.Is(err, hydra.ErrUserCodeRejected) {
			return apierror.Refuse(c, fiber.StatusBadRequest, "user_code_invalid", "That code is not valid. Check the code on your device, or start again there if it has expired.")
		}
		h.logger.Warn("device flow: accepting the user code failed", zap.Error(err))
		return respondFlowLookupError(c, err)
	}
	return c.JSON(fiber.Map{"next": "redirect", "redirect_to": accepted.RedirectTo})
}
