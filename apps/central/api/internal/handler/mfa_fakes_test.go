package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"authway/apps/central/api/pkg/claims"
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

// fakeUserService is a minimal in-memory user.Service — only GetByEmail and
// GetByID are exercised by AuthHandler; the rest are unused stubs.
type fakeUserService struct {
	byEmail map[string]*user.User
	byID    map[uuid.UUID]*user.User
}

func newFakeUserService(users ...*user.User) *fakeUserService {
	f := &fakeUserService{byEmail: map[string]*user.User{}, byID: map[uuid.UUID]*user.User{}}
	for _, u := range users {
		f.byEmail[u.Email] = u
		f.byID[u.ID] = u
	}
	return f
}

func (f *fakeUserService) Create(uuid.UUID, *user.CreateUserRequest) (*user.User, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserService) GetByID(id uuid.UUID) (*user.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("user not found")
}
func (f *fakeUserService) GetByEmail(email string) (*user.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("user not found")
}
func (f *fakeUserService) GetByEmailAndTenant(uuid.UUID, string) (*user.User, error) {
	return nil, fmt.Errorf("not implemented")
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
