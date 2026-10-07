package client

import (
	"reflect"
	"strings"
	"testing"
)

// An editor loads a client, changes some fields and sends the whole form back.
// A field it may write but cannot read comes back as its zero value, so saving
// an untouched form silently resets it — skip_consent was lost this way. Every
// updatable field is therefore readable, except credentials, which are
// write-only by design.
func TestPublicClientReadsEveryUpdatableField(t *testing.T) {
	writeOnly := map[string]bool{
		"google_client_id": true, "google_client_secret": true,
		"microsoft_client_id": true, "microsoft_client_secret": true,
		"apple_client_id": true, "apple_team_id": true, "apple_key_id": true, "apple_private_key": true,
	}

	readable := jsonNames(reflect.TypeOf(PublicClient{}))
	for name := range jsonNames(reflect.TypeOf(UpdateClientRequest{})) {
		if writeOnly[name] {
			if readable[name] {
				t.Errorf("%s is a credential but PublicClient exposes it", name)
			}
			continue
		}
		if !readable[name] {
			t.Errorf("UpdateClientRequest accepts %s but PublicClient does not return it", name)
		}
	}
}

func jsonNames(t reflect.Type) map[string]bool {
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			names[tag] = true
		}
	}
	return names
}
