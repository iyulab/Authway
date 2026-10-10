package audit

import (
	"time"

	"github.com/google/uuid"
)

// AuditAction represents the type of action being audited
type AuditAction string

const (
	ActionUserCreated          AuditAction = "user.created"
	ActionUserUpdated          AuditAction = "user.updated"
	ActionUserDeleted          AuditAction = "user.deleted"
	ActionUserLogin            AuditAction = "user.login"
	ActionUserLoginFailed      AuditAction = "user.login_failed"
	ActionUserLogout           AuditAction = "user.logout"
	ActionUserPasswordReset    AuditAction = "user.password_reset"
	ActionUserMFAEnabled       AuditAction = "user.mfa_enabled"
	ActionUserMFADisabled      AuditAction = "user.mfa_disabled"
	ActionUserMFAVerified      AuditAction = "user.mfa_verified"
	ActionUserMFAFailed        AuditAction = "user.mfa_failed"
	ActionUserEmailVerified    AuditAction = "user.email_verified"
	ActionClientCreated        AuditAction = "client.created"
	ActionClientUpdated        AuditAction = "client.updated"
	ActionClientDeleted        AuditAction = "client.deleted"
	ActionTenantCreated        AuditAction = "tenant.created"
	ActionTenantUpdated        AuditAction = "tenant.updated"
	ActionTenantDeleted        AuditAction = "tenant.deleted"
	ActionConsentGranted       AuditAction = "consent.granted"
	ActionConsentRevoked       AuditAction = "consent.revoked"
	ActionWebhookCreated       AuditAction = "webhook.created"
	ActionWebhookUpdated       AuditAction = "webhook.updated"
	ActionWebhookDeleted       AuditAction = "webhook.deleted"
	ActionServiceClientCreated AuditAction = "service_client.created"
	ActionServiceClientRevoked AuditAction = "service_client.revoked"
	ActionAdminLoginSuccess    AuditAction = "admin.login_success"
	ActionAdminLogout          AuditAction = "admin.logout"
)

// AuditSeverity represents the severity level of the audit event
type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "info"
	SeverityWarning  AuditSeverity = "warning"
	SeverityError    AuditSeverity = "error"
	SeverityCritical AuditSeverity = "critical"
)

// Actions lists every action an entry can carry, with what it records.
// Every action listed is one the API records: an action nothing records is
// not offered as a filter.
var Actions = []struct {
	Action      AuditAction
	Description string
}{
	{ActionUserCreated, "User account created"},
	{ActionUserUpdated, "User account updated"},
	{ActionUserDeleted, "User account deleted"},
	{ActionUserLogin, "User signed in"},
	{ActionUserLoginFailed, "Sign-in refused"},
	{ActionUserLogout, "User signed out"},
	{ActionUserPasswordReset, "Password reset with an emailed link"},
	{ActionUserMFAEnabled, "MFA enabled"},
	{ActionUserMFADisabled, "MFA disabled"},
	{ActionUserMFAVerified, "MFA code accepted"},
	{ActionUserMFAFailed, "MFA code refused"},
	{ActionUserEmailVerified, "Email address verified"},
	{ActionClientCreated, "OAuth client created"},
	{ActionClientUpdated, "OAuth client updated"},
	{ActionClientDeleted, "OAuth client deleted"},
	{ActionTenantCreated, "Tenant created"},
	{ActionTenantUpdated, "Tenant updated"},
	{ActionTenantDeleted, "Tenant deleted"},
	{ActionConsentGranted, "Consent granted"},
	{ActionConsentRevoked, "Consent revoked"},
	{ActionWebhookCreated, "Webhook created"},
	{ActionWebhookUpdated, "Webhook updated"},
	{ActionWebhookDeleted, "Webhook deleted"},
	{ActionServiceClientCreated, "Service client created"},
	{ActionServiceClientRevoked, "Service client revoked"},
	{ActionAdminLoginSuccess, "Admin console sign-in"},
	{ActionAdminLogout, "Admin console sign-out"},
}

// Severities lists the severities an entry can carry.
var Severities = []struct {
	Severity    AuditSeverity
	Description string
}{
	{SeverityInfo, "Informational events"},
	{SeverityWarning, "Warning events"},
	{SeverityError, "Error events"},
	{SeverityCritical, "Critical security events"},
}

func knownAction(a string) bool {
	for _, x := range Actions {
		if string(x.Action) == a {
			return true
		}
	}
	return false
}

func knownSeverity(v string) bool {
	for _, x := range Severities {
		if string(x.Severity) == v {
			return true
		}
	}
	return false
}

// MinRetentionDays is the shortest retention a purge accepts.
const MinRetentionDays = 30

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           uuid.UUID     `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID     `json:"tenant_id" gorm:"type:uuid;not null;index"`
	ActorID      *uuid.UUID    `json:"actor_id" gorm:"type:uuid;index"`
	ActorEmail   string        `json:"actor_email" gorm:"size:255"`
	ActorType    string        `json:"actor_type" gorm:"size:50"`
	Action       AuditAction   `json:"action" gorm:"size:100;not null;index"`
	Severity     AuditSeverity `json:"severity" gorm:"size:20;default:info"`
	ResourceType string        `json:"resource_type" gorm:"size:100"`
	ResourceID   string        `json:"resource_id" gorm:"size:255"`
	IPAddress    string        `json:"ip_address" gorm:"size:45"`
	UserAgent    string        `json:"user_agent" gorm:"size:512"`
	Details      string        `json:"details" gorm:"type:jsonb"`
	Success      bool          `json:"success"`
	ErrorMsg     string        `json:"error_msg" gorm:"type:text"`
	CreatedAt    time.Time     `json:"created_at" gorm:"index"`
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
	Details      map[string]any
	Success      bool
	ErrorMsg     string
}
