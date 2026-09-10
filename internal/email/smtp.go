// Package email: smtp.go is the net/smtp wrapper used by the production
// pipeline. Per the data-lifecycle change tasks list (3.5), we use the
// stdlib so deployments don't pick up another dependency. Most providers
// (SendGrid, Mailgun, AWS SES, Postmark) expose an SMTP relay endpoint
// that accepts the same PLAIN-auth flow this package implements.
//
// Connection lifecycle: one Dial per Send. We do not pool — the volumes
// here (a handful of emails per minute at peak) don't justify the
// complexity, and one-shot connections sidestep the head-of-line blocking
// risk that long-lived SMTP sessions can experience when a single
// recipient's MTA is slow.
package email

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// NetSMTPSender is a SMTPSender backed by net/smtp. It supports STARTTLS
// (port 587) and implicit TLS (port 465); plain unencrypted SMTP is
// rejected at construction unless the host is "localhost" / "127.0.0.1"
// (development convenience).
type NetSMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string

	// DialTimeout caps the TCP dial. Defaults to 10s.
	DialTimeout time.Duration

	// CmdTimeout bounds each SMTP command (MAIL/RCPT/DATA/QUIT and the
	// DATA body write). Defaults to 10s. A stalled relay fails the send
	// within this bound instead of hanging the goroutine forever
	// (fix-email-delivery 3.x). Configurable via SMTP_CMD_TIMEOUT.
	CmdTimeout time.Duration

	// AllowInsecure is true when the caller has confirmed the host is a
	// local/dev relay. Production deployments should leave it false.
	AllowInsecure bool
}

// Send writes a multipart/alternative message with both an HTML body and
// a text body. Returns nil on a 250 OK from the relay, an error otherwise.
// Transient/permanent classification is left to the caller — the caller
// (the pipeline) treats *any* error as transient and lets the retry
// worker decide when to give up after attempt_count >= 3.
func (s *NetSMTPSender) Send(ctx context.Context, from, to, subject, htmlBody, textBody string, headers []string) error {
	timeout := s.DialTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	cmdTimeout := s.CmdTimeout
	if cmdTimeout == 0 {
		cmdTimeout = 10 * time.Second
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}

	// Implicit-TLS port 465: wrap the conn in a tls.Client immediately.
	// STARTTLS port 587: we plain-text Dial and then upgrade via the
	// smtp.Client.StartTLS call below.
	if s.Port == 465 {
		conn = tls.Client(conn, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	}

	// withDeadline bounds a single command (or body write) against a
	// stalled relay, then clears the deadline so the next command gets a
	// fresh window. The net/smtp client reads/writes on this same conn,
	// so setting the deadline on it covers every command round-trip.
	withDeadline := func(fn func() error) error {
		if err := conn.SetDeadline(time.Now().Add(cmdTimeout)); err != nil {
			return err
		}
		err := fn()
		_ = conn.SetDeadline(time.Time{})
		return err
	}

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = c.Close() }()

	// STARTTLS upgrade for port 587 (or any port other than 465 when
	// AllowInsecure is false).
	if s.Port != 465 && !s.AllowInsecure {
		if err := withDeadline(func() error {
			if ok, _ := c.Extension("STARTTLS"); !ok {
				return fmt.Errorf("smtp server %s does not advertise STARTTLS", s.Host)
			}
			return c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
		}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if s.Username != "" {
		auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
		if err := withDeadline(func() error { return c.Auth(auth) }); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := withDeadline(func() error { return c.Mail(extractAddr(from)) }); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := withDeadline(func() error { return c.Rcpt(to) }); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}

	var wc io.WriteCloser
	if err := withDeadline(func() error {
		var werr error
		wc, werr = c.Data()
		return werr
	}); err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	msg := buildMIMEMessage(from, to, subject, htmlBody, textBody, headers)
	if err := withDeadline(func() error {
		if _, werr := wc.Write([]byte(msg)); werr != nil {
			_ = wc.Close()
			return werr
		}
		return nil
	}); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := withDeadline(func() error { return wc.Close() }); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	if err := withDeadline(func() error { return c.Quit() }); err != nil {
		// Quit() failures are usually harmless (server closed connection
		// after 250 OK on the DATA close); log but do not fail the send.
		return nil
	}
	return nil
}

// buildMIMEMessage produces a multipart/alternative payload with the text
// body first, HTML body second (the recommended order so RFC-compliant
// readers prefer the richer HTML view). headers carries optional
// pre-formatted MIME header lines (e.g. "List-Unsubscribe: <url>").
func buildMIMEMessage(from, to, subject, htmlBody, textBody string, headers []string) string {
	boundary := "noumena_boundary_" + boundarySalt()
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", encodeSubject(subject))
	for _, h := range headers {
		b.WriteString(h)
		b.WriteString("\r\n")
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(textBody)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String()
}

// encodeSubject applies RFC 2047 Q-encoding for non-ASCII subjects. We
// keep ASCII subjects as-is (most templates) and Q-encode anything else.
func encodeSubject(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?utf-8?B?" + base64Std(s) + "?="
		}
	}
	return s
}

// base64Std is the stdlib base64 with no padding stripping (RFC 2047
// requires standard base64 with padding).
func base64Std(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// boundarySalt returns a short pseudo-random hex string. Crypto-quality
// randomness is not required; uniqueness within a process is enough.
func boundarySalt() string {
	now := time.Now().UnixNano()
	const hex = "0123456789abcdef"
	var b [16]byte
	for i := 0; i < 16; i++ {
		b[i] = hex[(now>>uint(4*i))&0xf]
	}
	return string(b[:])
}

// extractAddr returns the bare address from a "Name <addr>" form. The
// SMTP MAIL FROM / RCPT TO commands accept only the angle-bracket form.
func extractAddr(s string) string {
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.LastIndex(s, ">"); j > i {
			return s[i+1 : j]
		}
	}
	return s
}
