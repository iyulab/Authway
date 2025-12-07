package webhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service provides webhook management functionality
type Service interface {
	Create(tenantID uuid.UUID, req *CreateWebhookRequest) (*Webhook, error)
	GetByID(id uuid.UUID) (*Webhook, error)
	ListByTenant(tenantID uuid.UUID) ([]Webhook, error)
	Update(id uuid.UUID, req *UpdateWebhookRequest) (*Webhook, error)
	Delete(id uuid.UUID) error
	Trigger(tenantID uuid.UUID, eventType EventType, data interface{}) error
	GetDeliveries(webhookID uuid.UUID, limit int) ([]WebhookDelivery, error)
}

type service struct {
	db         *gorm.DB
	logger     *zap.Logger
	httpClient *http.Client
}

func NewService(db *gorm.DB, logger *zap.Logger) Service {
	return &service{
		db:     db,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type CreateWebhookRequest struct {
	Name        string   `json:"name" validate:"required,min=1,max=255"`
	URL         string   `json:"url" validate:"required,url"`
	Events      []string `json:"events" validate:"required,min=1"`
	RetryCount  int      `json:"retry_count"`
	TimeoutSecs int      `json:"timeout_secs"`
}

type UpdateWebhookRequest struct {
	Name        *string  `json:"name"`
	URL         *string  `json:"url"`
	Events      []string `json:"events"`
	Enabled     *bool    `json:"enabled"`
	RetryCount  *int     `json:"retry_count"`
	TimeoutSecs *int     `json:"timeout_secs"`
}

func generateSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *service) Create(tenantID uuid.UUID, req *CreateWebhookRequest) (*Webhook, error) {
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	retryCount := 3
	if req.RetryCount > 0 && req.RetryCount <= 10 {
		retryCount = req.RetryCount
	}
	timeoutSecs := 30
	if req.TimeoutSecs > 0 && req.TimeoutSecs <= 120 {
		timeoutSecs = req.TimeoutSecs
	}
	webhook := &Webhook{
		TenantID:    tenantID,
		Name:        req.Name,
		URL:         req.URL,
		Secret:      secret,
		Events:      req.Events,
		Enabled:     true,
		RetryCount:  retryCount,
		TimeoutSecs: timeoutSecs,
	}
	if err := s.db.Create(webhook).Error; err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}
	s.logger.Info("Webhook created", zap.String("webhook_id", webhook.ID.String()), zap.String("tenant_id", tenantID.String()))
	return webhook, nil
}

func (s *service) GetByID(id uuid.UUID) (*Webhook, error) {
	var webhook Webhook
	if err := s.db.Where("id = ? AND deleted_at IS NULL", id).First(&webhook).Error; err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}
	return &webhook, nil
}

func (s *service) ListByTenant(tenantID uuid.UUID) ([]Webhook, error) {
	var webhooks []Webhook
	if err := s.db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	return webhooks, nil
}

func (s *service) Update(id uuid.UUID, req *UpdateWebhookRequest) (*Webhook, error) {
	webhook, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.URL != nil {
		updates["url"] = *req.URL
	}
	if req.Events != nil {
		updates["events"] = req.Events
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.RetryCount != nil && *req.RetryCount >= 0 && *req.RetryCount <= 10 {
		updates["retry_count"] = *req.RetryCount
	}
	if req.TimeoutSecs != nil && *req.TimeoutSecs > 0 && *req.TimeoutSecs <= 120 {
		updates["timeout_secs"] = *req.TimeoutSecs
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := s.db.Model(webhook).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update webhook: %w", err)
		}
	}
	return s.GetByID(id)
}

func (s *service) Delete(id uuid.UUID) error {
	now := time.Now()
	if err := s.db.Model(&Webhook{}).Where("id = ?", id).Update("deleted_at", now).Error; err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	s.logger.Info("Webhook deleted", zap.String("webhook_id", id.String()))
	return nil
}

func (s *service) Trigger(tenantID uuid.UUID, eventType EventType, data interface{}) error {
	var webhooks []Webhook
	if err := s.db.Where("tenant_id = ? AND enabled = true AND deleted_at IS NULL", tenantID).Find(&webhooks).Error; err != nil {
		return fmt.Errorf("failed to fetch webhooks: %w", err)
	}
	for _, webhook := range webhooks {
		if !containsEvent(webhook.Events, string(eventType)) {
			continue
		}
		go s.deliverWebhook(webhook, eventType, data)
	}
	return nil
}

func containsEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event || e == "*" {
			return true
		}
	}
	return false
}

func (s *service) deliverWebhook(webhook Webhook, eventType EventType, data interface{}) {
	payload := WebhookPayload{
		ID:        uuid.New().String(),
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		TenantID:  webhook.TenantID.String(),
		Data:      data,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("Failed to marshal webhook payload", zap.Error(err))
		return
	}
	signature := SignPayload(payloadBytes, webhook.Secret)
	for attempt := 1; attempt <= webhook.RetryCount; attempt++ {
		delivery := WebhookDelivery{
			WebhookID:   webhook.ID,
			EventType:   string(eventType),
			Payload:     string(payloadBytes),
			Attempt:     attempt,
			DeliveredAt: time.Now(),
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(webhook.TimeoutSecs)*time.Second)
		req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(payloadBytes))
		if err != nil {
			cancel()
			delivery.ErrorMessage = err.Error()
			s.db.Create(&delivery)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-ID", webhook.ID.String())
		req.Header.Set("X-Webhook-Signature", signature)
		req.Header.Set("X-Webhook-Event", string(eventType))
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		resp, err := s.httpClient.Do(req)
		cancel()
		if err != nil {
			delivery.ErrorMessage = err.Error()
			s.db.Create(&delivery)
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
			continue
		}
		body, _ := readResponseBody(resp)
		delivery.StatusCode = resp.StatusCode
		delivery.ResponseBody = string(body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			delivery.Success = true
			s.db.Create(&delivery)
			s.logger.Info("Webhook delivered", zap.String("webhook_id", webhook.ID.String()), zap.String("event", string(eventType)), zap.Int("attempt", attempt))
			return
		}
		delivery.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
		s.db.Create(&delivery)
		time.Sleep(time.Duration(attempt*attempt) * time.Second)
	}
	s.logger.Warn("Webhook delivery failed after all retries", zap.String("webhook_id", webhook.ID.String()), zap.String("event", string(eventType)))
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return buf.Bytes(), nil
}

func (s *service) GetDeliveries(webhookID uuid.UUID, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var deliveries []WebhookDelivery
	if err := s.db.Where("webhook_id = ?", webhookID).Order("delivered_at DESC").Limit(limit).Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("failed to get deliveries: %w", err)
	}
	return deliveries, nil
}
