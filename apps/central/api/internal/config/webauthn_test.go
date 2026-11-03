package config

import "testing"

func TestPasskeyRelyingParty(t *testing.T) {
	for name, tc := range map[string]struct {
		frontend, rpID string
		wantID         string
		wantOrigin     string
		wantErr        bool
	}{
		"the login UI's host by default":    {frontend: "https://auth.example.com", wantID: "auth.example.com", wantOrigin: "https://auth.example.com"},
		"a port is part of the origin only": {frontend: "http://localhost:3001", wantID: "localhost", wantOrigin: "http://localhost:3001"},
		"a domain above the host":           {frontend: "https://auth.example.com", rpID: "example.com", wantID: "example.com", wantOrigin: "https://auth.example.com"},
		"case is ignored":                   {frontend: "https://Auth.Example.com", rpID: "EXAMPLE.com", wantID: "example.com", wantOrigin: "https://Auth.Example.com"},
		"an id the host is not within":      {frontend: "https://auth.example.com", rpID: "other.example", wantErr: true},
		"a sibling is not above":            {frontend: "https://auth.example.com", rpID: "uth.example.com", wantErr: true},
		"an IP address cannot be an id":     {frontend: "https://10.0.0.5", wantErr: true},
		"a missing address":                 {frontend: "", wantErr: true},
		"not an http address":               {frontend: "ftp://auth.example.com", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := &Config{}
			c.App.FrontendURL = tc.frontend
			c.WebAuthn.RPID = tc.rpID
			id, origins, err := c.PasskeyRelyingParty()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got id %q, want an error", id)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id != tc.wantID || len(origins) != 1 || origins[0] != tc.wantOrigin {
				t.Errorf("got %q %v, want %q [%s]", id, origins, tc.wantID, tc.wantOrigin)
			}
		})
	}
}
