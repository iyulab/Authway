package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/ephemeral"
)

// A few messages of a kind to one account in a short while, and then none:
// however many sources ask, the mailbox is not filled.
func TestMailPerAccount_StopsAfterAFewOfAKind(t *testing.T) {
	h := (&EmailHandler{logger: zap.NewNop()}).LimitsMailPerAccount(ephemeral.NewMemory(context.Background()))
	account, other := uuid.New(), uuid.New()

	for i := 1; i <= mailsPerAccount; i++ {
		if !h.mayMail("reset", account) {
			t.Fatalf("message %d to the account was refused, want the first %d sent", i, mailsPerAccount)
		}
	}
	if h.mayMail("reset", account) {
		t.Error("a message past the limit was sent")
	}
	if !h.mayMail("verification", account) {
		t.Error("the limit on resets also stopped the verification messages")
	}
	if !h.mayMail("reset", other) {
		t.Error("the limit on one account also stopped another's")
	}
}

// Without a store there is no limit to apply, and an unavailable store does not
// keep a person from getting their own mail.
func TestMailPerAccount_WithoutAStoreSendsEveryMessage(t *testing.T) {
	h := &EmailHandler{logger: zap.NewNop()}
	account := uuid.New()
	for i := 0; i < 10; i++ {
		if !h.mayMail("reset", account) {
			t.Fatalf("message %d refused with no limit set", i+1)
		}
	}
}
