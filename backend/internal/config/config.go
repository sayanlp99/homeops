package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr        string // listen address, e.g. ":8080"
	DatabaseURL string
	PublicURL   string // e.g. https://home.example.com — used for passkeys and push
	StaticDir   string // built frontend; empty disables static serving
	DataDir     string // local file storage when no S3 endpoint is set (dev/tests)
	S3Endpoint  string // host:port of the S3 API (SeaweedFS); empty = use DataDir
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
	VAPIDEmail  string // contact address sent to push services
	DevMode     bool   // relaxes Secure cookies for http://localhost
	SMTPHost    string // empty = log emails instead of sending (dev)
	SMTPPort    int
	SMTPUser    string
	SMTPPass    string
	MailFrom    string   // e.g. "HomeOps <no-reply@example.com>"
	GeminiKey   string   // Google AI Studio key; empty switches the AI helpers off
	GeminiModel []string // tried in order
}

func Load() (Config, error) {
	c := Config{
		Addr:        env("HOMEOPS_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://homeops:homeops@localhost:5432/homeops?sslmode=disable"),
		PublicURL:   strings.TrimRight(env("HOMEOPS_PUBLIC_URL", "http://localhost:5173"), "/"),
		StaticDir:   env("HOMEOPS_STATIC_DIR", ""),
		DataDir:     env("HOMEOPS_DATA_DIR", "./data"),
		S3Endpoint:  env("HOMEOPS_S3_ENDPOINT", ""),
		S3Bucket:    env("HOMEOPS_S3_BUCKET", "homeops"),
		S3AccessKey: env("HOMEOPS_S3_ACCESS_KEY", ""),
		S3SecretKey: env("HOMEOPS_S3_SECRET_KEY", ""),
		S3UseSSL:    env("HOMEOPS_S3_USE_SSL", "") == "1",
		VAPIDEmail:  env("HOMEOPS_VAPID_EMAIL", "admin@localhost"),
		DevMode:     env("HOMEOPS_DEV", "") == "1",
		SMTPHost:    env("HOMEOPS_SMTP_HOST", ""),
		SMTPUser:    env("HOMEOPS_SMTP_USER", ""),
		SMTPPass:    env("HOMEOPS_SMTP_PASSWORD", ""),
		MailFrom:    env("HOMEOPS_MAIL_FROM", "HomeOps <no-reply@localhost>"),
		GeminiKey:   env("HOMEOPS_GEMINI_API_KEY", ""),
		GeminiModel: strings.Split(env("HOMEOPS_GEMINI_MODELS", "gemini-3.5-flash-lite,gemini-3.5-flash"), ","),
	}
	if p := env("HOMEOPS_SMTP_PORT", "587"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			return c, fmt.Errorf("HOMEOPS_SMTP_PORT must be a number, got %q", p)
		}
		c.SMTPPort = n
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" {
		return c, fmt.Errorf("HOMEOPS_PUBLIC_URL must be an absolute URL, got %q", c.PublicURL)
	}
	if c.S3Endpoint != "" && (c.S3AccessKey == "" || c.S3SecretKey == "") {
		return c, fmt.Errorf("HOMEOPS_S3_ACCESS_KEY and HOMEOPS_S3_SECRET_KEY are required with HOMEOPS_S3_ENDPOINT")
	}
	return c, nil
}

// RPID is the WebAuthn relying party ID: the public hostname without port.
func (c Config) RPID() string {
	u, _ := url.Parse(c.PublicURL)
	return u.Hostname()
}

func (c Config) SecureCookies() bool {
	return strings.HasPrefix(c.PublicURL, "https://") && !c.DevMode
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
