package email

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"authway/apps/central/api/pkg/maillink"
	"authway/packages/mail"
)

// SendwayEmailService sends email through a Sendway deployment
// (https://github.com/iyulab/Sendway) using its POST /messages/email contract.
type SendwayEmailService struct {
	baseURL     string
	apiKey      string
	frontendURL string
	httpClient  *http.Client
	logger      *zap.Logger
}

// SendwayEmailConfig configures SendwayEmailService. There is no from-address field:
// Sendway's /messages/email request carries no sender identity — the "From" a
// recipient sees is a per-tenant credential Sendway's admin configures server-side
// (PUT /admin/tenants/{id}/credentials/email), not something a request can set.
type SendwayEmailConfig struct {
	BaseURL     string
	APIKey      string
	FrontendURL string
	// Senders is how many messages are handed over at once. That many
	// connections are kept open between messages, so a burst does not pay
	// for a new TLS handshake per message. Zero keeps the transport default.
	Senders int
}

// sendwayEmailRequest mirrors POST /messages/email. HtmlBody is optional — Sendway
// sends multipart/alternative when set, keeping Body as the RFC 2046 fallback part.
type sendwayEmailRequest struct {
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	HtmlBody string   `json:"htmlBody,omitempty"`
}

// sendwaySuccessResponse is what Sendway answers once it has taken a message:
// 202 Accepted with the id it can be looked up by (GET /messages/{id}).
// Delivery happens afterwards, and Sendway retries a failed delivery itself.
type sendwaySuccessResponse struct {
	ID string `json:"id"`
}

// sendwayErrorResponse covers both error bodies: a refused request uses
// "error", a failure of the service itself (or a gateway in front of it) uses
// "detail". messageId is present when the message was stored.
type sendwayErrorResponse struct {
	Error     string `json:"error"`
	Detail    string `json:"detail"`
	MessageID string `json:"messageId"`
}

// acceptTimeout bounds one attempt to hand a message over. Sendway answers as
// soon as it has queued the message, so an attempt that takes longer than this
// is not going to answer; the message is tried again under the same
// idempotency key.
const acceptTimeout = 15 * time.Second

// NewSendwayEmailService creates a new Sendway-backed email service.
func NewSendwayEmailService(config SendwayEmailConfig, logger *zap.Logger) *SendwayEmailService {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.Senders > transport.MaxIdleConnsPerHost {
		transport.MaxIdleConnsPerHost = config.Senders
	}
	return &SendwayEmailService{
		baseURL:     config.BaseURL,
		apiKey:      config.APIKey,
		frontendURL: config.FrontendURL,
		httpClient: &http.Client{
			Timeout:   acceptTimeout,
			Transport: transport,
		},
		logger: logger,
	}
}

// SendVerificationEmail sends an email verification link via Sendway.
func (s *SendwayEmailService) SendVerificationEmail(toEmail, locale, organization, token string) error {
	return s.send(toEmail)(verificationMessage(locale, organization, maillink.VerifyEmail(s.frontendURL, token)))
}

// SendPasswordResetEmail sends a password reset link via Sendway.
func (s *SendwayEmailService) SendPasswordResetEmail(toEmail, locale, organization, token string) error {
	return s.send(toEmail)(resetMessage(locale, organization, maillink.ResetPassword(s.frontendURL, token)))
}

// SendInvitationEmail sends a tenant invitation link via Sendway.
func (s *SendwayEmailService) SendInvitationEmail(toEmail, locale, inviterName, tenantName, message, inviteURL string) error {
	return s.send(toEmail)(invitationMessage(locale, inviterName, tenantName, message, inviteURL))
}

// SendMagicLinkEmail sends a passwordless magic link via Sendway.
func (s *SendwayEmailService) SendMagicLinkEmail(toEmail, locale, organization, linkURL, code string, isNewUser bool) error {
	return s.send(toEmail)(magicLinkMessage(locale, organization, linkURL, code, isNewUser))
}

// send hands a rendered message to Sendway; a message that failed to render
// is not sent.
func (s *SendwayEmailService) send(to string) func(mail.Message, error) error {
	return func(m mail.Message, err error) error {
		if err != nil {
			return err
		}
		return s.sendEmail(to, m.Subject, m.Text, m.HTML)
	}
}

// sendEmail hands an email to Sendway's POST /messages/email, with an HTML
// alternative alongside the required plain-text body. A nil error means
// Sendway took the message, not that it has been delivered.
func (s *SendwayEmailService) sendEmail(to, subject, body, htmlBody string) error {
	req := sendwayEmailRequest{
		To:       []string{to},
		Subject:  subject,
		Body:     body,
		HtmlBody: htmlBody,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		s.logger.Error("Failed to marshal Sendway email request",
			zap.Error(err),
			zap.String("to", to))
		return fmt.Errorf("failed to marshal Sendway email request: %w", err)
	}

	url := fmt.Sprintf("https://%s/messages/email", s.baseURL)

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		s.logger.Error("Failed to create HTTP request", zap.Error(err))
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", s.apiKey)
	// Every message carries a token of its own, so its content identifies it:
	// a retry of the same message sends the same key and Sendway answers with
	// the message it already holds instead of sending it twice — also while
	// the first attempt is still being handled.
	httpReq.Header.Set("Idempotency-Key", idempotencyKey(reqBody))

	s.logger.Info("Sending email via Sendway",
		zap.String("to", to),
		zap.String("subject", subject))

	// took covers the whole attempt as the caller sees it — connecting included,
	// which is most of it when the previous message went out too long ago for
	// its connection to still be open.
	start := time.Now()
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		s.logger.Error("Failed to send email via Sendway",
			zap.Error(err),
			zap.Duration("took", time.Since(start)),
			zap.String("to", to))
		return fmt.Errorf("%w: failed to send email via Sendway: %w", ErrTransient, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Error("Failed to read Sendway response body", zap.Error(err))
		return fmt.Errorf("failed to read Sendway response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var errResp sendwayErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		reason := errResp.Error
		if reason == "" {
			reason = errResp.Detail
		}
		s.logger.Error("Sendway returned an error",
			zap.Int("status", resp.StatusCode),
			zap.Duration("took", time.Since(start)),
			zap.String("reason", reason),
			zap.String("messageId", errResp.MessageID))
		err := fmt.Errorf("sendway returned status %d: %s (messageId=%s)", resp.StatusCode, reason, errResp.MessageID)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return fmt.Errorf("%w: %w", ErrTransient, err)
		}
		return err
	}

	var okResp sendwaySuccessResponse
	if err := json.Unmarshal(respBody, &okResp); err != nil {
		s.logger.Error("Failed to parse Sendway success response",
			zap.Error(err),
			zap.String("body", string(respBody)))
		return fmt.Errorf("failed to parse Sendway response: %w", err)
	}

	s.logger.Info("Email accepted by Sendway",
		zap.String("to", to),
		zap.Int("status", resp.StatusCode),
		zap.Duration("took", time.Since(start)),
		zap.String("messageId", okResp.ID))

	return nil
}

// idempotencyKey names a message by its content.
func idempotencyKey(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
