package email

// EmailService interface defines email operations
type EmailService interface {
	// organization is the name of the tenant the account belongs to: one
	// address can have accounts in several, and the mail says which one it is
	// about. "" leaves it out.
	SendVerificationEmail(toEmail, organization, token string) error
	SendPasswordResetEmail(toEmail, organization, token string) error
	SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error
	SendMagicLinkEmail(toEmail, organization, linkURL, code string, isNewUser bool) error
}
