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
}

var (
	byPassword                = signInMethod{"password", []string{"pwd"}}
	byPasswordAndTOTP         = signInMethod{"password+totp", []string{"pwd", "otp", "mfa"}}
	byPasswordAndRecoveryCode = signInMethod{"password+recovery_code", []string{"pwd", "mfa"}}
	byEmailedLink             = signInMethod{"magic_link", []string{"email"}}
)

// amrFederated is the amr of a sign-in through a social provider.
var amrFederated = []string{"fed"}
