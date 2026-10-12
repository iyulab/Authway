package mfa

import (
	"errors"
	"testing"
)

// With the second factor on, a call that needs nothing but the user's
// identity must not replace it: before this was refused, a second setup
// swapped the secret in use, so the caller's authenticator passed and the
// owner's no longer did.
func TestSetupTOTP_DoesNotReplaceAFactorThatIsOn(t *testing.T) {
	db := setupPostgres(t)
	svc, us, ts := newTestService(t, db)
	u := newTestUser(t, db, us, freshTenant(t, ts).ID)

	first, err := svc.SetupTOTP(u.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	// Before it is turned on, setting up again is how a user starts over.
	if first, err = svc.SetupTOTP(u.ID); err != nil {
		t.Fatalf("a second setup before enabling: %v", err)
	}
	if _, err := svc.VerifyAndEnable(u.ID, currentCode(t, first.Secret)); err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}

	if _, err := svc.SetupTOTP(u.ID); !errors.Is(err, ErrAlreadyEnabled) {
		t.Fatalf("a setup while the factor is on: %v, want ErrAlreadyEnabled", err)
	}
	if ok, err := svc.Verify(u.ID, currentCode(t, first.Secret)); err != nil || !ok {
		t.Errorf("after the refused setup the owner's authenticator no longer passes: %v, %v", ok, err)
	}
}

// Turning the factor off, or replacing the recovery codes, takes proof of the
// factor itself — a current code or an unused recovery code.
func TestSecondFactor_ChangesNeedProofOfTheFactor(t *testing.T) {
	db := setupPostgres(t)
	svc, us, ts := newTestService(t, db)
	u := newTestUser(t, db, us, freshTenant(t, ts).ID)

	setup, err := svc.SetupTOTP(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := svc.VerifyAndEnable(u.ID, currentCode(t, setup.Secret))
	if err != nil {
		t.Fatal(err)
	}

	for _, proof := range []string{"", "000000", "not-a-code"} {
		if proof == currentCode(t, setup.Secret) {
			continue
		}
		if err := svc.Disable(u.ID, proof); !errors.Is(err, ErrProofRequired) {
			t.Errorf("Disable with proof %q: %v, want ErrProofRequired", proof, err)
		}
		if _, err := svc.RegenerateRecoveryCodes(u.ID, proof); !errors.Is(err, ErrProofRequired) {
			t.Errorf("RegenerateRecoveryCodes with proof %q: %v, want ErrProofRequired", proof, err)
		}
	}
	if status, _ := svc.GetStatus(u.ID); status == nil || !status.Enabled || status.RecoveryCodesLeft != 8 {
		t.Fatalf("after refused changes: %+v, want the factor on with all 8 recovery codes", status)
	}

	// A recovery code is proof too, and is used up by it.
	regenerated, err := svc.RegenerateRecoveryCodes(u.ID, enabled.RecoveryCodes[0])
	if err != nil {
		t.Fatalf("RegenerateRecoveryCodes with a recovery code: %v", err)
	}
	if err := svc.Disable(u.ID, enabled.RecoveryCodes[1]); !errors.Is(err, ErrProofRequired) {
		t.Errorf("a recovery code from before the regeneration still counts: %v", err)
	}
	if err := svc.Disable(u.ID, regenerated.RecoveryCodes[0]); err != nil {
		t.Fatalf("Disable with a current recovery code: %v", err)
	}
	if status, _ := svc.GetStatus(u.ID); status == nil || status.Enabled {
		t.Errorf("after Disable: %+v, want the factor off", status)
	}

	// Off again, the user can set up a new authenticator.
	if _, err := svc.SetupTOTP(u.ID); err != nil {
		t.Errorf("SetupTOTP after turning the factor off: %v", err)
	}
}
