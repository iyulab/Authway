package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// breachFake answers for a fixed set of passwords and counts what it was asked.
type breachFake struct {
	breached map[string]bool
	err      error
	asked    []string
}

func (f *breachFake) Breached(_ context.Context, password string) (bool, error) {
	f.asked = append(f.asked, password)
	return f.breached[password], f.err
}

func TestCheckPassword_RefusesABreachedPassword(t *testing.T) {
	fake := &breachFake{breached: map[string]bool{"correct-horse-9": true}}
	s := &service{logger: zap.NewNop(), breach: fake}

	err := s.checkPassword(uuid.New(), "correct-horse-9")
	var refused *PasswordError
	if !errors.As(err, &refused) || refused.Code() != "password_breached" {
		t.Fatalf("a breached password: %v, want a PasswordError with code password_breached", err)
	}
	if refused.MinLength != MinPasswordLength {
		t.Errorf("MinLength = %d, want %d (the answer still names the length rule)", refused.MinLength, MinPasswordLength)
	}
	if err := s.checkPassword(uuid.New(), "an-unlisted-passphrase"); err != nil {
		t.Errorf("a password that is not listed: %v, want it accepted", err)
	}
}

// The length rule comes first, and a password it refuses is never sent out.
func TestCheckPassword_AsksNoOneAboutAPasswordTheLengthRuleRefuses(t *testing.T) {
	fake := &breachFake{}
	s := &service{logger: zap.NewNop(), breach: fake}

	for name, password := range map[string]string{"short": "abc", "long": strings.Repeat("x", MaxPasswordLength+1)} {
		var refused *PasswordError
		if err := s.checkPassword(uuid.New(), password); !errors.As(err, &refused) || refused.Code() == "password_breached" {
			t.Errorf("%s: %v, want the length refusal", name, err)
		}
	}
	if len(fake.asked) != 0 {
		t.Errorf("the checker was asked about %d passwords the length rule had refused", len(fake.asked))
	}
}

// A check that cannot be made lets the password through and says so — without
// the password in what it says.
func TestCheckPassword_LetsAPasswordThroughWhenTheCheckFails(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	fake := &breachFake{err: errors.New("the range service answered 503")}
	s := &service{logger: zap.New(core), breach: fake}

	if err := s.checkPassword(uuid.New(), "correct-horse-9"); err != nil {
		t.Fatalf("with the service down: %v, want the password accepted", err)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("logged %d warnings, want 1", len(entries))
	}
	if strings.Contains(entries[0].Message, "correct-horse-9") || strings.Contains(entries[0].ContextMap()["error"].(string), "correct-horse-9") {
		t.Error("the warning carries the password")
	}
}

func TestCheckPassword_WithoutACheckerOnlyTheLengthRuleApplies(t *testing.T) {
	s := &service{logger: zap.NewNop()}
	if err := s.checkPassword(uuid.New(), "correct-horse-9"); err != nil {
		t.Errorf("no checker configured: %v, want the password accepted", err)
	}
}
