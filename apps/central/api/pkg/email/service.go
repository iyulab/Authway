package email

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"authway/apps/central/api/pkg/maillink"
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
func (s *Service) SendVerificationEmail(toEmail, token string) error {
	verificationLink := maillink.VerifyEmail(s.frontendURL, token)

	subject := "Authway - 이메일 인증"
	body, err := renderEmailTemplate("verification", verificationLink)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, subject, body)
}

// SendPasswordResetEmail sends a password reset link
func (s *Service) SendPasswordResetEmail(toEmail, token string) error {
	resetLink := maillink.ResetPassword(s.frontendURL, token)

	subject := "Authway - 비밀번호 재설정"
	body, err := renderEmailTemplate("reset", resetLink)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, subject, body)
}

// SendInvitationEmail sends a tenant invitation link via SMTP
func (s *Service) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	subject := fmt.Sprintf("Authway - %s 초대", tenantName)
	body, err := renderInvitationTemplate(inviterName, tenantName, message, inviteURL)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, subject, body)
}

// SendMagicLinkEmail sends a passwordless magic link via SMTP
func (s *Service) SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error {
	subject := "Authway - 로그인 링크"
	if isNewUser {
		subject = "Authway - 가입 및 로그인 링크"
	}
	body, err := renderMagicLinkTemplate(linkURL, isNewUser)
	if err != nil {
		return err
	}

	return s.sendEmail(toEmail, subject, body)
}

// sendEmail sends an email via SMTP. A failure worth trying again wraps
// ErrTransient (Async retries it).
func (s *Service) sendEmail(to, subject, body string) error {
	message, err := smtpMessage(s.fromName, s.fromEmail, to, subject, body, time.Now())
	if err != nil {
		return err
	}
	if err := s.transport.send(s.fromEmail, to, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	s.logger.Info("Email sent", zap.String("to", to), zap.String("subject", subject))
	return nil
}

// The call-to-action link every message carries is styled inline. Mail
// clients drop a <style> block or apply only part of it, and paint links in
// their own colour, so a button styled only by a class came out as link-blue
// text on the button colour — unreadable. The colour is on the table cell
// (bgcolor survives without any CSS), on the link and on a span inside it,
// which is the usual way to make a link look like a button in mail.
const emailButtonTemplate = `{{define "button"}}<table role="presentation" cellspacing="0" cellpadding="0" border="0" style="margin:20px 0;border-collapse:separate;"><tr><td bgcolor="{{.Color}}" style="background-color:{{.Color}};border-radius:6px;"><a href="{{.Link}}" target="_blank" style="display:inline-block;padding:12px 30px;font-size:16px;font-weight:600;line-height:1.2;color:#ffffff;text-decoration:none;border-radius:6px;background-color:{{.Color}};"><span style="color:#ffffff;">{{.Label}}</span></a></td></tr></table>{{end}}`

type emailButton struct {
	Link  string
	Label string
	Color template.CSS
}

// parseEmailTemplate parses a message template together with the shared
// button: {{template "button" (button <link> <label> <colour>)}}.
func parseEmailTemplate(name, body string) (*template.Template, error) {
	t := template.New(name).Funcs(template.FuncMap{
		"button": func(link, label, color string) emailButton {
			return emailButton{Link: link, Label: label, Color: template.CSS(color)}
		},
	})
	if _, err := t.Parse(emailButtonTemplate); err != nil {
		return nil, err
	}
	return t.Parse(body)
}

// renderEmailTemplate renders the verification or password reset message.
func renderEmailTemplate(templateType, link string) (string, error) {
	var tmpl string
	switch templateType {
	case "verification":
		tmpl = getVerificationTemplate()
	case "reset":
		tmpl = getPasswordResetTemplate()
	default:
		return "", fmt.Errorf("email: no %q template", templateType)
	}
	return executeEmailTemplate(templateType, tmpl, map[string]string{"Link": link})
}

// executeEmailTemplate renders a message. A message that fails to render is
// an error, never a partial body: a partial body would still be sent.
func executeEmailTemplate(name, body string, data any) (string, error) {
	t, err := parseEmailTemplate(name, body)
	if err != nil {
		return "", fmt.Errorf("email: %s template: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("email: rendering the %s message: %w", name, err)
	}
	return buf.String(), nil
}

// getVerificationTemplate returns verification email template
func getVerificationTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="color-scheme" content="light">
    <meta name="supported-color-schemes" content="light">
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
        {{template "button" (button .Link "이메일 인증하기" "#4F46E5")}}
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
}

// getPasswordResetTemplate returns password reset email template
func getPasswordResetTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="color-scheme" content="light">
    <meta name="supported-color-schemes" content="light">
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
        {{template "button" (button .Link "비밀번호 재설정하기" "#DC2626")}}
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
}

// renderInvitationTemplate renders tenant invitation HTML template
func renderInvitationTemplate(inviterName, tenantName, message, inviteURL string) (string, error) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="color-scheme" content="light">
    <meta name="supported-color-schemes" content="light">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .container { background: #ffffff; border-radius: 8px; padding: 40px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #4F46E5; margin-bottom: 20px; }
        .message { background: #F9FAFB; border-left: 4px solid #4F46E5; padding: 12px; margin: 20px 0; white-space: pre-wrap; }
        .footer { margin-top: 30px; padding-top: 20px; border-top: 1px solid #eee; color: #666; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>📨 워크스페이스 초대</h1>
        <p><strong>{{.InviterName}}</strong> 님이 <strong>{{.TenantName}}</strong> 워크스페이스에 초대했습니다.</p>
        {{if .Message}}<div class="message">{{.Message}}</div>{{end}}
        <p>아래 버튼을 클릭하여 초대를 수락하세요.</p>
        {{template "button" (button .InviteURL "초대 수락" "#4F46E5")}}
        <p>또는 아래 링크를 복사하여 브라우저에 붙여넣으세요:</p>
        <p style="background: #f5f5f5; padding: 10px; border-radius: 4px; word-break: break-all;">{{.InviteURL}}</p>
        <div class="footer">
            <p>본인이 알지 못하는 초대인 경우, 이 이메일을 무시하셔도 됩니다.</p>
        </div>
    </div>
</body>
</html>
`
	return executeEmailTemplate("invitation", tmpl, map[string]string{
		"InviterName": inviterName,
		"TenantName":  tenantName,
		"Message":     message,
		"InviteURL":   inviteURL,
	})
}

// renderMagicLinkTemplate renders passwordless magic link HTML template
func renderMagicLinkTemplate(linkURL string, isNewUser bool) (string, error) {
	headline := "🔗 로그인 링크"
	intro := "아래 버튼을 클릭하여 Authway에 로그인하세요."
	if isNewUser {
		headline = "🎉 환영합니다 — 가입 및 로그인 링크"
		intro = "Authway에 가입과 로그인을 함께 진행할 링크를 보내드렸습니다. 아래 버튼을 클릭하여 시작하세요."
	}

	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="color-scheme" content="light">
    <meta name="supported-color-schemes" content="light">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .container { background: #ffffff; border-radius: 8px; padding: 40px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #4F46E5; margin-bottom: 20px; }
        .footer { margin-top: 30px; padding-top: 20px; border-top: 1px solid #eee; color: #666; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Headline}}</h1>
        <p>{{.Intro}}</p>
        {{template "button" (button .LinkURL "로그인하기" "#4F46E5")}}
        <p>또는 아래 링크를 복사하여 브라우저에 붙여넣으세요:</p>
        <p style="background: #f5f5f5; padding: 10px; border-radius: 4px; word-break: break-all;">{{.LinkURL}}</p>
        <div class="footer">
            <p>이 링크는 일정 시간 후 만료됩니다.</p>
            <p>본인이 요청하지 않은 경우, 이 이메일을 무시하셔도 됩니다.</p>
        </div>
    </div>
</body>
</html>
`
	return executeEmailTemplate("magiclink", tmpl, map[string]string{
		"Headline": headline,
		"Intro":    intro,
		"LinkURL":  linkURL,
	})
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
