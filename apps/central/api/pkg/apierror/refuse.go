package apierror

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
)

// ContentType is the media type of every refusal (RFC 9457).
const ContentType = "application/problem+json"

// reserved are the members a refusal always writes itself; an extension may
// not replace them.
var reserved = map[string]bool{"type": true, "title": true, "status": true, "detail": true, "code": true}

// Refuse writes a refusal as RFC 9457 problem details, in the form the *way
// family shares: type "about:blank", title the status's reason phrase,
// detail a message for a person, and the extension member code — a stable
// identifier a caller branches on.
func Refuse(c *fiber.Ctx, status int, code, detail string) error {
	return RefuseWith(c, status, code, detail, nil)
}

// RefuseWith is Refuse with extension members beside code — field (the request
// member at fault), hint, retry_after and the like.
func RefuseWith(c *fiber.Ctx, status int, code, detail string, extensions fiber.Map) error {
	body := fiber.Map{
		"type":   "about:blank",
		"title":  utils.StatusMessage(status),
		"status": status,
		"detail": detail,
		"code":   code,
	}
	for k, v := range extensions {
		// An empty string says nothing; a member that is there should mean
		// something (a hint of "" is no hint).
		if s, isString := v.(string); reserved[k] || (isString && s == "") {
			continue
		}
		body[k] = v
	}
	// What the status itself obliges: a 401 names the scheme (RFC 9110
	// 15.5.2) — every credential this API takes is a bearer token — unless
	// the handler named a more specific challenge; a 429 that knows when to
	// come back says so (RFC 6585 4).
	if status == fiber.StatusUnauthorized && len(c.Response().Header.Peek(fiber.HeaderWWWAuthenticate)) == 0 {
		c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
	}
	if secs, ok := extensions["retry_after"].(int); ok && status == fiber.StatusTooManyRequests {
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(secs))
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	c.Status(status)
	c.Set(fiber.HeaderContentType, ContentType)
	return c.Send(raw)
}

// Refusal is a refusal carried as an error, for code that decides to refuse
// before it holds the response — a service judging what the caller sent, or a
// handler helper. Its text is worded for the caller. A handler returns it
// unchanged and the framework error handler writes it with Write.
type Refusal struct {
	// Status is the HTTP status; zero means 400 — a refusal of what was sent.
	Status int
	Code   string
	// Field is the request member at fault, when there is one, and Hint what
	// to do about it.
	Field      string
	Message    string
	Hint       string
	Extensions fiber.Map
}

func (r *Refusal) Error() string { return r.Message }

// Reject builds a Refusal.
func Reject(status int, code, message string) *Refusal {
	return &Refusal{Status: status, Code: code, Message: message}
}

// RejectField builds a 400 invalid_request Refusal of one request member.
func RejectField(field, message string) *Refusal {
	return &Refusal{Status: fiber.StatusBadRequest, Code: "invalid_request", Field: field, Message: message}
}

// Write answers with the refusal r.
func Write(c *fiber.Ctx, r *Refusal) error {
	status := r.Status
	if status == 0 {
		status = fiber.StatusBadRequest
	}
	extensions := fiber.Map{}
	for k, v := range r.Extensions {
		extensions[k] = v
	}
	if r.Field != "" {
		extensions["field"] = r.Field
	}
	if r.Hint != "" {
		extensions["hint"] = r.Hint
	}
	return RefuseWith(c, status, r.Code, r.Message, extensions)
}

// CodeForStatus names a refusal that carries only a status — one the framework
// produced (no route, wrong method, unreadable request) or a handler returned
// as fiber.NewError. A handler with a more specific reason uses its own code.
func CodeForStatus(status int) string {
	switch status {
	case fiber.StatusBadRequest:
		return "invalid_request"
	case fiber.StatusUnauthorized:
		return "unauthorized"
	case fiber.StatusForbidden:
		return "forbidden"
	case fiber.StatusNotFound:
		return "not_found"
	case fiber.StatusMethodNotAllowed:
		return "method_not_allowed"
	case fiber.StatusConflict:
		return "conflict"
	case fiber.StatusRequestEntityTooLarge:
		return "request_too_large"
	case fiber.StatusTooManyRequests:
		return "too_many_requests"
	case fiber.StatusBadGateway:
		return "bad_gateway"
	case fiber.StatusServiceUnavailable:
		return "service_unavailable"
	}
	if status >= 500 {
		return "internal_server_error"
	}
	return "request_error"
}

// Invalid refuses a request whose body failed validation: 400 invalid_request
// naming the first member at fault in field, never the validator's own text —
// that names Go struct fields and rules, not what the caller sent.
func Invalid(c *fiber.Ctx, err error) error {
	var fields validator.ValidationErrors
	if errors.As(err, &fields) && len(fields) > 0 {
		field := memberPath(fields[0].StructNamespace())
		detail := "Invalid value for " + field
		if fields[0].Tag() == "required" {
			detail = field + " is required"
		}
		return RefuseWith(c, fiber.StatusBadRequest, "invalid_request", detail, fiber.Map{"field": field})
	}
	return Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request")
}

// memberPath turns a validator namespace into the request member it names:
// the body's own type dropped, each Go field as the JSON member it is sent as,
// and an element's index dropped — the member is the list, as a refusal of one
// redirect URI names redirect_uris
// (CreateTenantRequest.Settings.AllowedDomains[1] → settings.allowed_domains).
func memberPath(namespace string) string {
	segments := strings.Split(namespace, ".")
	if len(segments) > 1 {
		segments = segments[1:]
	}
	for i, s := range segments {
		if open := strings.IndexByte(s, '['); open >= 0 {
			s = s[:open]
		}
		segments[i] = snakeCase(s)
	}
	return strings.Join(segments, ".")
}

// snakeCase turns a Go field name into the JSON member it is sent as
// (ClientID → client_id, RedirectURIs → redirect_uris).
func snakeCase(s string) string {
	runes := []rune(s)
	var words []string
	for i := 0; i < len(runes); {
		j := i + 1
		if unicode.IsUpper(runes[i]) {
			for j < len(runes) && unicode.IsUpper(runes[j]) {
				j++
			}
			switch {
			case j-i > 1 && j < len(runes) && runes[j] == 's' && (j+1 == len(runes) || unicode.IsUpper(runes[j+1])):
				j++ // an acronym's plural: URIs
			case j-i > 1 && j < len(runes) && unicode.IsLower(runes[j]):
				j-- // the last capital starts the next word: HTTPServer
			}
		}
		for j < len(runes) && !unicode.IsUpper(runes[j]) {
			j++
		}
		words = append(words, strings.ToLower(string(runes[i:j])))
		i = j
	}
	return strings.Join(words, "_")
}
