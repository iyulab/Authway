package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service provides audit logging functionality
type Service interface {
	Log(ctx context.Context, entry *AuditEntry) error
	LogAsync(entry *AuditEntry)
	Query(query *AuditLogQuery) ([]AuditLog, int64, error)
	GetByID(id uuid.UUID) (*AuditLog, error)
	GetUserActivity(tenantID, userID uuid.UUID, limit int) ([]AuditLog, error)
	GetRecentSecurityEvents(tenantID uuid.UUID, hours int) ([]AuditLog, error)
	PurgeOldLogs(tenantID uuid.UUID, retentionDays int) (int64, error)
}

type service struct {
	db     *gorm.DB
	logger *zap.Logger
	async  chan *AuditEntry
}

func NewService(db *gorm.DB, logger *zap.Logger) Service {
	s := &service{
		db:     db,
		logger: logger,
		async:  make(chan *AuditEntry, 1000),
	}
	go s.processAsyncLogs()
	return s
}

func (s *service) processAsyncLogs() {
	for entry := range s.async {
		if err := s.Log(context.Background(), entry); err != nil {
			s.logger.Error("Failed to write async audit log", zap.Error(err), zap.String("action", string(entry.Action)))
		}
	}
}

func (s *service) Log(ctx context.Context, entry *AuditEntry) error {
	detailsJSON := "{}"
	if entry.Details != nil {
		if bytes, err := json.Marshal(entry.Details); err == nil {
			detailsJSON = string(bytes)
		}
	}
	severity := entry.Severity
	if severity == "" {
		severity = SeverityInfo
	}
	actorType := entry.ActorType
	if actorType == "" {
		if entry.ActorID != nil {
			actorType = "user"
		} else {
			actorType = "system"
		}
	}
	auditLog := &AuditLog{
		TenantID:     entry.TenantID,
		ActorID:      entry.ActorID,
		ActorEmail:   entry.ActorEmail,
		ActorType:    actorType,
		Action:       entry.Action,
		Severity:     severity,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		IPAddress:    entry.IPAddress,
		UserAgent:    entry.UserAgent,
		Details:      detailsJSON,
		Success:      entry.Success,
		ErrorMsg:     entry.ErrorMsg,
	}
	if err := s.db.WithContext(ctx).Create(auditLog).Error; err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}
	return nil
}

func (s *service) LogAsync(entry *AuditEntry) {
	select {
	case s.async <- entry:
	default:
		s.logger.Warn("Audit log buffer full, dropping log", zap.String("action", string(entry.Action)))
	}
}

func (s *service) Query(query *AuditLogQuery) ([]AuditLog, int64, error) {
	db := s.db.Model(&AuditLog{}).Where("tenant_id = ?", query.TenantID)
	if query.ActorID != nil {
		db = db.Where("actor_id = ?", *query.ActorID)
	}
	if query.Action != "" {
		db = db.Where("action = ?", query.Action)
	}
	if query.ResourceType != "" {
		db = db.Where("resource_type = ?", query.ResourceType)
	}
	if query.ResourceID != "" {
		db = db.Where("resource_id = ?", query.ResourceID)
	}
	if query.Severity != "" {
		db = db.Where("severity = ?", query.Severity)
	}
	if query.Success != nil {
		db = db.Where("success = ?", *query.Success)
	}
	if query.StartTime != nil {
		db = db.Where("created_at >= ?", *query.StartTime)
	}
	if query.EndTime != nil {
		db = db.Where("created_at <= ?", *query.EndTime)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var logs []AuditLog
	if err := db.Order("created_at DESC").Offset(query.Offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query audit logs: %w", err)
	}
	return logs, total, nil
}

func (s *service) GetByID(id uuid.UUID) (*AuditLog, error) {
	var log AuditLog
	if err := s.db.Where("id = ?", id).First(&log).Error; err != nil {
		return nil, fmt.Errorf("audit log not found: %w", err)
	}
	return &log, nil
}

func (s *service) GetUserActivity(tenantID, userID uuid.UUID, limit int) ([]AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var logs []AuditLog
	if err := s.db.Where("tenant_id = ? AND actor_id = ?", tenantID, userID).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to get user activity: %w", err)
	}
	return logs, nil
}

func (s *service) GetRecentSecurityEvents(tenantID uuid.UUID, hours int) ([]AuditLog, error) {
	if hours <= 0 || hours > 168 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	securityActions := []AuditAction{
		ActionUserLoginFailed,
		ActionUserLocked,
		ActionUserPasswordChanged,
		ActionUserMFADisabled,
		ActionSessionRevoked,
		ActionTokenRevoked,
	}
	var logs []AuditLog
	if err := s.db.Where("tenant_id = ? AND action IN ? AND created_at >= ?", tenantID, securityActions, since).Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to get security events: %w", err)
	}
	return logs, nil
}

func (s *service) PurgeOldLogs(tenantID uuid.UUID, retentionDays int) (int64, error) {
	if retentionDays < 30 {
		retentionDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result := s.db.Where("tenant_id = ? AND created_at < ?", tenantID, cutoff).Delete(&AuditLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to purge audit logs: %w", result.Error)
	}
	s.logger.Info("Purged old audit logs", zap.String("tenant_id", tenantID.String()), zap.Int64("deleted", result.RowsAffected), zap.Int("retention_days", retentionDays))
	return result.RowsAffected, nil
}
