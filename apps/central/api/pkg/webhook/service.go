package webhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/outbound"
	"authway/apps/central/api/pkg/workqueue"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Limits on a webhook's delivery settings. A delivery makes 1 + RetryCount
// attempts, each allowed TimeoutSecs to answer.
const (
	DefaultRetryCount  = 3
	MaxRetryCount      = 10
	DefaultTimeoutSecs = 30
	MaxTimeoutSecs     = 60
)

// ErrNotFound reports that no live webhook has the requested id.
var ErrNotFound = errors.New("webhook not found")

// Service provides webhook management functionality
type Service interface {
	Create(tenantID uuid.UUID, req *CreateWebhookRequest) (*Webhook, error)
	GetByID(id uuid.UUID) (*Webhook, error)
	ListByTenant(tenantID uuid.UUID) ([]Webhook, error)
	Update(id uuid.UUID, req *UpdateWebhookRequest) (*Webhook, error)
	Delete(id uuid.UUID) error
	// Trigger queues a delivery of the event to each of the tenant's enabled
	// webhooks subscribed to it, and returns without waiting for any of them.
	Trigger(tenantID uuid.UUID, eventType EventType, data any) error
	// Close stops taking events and waits until the queued deliveries have
	// had their attempt, or until ctx ends.
	Close(ctx context.Context) error
	// Test sends one test event to the webhook, whatever it subscribes to and
	// whether or not it is enabled, and returns the recorded delivery.
	Test(id uuid.UUID) (*WebhookDelivery, error)
	GetDeliveries(webhookID uuid.UUID, limit int) ([]WebhookDelivery, error)
	// PurgeDeliveries removes the records of deliveries made before
	// olderThan ago — with them the receivers' answers.
	PurgeDeliveries(olderThan time.Duration) (int64, error)
	// RotateSecret replaces the signing secret and returns the new one.
	RotateSecret(id uuid.UUID) (string, error)
}

type service struct {
	db         *gorm.DB
	logger     *zap.Logger
	httpClient *http.Client
	policy     outbound.Policy
	// names looks up a receiver's host when it is registered.
	names outbound.Resolver
	// Deliveries run on a bounded queue: an event as frequent as a sign-in
	// must not start a goroutine per receiver, and a receiver that keeps
	// failing must not hold one through its retry waits.
	queue *workqueue.Queue
}

// Senders and queued deliveries a service starts with.
const (
	deliveryWorkers = 8
	deliveryQueue   = 1024
)

// Option configures a webhook service.
type Option func(*options)

type options struct {
	policy outbound.Policy
	names  outbound.Resolver
}

// AllowPrivateTargets lets deliveries reach this host and private networks —
// for local development, where receivers run on the same machine.
func AllowPrivateTargets(allow bool) Option {
	return func(o *options) { o.policy.AllowPrivate = allow }
}

func NewService(db *gorm.DB, logger *zap.Logger, opts ...Option) Service {
	o := options{names: net.DefaultResolver}
	for _, opt := range opts {
		opt(&o)
	}
	return &service{
		db:         db,
		logger:     logger,
		httpClient: deliveryClient(o.policy),
		policy:     o.policy,
		names:      o.names,
		queue:      workqueue.New(deliveryWorkers, deliveryQueue),
	}
}

func (s *service) Close(ctx context.Context) error {
	return s.queue.Close(ctx)
}

// CreateWebhookRequest omits nothing silently: a value outside its range is
// refused rather than replaced. Unset optional fields take their defaults.
type CreateWebhookRequest struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Events      []string `json:"events"`
	Enabled     *bool    `json:"enabled"`
	RetryCount  *int     `json:"retry_count"`
	TimeoutSecs *int     `json:"timeout_secs"`
}

type UpdateWebhookRequest struct {
	Name        *string  `json:"name"`
	URL         *string  `json:"url"`
	Events      []string `json:"events"`
	Enabled     *bool    `json:"enabled"`
	RetryCount  *int     `json:"retry_count"`
	TimeoutSecs *int     `json:"timeout_secs"`
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return apierror.RejectField("name", "name is required")
	}
	if len(name) > 255 {
		return apierror.RejectField("name", "name must be at most 255 characters")
	}
	return nil
}

// validateURL refuses what is plainly not deliverable. A host that is, or
// resolves to, a local or private address is refused here for a clear
// answer; since a name can resolve differently later, a delivery checks again
// when it connects.
func (s *service) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return apierror.RejectField("url", "url must be an absolute http or https URL")
	}
	if len(raw) > 2048 {
		return apierror.RejectField("url", "url must be at most 2048 characters")
	}
	names := s.names
	if names == nil {
		names = net.DefaultResolver
	}
	if s.policy.RefusesName(context.Background(), names, u.Hostname()) {
		return apierror.RejectField("url", "url must not point at a local or private address")
	}
	return nil
}

func validateEvents(events []string) error {
	if len(events) == 0 {
		return apierror.RejectField("events", "at least one event is required")
	}
	for _, e := range events {
		if !knownEvent(e) {
			return apierror.RejectField("events", fmt.Sprintf("unknown event %q; GET /api/v1/webhooks/events lists them", e))
		}
	}
	return nil
}

func validateRetryCount(n int) error {
	if n < 0 || n > MaxRetryCount {
		return apierror.RejectField("retry_count", fmt.Sprintf("retry_count must be between 0 and %d", MaxRetryCount))
	}
	return nil
}

func validateTimeout(n int) error {
	if n < 1 || n > MaxTimeoutSecs {
		return apierror.RejectField("timeout_secs", fmt.Sprintf("timeout_secs must be between 1 and %d", MaxTimeoutSecs))
	}
	return nil
}

// generateSecret issues a signing secret: 32 random key bytes in the Standard
// Webhooks form, "whsec_<base64>".
func generateSecret() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(key), nil
}

func (s *service) Create(tenantID uuid.UUID, req *CreateWebhookRequest) (*Webhook, error) {
	webhook := &Webhook{
		TenantID:    tenantID,
		Name:        req.Name,
		URL:         req.URL,
		Events:      req.Events,
		Enabled:     true,
		RetryCount:  DefaultRetryCount,
		TimeoutSecs: DefaultTimeoutSecs,
	}
	if req.Enabled != nil {
		webhook.Enabled = *req.Enabled
	}
	if req.RetryCount != nil {
		webhook.RetryCount = *req.RetryCount
	}
	if req.TimeoutSecs != nil {
		webhook.TimeoutSecs = *req.TimeoutSecs
	}
	if err := errors.Join(
		validateName(webhook.Name),
		s.validateURL(webhook.URL),
		validateEvents(webhook.Events),
		validateRetryCount(webhook.RetryCount),
		validateTimeout(webhook.TimeoutSecs),
	); err != nil {
		return nil, firstRefusal(err)
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	webhook.Secret = secret
	if err := s.db.Create(webhook).Error; err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}
	s.logger.Info("Webhook created", zap.String("webhook_id", webhook.ID.String()), zap.String("tenant_id", tenantID.String()))
	return webhook, nil
}

// firstRefusal returns the first refusal errors.Join collected, so a response
// names one problem in plain words.
func firstRefusal(err error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			if e != nil {
				return e
			}
		}
	}
	return err
}

func (s *service) GetByID(id uuid.UUID) (*Webhook, error) {
	var webhook Webhook
	err := s.db.Where("id = ? AND deleted_at IS NULL", id).First(&webhook).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	return &webhook, nil
}

func (s *service) ListByTenant(tenantID uuid.UUID) ([]Webhook, error) {
	webhooks := []Webhook{}
	if err := s.db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Order("created_at").Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	return webhooks, nil
}

func (s *service) Update(id uuid.UUID, req *UpdateWebhookRequest) (*Webhook, error) {
	webhook, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	updates := make(map[string]any)
	var errs []error
	if req.Name != nil {
		errs = append(errs, validateName(*req.Name))
		updates["name"] = *req.Name
	}
	if req.URL != nil {
		errs = append(errs, s.validateURL(*req.URL))
		updates["url"] = *req.URL
	}
	if req.Events != nil {
		errs = append(errs, validateEvents(req.Events))
		// text[] needs pq.StringArray; a plain []string fails to encode.
		updates["events"] = pq.StringArray(req.Events)
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.RetryCount != nil {
		errs = append(errs, validateRetryCount(*req.RetryCount))
		updates["retry_count"] = *req.RetryCount
	}
	if req.TimeoutSecs != nil {
		errs = append(errs, validateTimeout(*req.TimeoutSecs))
		updates["timeout_secs"] = *req.TimeoutSecs
	}
	if err := errors.Join(errs...); err != nil {
		return nil, firstRefusal(err)
	}
	if len(updates) > 0 {
		if err := s.db.Model(webhook).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update webhook: %w", err)
		}
	}
	return s.GetByID(id)
}

func (s *service) Delete(id uuid.UUID) error {
	now := time.Now()
	res := s.db.Model(&Webhook{}).Where("id = ? AND deleted_at IS NULL", id).Update("deleted_at", now)
	if res.Error != nil {
		return fmt.Errorf("failed to delete webhook: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Trigger does no work itself: finding the tenant's webhooks is queued along
// with the deliveries, so whoever raises an event — the audit log, as it
// records one — is never held up by the database or by a receiver.
func (s *service) Trigger(tenantID uuid.UUID, eventType EventType, data any) error {
	err := s.queue.Submit(func(int) time.Duration {
		var webhooks []Webhook
		if err := s.db.Where("tenant_id = ? AND enabled = true AND deleted_at IS NULL", tenantID).Find(&webhooks).Error; err != nil {
			s.logger.Error("Failed to fetch webhooks for an event", zap.String("event", string(eventType)), zap.Error(err))
			return workqueue.Done
		}
		for _, webhook := range webhooks {
			if containsEvent(webhook.Events, string(eventType)) {
				s.deliver(webhook, eventType, data)
			}
		}
		return workqueue.Done
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to queue webhook event %s: %w", eventType, err)
	}
	return nil
}

func (s *service) Test(id uuid.UUID) (*WebhookDelivery, error) {
	webhook, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	payload, msgID, err := buildPayload(*webhook, EventTypeTest, map[string]any{
		"test":    true,
		"message": "This is a test webhook delivery",
	})
	if err != nil {
		return nil, err
	}
	delivery := s.attempt(*webhook, EventTypeTest, msgID, payload, 1)
	return &delivery, nil
}

func containsEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event || e == string(EventAll) {
			return true
		}
	}
	return false
}

// buildPayload builds the body once per event and returns it with the event's
// id, so every attempt carries the same id (the body's and webhook-id); each
// attempt signs it afresh with its own time.
func buildPayload(webhook Webhook, eventType EventType, data any) ([]byte, string, error) {
	id := uuid.New().String()
	payload, err := json.Marshal(WebhookPayload{
		ID:        id,
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		TenantID:  webhook.TenantID.String(),
		Data:      data,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal webhook payload: %w", err)
	}
	return payload, id, nil
}

// deliver queues the first attempt at webhook; up to RetryCount more follow,
// each 1, 4, 9, … seconds after the one before, stopping at the first 2xx
// answer.
func (s *service) deliver(webhook Webhook, eventType EventType, data any) {
	payload, msgID, err := buildPayload(webhook, eventType, data)
	if err != nil {
		s.logger.Error("Failed to build webhook payload", zap.Error(err))
		return
	}
	fields := []zap.Field{zap.String("webhook_id", webhook.ID.String()), zap.String("event", string(eventType))}
	attempts := 1 + webhook.RetryCount
	err = s.queue.Submit(func(attempt int) time.Duration {
		if s.attempt(webhook, eventType, msgID, payload, attempt).Success {
			s.logger.Info("Webhook delivered", append(fields, zap.Int("attempt", attempt))...)
			return workqueue.Done
		}
		if attempt >= attempts {
			s.logger.Warn("Webhook delivery failed after all retries", fields...)
			return workqueue.Done
		}
		return time.Duration(attempt*attempt) * time.Second
	}, func(reason string) {
		s.logger.Warn("Webhook delivery given up; "+reason, fields...)
	})
	if err != nil {
		s.logger.Warn("Webhook delivery not queued", append(fields, zap.Error(err))...)
	}
}

// attempt posts the payload once and records the outcome. It returns the
// recorded row, so the caller sees the id the database gave it.
func (s *service) attempt(webhook Webhook, eventType EventType, msgID string, payload []byte, n int) WebhookDelivery {
	delivery := WebhookDelivery{
		WebhookID:   webhook.ID,
		EventType:   string(eventType),
		Payload:     string(payload),
		Attempt:     n,
		DeliveredAt: time.Now(),
	}
	s.post(&delivery, webhook, msgID, payload)
	if err := s.db.Create(&delivery).Error; err != nil {
		s.logger.Error("Failed to record webhook delivery", zap.Error(err), zap.String("webhook_id", webhook.ID.String()))
	}
	return delivery
}

// post sends the request and fills in the delivery's outcome.
func (s *service) post(delivery *WebhookDelivery, webhook Webhook, msgID string, payload []byte) {
	now := time.Now().Unix()
	sig, err := Sign(webhook.Secret, msgID, now, payload)
	if err != nil {
		delivery.ErrorMessage = err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(webhook.TimeoutSecs)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(payload))
	if err != nil {
		delivery.ErrorMessage = err.Error()
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderID, msgID)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(now, 10))
	req.Header.Set(HeaderSignature, sig)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		delivery.ErrorMessage = err.Error()
		return
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	// Keep what is recorded bounded; a receiver can answer with anything.
	_, _ = body.ReadFrom(io.LimitReader(resp.Body, 64<<10))
	delivery.StatusCode = resp.StatusCode
	delivery.ResponseBody = body.String()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Success = true
	} else {
		delivery.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
}

func (s *service) PurgeDeliveries(olderThan time.Duration) (int64, error) {
	result := s.db.Where("delivered_at < ?", time.Now().Add(-olderThan)).Delete(&WebhookDelivery{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to purge webhook deliveries: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (s *service) GetDeliveries(webhookID uuid.UUID, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	deliveries := []WebhookDelivery{}
	if err := s.db.Where("webhook_id = ?", webhookID).Order("delivered_at DESC").Limit(limit).Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("failed to get deliveries: %w", err)
	}
	return deliveries, nil
}

func (s *service) RotateSecret(id uuid.UUID) (string, error) {
	secret, err := generateSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	res := s.db.Model(&Webhook{}).Where("id = ? AND deleted_at IS NULL", id).Update("secret", secret)
	if res.Error != nil {
		return "", fmt.Errorf("failed to rotate secret: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return "", ErrNotFound
	}
	return secret, nil
}
