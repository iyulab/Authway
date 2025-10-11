package tenant

import "errors"

// Tenant-specific errors
var (
	// ErrNotFound is returned when a tenant is not found
	ErrNotFound = errors.New("tenant not found")

	// ErrDuplicateSlug is returned when attempting to create a tenant with an existing slug
	ErrDuplicateSlug = errors.New("tenant with this slug already exists")

	// ErrCannotDeleteDefault is returned when attempting to delete the default tenant
	ErrCannotDeleteDefault = errors.New("cannot delete default tenant")

	// ErrCannotDeactivateDefault is returned when attempting to deactivate the default tenant
	ErrCannotDeactivateDefault = errors.New("cannot deactivate default tenant")

	// ErrHasUsers is returned when attempting to delete a tenant with existing users
	ErrHasUsers = errors.New("cannot delete tenant with existing users")

	// ErrHasClients is returned when attempting to delete a tenant with existing clients
	ErrHasClients = errors.New("cannot delete tenant with existing clients")
)
