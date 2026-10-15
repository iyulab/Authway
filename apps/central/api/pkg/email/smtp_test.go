package email

import (
	"bufio"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeSMTP is a mail server on the loopback that speaks just enough SMTP to
// take one message: it records the commands it was sent and the message, and
// answers RCPT TO with rcptReply.
type fakeSMTP struct {
	addr      string
	extension []string // EHLO extensions offered besides the greeting line
	rcptReply string

	mu       sync.Mutex
	commands []string
	data     string
}

func startFakeSMTP(t *testing.T, rcptReply string, extensions ...string) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{addr: ln.Addr().String(), extension: extensions, rcptReply: rcptReply}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		f.mu.Lock()
		f.commands = append(f.commands, strings.SplitN(line, " ", 2)[0])
		f.mu.Unlock()
		switch verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0]); verb {
		case "EHLO", "HELO":
			if len(f.extension) == 0 {
				say("250 fake")
				continue
			}
			say("250-fake")
			for i, ext := range f.extension {
				if i == len(f.extension)-1 {
					say("250 " + ext)
				} else {
					say("250-" + ext)
				}
			}
		case "AUTH":
			say("235 ok")
		case "MAIL":
			say("250 ok")
		case "RCPT":
			say(f.rcptReply)
		case "DATA":
			say("354 go on")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("502 not here")
		}
	}
}

func (f *fakeSMTP) service(t *testing.T, user string) *Service {
	t.Helper()
	host, port, _ := net.SplitHostPort(f.addr)
	p, _ := strconv.Atoi(port)
	return NewService(Config{
		SMTPHost: host, SMTPPort: p, SMTPUsername: user, SMTPPassword: "pw",
		FromEmail: "noreply@example.com", FromName: "인증 서비스", FrontendURL: "https://login.example.com",
	}, zap.NewNop())
}

func (f *fakeSMTP) sent() ([]string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...), f.data
}

func TestSMTP_SendsAMessageWithTextAndHTMLAndAnEncodedSubject(t *testing.T) {
	f := startFakeSMTP(t, "250 ok")
	if err := f.service(t, "").SendMagicLinkEmail("user@example.com", "https://login.example.com/magic-link?token=abc", "BCDF-GHJK", false); err != nil {
		t.Fatalf("send: %v", err)
	}
	commands, data := f.sent()
	for _, c := range commands {
		if c == "AUTH" {
			t.Fatal("authenticated although no username is configured")
		}
	}

	msg, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("not a message: %v\n%s", err, data)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != "Authway - 로그인 링크" {
		t.Fatalf("Subject = %q (%v), raw %q", subject, err, msg.Header.Get("Subject"))
	}
	if strings.ContainsAny(msg.Header.Get("Subject"), "로그인") {
		t.Fatal("Subject is not RFC 2047 encoded")
	}
	if msg.Header.Get("Date") == "" || msg.Header.Get("Message-ID") == "" {
		t.Fatalf("Date or Message-ID missing: %v", msg.Header)
	}
	from, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil || from.Name != "인증 서비스" || from.Address != "noreply@example.com" {
		t.Fatalf("From = %+v (%v)", from, err)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q", msg.Header.Get("Content-Type"))
	}
	parts := multipart.NewReader(msg.Body, params["boundary"])
	var types []string
	for {
		p, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(p) // NextPart decodes quoted-printable
		types = append(types, strings.SplitN(p.Header.Get("Content-Type"), ";", 2)[0])
		if !strings.Contains(string(body), "https://login.example.com/magic-link?token=abc") {
			t.Fatalf("%s part does not carry the link:\n%s", p.Header.Get("Content-Type"), body)
		}
		if strings.HasPrefix(p.Header.Get("Content-Type"), "text/plain") && strings.Contains(string(body), "<") {
			t.Fatalf("plain part has markup:\n%s", body)
		}
	}
	if strings.Join(types, ",") != "text/plain,text/html" {
		t.Fatalf("parts = %v", types)
	}
}

func TestSMTP_AuthenticatesWhenGivenAUsername(t *testing.T) {
	f := startFakeSMTP(t, "250 ok", "AUTH PLAIN")
	if err := f.service(t, "mailer").SendVerificationEmail("user@example.com", "t"); err != nil {
		t.Fatalf("send: %v", err)
	}
	commands, _ := f.sent()
	if !strings.Contains(strings.Join(commands, " "), "AUTH") {
		t.Fatalf("no AUTH with a username: %v", commands)
	}
}

func TestSMTP_ADeferralIsTriedAgainARefusalIsNot(t *testing.T) {
	deferred := startFakeSMTP(t, "451 try later")
	err := deferred.service(t, "").SendVerificationEmail("user@example.com", "t")
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("4xx: %v, want ErrTransient", err)
	}
	refused := startFakeSMTP(t, "550 no such user")
	err = refused.service(t, "").SendVerificationEmail("user@example.com", "t")
	if err == nil || errors.Is(err, ErrTransient) {
		t.Fatalf("5xx: %v, want a final failure", err)
	}
}

func TestSMTP_AnUnreachableServerIsTriedAgain(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().(*net.TCPAddr)
	ln.Close()
	s := NewService(Config{SMTPHost: "127.0.0.1", SMTPPort: addr.Port, FromEmail: "a@example.com"}, zap.NewNop())
	start := time.Now()
	if err := s.SendVerificationEmail("user@example.com", "t"); !errors.Is(err, ErrTransient) {
		t.Fatalf("closed port: %v, want ErrTransient", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("took too long to fail")
	}
}

func TestSMTP_RequiredStartTLSIsNotSkipped(t *testing.T) {
	f := startFakeSMTP(t, "250 ok")
	host, port, _ := net.SplitHostPort(f.addr)
	p, _ := strconv.Atoi(port)
	s := NewService(Config{SMTPHost: host, SMTPPort: p, SMTPSecurity: SMTPSecurityStartTLS, FromEmail: "a@example.com"}, zap.NewNop())
	if err := s.SendVerificationEmail("user@example.com", "t"); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("server without STARTTLS: %v, want a refusal", err)
	}
	if _, data := f.sent(); data != "" {
		t.Fatal("the message went out in the clear")
	}
}

func TestSMTP_ImplicitTLSOnPort465(t *testing.T) {
	if !(smtpTransport{port: 465}).implicitTLS() || (smtpTransport{port: 587}).implicitTLS() ||
		!(smtpTransport{port: 2525, security: SMTPSecurityTLS}).implicitTLS() || (smtpTransport{port: 465, security: SMTPSecurityNone}).implicitTLS() {
		t.Fatal("implicit TLS is not chosen by port 465 or security=tls alone")
	}
}

func TestPlainText_KeepsWordsAndLinks(t *testing.T) {
	got := plainText(`<html><head><style>a{color:red}</style></head><body><h1>Hello &amp; welcome</h1>` +
		`<p>Click <a href="https://x.example/a?b=1&amp;c=2" style="color:#fff">Sign in</a> now.</p></body></html>`)
	if strings.Contains(got, "color") || !strings.Contains(got, "Hello & welcome") ||
		!strings.Contains(got, "Sign in: https://x.example/a?b=1&c=2") {
		t.Fatalf("plainText = %q", got)
	}
}
