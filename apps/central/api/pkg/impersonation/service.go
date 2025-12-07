package impersonation

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service provides user impersonation functionality
type Service interface {
	StartImpersonation(tenantID, adminID uuid.UUID, req *StartImpersonationRequest, ipAddress, userAgent string) (*ImpersonationTokenResponse, error)
	ValidateToken(token string) (*ImpersonationSession, error)
	EndImpersonation(sessionID uuid.UUID) error
	GetActiveSessions(tenantID uuid.UUID) ([]ImpersonationSession, error)
	GetSessionHistory(tenantID uuid.UUID, limit int) ([]ImpersonationSession, error)
}

type service struct {
	db           *gorm.DB
	userService  user.Service
	auditService audit.Service
	logger       *zap.Logger
	maxDuration  time.Duration
}

func NewService(db *gorm.DB, userService user.Service, auditService audit.Service, logger *zap.Logger) Service {
	return &service{
		db:           db,
		userService:  userService,
		auditService: auditService,
		logger:       logger,
		maxDuration:  60 * time.Minute,
	}
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func (s *service) StartImpersonation(tenantID, adminID uuid.UUID, req *StartImpersonationRequest, ipAddress, userAgent string) (*ImpersonationTokenResponse, error) {
	admin, err := s.userService.GetByID(adminID)
	if err != nil {
		return nil, fmt.Errorf("admin not found: %w", err)
	}
	if admin.TenantID != tenantID {
		return nil, fmt.Errorf("admin does not belong to tenant")
	}
	targetUser, err := s.userService.GetByID(req.TargetUserID)
	if err != nil {
		return nil, fmt.Errorf("target user not found: %w", err)
	}
	if targetUser.TenantID != tenantID {
		return nil, fmt.Errorf("target user does not belong to tenant")
	}
	if targetUser.ID == adminID {
		return nil, fmt.Errorf("cannot impersonate yourself")
	}
	duration := time.Duration(req.Duration) * time.Minute
	if duration <= 0 || duration > s.maxDuration {
		duration = 30 * time.Minute
	}
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	session := &ImpersonationSession{
		TenantID:        tenantID,
		AdminID:         adminID,
		AdminEmail:      admin.Email,
		TargetUserID:    targetUser.ID,
		TargetUserEmail: targetUser.Email,
		Reason:          req.Reason,
		IPAddress:       ipAddress,
		UserAgent:       userAgent,
		Token:           token,
		Active:          true,
		StartedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(duration),
	}
	if err := s.db.Create(session).Error; err != nil {
		return nil, fmt.Errorf("failed to create impersonation session: %w", err)
	}
	s.auditService.LogAsync(&audit.AuditEntry{
		TenantID:     tenantID,
		ActorID:      &adminID,
		ActorEmail:   admin.Email,
		ActorType:    "admin",
		Action:       audit.ActionAdminAction,
		Severity:     audit.SeverityWarning,
		ResourceType: "user",
		ResourceID:   targetUser.ID.String(),
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Details:      map[string]interface{}{"action": "impersonation_started", "reason": req.Reason, "target_email": targetUser.Email},
		Success:      true,
	})
	s.logger.Info("Impersonation started", zap.String("admin_id", adminID.String()), zap.String("target_user_id", targetUser.ID.String()), zap.String("reason", req.Reason))
	resp := &ImpersonationTokenResponse{
		Token:     token,
		ExpiresAt: session.ExpiresAt,
	}
	resp.TargetUser.ID = targetUser.ID.String()
	resp.TargetUser.Email = targetUser.Email
	if targetUser.Name != nil { resp.TargetUser.Name = *targetUser.Name }
	return resp, nil
}

func (s *service) ValidateToken(token string) (*ImpersonationSession, error) {
	var session ImpersonationSession
	if err := s.db.Where("token = ? AND active = true", token).First(&session).Error; err != nil {
		return nil, fmt.Errorf("invalid impersonation token")
	}
	if session.IsExpired() {
		session.Active = false
		now := time.Now()
		session.EndedAt = &now
		s.db.Save(&session)
		return nil, fmt.Errorf("impersonation session expired")
	}
	return &session, nil
}

func (s *service) EndImpersonation(sessionID uuid.UUID) error {
	var session ImpersonationSession
	if err := s.db.Where("id = ? AND active = true", sessionID).First(&session).Error; err != nil {
		return fmt.Errorf("session not found or already ended")
	}
	now := time.Now()
	session.Active = false
	session.EndedAt = &now
	if err := s.db.Save(&session).Error; err != nil {
		return fmt.Errorf("failed to end impersonation session: %w", err)
	}
	s.auditService.LogAsync(&audit.AuditEntry{
		TenantID:     session.TenantID,
		ActorID:      &session.AdminID,
		ActorEmail:   session.AdminEmail,
		ActorType:    "admin",
		Action:       audit.ActionAdminAction,
		Severity:     audit.SeverityInfo,
		ResourceType: "user",
		ResourceID:   session.TargetUserID.String(),
		Details:      map[string]interface{}{"action": "impersonation_ended", "target_email": session.TargetUserEmail},
		Success:      true,
	})
	s.logger.Info("Impersonation ended", zap.String("session_id", sessionID.String()))
	return nil
}

func (s *service) GetActiveSessions(tenantID uuid.UUID) ([]ImpersonationSession, error) {
	var sessions []ImpersonationSession
	if err := s.db.Where("tenant_id = ? AND active = true AND expires_at > ?", tenantID, time.Now()).Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}
	return sessions, nil
}

func (s *service) GetSessionHistory(tenantID uuid.UUID, limit int) ([]ImpersonationSession, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var sessions []ImpersonationSession
	if err := s.db.Where("tenant_id = ?", tenantID).Order("started_at DESC").Limit(limit).Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("failed to get session history: %w", err)
	}
	return sessions, nil
}
