package mail

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

const link = "https://login.example.com/x?token=abc&next=1"

func mustRender(t *testing.T, name string, vars map[string]any) Message {
	t.Helper()
	m, err := Render(name, DefaultLocale, vars)
	if err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return m
}

func everyMessage() map[string]map[string]any {
	return map[string]map[string]any{
		"verification": {"link": link},
		"reset":        {"link": link},
		"invitation":   {"link": link, "inviterName": "Alice", "tenantName": "Acme"},
		"magiclink":    {"link": link, "code": "BCDF-GHJK", "newUser": false},
	}
}

func TestRender_EveryMessageCarriesItsLinkInBothBodiesAndHasASubject(t *testing.T) {
	for name, vars := range everyMessage() {
		m := mustRender(t, name, vars)
		if m.Subject == "" {
			t.Errorf("%s: no subject", name)
		}
		if !strings.Contains(m.HTML, `href="https://login.example.com/x?token=abc&amp;next=1"`) {
			t.Errorf("%s: the HTML body has no escaped link:\n%s", name, m.HTML)
		}
		if !strings.Contains(m.Text, link) {
			t.Errorf("%s: the text body has no link:\n%s", name, m.Text)
		}
		if strings.Contains(m.Text, "<") || strings.Contains(m.Text, "{{") || strings.Contains(m.HTML, "{{") {
			t.Errorf("%s: markup or an unrendered tag:\n%s\n%s", name, m.Text, m.HTML)
		}
		if !strings.Contains(m.HTML, `bgcolor="#`) {
			t.Errorf("%s: the button is not styled inline", name)
		}
	}
}

func TestRender_NamesTheOrganizationOnlyWhenThereIsOne(t *testing.T) {
	with := mustRender(t, "reset", map[string]any{"link": link, "organization": "Acme <b>Labs</b>"})
	if with.Subject != "Authway - 비밀번호 재설정 (Acme <b>Labs</b>)" {
		t.Errorf("subject %q", with.Subject)
	}
	if !strings.Contains(with.HTML, "«Acme &lt;b&gt;Labs&lt;/b&gt;»") || strings.Contains(with.HTML, "<b>Labs") {
		t.Errorf("the organization is not named, escaped, in the HTML body:\n%s", with.HTML)
	}
	if !strings.Contains(with.Text, "«Acme <b>Labs</b>»") {
		t.Errorf("the organization is not named in the text body:\n%s", with.Text)
	}
	without := mustRender(t, "reset", map[string]any{"link": link, "organization": ""})
	if without.Subject != "Authway - 비밀번호 재설정" || strings.Contains(without.HTML, "의 계정에 대한 것입니다") || strings.Contains(without.Text, "의 계정에 대한 것입니다") {
		t.Errorf("an empty organization was named: %q\n%s", without.Subject, without.Text)
	}
}

func TestRender_TheInvitationShowsItsMessageOnlyWhenThereIsOne(t *testing.T) {
	without := mustRender(t, "invitation", map[string]any{"link": link, "inviterName": "Alice", "tenantName": "Acme"})
	if strings.Contains(without.HTML, `class="message"`) || strings.Contains(without.Text, "메시지:") {
		t.Error("an invitation without a message shows a message block")
	}
	with := mustRender(t, "invitation", map[string]any{"link": link, "inviterName": "Alice", "tenantName": "Acme", "message": "Welcome <aboard>"})
	if !strings.Contains(with.HTML, "Welcome &lt;aboard&gt;") || !strings.Contains(with.Text, "메시지:\nWelcome <aboard>") {
		t.Errorf("the message is missing:\n%s\n%s", with.HTML, with.Text)
	}
	if with.Subject != "Authway - Acme 초대" {
		t.Errorf("subject %q", with.Subject)
	}
}

func TestRender_TheMagicLinkWelcomesANewUser(t *testing.T) {
	returning := mustRender(t, "magiclink", map[string]any{"link": link, "code": "BCDF-GHJK", "newUser": false})
	newUser := mustRender(t, "magiclink", map[string]any{"link": link, "code": "BCDF-GHJK", "newUser": true})
	if returning.Subject != "Authway - 로그인 링크" || newUser.Subject != "Authway - 가입 및 로그인 링크" {
		t.Errorf("subjects %q, %q", returning.Subject, newUser.Subject)
	}
	if strings.Contains(returning.HTML, "환영합니다") || !strings.Contains(newUser.HTML, "환영합니다") {
		t.Error("the headline does not follow newUser")
	}
	if !strings.Contains(returning.HTML, "BCDF-GHJK") || !strings.Contains(returning.Text, "BCDF-GHJK") {
		t.Error("the code is missing")
	}
}

func TestRender_RefusesALinkThatIsNotAnHTTPAddress(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)", "/relative", "mailto:a@example.com", "https://"} {
		if _, err := Render("verification", DefaultLocale, map[string]any{"link": bad}); err == nil {
			t.Errorf("%q rendered", bad)
		}
	}
}

func TestRender_RefusesAnUnknownMessageAndFallsBackFromAnUnknownLocale(t *testing.T) {
	if _, err := Render("welcome", DefaultLocale, nil); err == nil {
		t.Error("an unknown message rendered")
	}
	if m, err := Render("verification", "xx", map[string]any{"link": link}); err != nil || m.Subject != "Authway - 이메일 인증" {
		t.Errorf("an unknown locale did not fall back: %q %v", m.Subject, err)
	}
}

// Every locale has the strings every message uses, and every message has a
// style: a file another implementation renders must be complete on its own.
func TestFiles_EveryLocaleAndStyleCoversEveryMessage(t *testing.T) {
	var styles map[string]any
	src, _ := files.ReadFile("messages/styles.json")
	if err := json.Unmarshal(src, &styles); err != nil {
		t.Fatal(err)
	}
	locales, _ := fs.Glob(files, "locales/*.json")
	if len(locales) == 0 {
		t.Fatal("no locales")
	}
	for name := range everyMessage() {
		if _, ok := styles[name]; !ok {
			t.Errorf("no style for %s", name)
		}
		for _, l := range locales {
			locale := strings.TrimSuffix(strings.TrimPrefix(l, "locales/"), ".json")
			if _, err := Render(name, locale, everyMessage()[name]); err != nil {
				t.Errorf("%s in %s: %v", name, locale, err)
			}
		}
	}
}
