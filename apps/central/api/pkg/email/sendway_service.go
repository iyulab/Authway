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
func (s *SendwayEmailService) SendVerificationEmail(toEmail, token string) error {
	verificationLink := maillink.VerifyEmail(s.frontendURL, token)

	body := fmt.Sprintf("Authway 이메일 인증\n\n아래 링크를 클릭하여 이메일을 인증해주세요:\n%s\n\n이 링크는 6시간 동안 유효합니다.", verificationLink)
	htmlBody, err := renderEmailTemplate("verification", verificationLink)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, "Authway - 이메일 인증", body, htmlBody)
}

// SendPasswordResetEmail sends a password reset link via Sendway.
func (s *SendwayEmailService) SendPasswordResetEmail(toEmail, token string) error {
	resetLink := maillink.ResetPassword(s.frontendURL, token)

	body := fmt.Sprintf("Authway 비밀번호 재설정\n\n아래 링크를 클릭하여 비밀번호를 재설정하세요:\n%s\n\n이 링크는 1시간 동안 유효합니다.\n\n본인이 요청하지 않은 경우, 즉시 비밀번호를 변경하시기 바랍니다.", resetLink)
	htmlBody, err := renderEmailTemplate("reset", resetLink)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, "Authway - 비밀번호 재설정", body, htmlBody)
}

// SendInvitationEmail sends a tenant invitation link via Sendway.
func (s *SendwayEmailService) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	subject := fmt.Sprintf("Authway - %s 초대", tenantName)

	body := fmt.Sprintf("Authway 초대\n\n%s 님이 %s 워크스페이스에 초대했습니다.\n\n초대 링크:\n%s\n\n",
		inviterName, tenantName, inviteURL)
	if message != "" {
		body += fmt.Sprintf("메시지:\n%s\n\n", message)
	}
	body += "본인이 알지 못하는 초대인 경우 이 이메일을 무시하셔도 됩니다."
	htmlBody, err := renderInvitationTemplate(inviterName, tenantName, message, inviteURL)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, subject, body, htmlBody)
}

// SendMagicLinkEmail sends a passwordless magic link via Sendway.
func (s *SendwayEmailService) SendMagicLinkEmail(toEmail, linkURL, code string, isNewUser bool) error {
	subject := "Authway - 로그인 링크"
	if isNewUser {
		subject = "Authway - 가입 및 로그인 링크"
	}

	body := fmt.Sprintf("Authway 매직 링크\n\n아래 링크를 클릭하여 로그인하세요:\n%s\n\n다른 기기에서 이 메일을 읽고 있다면, 로그인을 시작한 화면에 이 코드를 입력하세요: %s\n\n링크와 코드는 일정 시간 후 만료되며, 한 번만 쓸 수 있습니다.\n\n본인이 요청하지 않은 경우, 이 이메일을 무시하셔도 됩니다.", linkURL, code)
	htmlBody, err := renderMagicLinkTemplate(linkURL, code, isNewUser)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, subject, body, htmlBody)
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
