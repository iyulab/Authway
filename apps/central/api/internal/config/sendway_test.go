package config

import (
	"strings"
	"testing"
)

// The Sendway base URL names a deployment, so it has no sensible default. With
// none configured the service would build request URLs against an empty host and
// every send would fail at runtime; Validate refuses it at startup instead.

func TestValidateRejectsSendwayWithoutBaseURL(t *testing.T) {
	c := validConfig()
	c.Email.UseSendway = true
	c.Email.SendwayBaseURL = " "

	err := c.Validate()
	if err == nil {
		t.Fatal("use_sendway without a base URL must be rejected")
	}
	if !strings.Contains(err.Error(), "AUTHWAY_EMAIL_SENDWAY_BASE_URL") {
		t.Errorf("error should name the env var an operator has to set, got: %v", err)
	}
}

func TestValidateAcceptsSendwayWithBaseURL(t *testing.T) {
	c := validConfig()
	c.Email.UseSendway = true
	c.Email.SendwayBaseURL = "sendway.example.com"

	if err := c.Validate(); err != nil {
		t.Fatalf("configured Sendway should validate, got: %v", err)
	}
}

func TestValidateIgnoresSendwayBaseURLWhenSMTPIsUsed(t *testing.T) {
	c := validConfig()
	c.Email.UseSendway = false

	if err := c.Validate(); err != nil {
		t.Fatalf("SMTP config needs no Sendway URL, got: %v", err)
	}
}
