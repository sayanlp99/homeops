package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Addr        string // listen address, e.g. ":8080"
	DatabaseURL string
	PublicURL   string // e.g. https://home.example.com — used for passkeys and push
	StaticDir   string // built frontend; empty disables static serving
	VAPIDEmail  string // contact address sent to push services
	DevMode     bool   // relaxes Secure cookies for http://localhost
}

func Load() (Config, error) {
	c := Config{
		Addr:        env("HOMEOPS_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://homeops:homeops@localhost:5432/homeops?sslmode=disable"),
		PublicURL:   strings.TrimRight(env("HOMEOPS_PUBLIC_URL", "http://localhost:5173"), "/"),
		StaticDir:   env("HOMEOPS_STATIC_DIR", ""),
		VAPIDEmail:  env("HOMEOPS_VAPID_EMAIL", "admin@localhost"),
		DevMode:     env("HOMEOPS_DEV", "") == "1",
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" {
		return c, fmt.Errorf("HOMEOPS_PUBLIC_URL must be an absolute URL, got %q", c.PublicURL)
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
