package email

import "authway/packages/mail"

// The messages themselves are files every Authway implementation renders
// (packages/mail); these name each message's values.

func verificationMessage(locale, organization, link string) (mail.Message, error) {
	return mail.Render("verification", locale, map[string]any{"link": link, "organization": organization})
}

func resetMessage(locale, organization, link string) (mail.Message, error) {
	return mail.Render("reset", locale, map[string]any{"link": link, "organization": organization})
}

func invitationMessage(locale, inviterName, tenantName, message, link string) (mail.Message, error) {
	return mail.Render("invitation", locale, map[string]any{
		"link": link, "inviterName": inviterName, "tenantName": tenantName, "message": message,
	})
}

func magicLinkMessage(locale, organization, link, code string, isNewUser bool) (mail.Message, error) {
	return mail.Render("magiclink", locale, map[string]any{
		"link": link, "organization": organization, "code": code, "newUser": isNewUser,
	})
}
