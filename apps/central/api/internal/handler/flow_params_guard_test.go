package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Flow ids are the authorization server's challenges, which can end in "="
// and arrive as %3D; Fiber does not decode path parameters. flowParam
// decodes them. A handler that reads Params("flow") itself works in tests
// whose challenge happens to have no "=", and fails for real users whose
// challenge does — it happened (the device flow, 2026-10-14). So no handler
// reads it any other way.
func TestFlowRoutesReadTheFlowThroughFlowParam(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "flow_params.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), `Params("flow")`) {
			t.Errorf("%s reads the flow id with Params(\"flow\"); use flowParam(c)", f)
		}
	}
}
