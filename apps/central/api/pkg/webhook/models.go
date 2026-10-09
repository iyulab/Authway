package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	EventSessionCreated      EventType = "session.created"
	EventSessionRevoked      EventType = "session.revoked"
	EventClientCreated       EventType = "client.created"
	EventClientUpdated       EventType = "client.updated"
	EventClientDeleted       EventType = "client.deleted"
	EventTypeTest            EventType = "test"

	// EventAll subscribes a webhook to every event.
	EventAll EventType = "*"
)

// Events lists what a webhook can subscribe to, with what triggers each.
var Events = []struct {
	Type        EventType
	Description string
}{
	{EventUserCreated, "Triggered when a new user is created"},
	{EventUserUpdated, "Triggered when a user is updated"},
	{EventUserDeleted, "Triggered when a user is deleted"},
	{EventUserLogin, "Triggered when a user logs in"},
	{EventUserLogout, "Triggered when a user logs out"},
	{EventUserPasswordChanged, "Triggered when a user changes their password"},
	{EventUserMFAEnabled, "Triggered when MFA is enabled"},
	{EventUserMFADisabled, "Triggered when MFA is disabled"},
	{EventSessionCreated, "Triggered when a new session is created"},
	{EventSessionRevoked, "Triggered when a session is revoked"},
	{EventClientCreated, "Triggered when a new OAuth client is created"},
	{EventClientUpdated, "Triggered when an OAuth client is updated"},
	{EventClientDeleted, "Triggered when an OAuth client is deleted"},
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
	ID          uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null;index"`
	Name        string     `json:"name" gorm:"size:255;not null"`
	URL         string     `json:"url" gorm:"size:2048;not null"`
	Secret      string     `json:"-" gorm:"size:255;not null"`
	// pq.StringArray, not []string: the column is Postgres text[], and a plain
	// []string is handed to the driver as a bare value it cannot encode
	// ("malformed array literal"), so every webhook insert failed. Clients
	// already use pq.StringArray for the same reason.
	Events      pq.StringArray `json:"events" gorm:"type:text[];not null"`
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
	Attempt      int       `json:"attempt" gorm:"default:1"`
	DeliveredAt  time.Time `json:"delivered_at"`
	Success      bool      `json:"success" gorm:"default:false"`
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
}

// WebhookPayload represents the standard webhook payload
type WebhookPayload struct {
	ID        string      `json:"id"`
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	TenantID  string      `json:"tenant_id"`
	Data      any `json:"data"`
}

// SignPayload generates HMAC-SHA256 signature for payload
func SignPayload(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature verifies the HMAC-SHA256 signature
func VerifySignature(payload []byte, signature, secret string) bool {
	expected := SignPayload(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}
