package mail

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestLocales_AreTheFilesInLocales(t *testing.T) {
	if got, want := Locales(), []string{"en", "ko"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Locales() = %v, want %v", got, want)
	}
	if DefaultLocale != "en" {
		t.Errorf("DefaultLocale = %q, want en", DefaultLocale)
	}
}

func TestNegotiate(t *testing.T) {
	for _, c := range []struct{ header, want string }{
		{"", "en"},
		{"ko", "ko"},
		{"ko-KR", "ko"},
		{"KO-kr", "ko"},
		{"ko_KR", "ko"},
		{"en-US", "en"},
		{"ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7", "ko"},
		{"en-US,en;q=0.9,ko;q=0.8", "en"},
		// weight beats position
		{"en;q=0.5, ko;q=0.9", "ko"},
		// equal weights keep the written order
		{"ko, en", "ko"},
		{"en, ko", "en"},
		// a language with no locale is skipped for the next one that has it
		{"fr-FR,fr;q=0.9,ko;q=0.8", "ko"},
		{"ja,zh;q=0.8", "en"},
		{"*", "en"},
		{"*;q=0.1", "en"},
		// q=0 refuses a language
		{"ko;q=0, en", "en"},
		{"ko;q=0", "en"},
		// a malformed weight counts as 1
		{"ko;q=abc", "ko"},
		{" ko ; q=0.8 ,en;q=0.9", "en"},
		{"garbage;;,,", "en"},
	} {
		if got := Negotiate(c.header); got != c.want {
			t.Errorf("Negotiate(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestPick(t *testing.T) {
	for _, c := range []struct {
		tag  string
		want string
		ok   bool
	}{
		{"ko", "ko", true},
		{"en", "en", true},
		{"ko-KR", "ko", true},
		{"KO_kr", "ko", true},
		{" en-US ", "en", true},
		// a language with no locale is refused, not answered in another one
		{"fr", "", false},
		{"fr-FR", "", false},
		{"", "", false},
		{"   ", "", false},
		{"*", "", false},
		// a list is a preference (Negotiate), not a language that was named
		{"ko, en", "", false},
		{"ko;q=0.9", "", false},
		{"garbage;;,,", "", false},
	} {
		got, ok := Pick(c.tag)
		if got != c.want || ok != c.ok {
			t.Errorf("Pick(%q) = (%q, %v), want (%q, %v)", c.tag, got, ok, c.want, c.ok)
		}
	}
}

func TestRender_ReadsALocaleTagOrAnAcceptLanguageValue(t *testing.T) {
	vars := map[string]any{"link": link}
	for locale, subject := range map[string]string{
		"ko":                      "Authway - 이메일 인증",
		"ko-KR":                   "Authway - 이메일 인증",
		"ko-KR,ko;q=0.9,en;q=0.8": "Authway - 이메일 인증",
		"en":                      "Authway - Verify your email",
		"en-GB":                   "Authway - Verify your email",
		"":                        "Authway - Verify your email",
		"../../etc/passwd":        "Authway - Verify your email",
	} {
		m, err := Render("verification", locale, vars)
		if err != nil || m.Subject != subject {
			t.Errorf("Render(%q) subject = %q (%v), want %q", locale, m.Subject, err, subject)
		}
	}
}

func TestRender_TheHTMLDeclaresItsLanguage(t *testing.T) {
	for locale, tag := range map[string]string{"ko": `<html lang="ko">`, "en": `<html lang="en">`} {
		m, err := Render("reset", locale, map[string]any{"link": link})
		if err != nil || !strings.Contains(m.HTML, tag) {
			t.Errorf("%s: no %s in the HTML body (%v)", locale, tag, err)
		}
	}
}

func TestRender_EnglishReadsAsEnglish(t *testing.T) {
	invite, err := Render("invitation", "en", map[string]any{"link": link, "inviterName": "Alice", "tenantName": "Acme", "message": "Welcome"})
	if err != nil {
		t.Fatal(err)
	}
	if invite.Subject != "Authway - Invitation to Acme" || !strings.Contains(invite.Text, "Alice invited you to the Acme workspace.") || !strings.Contains(invite.Text, "Message:\nWelcome") {
		t.Errorf("invitation: %q\n%s", invite.Subject, invite.Text)
	}
	newUser, err := Render("magiclink", "en", map[string]any{"link": link, "code": "BCDF-GHJK", "newUser": true, "organization": "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if newUser.Subject != "Authway - Your sign-up and sign-in link (Acme)" || !strings.Contains(newUser.HTML, "Welcome") {
		t.Errorf("magic link: %q", newUser.Subject)
	}
	for name, vars := range everyMessage() {
		m, err := Render(name, "en", vars)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, body := range []string{m.Subject, m.Text} {
			if hangul.MatchString(body) {
				t.Errorf("%s in en has Korean text: %q", name, body)
			}
		}
	}
}

var hangul = regexp.MustCompile(`[\x{1100}-\x{11FF}\x{3130}-\x{318F}\x{AC00}-\x{D7AF}]`)

// A string added to one locale and not the others would render as a missing
// string in a language: every locale carries the same keys, and each key the
// same values in its template (a value one language drops or invents is a
// translation that lost or made up a fact).
func TestFiles_EveryLocaleHasTheSameStringsAndValues(t *testing.T) {
	locales := Locales()
	base, err := readLocale(locales[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range locales[1:] {
		got, err := readLocale(other)
		if err != nil {
			t.Fatal(err)
		}
		if a, b := keysOf(base), keysOf(got); !reflect.DeepEqual(a, b) {
			t.Errorf("%s and %s have different strings:\n%v\n%v", locales[0], other, difference(a, b), difference(b, a))
			continue
		}
		for section, strs := range base {
			for key, src := range strs {
				if a, b := tagsOf(src), tagsOf(got[section][key]); !reflect.DeepEqual(a, b) {
					t.Errorf("%s.%s: %s uses %v, %s uses %v", section, key, locales[0], a, other, b)
				}
			}
		}
	}
}

func TestFiles_NoLocaleLeavesAStringEmptyOrUntranslated(t *testing.T) {
	en, _ := readLocale("en")
	ko, _ := readLocale("ko")
	for section, strs := range en {
		for key, enText := range strs {
			if strings.TrimSpace(enText) == "" || strings.TrimSpace(ko[section][key]) == "" {
				t.Errorf("%s.%s is empty in a locale", section, key)
			}
			if key != "lang" && enText == ko[section][key] {
				t.Errorf("%s.%s is the same in en and ko: %q", section, key, enText)
			}
			if hangul.MatchString(enText) {
				t.Errorf("%s.%s in en has Korean text", section, key)
			}
		}
	}
}

var tagPattern = regexp.MustCompile(`\{\{\s*[#^/&{]?\s*([\w.]+)\s*\}?\}\}`)

func tagsOf(src string) []string {
	seen := map[string]bool{}
	for _, m := range tagPattern.FindAllStringSubmatch(src, -1) {
		seen[m[1]] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysOf(all map[string]map[string]string) []string {
	var out []string
	for section, strs := range all {
		for key := range strs {
			out = append(out, section+"."+key)
		}
	}
	sort.Strings(out)
	return out
}

func difference(a, b []string) []string {
	in := map[string]bool{}
	for _, k := range b {
		in[k] = true
	}
	var out []string
	for _, k := range a {
		if !in[k] {
			out = append(out, k)
		}
	}
	return out
}
