package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// AzureEmailService handles email operations via Azure Functions
type AzureEmailService struct {
	baseURL     string
	functionKey string
	profileID   string
	fromEmail   string
	fromName    string
	frontendURL string
	httpClient  *http.Client
	logger      *zap.Logger
}

// AzureEmailConfig represents Azure Functions email service configuration
type AzureEmailConfig struct {
	BaseURL     string
	FunctionKey string
	ProfileID   string
	FromEmail   string
	FromName    string
	FrontendURL string
}

// AzureEmailRequest represents the request to Azure Functions email service
type AzureEmailRequest struct {
	To            []string          `json:"to"`
	Subject       string            `json:"subject"`
	TextBody      string            `json:"textBody,omitempty"`
	HtmlBody      string            `json:"htmlBody,omitempty"`
	FromEmail     string            `json:"fromEmail,omitempty"`
	FromName      string            `json:"fromName,omitempty"`
	ProfileID     string            `json:"profileId,omitempty"`
	Priority      string            `json:"priority,omitempty"`
	CustomHeaders map[string]string `json:"customHeaders,omitempty"`
}

// AzureEmailResponse represents the response from Azure Functions email service
type AzureEmailResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	MessageID string   `json:"messageId,omitempty"`
	SentAt    string   `json:"sentAt,omitempty"`
	RequestID string   `json:"requestId,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// NewAzureEmailService creates a new Azure email service
func NewAzureEmailService(config AzureEmailConfig, logger *zap.Logger) *AzureEmailService {
	return &AzureEmailService{
		baseURL:     config.BaseURL,
		functionKey: config.FunctionKey,
		profileID:   config.ProfileID,
		fromEmail:   config.FromEmail,
		fromName:    config.FromName,
		frontendURL: config.FrontendURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// SendVerificationEmail sends an email verification link via Azure Functions
func (s *AzureEmailService) SendVerificationEmail(toEmail, token string) error {
	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, token)

	req := AzureEmailRequest{
		To:       []string{toEmail},
		Subject:  "Authway - 이메일 인증",
		HtmlBody: s.renderVerificationTemplate(verificationLink),
		TextBody: fmt.Sprintf("Authway 이메일 인증\n\n아래 링크를 클릭하여 이메일을 인증해주세요:\n%s\n\n이 링크는 6시간 동안 유효합니다.", verificationLink),
		Priority: "High",
		CustomHeaders: map[string]string{
			"X-Email-Type": "Verification",
		},
	}

	return s.sendEmail(req)
}

// SendPasswordResetEmail sends a password reset link via Azure Functions
func (s *AzureEmailService) SendPasswordResetEmail(toEmail, token string) error {
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, token)

	req := AzureEmailRequest{
		To:       []string{toEmail},
		Subject:  "Authway - 비밀번호 재설정",
		HtmlBody: s.renderPasswordResetTemplate(resetLink),
		TextBody: fmt.Sprintf("Authway 비밀번호 재설정\n\n아래 링크를 클릭하여 비밀번호를 재설정하세요:\n%s\n\n이 링크는 1시간 동안 유효합니다.\n\n본인이 요청하지 않은 경우, 즉시 비밀번호를 변경하시기 바랍니다.", resetLink),
		Priority: "High",
		CustomHeaders: map[string]string{
			"X-Email-Type": "PasswordReset",
		},
	}

	return s.sendEmail(req)
}

// sendEmail sends an email via Azure Functions
func (s *AzureEmailService) sendEmail(emailReq AzureEmailRequest) error {
	// Set default from email/name if not provided
	if emailReq.FromEmail == "" {
		emailReq.FromEmail = s.fromEmail
	}
	if emailReq.FromName == "" {
		emailReq.FromName = s.fromName
	}
	if emailReq.ProfileID == "" {
		emailReq.ProfileID = s.profileID
	}

	// Build request URL
	url := fmt.Sprintf("https://%s/api/email/send?code=%s", s.baseURL, s.functionKey)

	// Marshal request body
	body, err := json.Marshal(emailReq)
	if err != nil {
		s.logger.Error("Failed to marshal email request",
			zap.Error(err),
			zap.Strings("to", emailReq.To))
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		s.logger.Error("Failed to create HTTP request",
			zap.Error(err))
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	s.logger.Info("Sending email via Azure Functions",
		zap.Strings("to", emailReq.To),
		zap.String("subject", emailReq.Subject))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		s.logger.Error("Failed to send email via Azure Functions",
			zap.Error(err),
			zap.Strings("to", emailReq.To))
		return fmt.Errorf("failed to send email via Azure Functions: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Error("Failed to read response body",
			zap.Error(err))
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var azureResp AzureEmailResponse
	if err := json.Unmarshal(respBody, &azureResp); err != nil {
		s.logger.Error("Failed to parse Azure response",
			zap.Error(err),
			zap.String("body", string(respBody)))
		return fmt.Errorf("failed to parse Azure response: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK || !azureResp.Success {
		s.logger.Error("Azure Functions returned error",
			zap.Int("status", resp.StatusCode),
			zap.String("message", azureResp.Message),
			zap.Strings("errors", azureResp.Errors))
		return fmt.Errorf("Azure Functions returned error: %s", azureResp.Message)
	}

	s.logger.Info("Email sent successfully via Azure Functions",
		zap.Strings("to", emailReq.To),
		zap.String("messageId", azureResp.MessageID),
		zap.String("requestId", azureResp.RequestID))

	return nil
}

// renderVerificationTemplate renders email verification HTML template
func (s *AzureEmailService) renderVerificationTemplate(verificationLink string) string {
	// Reuse the same template from service.go
	return renderEmailTemplate("verification", verificationLink)
}

// renderPasswordResetTemplate renders password reset HTML template
func (s *AzureEmailService) renderPasswordResetTemplate(resetLink string) string {
	// Reuse the same template from service.go
	return renderEmailTemplate("reset", resetLink)
}
