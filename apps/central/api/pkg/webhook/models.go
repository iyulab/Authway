package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// EventType represents the type of webhook event
type EventType string

const (
	EventUserCreated         EventType = "user.created"
	EventUserUpdated         EventType = "user.updated"
	EventUserDeleted         EventType = "user.deleted"
	EventUserLogin           EventType = "user.login"
	EventUserLogout          EventType = "user.logout"
	EventUserPasswordChanged EventType = "user.password_changed"
	EventUserMFAEnabled      EventType = "user.mfa_enabled"
	EventUserMFADisabled     EventType = "user.mfa_disabled"
	EventUserSuspended       EventType = "user.suspended"
	EventUserReactivated     EventType = "user.reactivated"
	EventClientCreated       EventType = "client.created"
	EventClientUpdated       EventType = "client.updated"
	EventClientDeleted       EventType = "client.deleted"
	EventTypeTest            EventType = "test"

	// EventAll subscribes a webhook to every event.
	EventAll EventType = "*"
)

// Events lists what a webhook can subscribe to, with what triggers each.
// Every event listed here is sent: one that nothing sends is not offered.
var Events = []struct {
	Type        EventType
	Description string
}{
	{EventUserCreated, "A user account was created"},
	{EventUserUpdated, "A user account was changed by an administrator"},
	{EventUserDeleted, "A user account was deleted"},
	{EventUserLogin, "A user signed in"},
	{EventUserLogout, "A user signed out"},
	{EventUserPasswordChanged, "A user set a new password"},
	{EventUserMFAEnabled, "A user turned on a second factor"},
	{EventUserMFADisabled, "A user turned off a second factor"},
	{EventUserSuspended, "An administrator suspended a user account"},
	{EventUserReactivated, "An administrator lifted a user account's suspension"},
	{EventClientCreated, "An OAuth client was registered"},
	{EventClientUpdated, "An OAuth client was changed"},
	{EventClientDeleted, "An OAuth client was deleted"},
	{EventTypeTest, "Sent by the test endpoint"},
	{EventAll, "Every event"},
}

func knownEvent(e string) bool {
	for _, ev := range Events {
		if string(ev.Type) == e {
			return true
		}
	}
	return false
}

// Webhook represents a webhook endpoint configuration
type Webhook struct {
	ID       uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID uuid.UUID `json:"tenant_id" gorm:"type:uuid;not null;index"`
	Name     string    `json:"name" gorm:"size:255;not null"`
	URL      string    `json:"url" gorm:"size:2048;not null"`
	Secret   string    `json:"-" gorm:"size:255;not null"`
	// pq.StringArray, not []string: the column is Postgres text[], and a plain
	// []string is handed to the driver as a bare value it cannot encode
	// ("malformed array literal"), so every webhook insert failed. Clients
	// already use pq.StringArray for the same reason.
	Events pq.StringArray `json:"events" gorm:"type:text[];not null"`
	// No gorm default tags: GORM leaves a zero value out of the INSERT when the
	// field declares a default, so enabled=false and retry_count=0 were stored
	// as the column defaults. The service always sets all three.
	Enabled     bool       `json:"enabled"`
	RetryCount  int        `json:"retry_count"`
	TimeoutSecs int        `json:"timeout_secs"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"-" gorm:"index"`
}

// WebhookDelivery represents a webhook delivery attempt
type WebhookDelivery struct {
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	WebhookID    uuid.UUID `json:"webhook_id" gorm:"type:uuid;not null;index"`
	EventType    string    `json:"event_type" gorm:"size:100;not null"`
	Payload      string    `json:"payload" gorm:"type:text;not null"`
	StatusCode   int       `json:"status_code"`
	ResponseBody string    `json:"response_body" gorm:"type:text"`
	Attempt      int       `json:"attempt"`
	DeliveredAt  time.Time `json:"delivered_at"`
	Success      bool      `json:"success" gorm:"default:false"`
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
}

// WebhookPayload represents the standard webhook payload
type WebhookPayload struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	TenantID  string    `json:"tenant_id"`
	Data      any       `json:"data"`
}

// The headers every delivery carries, as Standard Webhooks names them
// (https://www.standardwebhooks.com). webhook-id is the event's id and stays
// the same across retries; webhook-timestamp is when this attempt was signed.
const (
	HeaderID        = "webhook-id"
	HeaderTimestamp = "webhook-timestamp"
	HeaderSignature = "webhook-signature"
)

// secretPrefix marks a signing secret: "whsec_" followed by the base64 of the
// HMAC key.
const secretPrefix = "whsec_"

// signingKey decodes a signing secret into its key bytes. Like the reference
// libraries it accepts the secret with or without the prefix.
func signingKey(secret string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, secretPrefix))
	if err != nil || len(key) == 0 {
		return nil, errors.New("webhook signing secret is not whsec_<base64>")
	}
	return key, nil
}

// signature is base64(HMAC-SHA256(key, "<msgID>.<t>.<body>")).
func signature(key []byte, msgID string, t int64, body []byte) string {
	h := hmac.New(sha256.New, key)
	fmt.Fprintf(h, "%s.%d.", msgID, t)
	h.Write(body)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Sign returns the webhook-signature value for a body with id msgID sent at t:
// "v1,<signature>". Signing the id and the time with the body lets a receiver
// refuse a delivery replayed later or moved onto another event.
func Sign(secret, msgID string, t int64, body []byte) (string, error) {
	key, err := signingKey(secret)
	if err != nil {
		return "", err
	}
	return "v1," + signature(key, msgID, t, body), nil
}

// Verify reports whether the Standard Webhooks headers in h sign body under
// secret, with a timestamp within tolerance of now. The signature header may
// list several space-separated signatures (as during a secret change); one
// that matches is enough. It is what a receiver written in Go runs.
func Verify(secret string, h http.Header, body []byte, tolerance time.Duration, now time.Time) bool {
	key, err := signingKey(secret)
	if err != nil {
		return false
	}
	msgID := h.Get(HeaderID)
	t, err := strconv.ParseInt(h.Get(HeaderTimestamp), 10, 64)
	if msgID == "" || err != nil {
		return false
	}
	age := now.Sub(time.Unix(t, 0))
	if age > tolerance || age < -tolerance {
		return false
	}
	want := signature(key, msgID, t, body)
	for _, sig := range strings.Fields(h.Get(HeaderSignature)) {
		version, value, ok := strings.Cut(sig, ",")
		if ok && version == "v1" && hmac.Equal([]byte(value), []byte(want)) {
			return true
		}
	}
	return false
}
