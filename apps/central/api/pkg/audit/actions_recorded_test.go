package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Every action an administrator can filter by must be one the API records.
// The list grew to ten actions nothing recorded — sessions, tokens, lockouts —
// and the security view queried four of them, so it could never show what its
// name promised. This walks the API's own source: an action counts as
// recorded when non-test code outside this package refers to its constant.
func TestEveryListedActionIsRecordedSomewhere(t *testing.T) {
	names := actionConstantNames(t)
	if len(names) != len(Actions) {
		t.Fatalf("found %d action constants for %d listed actions; every listed action needs its own constant", len(names), len(Actions))
	}

	used := map[string]bool{}
	root := filepath.Join("..", "..") // apps/central/api
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Not recordings: this package's own declarations and queries, and the
		// table that maps recorded actions to webhook events.
		dir := filepath.Base(filepath.Dir(path))
		if dir == "audit" || (dir == "webhook" && filepath.Base(path) == "events.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok { // audit.ActionX
				used[sel.Sel.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk the API source: %v", err)
	}

	for _, name := range names {
		if !used[name] {
			t.Errorf("%s is listed as an audit action but nothing records it", name)
		}
	}
}

// actionConstantNames returns the names of the AuditAction constants declared
// in models.go.
func actionConstantNames(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "models.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse models.go: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == "AuditAction" {
				for _, n := range vs.Names {
					names = append(names, n.Name)
				}
			}
		}
	}
	return names
}
