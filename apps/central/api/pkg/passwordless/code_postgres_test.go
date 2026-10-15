package passwordless

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/user"
)

// The code an emailed link carries does what the link does, typed into the
// screen that started the sign-in — for a reader on another device.

func (f *fixture) member(t *testing.T) string {
	t.Helper()
	email := fmt.Sprintf("code-%s@example.com", uuid.New().String()[:8])
	u, err := user.NewService(f.db, zap.NewNop()).Create(f.tenant.ID, &user.CreateUserRequest{
		Email: email, Password: "correct-horse-battery", Name: "Member",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { f.db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	return email
}

func (f *fixture) send(t *testing.T, email, flow string) {
	t.Helper()
	if _, err := f.svc.SendMagicLink(f.tenant.ID, email, flow, "127.0.0.1", "test"); err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Cleanup(func() { f.db.Exec(`DELETE FROM magic_link_tokens WHERE login_flow = ?`, flow) })
}

func TestMagicLinkCode_SignsInOnceForItsFlow(t *testing.T) {
	f := newFixture(t)
	email := f.member(t)
	flow := "flow-" + uuid.New().String()
	f.send(t, email, flow)

	code := f.sender.lastCode
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("the email shows %q, want XXXX-XXXX", code)
	}
	if !strings.Contains(f.sender.lastLinkURL, "code="+code) {
		t.Errorf("the link %q does not carry the code", f.sender.lastLinkURL)
	}

	if _, _, err := f.svc.VerifyMagicLinkCode("another-flow", code); err == nil {
		t.Fatal("a code signed in to a flow it was not sent for")
	}
	// Typed in lower case without the hyphen, it is the same code.
	link, u, err := f.svc.VerifyMagicLinkCode(flow, strings.ToLower(strings.ReplaceAll(code, "-", "")))
	if err != nil {
		t.Fatalf("right code: %v", err)
	}
	if u.Email != email || link.LoginFlow != flow {
		t.Errorf("signed in %s to %q", u.Email, link.LoginFlow)
	}
	if _, _, err := f.svc.VerifyMagicLinkCode(flow, code); err == nil {
		t.Error("the code signed in twice")
	}
	// The code used the link up too.
	if _, _, err := f.svc.VerifyMagicLink(f.lastToken(t)); err == nil {
		t.Error("the link still worked after its code was used")
	}
}

func TestMagicLinkCode_FiveWrongCodesEndIt_TheLinkStillWorks(t *testing.T) {
	f := newFixture(t)
	email := f.member(t)
	flow := "flow-" + uuid.New().String()
	f.send(t, email, flow)
	code := f.sender.lastCode

	wrong := "ZZZZ-ZZZZ"
	if code == wrong {
		wrong = "BBBB-BBBB"
	}
	for i := 0; i < maxCodeAttempts; i++ {
		if _, _, err := f.svc.VerifyMagicLinkCode(flow, wrong); err == nil {
			t.Fatal("a wrong code signed in")
		}
	}
	if _, _, err := f.svc.VerifyMagicLinkCode(flow, code); err == nil {
		t.Fatalf("the right code still worked after %d wrong ones", maxCodeAttempts)
	}
	if _, _, err := f.svc.VerifyMagicLink(f.lastToken(t)); err != nil {
		t.Errorf("the link stopped working with its code: %v", err)
	}
}

func TestMagicLinkCode_IsSealedAtRest(t *testing.T) {
	f := newFixture(t)
	email := f.member(t)
	flow := "flow-" + uuid.New().String()
	f.send(t, email, flow)
	code := strings.ReplaceAll(f.sender.lastCode, "-", "")

	var stored string
	if err := f.db.Raw(`SELECT code_sealed FROM magic_link_tokens WHERE login_flow = ?`, flow).Scan(&stored).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored == "" || strings.Contains(stored, code) {
		t.Errorf("code_sealed = %q holds the code %q in the clear", stored, code)
	}
}

func TestMagicLinkCode_EachLinkOfAFlowHasItsOwn(t *testing.T) {
	f := newFixture(t)
	email := f.member(t)
	flow := "flow-" + uuid.New().String()
	f.send(t, email, flow)
	first := f.sender.lastCode
	f.send(t, email, flow)
	second := f.sender.lastCode
	if first == second {
		t.Skip("two random codes matched")
	}
	// Asking for another email does not take back the first one.
	if _, _, err := f.svc.VerifyMagicLinkCode(flow, first); err != nil {
		t.Errorf("the first email's code: %v", err)
	}
}

func TestGeneratedCodesUseTheAlphabet(t *testing.T) {
	for i := 0; i < 200; i++ {
		c, err := generateCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != codeLength || strings.Trim(c, codeAlphabet) != "" {
			t.Fatalf("code %q is not %d letters from %s", c, codeLength, codeAlphabet)
		}
	}
}

// A mailbox gets a few links at a time, whoever asks; past that the answer is
// the same and no link is made.
func TestSendMagicLink_AMailboxGetsAFewAtATime(t *testing.T) {
	f := newFixture(t)
	email := f.member(t)
	flow := "flow-" + uuid.New().String()
	for i := 0; i < maxLinksPerMailbox; i++ {
		f.send(t, email, flow)
	}
	before := f.sender.lastLinkURL
	resp, err := f.svc.SendMagicLink(f.tenant.ID, strings.ToUpper(email), flow, "127.0.0.1", "test")
	if err != nil || resp == nil || resp.Message == "" {
		t.Fatalf("the answer past the limit must look like any other: %v %v", resp, err)
	}
	if n := f.linkCount(t, email); n != maxLinksPerMailbox {
		t.Errorf("%d links for one address, want %d", n, maxLinksPerMailbox)
	}
	if f.sender.lastLinkURL != before {
		t.Error("an email went out past the limit")
	}
}
