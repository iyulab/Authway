package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/maillink"
)

// SendwayEmailService sends email through Sendway (https://github.com/iyulab/Sendway),
// iyulab's own notification-service deployment. See org/dev-docs/sendway.md for the
// deployment this talks to and the exact request/response contract.
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
}

// sendwayEmailRequest mirrors POST /messages/email. HtmlBody is optional — Sendway
// sends multipart/alternative when set, keeping Body as the RFC 2046 fallback part
// (org/dev-docs/sendway.md "Using your own sender identity" / docket iyulab/Sendway#105).
type sendwayEmailRequest struct {
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	HtmlBody string   `json:"htmlBody,omitempty"`
}

// sendwaySuccessResponse is the 200 shape.
type sendwaySuccessResponse struct {
	ID string `json:"id"`
}

// sendwayErrorResponse covers both error shapes Sendway returns: 400 uses "error",
// 502 uses "detail". Both carry messageId so a failed send can still be looked up.
type sendwayErrorResponse struct {
	Error     string `json:"error"`
	Detail    string `json:"detail"`
	MessageID string `json:"messageId"`
}

// NewSendwayEmailService creates a new Sendway-backed email service.
func NewSendwayEmailService(config SendwayEmailConfig, logger *zap.Logger) *SendwayEmailService {
	return &SendwayEmailService{
		baseURL:     config.BaseURL,
		apiKey:      config.APIKey,
		frontendURL: config.FrontendURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// SendVerificationEmail sends an email verification link via Sendway.
func (s *SendwayEmailService) SendVerificationEmail(toEmail, token string) error {
	verificationLink := maillink.VerifyEmail(s.frontendURL, token)

	body := fmt.Sprintf("Authway 이메일 인증\n\n아래 링크를 클릭하여 이메일을 인증해주세요:\n%s\n\n이 링크는 6시간 동안 유효합니다.", verificationLink)
	htmlBody := renderEmailTemplate("verification", verificationLink)

	return s.sendEmail(toEmail, "Authway - 이메일 인증", body, htmlBody)
}

// SendPasswordResetEmail sends a password reset link via Sendway.
func (s *SendwayEmailService) SendPasswordResetEmail(toEmail, token string) error {
	resetLink := maillink.ResetPassword(s.frontendURL, token)

	body := fmt.Sprintf("Authway 비밀번호 재설정\n\n아래 링크를 클릭하여 비밀번호를 재설정하세요:\n%s\n\n이 링크는 1시간 동안 유효합니다.\n\n본인이 요청하지 않은 경우, 즉시 비밀번호를 변경하시기 바랍니다.", resetLink)
	htmlBody := renderEmailTemplate("reset", resetLink)

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
	htmlBody := renderInvitationTemplate(inviterName, tenantName, message, inviteURL)

	return s.sendEmail(toEmail, subject, body, htmlBody)
}

// SendMagicLinkEmail sends a passwordless magic link via Sendway.
func (s *SendwayEmailService) SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error {
	subject := "Authway - 로그인 링크"
	if isNewUser {
		subject = "Authway - 가입 및 로그인 링크"
	}

	body := fmt.Sprintf("Authway 매직 링크\n\n아래 링크를 클릭하여 로그인하세요:\n%s\n\n이 링크는 일정 시간 후 만료됩니다.\n\n본인이 요청하지 않은 경우, 이 이메일을 무시하셔도 됩니다.", linkURL)
	htmlBody := renderMagicLinkTemplate(linkURL, isNewUser)

	return s.sendEmail(toEmail, subject, body, htmlBody)
}

// sendEmail sends an email via Sendway's POST /messages/email, with an HTML
// alternative alongside the required plain-text body (docket iyulab/Sendway#105 —
// resolved 2026-08-30, live on sendway.u-platform.kr).
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
	// Lets a transient failure (502) be retried with the same key instead of
	// risking a duplicate send — see org/dev-docs/sendway.md "Idempotency".
	httpReq.Header.Set("Idempotency-Key", uuid.New().String())

	s.logger.Info("Sending email via Sendway",
		zap.String("to", to),
		zap.String("subject", subject))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		s.logger.Error("Failed to send email via Sendway",
			zap.Error(err),
			zap.String("to", to))
		return fmt.Errorf("failed to send email via Sendway: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Error("Failed to read Sendway response body", zap.Error(err))
		return fmt.Errorf("failed to read Sendway response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp sendwayErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		reason := errResp.Error
		if reason == "" {
			reason = errResp.Detail
		}
		s.logger.Error("Sendway returned an error",
			zap.Int("status", resp.StatusCode),
			zap.String("reason", reason),
			zap.String("messageId", errResp.MessageID))
		return fmt.Errorf("sendway returned status %d: %s (messageId=%s)", resp.StatusCode, reason, errResp.MessageID)
	}

	var okResp sendwaySuccessResponse
	if err := json.Unmarshal(respBody, &okResp); err != nil {
		s.logger.Error("Failed to parse Sendway success response",
			zap.Error(err),
			zap.String("body", string(respBody)))
		return fmt.Errorf("failed to parse Sendway response: %w", err)
	}

	s.logger.Info("Email sent successfully via Sendway",
		zap.String("to", to),
		zap.String("messageId", okResp.ID))

	return nil
}
