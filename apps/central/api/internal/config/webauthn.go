package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// WebAuthnConfig is what passkeys are scoped to.
type WebAuthnConfig struct {
	// RPID (AUTHWAY_WEBAUTHN_RP_ID) is the relying party id: the domain passkeys
	// are registered for. Empty: the host of app.frontend_url, the login UI —
	// which is the only page that asks for a passkey, so the narrowest domain
	// that works. A passkey is bound to it; changing it, or moving the login UI
	// to a host outside it, makes every registered passkey stop working.
	RPID string `mapstructure:"rp_id"`
}

// PasskeyRelyingParty returns the relying party id passkeys are registered for
// and the origins that may ask for one. An error means passkeys cannot be
// offered here: a login UI at an IP address (a relying party id is a domain), or
// an id the login UI's host is not within.
func (c *Config) PasskeyRelyingParty() (rpID string, origins []string, err error) {
	ui, err := url.Parse(c.App.FrontendURL)
	if err != nil || ui.Hostname() == "" || (ui.Scheme != "https" && ui.Scheme != "http") {
		return "", nil, fmt.Errorf("app.frontend_url %q is not an http(s) address", c.App.FrontendURL)
	}
	host := strings.ToLower(ui.Hostname())
	if net.ParseIP(host) != nil {
		return "", nil, fmt.Errorf("the login UI is at an IP address (%s); a passkey is bound to a domain name", host)
	}
	rpID = strings.ToLower(strings.TrimSpace(c.WebAuthn.RPID))
	if rpID == "" {
		rpID = host
	}
	if host != rpID && !strings.HasSuffix(host, "."+rpID) {
		return "", nil, fmt.Errorf("webauthn.rp_id %q is not the login UI's host %q or a domain above it", rpID, host)
	}
	return rpID, []string{ui.Scheme + "://" + ui.Host}, nil
}
