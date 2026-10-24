package email

import "authway/packages/mail"

// The messages themselves are files every Authway implementation renders
// (packages/mail); these name each message's values.

func verificationMessage(organization, link string) (mail.Message, error) {
	return mail.Render("verification", mail.DefaultLocale, map[string]any{"link": link, "organization": organization})
}

func resetMessage(organization, link string) (mail.Message, error) {
	return mail.Render("reset", mail.DefaultLocale, map[string]any{"link": link, "organization": organization})
}

func invitationMessage(inviterName, tenantName, message, link string) (mail.Message, error) {
	return mail.Render("invitation", mail.DefaultLocale, map[string]any{
		"link": link, "inviterName": inviterName, "tenantName": tenantName, "message": message,
	})
}

func magicLinkMessage(organization, link, code string, isNewUser bool) (mail.Message, error) {
	return mail.Render("magiclink", mail.DefaultLocale, map[string]any{
		"link": link, "organization": organization, "code": code, "newUser": isNewUser,
	})
}
