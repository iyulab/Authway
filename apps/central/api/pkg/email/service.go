package email

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"

	"go.uber.org/zap"
)

// Service handles email operations
type Service struct {
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	fromEmail    string
	fromName     string
	frontendURL  string
	logger       *zap.Logger
}

// Config represents email service configuration
type Config struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
	FrontendURL  string
}

// NewService creates a new email service
func NewService(config Config, logger *zap.Logger) *Service {
	return &Service{
		smtpHost:     config.SMTPHost,
		smtpPort:     config.SMTPPort,
		smtpUsername: config.SMTPUsername,
		smtpPassword: config.SMTPPassword,
		fromEmail:    config.FromEmail,
		fromName:     config.FromName,
		frontendURL:  config.FrontendURL,
		logger:       logger,
	}
}

// SendVerificationEmail sends an email verification link
func (s *Service) SendVerificationEmail(toEmail, token string) error {
	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, token)

	subject := "Authway - 이메일 인증"
	body := s.renderVerificationTemplate(verificationLink)

	return s.sendEmail(toEmail, subject, body)
}

// SendPasswordResetEmail sends a password reset link
func (s *Service) SendPasswordResetEmail(toEmail, token string) error {
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, token)

	subject := "Authway - 비밀번호 재설정"
	body := s.renderPasswordResetTemplate(resetLink)

	return s.sendEmail(toEmail, subject, body)
}

// sendEmail sends an email via SMTP
func (s *Service) sendEmail(to, subject, body string) error {
	// Build email message
	from := fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail)
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	// SMTP authentication
	auth := smtp.PlainAuth("", s.smtpUsername, s.smtpPassword, s.smtpHost)

	// Send email
	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)
	err := smtp.SendMail(addr, auth, s.fromEmail, []string{to}, []byte(message))
	if err != nil {
		s.logger.Error("Failed to send email",
			zap.String("to", to),
			zap.String("subject", subject),
			zap.Error(err),
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	s.logger.Info("Email sent successfully",
		zap.String("to", to),
		zap.String("subject", subject),
	)

	return nil
}

// renderVerificationTemplate renders email verification HTML template
func (s *Service) renderVerificationTemplate(verificationLink string) string {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .container {
            background: #ffffff;
            border-radius: 8px;
            padding: 40px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 {
            color: #4F46E5;
            margin-bottom: 20px;
        }
        .button {
            display: inline-block;
            padding: 12px 30px;
            background-color: #4F46E5;
            color: white;
            text-decoration: none;
            border-radius: 6px;
            margin: 20px 0;
        }
        .footer {
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #eee;
            color: #666;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Authway 이메일 인증</h1>
        <p>안녕하세요!</p>
        <p>Authway 회원가입을 환영합니다. 아래 버튼을 클릭하여 이메일 주소를 인증해주세요.</p>
        <a href="{{.Link}}" class="button">이메일 인증하기</a>
        <p>또는 아래 링크를 복사하여 브라우저에 붙여넣으세요:</p>
        <p style="background: #f5f5f5; padding: 10px; border-radius: 4px; word-break: break-all;">
            {{.Link}}
        </p>
        <div class="footer">
            <p>이 링크는 6시간 동안 유효합니다.</p>
            <p>본인이 요청하지 않은 경우, 이 이메일을 무시하셔도 됩니다.</p>
        </div>
    </div>
</body>
</html>
`

	t := template.Must(template.New("verification").Parse(tmpl))
	var buf bytes.Buffer
	t.Execute(&buf, map[string]string{"Link": verificationLink})
	return buf.String()
}

// renderPasswordResetTemplate renders password reset HTML template
func (s *Service) renderPasswordResetTemplate(resetLink string) string {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .container {
            background: #ffffff;
            border-radius: 8px;
            padding: 40px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 {
            color: #DC2626;
            margin-bottom: 20px;
        }
        .button {
            display: inline-block;
            padding: 12px 30px;
            background-color: #DC2626;
            color: white;
            text-decoration: none;
            border-radius: 6px;
            margin: 20px 0;
        }
        .warning {
            background: #FEF3C7;
            border-left: 4px solid #F59E0B;
            padding: 12px;
            margin: 20px 0;
        }
        .footer {
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #eee;
            color: #666;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔑 비밀번호 재설정</h1>
        <p>안녕하세요!</p>
        <p>Authway 계정의 비밀번호 재설정을 요청하셨습니다.</p>
        <p>아래 버튼을 클릭하여 새로운 비밀번호를 설정하세요.</p>
        <a href="{{.Link}}" class="button">비밀번호 재설정하기</a>
        <p>또는 아래 링크를 복사하여 브라우저에 붙여넣으세요:</p>
        <p style="background: #f5f5f5; padding: 10px; border-radius: 4px; word-break: break-all;">
            {{.Link}}
        </p>
        <div class="warning">
            <strong>⚠️ 보안 안내</strong><br>
            본인이 요청하지 않은 경우, 즉시 비밀번호를 변경하시기 바랍니다.
        </div>
        <div class="footer">
            <p>이 링크는 1시간 동안 유효합니다.</p>
            <p>보안을 위해 비밀번호 재설정 후 모든 기기에서 재로그인이 필요합니다.</p>
        </div>
    </div>
</body>
</html>
`

	t := template.Must(template.New("reset").Parse(tmpl))
	var buf bytes.Buffer
	t.Execute(&buf, map[string]string{"Link": resetLink})
	return buf.String()
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
