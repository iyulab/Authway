package email

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// smtpMessage is one message as an SMTP server receives it (RFC 5322): the
// headers a mail client needs — Date and Message-ID included — and the body
// as multipart/alternative, a plain-text part before the HTML one, so a
// client that shows no HTML, or a filter that weighs HTML-only mail as spam,
// still has the text and the link.
func smtpMessage(fromName, fromAddress, to, subject, htmlBody string, now time.Time) ([]byte, error) {
	boundary, err := randomToken(12)
	if err != nil {
		return nil, err
	}
	id, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	domain := "localhost"
	if at := strings.LastIndex(fromAddress, "@"); at >= 0 && at < len(fromAddress)-1 {
		domain = fromAddress[at+1:]
	}

	var b bytes.Buffer
	header := func(name, value string) { fmt.Fprintf(&b, "%s: %s\r\n", name, value) }
	header("From", (&mail.Address{Name: fromName, Address: fromAddress}).String())
	header("To", (&mail.Address{Address: to}).String())
	// RFC 2047: a subject in Korean, or anything not ASCII, as an encoded word.
	header("Subject", mime.BEncoding.Encode("UTF-8", subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", fmt.Sprintf("<%s@%s>", id, domain))
	header("MIME-Version", "1.0")
	header("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
	b.WriteString("\r\n")

	for _, part := range []struct{ contentType, body string }{
		{"text/plain; charset=UTF-8", plainText(htmlBody)},
		{"text/html; charset=UTF-8", htmlBody},
	} {
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", part.contentType)
		w := quotedprintable.NewWriter(&b)
		if _, err := w.Write([]byte(part.body)); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes(), nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to make a message identifier: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

var (
	invisible  = regexp.MustCompile(`(?is)<(head|style|script)\b.*?</(head|style|script)>`)
	anchor     = regexp.MustCompile(`(?is)<a\b[^>]*\bhref\s*=\s*"([^"]*)"[^>]*>(.*?)</a>`)
	lineBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|h[1-6]|li|tr|table)>`)
	tags       = regexp.MustCompile(`(?s)<[^>]+>`)
	blankRuns  = regexp.MustCompile(`\n[ \t]*(\n[ \t]*)+`)
	spaceRuns  = regexp.MustCompile(`[ \t]+`)
)

// plainText is the text of a message's HTML: what it says, with each link's
// address after its words, so the plain part still carries the link.
func plainText(htmlBody string) string {
	s := invisible.ReplaceAllString(htmlBody, "")
	s = anchor.ReplaceAllStringFunc(s, func(a string) string {
		m := anchor.FindStringSubmatch(a)
		words := strings.TrimSpace(tags.ReplaceAllString(m[2], ""))
		if words == "" || words == m[1] {
			return " " + m[1] + " "
		}
		return words + ": " + m[1]
	})
	s = lineBreaks.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRuns.ReplaceAllString(l, " "))
	}
	s = strings.Join(lines, "\n")
	s = blankRuns.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s) + "\n"
}
