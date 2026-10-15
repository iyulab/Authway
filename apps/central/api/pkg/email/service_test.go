package email

import (
	"strings"
	"testing"
)

// TestValidateEmail covers the hand-rolled validator's boundary conditions —
// it is not using validator/v10's `email` tag, so its accept/reject behavior
// has no other source of truth than this.
func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"valid simple", "user@example.com", true},
		{"valid with subdomain", "user@mail.example.com", true},
		{"valid with plus tag", "user+tag@example.com", true},
		{"missing at sign", "userexample.com", false},
		{"missing domain dot", "user@examplecom", false},
		{"empty local part", "@example.com", false},
		{"two at signs", "user@@example.com", false},
		{"domain too short", "user@a.b", true}, // len("a.b") == 3, the validator's own floor
		{"domain below floor", "user@ab", false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"trims surrounding whitespace", "  user@example.com  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateEmail(tt.email); got != tt.want {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

// TestRenderEmailTemplate_EmbedsLinkForBothKnownTypes guards the two
// template-type branches renderEmailTemplate switches on — an unrecognized
// templateType silently falls through to an empty tmpl string, so this also
// pins that "verification" and "reset" are in fact the two live spellings.
func TestRenderEmailTemplate_EmbedsLinkForBothKnownTypes(t *testing.T) {
	link := "https://auth.example.com/verify?token=abc123"

	verification := mustRender(t)(renderEmailTemplate("verification", link))
	if !strings.Contains(verification, link) {
		t.Error("expected the verification template to embed the link")
	}
	if !strings.Contains(verification, "이메일 인증") {
		t.Error("expected the verification template to use its own subject-matter heading")
	}

	reset := mustRender(t)(renderEmailTemplate("reset", link))
	if !strings.Contains(reset, link) {
		t.Error("expected the reset template to embed the link")
	}
	if !strings.Contains(reset, "비밀번호 재설정") {
		t.Error("expected the reset template to use its own subject-matter heading")
	}
	if reset == verification {
		t.Error("expected the two template types to render different HTML")
	}
}

// TestRenderInvitationTemplate_MessageIsConditional guards the {{if .Message}}
// branch — an invitation sent with no personal message must not render an
// empty message box, and one sent with a message must include it verbatim.
func TestRenderInvitationTemplate_MessageIsConditional(t *testing.T) {
	withoutMessage := mustRender(t)(renderInvitationTemplate("Alice", "Acme Corp", "", "https://app.example.com/invite/abc"))
	if strings.Contains(withoutMessage, `class="message"`) {
		t.Error("expected no message box when Message is empty")
	}
	if !strings.Contains(withoutMessage, "Alice") || !strings.Contains(withoutMessage, "Acme Corp") {
		t.Error("expected inviter name and tenant name to appear regardless of Message")
	}

	withMessage := mustRender(t)(renderInvitationTemplate("Alice", "Acme Corp", "Welcome aboard!", "https://app.example.com/invite/abc"))
	if !strings.Contains(withMessage, "Welcome aboard!") {
		t.Error("expected the personal message to be embedded when provided")
	}
}

// TestRenderMagicLinkTemplate_HeadlineDependsOnIsNewUser guards the
// new-vs-returning-user copy branch, which is the only externally visible
// difference SendMagicLinkEmail's isNewUser parameter produces.
func TestRenderMagicLinkTemplate_HeadlineDependsOnIsNewUser(t *testing.T) {
	link := "https://auth.example.com/magic?token=xyz"

	returning := mustRender(t)(renderMagicLinkTemplate(link, "BCDF-GHJK", false))
	if strings.Contains(returning, "환영합니다") {
		t.Error("expected the returning-user template to not use the welcome-new-user headline")
	}

	newUser := mustRender(t)(renderMagicLinkTemplate(link, "BCDF-GHJK", true))
	if !strings.Contains(newUser, "환영합니다") {
		t.Error("expected the new-user template to use the welcome headline")
	}
	if !strings.Contains(newUser, "BCDF-GHJK") || !strings.Contains(returning, "BCDF-GHJK") {
		t.Error("expected both variants to show the code")
	}
	if !strings.Contains(newUser, link) || !strings.Contains(returning, link) {
		t.Error("expected both variants to embed the link")
	}
}

// Mail clients drop or partly apply <style> blocks and paint links in their
// own colour, so a button styled only by a class came out as link-blue text
// on the button colour. Every message's call to action carries its colours
// inline, on the cell, the link and the text, and no message relies on a
// class for it.
func TestEveryMessageStylesItsButtonInline(t *testing.T) {
	link := "https://auth.example.com/x?token=abc"
	must := mustRender(t)
	messages := map[string]string{
		"verification": must(renderEmailTemplate("verification", link)),
		"reset":        must(renderEmailTemplate("reset", link)),
		"invitation":   must(renderInvitationTemplate("Alice", "Acme", "", link)),
		"magic link":   must(renderMagicLinkTemplate(link, "BCDF-GHJK", false)),
	}
	for name, html := range messages {
		if strings.Contains(html, `class="button"`) || strings.Contains(html, ".button") {
			t.Errorf("%s: the button still relies on a class", name)
		}
		for _, want := range []string{
			`href="` + link + `"`,
			`bgcolor="#`,
			`color:#ffffff;text-decoration:none`,
			`<span style="color:#ffffff;">`,
			`<meta name="color-scheme" content="light">`,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("%s: missing %s", name, want)
			}
		}
		if strings.Contains(html, "ZgotmplZ") {
			t.Errorf("%s: the template escaper rejected a value", name)
		}
	}
	if !strings.Contains(messages["reset"], `bgcolor="#DC2626"`) {
		t.Error("reset: expected its own (red) button colour")
	}
}

// An unknown message type is an error, not an empty message that would
// still be sent.
func TestRenderEmailTemplate_RefusesAnUnknownType(t *testing.T) {
	if html, err := renderEmailTemplate("welcome", "https://x.example"); err == nil || html != "" {
		t.Fatalf("expected an error and no body, got %q, %v", html, err)
	}
}

// mustRender unwraps a template render in a test: a message that fails to
// render fails the test.
func mustRender(t *testing.T) func(string, error) string {
	return func(html string, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return html
	}
}
