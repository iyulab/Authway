package config

import (
	"strings"
	"testing"
)

// Mail goes out through SMTP unless Sendway is chosen. Outside development a
// mail server on the container's loopback — the default left in place — means
// every invitation and sign-in link fails at its first send, long after the
// deployment looked healthy.

func productionConfig() *Config {
	c := validConfig()
	c.App.Environment = "production"
	c.App.FrontendURL = "https://login.example.com"
	c.Admin.APIKey = "k"
	c.Security.TOTPEncryptionKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
	c.Email.SMTPHost = "smtp.example.com"
	return c
}

func TestValidateRejectsLoopbackMailServerOutsideDevelopment(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", ""} {
		c := productionConfig()
		c.Email.SMTPHost = host
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "smtp_host") {
			t.Errorf("%q: want a refusal naming smtp_host, got %v", host, err)
		}
	}
}

func TestValidateAcceptsAMailServerOrSendway(t *testing.T) {
	if err := productionConfig().Validate(); err != nil {
		t.Fatalf("a named mail server: %v", err)
	}
	c := productionConfig()
	c.Email.SMTPHost = "localhost"
	c.Email.UseSendway = true
	c.Email.SendwayBaseURL = "https://mail.example.com"
	if err := c.Validate(); err != nil {
		t.Fatalf("Sendway chosen, SMTP left at its default: %v", err)
	}
	dev := validConfig()
	dev.Email.SMTPHost = "localhost"
	if err := dev.Validate(); err != nil {
		t.Fatalf("development with a local mail capture: %v", err)
	}
}

func TestValidateRejectsAnUnknownSMTPSecurity(t *testing.T) {
	c := validConfig()
	c.Email.SMTPSecurity = "ssl"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "smtp_security") {
		t.Fatalf("want a refusal naming smtp_security, got %v", err)
	}
	for _, ok := range []string{"", "tls", "starttls", "none"} {
		c := validConfig()
		c.Email.SMTPSecurity = ok
		if err := c.Validate(); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
}

// The settings are read from the environment the way a deployment gives them.
// Unmarshal fills only keys viper knows, so a key without a default would be
// silently ignored — as the SMTP username and password were.
func TestLoadReadsTheSMTPSettingsFromTheEnvironment(t *testing.T) {
	t.Setenv("AUTHWAY_APP_ENVIRONMENT", "development")
	t.Setenv("AUTHWAY_ADMIN_PASSWORD", "x")
	t.Setenv("AUTHWAY_EMAIL_SMTP_HOST", "smtp.example.com")
	t.Setenv("AUTHWAY_EMAIL_SMTP_PORT", "465")
	t.Setenv("AUTHWAY_EMAIL_SMTP_USER", "mailer")
	t.Setenv("AUTHWAY_EMAIL_SMTP_PASSWORD", "secret")
	t.Setenv("AUTHWAY_EMAIL_SMTP_SECURITY", "tls")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e := c.Email
	if e.SMTPHost != "smtp.example.com" || e.SMTPPort != 465 || e.SMTPUser != "mailer" || e.SMTPPassword != "secret" || e.SMTPSecurity != "tls" {
		t.Fatalf("email settings = host %q port %d user %q password set %v security %q", e.SMTPHost, e.SMTPPort, e.SMTPUser, e.SMTPPassword != "", e.SMTPSecurity)
	}
}
