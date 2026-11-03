package user

import "errors"

// ErrNotLinked is a provider the account is not linked to.
var ErrNotLinked = errors.New("the account is not linked to this provider")

// ErrSeveralLinked refuses to pick one of several identities at a provider
// from the provider's name alone.
var ErrSeveralLinked = errors.New("the account signs in with several accounts at this provider; name the one to remove")

// ErrLastSignInMethod refuses to take away the account's only way to sign in.
var ErrLastSignInMethod = errors.New("this is the account's only way to sign in; set a password first")
