package handler

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

type recordingAccepter struct {
	challenge, code string
	err             error
}

func (r *recordingAccepter) AcceptUserCodeRequest(challenge, userCode string) (*hydra.LoginResponse, error) {
	r.challenge, r.code = challenge, userCode
	if r.err != nil {
		return nil, r.err
	}
	return &hydra.LoginResponse{RedirectTo: "https://issuer.example/oauth2/device/verify?x=1"}, nil
}

func submitDeviceCode(t *testing.T, accepter UserCodeAccepter, path, body string) (int, string) {
	t.Helper()
	app := fiber.New()
	app.Post("/api/v1/device-flows/:flow", NewDeviceFlowHandler(accepter, zap.NewNop()).SubmitUserCode)
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

// The authorization server's challenges can end in "=", which arrives as
// %3D in the path; the handler must hand the server the challenge itself.
func TestSubmitUserCode_DecodesTheFlowAndNormalizesTheCode(t *testing.T) {
	accepter := &recordingAccepter{}
	status, body := submitDeviceCode(t, accepter, "/api/v1/device-flows/abc-_123%3D", `{"user_code":" wxyz-abcd "}`)
	if status != 200 || !strings.Contains(body, `"next":"redirect"`) {
		t.Fatalf("got %d %s", status, body)
	}
	if accepter.challenge != "abc-_123=" {
		t.Fatalf("challenge sent to the authorization server: %q", accepter.challenge)
	}
	if accepter.code != "WXYZABCD" {
		t.Fatalf("code sent to the authorization server: %q", accepter.code)
	}
}

func TestSubmitUserCode_ExplainsARejectedCode(t *testing.T) {
	status, body := submitDeviceCode(t, &recordingAccepter{err: hydra.ErrUserCodeRejected}, "/api/v1/device-flows/f", `{"user_code":"WRONGONE"}`)
	if status != 400 || !strings.Contains(body, `"user_code_invalid"`) {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestSubmitUserCode_AsksForACodeFirst(t *testing.T) {
	accepter := &recordingAccepter{}
	status, body := submitDeviceCode(t, accepter, "/api/v1/device-flows/f", `{"user_code":"  "}`)
	if status != 400 || !strings.Contains(body, `"user_code_required"`) || accepter.challenge != "" {
		t.Fatalf("got %d %s (server asked: %v)", status, body, accepter.challenge != "")
	}
}
