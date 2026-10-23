// Command mailbridge receives email from a Cloudflare Email Worker over HTTPS (through
// the Cloudflare Tunnel) and hands it to the local mail server over SMTP. It lets a home
// server receive mail for its domain without opening port 25 or exposing its IP.
//
// The worker POSTs the raw message to /inbound with:
//
//	Authorization: Bearer <MAILBRIDGE_TOKEN>
//	X-Envelope-From: sender@example.com   (may be empty for bounces)
//	X-Envelope-To:   me@example.com
//
// Responses tell the worker what to do: 200 delivered, 422 rejected for good (the worker
// bounces it), 503 try again later (the worker falls back to forwarding).
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const maxMessage = 30 << 20 // Cloudflare Email Routing caps messages at 25 MiB

type bridge struct {
	token []byte
	smtp  string // host:port of the local SMTP listener
	helo  string
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	b := &bridge{
		token: []byte(os.Getenv("MAILBRIDGE_TOKEN")),
		smtp:  env("MAILBRIDGE_SMTP", "127.0.0.1:2525"),
		helo:  env("MAILBRIDGE_HELO", "bridge.localhost"),
	}
	if len(b.token) < 32 {
		slog.Error("MAILBRIDGE_TOKEN must be set to a random string of at least 32 characters")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /inbound", b.inbound)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok\n") })
	srv := &http.Server{
		Addr:              env("MAILBRIDGE_ADDR", "127.0.0.1:8025"),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(shut)
	}()
	slog.Info("mailbridge listening", "addr", srv.Addr, "smtp", b.smtp)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}

func (b *bridge) inbound(w http.ResponseWriter, r *http.Request) {
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(auth), b.token) != 1 {
		// Usually a missing or mistyped BRIDGE_TOKEN secret on the Email Worker.
		slog.Warn("unauthorized request", "ip", r.Header.Get("CF-Connecting-IP"), "hasToken", auth != "", "to", r.Header.Get("X-Envelope-To"))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	from, err := envelopeAddr(r.Header.Get("X-Envelope-From"), true)
	if err != nil {
		http.Error(w, "bad sender: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	to, err := envelopeAddr(r.Header.Get("X-Envelope-To"), false)
	if err != nil {
		http.Error(w, "bad recipient: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessage))
	if err != nil {
		http.Error(w, "message too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	start := time.Now()
	err = b.deliver(r.Context(), from, to, body)
	var perm *permanentError
	switch {
	case err == nil:
		slog.Info("delivered", "from", from, "to", to, "bytes", len(body), "ms", time.Since(start).Milliseconds())
		w.WriteHeader(http.StatusOK)
	case errors.As(err, &perm):
		slog.Warn("rejected", "from", from, "to", to, "reason", err)
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		slog.Error("deferred", "from", from, "to", to, "err", err)
		http.Error(w, "temporary failure: "+err.Error(), http.StatusServiceUnavailable)
	}
}

type permanentError struct{ msg string }

func (e *permanentError) Error() string { return e.msg }

// classify turns an SMTP reply into a permanent (5xx) or temporary error.
func classify(err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) && tp.Code >= 500 {
		return &permanentError{fmt.Sprintf("%d %s", tp.Code, tp.Msg)}
	}
	return err
}

func (b *bridge) deliver(ctx context.Context, from, to string, msg []byte) error {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", b.smtp)
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(90 * time.Second))
	host, _, _ := net.SplitHostPort(b.smtp)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Hello(b.helo); err != nil {
		return classify(err)
	}
	if err := c.Mail(from); err != nil {
		return classify(err)
	}
	if err := c.Rcpt(to); err != nil {
		return classify(err)
	}
	wc, err := c.Data()
	if err != nil {
		return classify(err)
	}
	if _, err := wc.Write(normalizeCRLF(msg)); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return classify(err)
	}
	return c.Quit()
}

// envelopeAddr validates a bare address; header injection (CR/LF, <>) is refused.
func envelopeAddr(s string, allowEmpty bool) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "<>" {
		if allowEmpty {
			return "", nil
		}
		return "", errors.New("missing")
	}
	if strings.ContainsAny(s, "\r\n<> ") || len(s) > 320 {
		return "", errors.New("invalid characters")
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return "", errors.New("not an email address")
	}
	return s, nil
}

// normalizeCRLF makes line endings CRLF, as SMTP requires.
func normalizeCRLF(b []byte) []byte {
	if !bytes.Contains(b, []byte("\n")) || bytes.Count(b, []byte("\r\n")) == bytes.Count(b, []byte("\n")) {
		return b
	}
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/40)
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), maxMessage)
	for sc.Scan() {
		out.Write(bytes.TrimSuffix(sc.Bytes(), []byte("\r")))
		out.WriteString("\r\n")
	}
	return out.Bytes()
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
