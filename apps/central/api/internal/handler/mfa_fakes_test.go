package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/mfa"
	"authway/apps/central/api/pkg/user"
)

// fakeMFAService is a minimal in-memory mfa.Service for handler tests — it
// exists to control Verify/VerifyRecoveryCode outcomes without a real DB or
// TOTP secret, not to re-test pkg/mfa's own logic.
type fakeMFAService struct {
	validTOTPCode     string
	validRecoveryCode string
	status            *mfa.MFAStatusResponse
}

func (f *fakeMFAService) SetupTOTP(uuid.UUID) (*mfa.TOTPSetupResponse, error) {
	return &mfa.TOTPSetupResponse{Secret: "SECRET", Issuer: "Authway"}, nil
}

func (f *fakeMFAService) VerifyAndEnable(uuid.UUID, string) (*mfa.RecoveryCodesResponse, error) {
	return &mfa.RecoveryCodesResponse{RecoveryCodes: []string{"AAAA-BBBB-CCCC"}}, nil
}

func (f *fakeMFAService) Verify(_ uuid.UUID, code string) (bool, error) {
	return code == f.validTOTPCode, nil
}

func (f *fakeMFAService) Disable(uuid.UUID) error { return nil }

func (f *fakeMFAService) VerifyRecoveryCode(_ uuid.UUID, code string) (bool, error) {
	return code == f.validRecoveryCode, nil
}

func (f *fakeMFAService) RegenerateRecoveryCodes(uuid.UUID) (*mfa.RecoveryCodesResponse, error) {
	return &mfa.RecoveryCodesResponse{RecoveryCodes: []string{"DDDD-EEEE-FFFF"}}, nil
}

func (f *fakeMFAService) GetStatus(uuid.UUID) (*mfa.MFAStatusResponse, error) {
	if f.status != nil {
		return f.status, nil
	}
	return &mfa.MFAStatusResponse{Enabled: true}, nil
}

// fakeUserService is a minimal in-memory user.Service — a flat slice, not a
// map keyed by email, because the whole point of GetByEmailAndTenant's
// regression coverage is two users sharing an email across tenants.
type fakeUserService struct {
	users []*user.User
}

func newFakeUserService(users ...*user.User) *fakeUserService {
	return &fakeUserService{users: users}
}

func (f *fakeUserService) Create(uuid.UUID, *user.CreateUserRequest) (*user.User, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserService) GetByID(id uuid.UUID) (*user.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}
func (f *fakeUserService) GetByEmailUnscoped(email string) (*user.User, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}
func (f *fakeUserService) GetByEmailAndTenant(tenantID uuid.UUID, email string) (*user.User, error) {
	for _, u := range f.users {
		if u.Email == email && u.TenantID == tenantID {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}
func (f *fakeUserService) GetByTenant(uuid.UUID, int, int) ([]*user.User, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func (f *fakeUserService) Update(uuid.UUID, *user.UpdateUserRequest) (*user.User, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserService) Delete(uuid.UUID) error { return fmt.Errorf("not implemented") }
func (f *fakeUserService) List(int, int) ([]*user.User, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func (f *fakeUserService) VerifyPassword(*user.User, string) bool { return false }
func (f *fakeUserService) ChangePassword(uuid.UUID, *user.ChangePasswordRequest) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeUserService) UpdateLastLogin(uuid.UUID) error           { return nil }
func (f *fakeUserService) UpdateEmailVerified(uuid.UUID, bool) error { return nil }
func (f *fakeUserService) UpdatePassword(uuid.UUID, string) error    { return nil }

// fakeClientService is a minimal in-memory client.Service — only
// GetByClientID is exercised by AuthHandler.Login.
type fakeClientService struct {
	byClientID map[string]*client.Client
	createFn   func(*client.CreateClientRequest) (*client.Client, *client.ClientCredentials, error)
}

func newFakeClientService(clients ...*client.Client) *fakeClientService {
	f := &fakeClientService{byClientID: map[string]*client.Client{}}
	for _, cl := range clients {
		f.byClientID[cl.ClientID] = cl
	}
	return f
}

func (f *fakeClientService) Create(req *client.CreateClientRequest) (*client.Client, *client.ClientCredentials, error) {
	if f.createFn != nil {
		return f.createFn(req)
	}
	return nil, nil, fmt.Errorf("not implemented")
}
func (f *fakeClientService) GetByID(uuid.UUID) (*client.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeClientService) GetByClientID(clientID string) (*client.Client, error) {
	if c, ok := f.byClientID[clientID]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("client not found")
}
func (f *fakeClientService) GetByTenant(uuid.UUID, int, int) ([]*client.Client, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func (f *fakeClientService) Update(uuid.UUID, *client.UpdateClientRequest) (*client.Client, client.SyncStatus, error) {
	return nil, client.SyncStatus{}, fmt.Errorf("not implemented")
}
func (f *fakeClientService) Delete(uuid.UUID) (client.SyncStatus, error) {
	return client.SyncStatus{}, fmt.Errorf("not implemented")
}
func (f *fakeClientService) List(int, int) ([]*client.Client, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func (f *fakeClientService) ValidateClient(string, string) (*client.Client, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeClientService) RegenerateSecret(uuid.UUID) (*client.ClientCredentials, client.SyncStatus, error) {
	return nil, client.SyncStatus{}, fmt.Errorf("not implemented")
}
func (f *fakeClientService) SyncAllClientsToHydra() (int, int, error) { return 0, 0, nil }

// fakeClaimsService is a no-op claims.Service — Login/completeLogin only call
// GetClaimsForLogin, and its result is merely logged (count), never branched
// on.
type fakeClaimsService struct{}

func (fakeClaimsService) UpdateClaims(context.Context, uuid.UUID, uuid.UUID, *claims.UpdateClaimsRequest) (*claims.UpdateClaimsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fakeClaimsService) GetClaims(context.Context, uuid.UUID, uuid.UUID) (*claims.GetClaimsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fakeClaimsService) DeleteClaim(context.Context, uuid.UUID, uuid.UUID, string) (*claims.DeleteClaimResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fakeClaimsService) UpdateUserClaims(context.Context, uuid.UUID, uuid.UUID, *claims.UpdateUserClaimsRequest) (*claims.UpdateUserClaimsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fakeClaimsService) GetUserClaims(context.Context, uuid.UUID, uuid.UUID) (*claims.GetUserClaimsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fakeClaimsService) GetClaimsForLogin(context.Context, uuid.UUID, uuid.UUID, string) (claims.ClaimMap, error) {
	return claims.ClaimMap{}, nil
}
func (fakeClaimsService) GetClaimsForConsent(context.Context, string, uuid.UUID, uuid.UUID, *claims.UserInfo) (claims.ClaimMap, error) {
	return claims.ClaimMap{}, nil
}
