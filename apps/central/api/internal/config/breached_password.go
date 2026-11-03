package config

// BreachedPasswordConfig is the check of passwords that are being set against
// known data breaches.
type BreachedPasswordConfig struct {
	// Check (AUTHWAY_BREACHED_PASSWORD_CHECK) turns the check on. It is off by
	// default: a deployment asks nobody outside it anything until this is set.
	// A password someone chooses is refused when it is in a known breach
	// (400 password_breached); a password already set is not looked at again.
	Check bool `mapstructure:"check"`

	// APIURL (AUTHWAY_BREACHED_PASSWORD_API_URL) is the range service to ask: an
	// address that, followed by the first five characters of a SHA-1 hash,
	// answers the hash suffixes that begin so (the Pwned Passwords range API).
	// Only those five characters are sent. A deployment in a closed network
	// points it at a mirror inside.
	APIURL string `mapstructure:"api_url"`
}

// DefaultBreachedPasswordAPIURL is the public Pwned Passwords range API.
const DefaultBreachedPasswordAPIURL = "https://api.pwnedpasswords.com/range/"
