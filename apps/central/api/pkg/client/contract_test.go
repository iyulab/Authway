package client

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The members a client registration or change accepts are the members the
// published contract declares for it — no more (a member the contract does
// not name is one no caller can rely on) and no fewer.
func TestRequestBodies_AreTheContractsOwn(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "packages", "contract", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// A path item holds its parameters beside its operations, so each
	// operation is decoded on its own.
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	type operation struct {
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Properties map[string]any `yaml:"properties"`
				} `yaml:"schema"`
			} `yaml:"content"`
		} `yaml:"requestBody"`
	}
	for _, tc := range []struct {
		path, method string
		body         any
	}{
		{"/api/v1/clients", "post", CreateClientRequest{}},
		{"/api/v1/clients/{id}", "put", UpdateClientRequest{}},
	} {
		node := doc.Paths[tc.path][tc.method]
		var op operation
		if err := node.Decode(&op); err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		declared := map[string]bool{}
		for name := range op.RequestBody.Content["application/json"].Schema.Properties {
			declared[name] = true
		}
		accepted := map[string]bool{}
		typ := reflect.TypeOf(tc.body)
		for i := 0; i < typ.NumField(); i++ {
			name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
			if name != "" && name != "-" {
				accepted[name] = true
			}
		}
		if missing, extra := diff(accepted, declared), diff(declared, accepted); len(missing) > 0 || len(extra) > 0 {
			t.Errorf("%s %s: accepted but not in the contract %v; in the contract but not accepted %v", tc.method, tc.path, missing, extra)
		}
	}
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
