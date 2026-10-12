package database

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// What the binary says it ships is every migration file in the tree it was
// built from, in order — a deployment check reads this instead of the tree.
func TestShippedVersions_AreTheMigrationFilesInOrder(t *testing.T) {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	name := regexp.MustCompile(`^(\d+)_.+\.sql$`)
	for _, e := range entries {
		if m := name.FindStringSubmatch(e.Name()); m != nil {
			want = append(want, m[1])
		}
	}
	sort.Strings(want)

	got, err := ShippedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("shipped %v, want %v", got, want)
	}
}
