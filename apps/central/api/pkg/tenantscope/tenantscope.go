// Package tenantscope reads the tenant an admin call acts on.
//
// The admin middleware records it from the X-Tenant-ID header or the tenant_id
// query parameter; a user's access token carries it as a uuid. A call that
// needs a tenant and names none is a malformed request, not a failed
// authentication — answering 401 would tell the admin console its session
// ended and sign the administrator out.
package tenantscope

import (
	"authway/apps/central/api/pkg/apierror"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// LocalKey is the fiber.Ctx local the authentication middleware fills.
const LocalKey = "tenant_id"

// FromRequest returns the tenant the request names, or a refusal to return
// as-is: 400 tenant_required when none is named, 400 invalid_request when the
// value is not a tenant id.
func FromRequest(c *fiber.Ctx) (uuid.UUID, error) {
	switch v := c.Locals(LocalKey).(type) {
	case uuid.UUID:
		if v != uuid.Nil {
			return v, nil
		}
	case string:
		if v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return uuid.Nil, apierror.Reject(fiber.StatusBadRequest, "invalid_request", "the tenant id is not a valid id")
			}
			return id, nil
		}
	}
	return uuid.Nil, apierror.Reject(fiber.StatusBadRequest, "tenant_required",
		"name the tenant with the X-Tenant-ID header or the tenant_id query parameter")
}
