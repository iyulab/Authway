package mail

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

// golden.json holds what the messages render to for a fixed list of inputs. It
// belongs to the message files, not to this implementation: every
// implementation of Authway renders the same files and must produce exactly
// these texts. Regenerate it after changing a message file:
//
//	go test ./packages/mail -run TestGolden -update
var update = flag.Bool("update", false, "rewrite golden.json from the current files")

type goldenCase struct {
	Name    string         `json:"name"`
	Message string         `json:"message"`
	Locale  string         `json:"locale"`
	Vars    map[string]any `json:"vars"`
}

type goldenResult struct {
	goldenCase
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

// MarshalJSON keeps the case's members first, then what they render to.
func (r goldenResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name    string         `json:"name"`
		Message string         `json:"message"`
		Locale  string         `json:"locale"`
		Vars    map[string]any `json:"vars"`
		Subject string         `json:"subject"`
		HTML    string         `json:"html"`
		Text    string         `json:"text"`
	}{r.Name, r.Message, r.Locale, r.Vars, r.Subject, r.HTML, r.Text})
}

func goldenCases() []goldenCase {
	const link = "https://login.example.test/verify?token=abc123&next=%2Fhome"
	// Values that need escaping in HTML and none in text.
	const tricky = `Acme <b>"R&D"</b> 'Lab'`
	var cases []goldenCase
	for _, locale := range []string{"en", "ko", "ko-KR,ko;q=0.9,en;q=0.8", "fr"} {
		cases = append(cases,
			goldenCase{"verification", "verification", locale, map[string]any{"link": link, "organization": "Acme"}},
			goldenCase{"verification-no-organization", "verification", locale, map[string]any{"link": link}},
			goldenCase{"verification-tricky", "verification", locale, map[string]any{"link": link, "organization": tricky}},
			goldenCase{"reset", "reset", locale, map[string]any{"link": link, "organization": "Acme"}},
			goldenCase{"reset-no-organization", "reset", locale, map[string]any{"link": link}},
			goldenCase{"invitation", "invitation", locale, map[string]any{"link": link, "inviterName": "Dana", "tenantName": "Acme", "message": "Welcome aboard"}},
			goldenCase{"invitation-no-message", "invitation", locale, map[string]any{"link": link, "inviterName": "Dana", "tenantName": "Acme"}},
			goldenCase{"invitation-tricky", "invitation", locale, map[string]any{"link": link, "inviterName": tricky, "tenantName": tricky, "message": "Line one\nLine <two> & \"three\""}},
			goldenCase{"magiclink-existing", "magiclink", locale, map[string]any{"link": link, "organization": "Acme", "code": "BCDF-GHJK", "newUser": false}},
			goldenCase{"magiclink-new", "magiclink", locale, map[string]any{"link": link, "organization": "Acme", "code": "BCDF-GHJK", "newUser": true}},
		)
	}
	return cases
}

func TestGolden(t *testing.T) {
	var results []goldenResult
	for _, c := range goldenCases() {
		msg, err := Render(c.Message, c.Locale, c.Vars)
		if err != nil {
			t.Fatalf("%s (%s): %v", c.Name, c.Locale, err)
		}
		results = append(results, goldenResult{c, msg.Subject, msg.HTML, msg.Text})
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		t.Fatal(err)
	}

	if *update {
		if err := os.WriteFile("golden.json", out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile("golden.json")
	if err != nil {
		t.Fatalf("golden.json: %v (run with -update to create it)", err)
	}
	// Line endings are not part of what a message says.
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), out.Bytes()) {
		t.Fatal("the messages no longer render to golden.json: if the change is meant, run with -update, and every other implementation must still match the new file")
	}
}
