// Package apierror is the API-boundary error-exposure convention:
// domain/validation errors are safe to return to a caller verbatim, but
// infrastructure errors (GORM, the Postgres driver, an HTTP client to
// another service) can leak schema/column/query internals if their raw
// .Error() text reaches an HTTP response.
//
// Public marks an error's message as author-reviewed and safe to expose. A
// handler that renders err.Error() unconditionally is fail-open — a new,
// unreviewed error path defaults to leaking. Message below is fail-closed
// instead: only an error that opted in via Public (directly, or wrapped
// inside a %w chain) surfaces its own text; everything else gets the
// caller-supplied generic fallback.
package apierror

import "errors"

// Public is an error whose .Error() text a service has explicitly reviewed
// as safe for an API response — a validation failure or business-rule
// rejection, never a raw driver/ORM error.
type Public struct {
	msg string
}

// NewPublic wraps msg as safe-to-expose. Construct it directly at the point
// the message is authored (e.g. `apierror.NewPublic("tenant not found")`),
// not by wrapping an arbitrary lower-level error — wrapping preserves
// whatever that error's own .Error() text says, which may itself be unsafe.
func NewPublic(msg string) *Public {
	return &Public{msg: msg}
}

func (e *Public) Error() string {
	return e.msg
}

// Message returns err's own text if err is (or wraps) a *Public, otherwise
// fallback. Handlers call this instead of err.Error() at every API-boundary
// response.
func Message(err error, fallback string) string {
	var pub *Public
	if errors.As(err, &pub) {
		return pub.Error()
	}
	return fallback
}
