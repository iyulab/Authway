package email

// EmailService interface defines email operations
type EmailService interface {
	SendVerificationEmail(toEmail, token string) error
	SendPasswordResetEmail(toEmail, token string) error
}
