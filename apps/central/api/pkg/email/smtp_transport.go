package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"
)

// SMTP security modes (AUTHWAY_EMAIL_SMTP_SECURITY).
const (
	// SMTPSecurityAuto uses implicit TLS on port 465 and STARTTLS elsewhere
	// whenever the server offers it — what mail clients do.
	SMTPSecurityAuto = ""
	// SMTPSecurityTLS connects over TLS from the first byte (port 465).
	SMTPSecurityTLS = "tls"
	// SMTPSecurityStartTLS requires STARTTLS: a server that does not offer it
	// is refused rather than written to in the clear.
	SMTPSecurityStartTLS = "starttls"
	// SMTPSecurityNone never encrypts — a mail server on the same host or a
	// development capture such as MailHog.
	SMTPSecurityNone = "none"
)

// smtpTimeout bounds each connection to the mail server, so a server that
// stops answering is a failure to try again, not a sender held forever.
const smtpTimeout = 30 * time.Second

type smtpTransport struct {
	host     string
	port     int
	username string
	password string
	security string
}

func (t smtpTransport) implicitTLS() bool {
	return t.security == SMTPSecurityTLS || (t.security == SMTPSecurityAuto && t.port == 465)
}

// send delivers message from from to to. A failure worth trying again — the
// server could not be reached, timed out, or answered 4xx — wraps
// ErrTransient; a 5xx answer is the server's final word.
func (t smtpTransport) send(from, to string, message []byte) error {
	addr := net.JoinHostPort(t.host, strconv.Itoa(t.port))
	dialer := &net.Dialer{Timeout: smtpTimeout}
	var conn net.Conn
	var err error
	if t.implicitTLS() {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: t.host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("%w: could not reach the mail server: %w", ErrTransient, err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))
	c, err := smtp.NewClient(conn, t.host)
	if err != nil {
		conn.Close()
		return classify("greeting", err)
	}
	defer c.Close()

	if !t.implicitTLS() && t.security != SMTPSecurityNone {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: t.host, MinVersion: tls.VersionTLS12}); err != nil {
				return classify("STARTTLS", err)
			}
		} else if t.security == SMTPSecurityStartTLS {
			return errors.New("the mail server does not offer STARTTLS, which the configuration requires")
		}
	}
	// Authenticate only when given credentials: a relay that accepts mail
	// from this host asks for none. Credentials never cross a connection in
	// the clear, except to a server on this host.
	if t.username != "" {
		if _, encrypted := c.TLSConnectionState(); !encrypted && !isLoopback(t.host) {
			return errors.New("refusing to send the mail server's credentials over an unencrypted connection; use TLS or STARTTLS")
		}
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("the mail server does not offer AUTH, but a username is configured")
		}
		if err := c.Auth(smtp.PlainAuth("", t.username, t.password, t.host)); err != nil {
			return classify("AUTH", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return classify("MAIL FROM", err)
	}
	if err := c.Rcpt(to); err != nil {
		return classify("RCPT TO", err)
	}
	w, err := c.Data()
	if err != nil {
		return classify("DATA", err)
	}
	if _, err := w.Write(message); err != nil {
		return classify("DATA", err)
	}
	if err := w.Close(); err != nil {
		return classify("DATA", err)
	}
	return c.Quit()
}

// classify wraps ErrTransient around a failure worth trying again: a 4xx
// answer (RFC 5321 4.2.1 — "the command was not accepted, but the error
// condition is temporary") or a broken connection. A 5xx answer is final.
func classify(step string, err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		if reply.Code >= 400 && reply.Code < 500 {
			return fmt.Errorf("%w: the mail server deferred %s: %w", ErrTransient, step, err)
		}
		return fmt.Errorf("the mail server refused %s: %w", step, err)
	}
	return fmt.Errorf("%w: %s failed: %w", ErrTransient, step, err)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
