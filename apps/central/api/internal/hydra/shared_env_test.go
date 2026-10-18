package hydra

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The authorization-server settings every deployment shares are written once,
// in configs/hydra.shared.env. They used to be repeated in the compose file,
// the deploy script and the CI workflow, and a setting added to two of them
// left the third behind. This keeps each of those reading the shared file and
// defining none of its keys again.
func TestSharedHydraSettingsAreWrittenOnce(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	read := func(rel string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(raw)
	}

	var keys []string
	for _, line := range strings.Split(read("configs/hydra.shared.env"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" || value == "" {
			t.Fatalf("configs/hydra.shared.env: not KEY=value: %q", line)
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		t.Fatal("configs/hydra.shared.env defines no settings")
	}

	// Each place that starts Hydra, and what shows it reads the shared file.
	starters := map[string]string{
		"docker-compose.yml":                                "./configs/hydra.shared.env",
		".github/workflows/ci.yml":                          "--env-file configs/hydra.shared.env",
		"scripts/deploy/_shared/lib/publish-hydra.core.ps1": "Get-HydraSharedEnv",
		// deploy/onprem/pack.sh copies the shared file next to it.
		"deploy/onprem/docker-compose.yml": "./hydra.shared.env",
	}
	// The one kind of exception: a protection that local development and CI
	// have to switch off because what it protects against is how they work.
	// Only those two may define the key, only with this value — a deployment
	// never does.
	developmentSetsTo := map[string]string{
		// Their back-channel logout receivers run on the same machine.
		"CLIENTS_HTTP_DISALLOW_PRIVATE_IP_RANGES": "false",
	}
	development := map[string]bool{"docker-compose.yml": true, ".github/workflows/ci.yml": true}

	for file, reads := range starters {
		text := read(file)
		if !strings.Contains(text, reads) {
			t.Errorf("%s no longer reads the shared settings (%q)", file, reads)
		}
		for _, key := range keys {
			// A definition: the key followed by `=` or `:` (compose) and its
			// value, not a mention in a comment.
			defined := regexp.MustCompile(`(?m)^[^#\n]*\b` + regexp.QuoteMeta(key) + `\s*[=:]\s*"?([^"\s\\]*)`)
			for _, m := range defined.FindAllStringSubmatch(text, -1) {
				if want, ok := developmentSetsTo[key]; ok && development[file] {
					if m[1] != want {
						t.Errorf("%s sets %s to %q; development may only set it to %q", file, key, m[1], want)
					}
					continue
				}
				t.Errorf("%s defines %s itself; it belongs in configs/hydra.shared.env only", file, key)
			}
		}
	}
	for key := range developmentSetsTo {
		if !slices.Contains(keys, key) {
			t.Errorf("%s is listed as a development exception but configs/hydra.shared.env no longer sets it", key)
		}
	}

	// The second CI job starts Hydra too.
	if n := strings.Count(read(".github/workflows/ci.yml"), "oryd/hydra:"); n != strings.Count(read(".github/workflows/ci.yml"), "--env-file configs/hydra.shared.env") {
		t.Errorf("ci.yml starts Hydra %d times but not every start reads the shared settings", n)
	}
}
