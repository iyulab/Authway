package tenant

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// session_timeout decides how long a sign-in session lasts, so a value that
// could not be one is refused where it is set.
func TestTenantSettings_SessionTimeoutIsBounded(t *testing.T) {
	validate := validator.New()
	for minutes, ok := range map[int]bool{-1: false, 0: true, 1: true, 60: true, 43200: true, 43201: false, 525600: false} {
		settings := TenantSettings{SessionTimeout: minutes}
		create := validate.Struct(CreateTenantRequest{Name: "Tenant", Slug: "tenant", Settings: settings})
		update := validate.Struct(UpdateTenantRequest{Settings: &settings})
		if (create == nil) != ok || (update == nil) != ok {
			t.Errorf("session_timeout %d: create %v, update %v — want accepted=%v", minutes, create, update, ok)
		}
	}
}
