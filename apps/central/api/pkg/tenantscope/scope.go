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

// Scope is what the authenticated caller administers: the platform's own
// actions (creating, turning off and deleting tenants, choosing who
// administers them) or not, and the tenants whose contents — users,
// applications, audit log and the rest — it reaches: every tenant (the admin
// key, the console session), or the ones given.
//
// The admin middleware sets one on every request it admits. Named,
// FromRequest and Admit read it, so a handler that asks them which tenant a
// call is about cannot be pointed at a tenant outside the caller's scope.
type Scope struct {
	platform bool
	all      bool
	tenants  map[uuid.UUID]Role
}

// All is the scope of a caller that administers the platform and reaches
// into every tenant: the admin key and the console session.
func All() Scope { return Scope{platform: true, all: true} }

// Operator is the scope of an operator's console account: the platform's
// actions, and the contents of the given tenants only — those it is a member
// of and those it has opened support access to. An operator reaches into any
// other tenant by opening support access to it, which that tenant's audit log
// records.
func Operator(tenants map[uuid.UUID]Role) Scope {
	s := Only(tenants)
	s.platform = true
	return s
}

// Only is the scope of a caller that administers the given tenants.
func Only(tenants map[uuid.UUID]Role) Scope {
	copied := make(map[uuid.UUID]Role, len(tenants))
	for id, role := range tenants {
		copied[id] = role
	}
	return Scope{tenants: copied}
}

// IsAll reports whether the scope reaches into every tenant.
func (s Scope) IsAll() bool { return s.all }

// IsPlatform reports whether the caller may take the platform's own actions.
func (s Scope) IsPlatform() bool { return s.platform }

// Sees reports whether the caller may read or change the tenant's own record
// (its name, its settings, whether it is on) — the platform sees every
// tenant's, without reaching into its contents.
func (s Scope) Sees(id uuid.UUID) bool { return s.platform || s.Covers(id) }

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
	// The platform names the tenant it means: an operator with one tenant
	// open does not mean that tenant by naming none.
	if s.platform || len(s.tenants) != 1 {
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
// does not administer. The platform knows every tenant exists; it is told
// what it lacks.
func (s Scope) outside(message string) *apierror.Refusal {
	if s.platform {
		return apierror.Reject(fiber.StatusForbidden, "support_access_required",
			"Open support access to this tenant to reach into it; its audit log records that you did.")
	}
	return apierror.Reject(fiber.StatusNotFound, "not_found", message)
}

// RequirePlatform admits only a caller that may take the platform's own
// actions.
func RequirePlatform() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if s, ok := ScopeOf(c); ok && s.platform {
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
			return apierror.Write(c, s.outside("tenant not found"))
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
			return apierror.Write(c, s.outside("tenant not found"))
		}
		return c.Next()
	}
}

// AdmitRecord admits a request whose route parameter param names a tenant
// whose own record the caller sees (Scope.Sees) — the tenant itself, not what
// is in it.
func AdmitRecord(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := uuid.Parse(c.Params(param))
		if err != nil {
			return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "the tenant id is not a valid id")
		}
		if s, _ := ScopeOf(c); !s.Sees(id) {
			return apierror.Write(c, s.outside("tenant not found"))
		}
		return c.Next()
	}
}

// RequireRecordOwnerOf admits a caller that may change the own record of the
// tenant named by the route parameter param: the platform, or the tenant's
// owner.
func RequireRecordOwnerOf(param string) fiber.Handler {
	owner := RequireOwnerOf(param)
	return func(c *fiber.Ctx) error {
		if s, _ := ScopeOf(c); s.platform {
			if _, err := uuid.Parse(c.Params(param)); err != nil {
				return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "the tenant id is not a valid id")
			}
			return c.Next()
		}
		return owner(c)
	}
}

func asRefusal(err error) *apierror.Refusal {
	if r, ok := err.(*apierror.Refusal); ok {
		return r
	}
	return apierror.Reject(fiber.StatusInternalServerError, "internal_server_error", "the tenant could not be read")
}
