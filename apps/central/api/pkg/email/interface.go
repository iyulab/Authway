package email

// EmailService interface defines email operations
type EmailService interface {
	// organization is the name of the tenant the account belongs to: one
	// address can have accounts in several, and the mail says which one it is
	// about. "" leaves it out.
	//
	// locale is the language of the message: a language tag or an
	// Accept-Language value (mail.Negotiate reads it); "" is the default.
	SendVerificationEmail(toEmail, locale, organization, token string) error
	SendPasswordResetEmail(toEmail, locale, organization, token string) error
	SendInvitationEmail(toEmail, locale, inviterName, tenantName, message, inviteURL string) error
	SendMagicLinkEmail(toEmail, locale, organization, linkURL, code string, isNewUser bool) error
}
