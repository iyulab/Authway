package delegation

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"authway/apps/central/api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeService struct {
	Service
	view    View
	revoked bool
}

func (f *fakeService) Find(context.Context, uuid.UUID) (*View, error) {
	if f.revoked {
		return nil, ErrNotFound
	}
	v := f.view
	return &v, nil
}

func (f *fakeService) Revoke(context.Context, uuid.UUID) error {
	f.revoked = true
	return nil
}

type fakeConsents struct {
	err     error
	subject string
	client  string
}

func (f *fakeConsents) RevokeClientConsent(subject, clientID string) error {
	f.subject, f.client = subject, clientID
	return f.err
}

func revokeAs(t *testing.T, h *Handler, user uuid.UUID, id uuid.UUID) int {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: middleware.ErrorHandler})
	app.Delete("/users/delegations/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user)
		return c.Next()
	}, h.RevokeMine)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodDelete, "/users/delegations/"+id.String(), nil))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

// Revoking a delegation withdraws the remembered consent first: otherwise the
// client's next sign-in for the user would be granted the delegation again
// without asking.
func TestRevoke_WithdrawsTheRememberedConsentFirst(t *testing.T) {
	user := uuid.New()
	view := View{ID: uuid.New(), UserID: user, ClientID: "orders-agent"}

	svc := &fakeService{view: view}
	consents := &fakeConsents{}
	if got := revokeAs(t, NewHandler(svc, consents, nil, zap.NewNop()), user, view.ID); got != fiber.StatusNoContent {
		t.Fatalf("status %d, want 204", got)
	}
	if consents.subject != user.String() || consents.client != "orders-agent" || !svc.revoked {
		t.Fatalf("consent for %q/%q, revoked %v", consents.subject, consents.client, svc.revoked)
	}

	svc = &fakeService{view: view}
	if got := revokeAs(t, NewHandler(svc, &fakeConsents{err: errors.New("down")}, nil, zap.NewNop()), user, view.ID); got != fiber.StatusBadGateway {
		t.Fatalf("status %d, want 502 when the consent cannot be withdrawn", got)
	}
	if svc.revoked {
		t.Fatal("the delegation was revoked although the consent behind it stands")
	}

	if got := revokeAs(t, NewHandler(&fakeService{view: view}, &fakeConsents{}, nil, zap.NewNop()), uuid.New(), view.ID); got != fiber.StatusNotFound {
		t.Fatalf("another user's delegation: %d, want 404", got)
	}
}
