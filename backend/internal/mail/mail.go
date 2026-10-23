// Package mail sends transactional email (invites, password resets) over SMTP.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
	Enabled() bool
}

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "HomeOps <no-reply@example.com>"
}

// New returns an SMTP sender, or one that only logs when no host is configured (dev).
func New(c Config) (Sender, error) {
	if c.Host == "" {
		return logSender{}, nil
	}
	if _, err := mail.ParseAddress(c.From); err != nil {
		return nil, fmt.Errorf("HOMEOPS_MAIL_FROM: %w", err)
	}
	if c.Port == 0 {
		c.Port = 587
	}
	return &smtpSender{c}, nil
}

type logSender struct{}

func (logSender) Enabled() bool { return false }
func (logSender) Send(_ context.Context, m Message) error {
	slog.Info("email (not sent: no SMTP configured)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

type smtpSender struct{ c Config }

func (s *smtpSender) Enabled() bool { return true }

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	from, _ := mail.ParseAddress(s.c.From)
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("bad recipient: %w", err)
	}
	msg := build(from, to, m)

	deadline := time.Now().Add(20 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	addr := net.JoinHostPort(s.c.Host, strconv.Itoa(s.c.Port))
	// A mail server on this machine typically has a self-signed certificate.
	tlsCfg := &tls.Config{ServerName: s.c.Host, InsecureSkipVerify: isLoopback(s.c.Host)}
	var conn net.Conn
	dialer := &net.Dialer{Deadline: deadline}
	if s.c.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Hello(helloName(from.Address)); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok && s.c.Port != 465 {
		if err := c.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if s.c.Username != "" {
		if err := c.Auth(loginAuth{s.c.Username, s.c.Password, s.c.Host}); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// loginAuth is PLAIN auth that, unlike smtp.PlainAuth, also allows an unencrypted
// connection to a loopback server.
type loginAuth struct{ user, pass, host string }

func (a loginAuth) Start(si *smtp.ServerInfo) (string, []byte, error) {
	if !si.TLS && !isLoopback(a.host) {
		return "", nil, errors.New("refusing to send SMTP password over an unencrypted connection")
	}
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.pass), nil
}

func (a loginAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected SMTP auth challenge")
	}
	return nil, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func helloName(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}

func build(from, to *mail.Address, m Message) []byte {
	var b bytes.Buffer
	id := randomHex(12)
	h := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	h("From", from.String())
	h("To", to.String())
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", time.Now().Format(time.RFC1123Z))
	h("Message-ID", fmt.Sprintf("<%s@%s>", id, helloName(from.Address)))
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")
	boundary := "ho-" + id
	if m.HTML == "" {
		h("Content-Type", `text/plain; charset="utf-8"`)
		h("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(&b, m.Text)
		return b.Bytes()
	}
	h("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
	b.WriteString("\r\n")
	for _, part := range []struct{ typ, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=\"utf-8\"\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, part.typ)
		writeQP(&b, part.body)
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

func writeQP(b *bytes.Buffer, s string) {
	w := quotedprintable.NewWriter(b)
	w.Write([]byte(strings.ReplaceAll(s, "\n", "\r\n")))
	w.Close()
}

func randomHex(n int) string {
	buf := make([]byte, n)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}
