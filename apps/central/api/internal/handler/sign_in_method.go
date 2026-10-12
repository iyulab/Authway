package handler

// signInMethod is a way a user proves who they are: the name the audit log
// records it under, and the Authentication Method Reference values (the amr
// claim of the ID token) that tell a relying party the same thing.
//
// The values are those of RFC 8176 where one applies: pwd, otp, and mfa when
// more than one factor was used. It registers none for an emailed link or
// for another identity provider; those are "email" and "fed".
type signInMethod struct {
	audit string
	amr   []string
	// provider names the identity provider of a sign-in through one.
	provider string
}

var (
	byPassword    = signInMethod{audit: "password", amr: []string{"pwd"}}
	byEmailedLink = signInMethod{audit: "magic_link", amr: []string{"email"}}
)

// byProvider is a sign-in through another identity provider.
func byProvider(provider string) signInMethod {
	return signInMethod{audit: "social", amr: []string{"fed"}, provider: provider}
}

// withTOTP is m followed by a code from the user's authenticator app.
func (m signInMethod) withTOTP() signInMethod {
	return m.followedBy("totp", "otp", "mfa")
}

// withRecoveryCode is m followed by one of the user's recovery codes.
func (m signInMethod) withRecoveryCode() signInMethod {
	return m.followedBy("recovery_code", "mfa")
}

func (m signInMethod) followedBy(second string, amr ...string) signInMethod {
	return signInMethod{
		audit:    m.audit + "+" + second,
		amr:      append(append([]string{}, m.amr...), amr...),
		provider: m.provider,
	}
}

// firstFactor is the method a parked sign-in recorded as its first one. A
// sign-in parked before the method was recorded came in by password — the
// only way to reach the second step at the time.
func firstFactor(audit, provider string) signInMethod {
	switch audit {
	case byEmailedLink.audit:
		return byEmailedLink
	case "social":
		return byProvider(provider)
	default:
		return byPassword
	}
}
