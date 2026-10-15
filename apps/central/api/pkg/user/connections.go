package user

import "errors"

// ErrNotLinked is a provider the account is not linked to.
var ErrNotLinked = errors.New("the account is not linked to this provider")

// ErrLastSignInMethod refuses to take away the account's only way to sign in.
var ErrLastSignInMethod = errors.New("this is the account's only way to sign in; set a password first")

// LinkedProviders names the sign-in providers the account is linked to, in a
// fixed order.
func (u *User) LinkedProviders() []string {
	var linked []string
	for _, p := range []struct {
		name string
		id   *string
	}{{"google", u.GoogleID}, {"github", u.GithubID}, {"microsoft", u.MicrosoftID}, {"apple", u.AppleID}} {
		if p.id != nil && *p.id != "" {
			linked = append(linked, p.name)
		}
	}
	return linked
}
