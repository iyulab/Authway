package audit

import (
	"time"

	"github.com/google/uuid"
)

// AuditAction represents the type of action being audited
type AuditAction string

const (
	ActionUserCreated         AuditAction = "user.created"
	ActionUserUpdated         AuditAction = "user.updated"
	ActionUserDeleted         AuditAction = "user.deleted"
	ActionUserLogin           AuditAction = "user.login"
	ActionUserLoginFailed     AuditAction = "user.login_failed"
	ActionUserLogout          AuditAction = "user.logout"
	ActionUserPasswordChanged AuditAction = "user.password_changed"
	ActionUserPasswordReset   AuditAction = "user.password_reset"
	ActionUserMFAEnabled      AuditAction = "user.mfa_enabled"
	ActionUserMFADisabled     AuditAction = "user.mfa_disabled"
	ActionUserMFAVerified     AuditAction = "user.mfa_verified"
	ActionUserMFAFailed       AuditAction = "user.mfa_failed"
	ActionUserEmailVerified   AuditAction = "user.email_verified"
	ActionUserLocked          AuditAction = "user.locked"
	ActionUserUnlocked        AuditAction = "user.unlocked"
	ActionSessionCreated      AuditAction = "session.created"
	ActionSessionRevoked      AuditAction = "session.revoked"
	ActionSessionExpired      AuditAction = "session.expired"
	ActionClientCreated       AuditAction = "client.created"
	ActionClientUpdated       AuditAction = "client.updated"
	ActionClientDeleted       AuditAction = "client.deleted"
	ActionTokenIssued         AuditAction = "token.issued"
	ActionTokenRefreshed      AuditAction = "token.refreshed"
	ActionTokenRevoked        AuditAction = "token.revoked"
	ActionConsentGranted      AuditAction = "consent.granted"
	ActionConsentRevoked      AuditAction = "consent.revoked"
	ActionWebhookCreated      AuditAction = "webhook.created"
	ActionWebhookUpdated      AuditAction = "webhook.updated"
	ActionWebhookDeleted      AuditAction = "webhook.deleted"
	ActionAdminAction         AuditAction = "admin.action"
)

// AuditSeverity represents the severity level of the audit event
type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "info"
	SeverityWarning  AuditSeverity = "warning"
	SeverityError    AuditSeverity = "error"
	SeverityCritical AuditSeverity = "critical"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID          uuid.UUID     `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID     `json:"tenant_id" gorm:"type:uuid;not null;index"`
	ActorID     *uuid.UUID    `json:"actor_id" gorm:"type:uuid;index"`
	ActorEmail  string        `json:"actor_email" gorm:"size:255"`
	ActorType   string        `json:"actor_type" gorm:"size:50"`
	Action      AuditAction   `json:"action" gorm:"size:100;not null;index"`
	Severity    AuditSeverity `json:"severity" gorm:"size:20;default:info"`
	ResourceType string       `json:"resource_type" gorm:"size:100"`
	ResourceID  string        `json:"resource_id" gorm:"size:255"`
	IPAddress   string        `json:"ip_address" gorm:"size:45"`
	UserAgent   string        `json:"user_agent" gorm:"size:512"`
	Details     string        `json:"details" gorm:"type:jsonb"`
	Success     bool          `json:"success" gorm:"default:true"`
	ErrorMsg    string        `json:"error_msg" gorm:"type:text"`
	CreatedAt   time.Time     `json:"created_at" gorm:"index"`
}

// AuditLogQuery represents query parameters for audit log search
type AuditLogQuery struct {
	TenantID     uuid.UUID
	ActorID      *uuid.UUID
	Action       AuditAction
	ResourceType string
	ResourceID   string
	Severity     AuditSeverity
	Success      *bool
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
}

// AuditEntry represents the data needed to create an audit log
type AuditEntry struct {
	TenantID     uuid.UUID
	ActorID      *uuid.UUID
	ActorEmail   string
	ActorType    string
	Action       AuditAction
	Severity     AuditSeverity
	ResourceType string
	ResourceID   string
	IPAddress    string
	UserAgent    string
	Details      map[string]interface{}
	Success      bool
	ErrorMsg     string
}
