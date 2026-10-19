package tenantscope

import (
	"sort"

	"authway/apps/central/api/pkg/apierror"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Role is what a caller may do in one tenant it administers.
type Role string

const (
	// RoleOwner administers the tenant, its settings and who else does.
	RoleOwner Role = "owner"
	// RoleAdmin administers the tenant's applications, users and the rest,
	// but not its settings or who administers it.
	RoleAdmin Role = "admin"
	// RoleService is a tenant's service credential: what it may do is its
	// granted scopes, checked by the route.
	RoleService Role = "service"
)

// Scope is the set of tenants the authenticated caller administers: every
// tenant (the admin key, an operator), or the ones it is a member of.
//
// The admin middleware sets one on every request it admits. Named,
// FromRequest and Admit read it, so a handler that asks them which tenant a
// call is about cannot be pointed at a tenant outside the caller's scope.
type Scope struct {
	all     bool
	tenants map[uuid.UUID]Role
}

// All is the scope of a caller that administers every tenant.
func All() Scope { return Scope{all: true} }

// Only is the scope of a caller that administers the given tenants.
func Only(tenants map[uuid.UUID]Role) Scope {
	copied := make(map[uuid.UUID]Role, len(tenants))
	for id, role := range tenants {
		copied[id] = role
	}
	return Scope{tenants: copied}
}

// IsAll reports whether the scope covers every tenant.
func (s Scope) IsAll() bool { return s.all }

// Covers reports whether the scope includes the tenant.
func (s Scope) Covers(id uuid.UUID) bool {
	if s.all {
		return true
	}
	_, ok := s.tenants[id]
	return ok
}

// Role returns the caller's role in the tenant. A caller that administers
// every tenant is an owner of each.
func (s Scope) Role(id uuid.UUID) (Role, bool) {
	if s.all {
		return RoleOwner, true
	}
	role, ok := s.tenants[id]
	return role, ok
}

// Tenants lists the tenants of a scope that does not cover every tenant, in
// a stable order.
func (s Scope) Tenants() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(s.tenants))
	for id := range s.tenants {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids
}

const scopeKey = "tenant_scope"

// SetScope records the caller's scope. When the scope holds exactly one
// tenant and the request names none, that tenant becomes the one the request
// is about — handlers that read the tenant local then see it too.
func SetScope(c *fiber.Ctx, s Scope) {
	c.Locals(scopeKey, s)
	if s.all || len(s.tenants) != 1 {
		return
	}
	if _, named, _ := named(c); !named {
		for id := range s.tenants {
			c.Locals(LocalKey, id)
		}
	}
}

// ScopeOf returns the caller's scope and whether the middleware set one.
func ScopeOf(c *fiber.Ctx) (Scope, bool) {
	s, ok := c.Locals(scopeKey).(Scope)
	return s, ok
}

// outside is the answer for a tenant outside the caller's scope: the same as
// for one that does not exist, so a caller learns nothing about tenants it
// does not administer.
func outside(message string) *apierror.Refusal {
	return apierror.Reject(fiber.StatusNotFound, "not_found", message)
}

// RequireAll admits only a caller that administers every tenant.
func RequireAll() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if s, ok := ScopeOf(c); ok && s.all {
			return c.Next()
		}
		return apierror.Refuse(c, fiber.StatusForbidden, "insufficient_scope",
			"Only an operator of this Authway, or its admin key, can do this.")
	}
}

// RequireOwnerOf admits a caller that owns the tenant named by the route
// parameter param — or, with param empty, the tenant the request names.
func RequireOwnerOf(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var id uuid.UUID
		if param != "" {
			parsed, err := uuid.Parse(c.Params(param))
			if err != nil {
				return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "the tenant id is not a valid id")
			}
			id = parsed
		} else {
			named, err := FromRequest(c)
			if err != nil {
				return apierror.Write(c, asRefusal(err))
			}
			id = named
		}
		s, _ := ScopeOf(c)
		role, ok := s.Role(id)
		if !ok {
			return apierror.Write(c, outside("tenant not found"))
		}
		if role != RoleOwner {
			return apierror.Refuse(c, fiber.StatusForbidden, "insufficient_scope",
				"Only an owner of the tenant can do this.")
		}
		return c.Next()
	}
}

// AdmitParam admits a request whose route parameter param names a tenant in
// the caller's scope. The path names the tenant the call is about; a tenant
// header beside it is not compared, as these routes never did.
func AdmitParam(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := uuid.Parse(c.Params(param))
		if err != nil {
			return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "the tenant id is not a valid id")
		}
		if s, _ := ScopeOf(c); !s.Covers(id) {
			return apierror.Write(c, outside("tenant not found"))
		}
		return c.Next()
	}
}

func asRefusal(err error) *apierror.Refusal {
	if r, ok := err.(*apierror.Refusal); ok {
		return r
	}
	return apierror.Reject(fiber.StatusInternalServerError, "internal_server_error", "the tenant could not be read")
}
