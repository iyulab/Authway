package email

import (
	"fmt"
	"strings"
	"time"

	"authway/apps/central/api/pkg/maillink"
	"authway/packages/mail"
	"go.uber.org/zap"
)

// Service sends Authway's mail through any SMTP server — the standard way a
// deployment hands mail over, with no particular mail service assumed.
type Service struct {
	transport   smtpTransport
	fromEmail   string
	fromName    string
	frontendURL string
	logger      *zap.Logger
}

// Config is how to reach the SMTP server and whom the mail is from.
type Config struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string // empty: the server is not asked to authenticate
	SMTPPassword string
	// SMTPSecurity is one of SMTPSecurityAuto, SMTPSecurityTLS,
	// SMTPSecurityStartTLS and SMTPSecurityNone.
	SMTPSecurity string
	FromEmail    string
	FromName     string
	FrontendURL  string
}

// NewService creates a new email service
func NewService(config Config, logger *zap.Logger) *Service {
	return &Service{
		transport: smtpTransport{
			host:     config.SMTPHost,
			port:     config.SMTPPort,
			username: config.SMTPUsername,
			password: config.SMTPPassword,
			security: config.SMTPSecurity,
		},
		fromEmail:   config.FromEmail,
		fromName:    config.FromName,
		frontendURL: config.FrontendURL,
		logger:      logger,
	}
}

// SendVerificationEmail sends an email verification link
func (s *Service) SendVerificationEmail(toEmail, organization, token string) error {
	return s.send(toEmail)(verificationMessage(organization, maillink.VerifyEmail(s.frontendURL, token)))
}

// SendPasswordResetEmail sends a password reset link
func (s *Service) SendPasswordResetEmail(toEmail, organization, token string) error {
	return s.send(toEmail)(resetMessage(organization, maillink.ResetPassword(s.frontendURL, token)))
}

// SendInvitationEmail sends a tenant invitation link via SMTP
func (s *Service) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	return s.send(toEmail)(invitationMessage(inviterName, tenantName, message, inviteURL))
}

// SendMagicLinkEmail sends a passwordless magic link via SMTP
func (s *Service) SendMagicLinkEmail(toEmail, organization, linkURL, code string, isNewUser bool) error {
	return s.send(toEmail)(magicLinkMessage(organization, linkURL, code, isNewUser))
}

// send sends a rendered message to `to`; a message that failed to render is
// not sent.
func (s *Service) send(to string) func(mail.Message, error) error {
	return func(m mail.Message, err error) error {
		if err != nil {
			return err
		}
		return s.sendEmail(to, m)
	}
}

// sendEmail sends an email via SMTP. A failure worth trying again wraps
// ErrTransient (Async retries it).
func (s *Service) sendEmail(to string, m mail.Message) error {
	message, err := smtpMessage(s.fromName, s.fromEmail, to, m, time.Now())
	if err != nil {
		return err
	}
	if err := s.transport.send(s.fromEmail, to, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	s.logger.Info("Email sent", zap.String("to", to), zap.String("subject", m.Subject))
	return nil
}

// ValidateEmail performs basic email validation
func ValidateEmail(email string) bool {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	if !strings.Contains(email, "@") {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	if len(parts[0]) < 1 || len(parts[1]) < 3 {
		return false
	}
	return strings.Contains(parts[1], ".")
}
