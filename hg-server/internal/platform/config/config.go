// Package config reads hg-server's settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting. See README.md for the full table.
type Config struct {
	HTTPAddr           string        // HG_HTTP_ADDR
	DatabaseURL        string        // HG_DATABASE_URL (required)
	StorageDir         string        // HG_STORAGE_DIR
	MediaBaseURL       string        // HG_MEDIA_BASE_URL: prefix for public media URLs
	CookieSecure       bool          // HG_COOKIE_SECURE
	AllowedOrigin      []string      // HG_ALLOWED_ORIGINS: comma-separated, for unsafe methods
	OwnerEmail         string        // HG_OWNER_EMAIL: creates the first owner when no staff exist
	OwnerPassword      string        // HG_OWNER_PASSWORD
	HoldTTL            time.Duration // HG_HOLD_TTL: how long Buy now holds stock
	VirtualAcctTTL     time.Duration // HG_VIRTUAL_ACCOUNT_TTL: deposit deadline for virtual accounts
	StreamDelay        time.Duration // HG_STREAM_DELAY: subtracted from capture offsets
	ShippingFeeWon     int64         // HG_SHIPPING_FEE_WON
	StaffSessionTTL    time.Duration // HG_STAFF_SESSION_TTL
	CustomerSessionTTL time.Duration // HG_CUSTOMER_SESSION_TTL
	BcryptCost         int           // HG_BCRYPT_COST
	PaymentProvider    string        // HG_PAYMENT_PROVIDER: "fake" until a real provider is chosen
	FakePaymentSecret  string        // HG_FAKE_PAYMENT_SECRET
	DevRoutes          bool          // HG_DEV_ROUTES: enables /api/dev/* (fake payment approval)

	Relay Relay
}

// Relay configures the broadcast relay (plan 5.5): an ffmpeg process that
// receives the show's SRT stream, copies it to YouTube and taps frames.
type Relay struct {
	Enabled       bool   // HG_RELAY_ENABLED
	Listen        string // HG_RELAY_LISTEN: SRT listen address, host:port
	PublicHost    string // HG_RELAY_PUBLIC_HOST: host:port the broadcaster connects to
	Secret        string // HG_RELAY_SECRET: derives per-show ingest keys (required when enabled)
	YouTubeURL    string // HG_YOUTUBE_RTMP_URL: ingest base URL
	YouTubeKey    string // HG_YOUTUBE_STREAM_KEY: empty = do not push to YouTube
	FFmpeg        string // HG_FFMPEG: ffmpeg binary
	LatencyMillis int    // HG_RELAY_SRT_LATENCY_MS: SRT receive buffer for lost packets
	LocalCopy     string // HG_RELAY_LOCAL_COPY: local UDP host:port between ingest and the YouTube pusher
}

// Load reads the environment.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:          env("HG_HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("HG_DATABASE_URL"),
		StorageDir:        env("HG_STORAGE_DIR", "./data/storage"),
		MediaBaseURL:      strings.TrimRight(env("HG_MEDIA_BASE_URL", "/media"), "/"),
		OwnerEmail:        os.Getenv("HG_OWNER_EMAIL"),
		OwnerPassword:     os.Getenv("HG_OWNER_PASSWORD"),
		PaymentProvider:   env("HG_PAYMENT_PROVIDER", "fake"),
		FakePaymentSecret: env("HG_FAKE_PAYMENT_SECRET", "dev-only-secret"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("HG_DATABASE_URL is required")
	}
	var err error
	if c.CookieSecure, err = envBool("HG_COOKIE_SECURE", true); err != nil {
		return c, err
	}
	if c.DevRoutes, err = envBool("HG_DEV_ROUTES", false); err != nil {
		return c, err
	}
	if v := os.Getenv("HG_ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigin = append(c.AllowedOrigin, o)
			}
		}
	}
	durations := []struct {
		dst  *time.Duration
		name string
		def  time.Duration
	}{
		{&c.HoldTTL, "HG_HOLD_TTL", 10 * time.Minute},
		{&c.VirtualAcctTTL, "HG_VIRTUAL_ACCOUNT_TTL", 24 * time.Hour},
		{&c.StreamDelay, "HG_STREAM_DELAY", 10 * time.Second},
		{&c.StaffSessionTTL, "HG_STAFF_SESSION_TTL", 12 * time.Hour},
		{&c.CustomerSessionTTL, "HG_CUSTOMER_SESSION_TTL", 30 * 24 * time.Hour},
	}
	for _, d := range durations {
		if *d.dst, err = envDuration(d.name, d.def); err != nil {
			return c, err
		}
	}
	if c.ShippingFeeWon, err = envInt64("HG_SHIPPING_FEE_WON", 0); err != nil {
		return c, err
	}
	cost, err := envInt64("HG_BCRYPT_COST", 12)
	if err != nil {
		return c, err
	}
	c.BcryptCost = int(cost)
	if c.Relay, err = loadRelay(); err != nil {
		return c, err
	}
	if c.PaymentProvider != "fake" {
		return c, fmt.Errorf("HG_PAYMENT_PROVIDER %q is not supported yet (only \"fake\")", c.PaymentProvider)
	}
	return c, nil
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envBool(name string, def bool) (bool, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return b, nil
}

func envDuration(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}

func envInt64(name string, def int64) (int64, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return n, nil
}

func loadRelay() (Relay, error) {
	r := Relay{
		Listen:     env("HG_RELAY_LISTEN", "0.0.0.0:9000"),
		PublicHost: env("HG_RELAY_PUBLIC_HOST", "localhost:9000"),
		Secret:     os.Getenv("HG_RELAY_SECRET"),
		YouTubeURL: strings.TrimRight(env("HG_YOUTUBE_RTMP_URL", "rtmps://a.rtmp.youtube.com/live2"), "/"),
		YouTubeKey: os.Getenv("HG_YOUTUBE_STREAM_KEY"),
		FFmpeg:     env("HG_FFMPEG", "ffmpeg"),
		LocalCopy:  env("HG_RELAY_LOCAL_COPY", "127.0.0.1:19400"),
	}
	var err error
	if r.Enabled, err = envBool("HG_RELAY_ENABLED", false); err != nil {
		return r, err
	}
	ms, err := envInt64("HG_RELAY_SRT_LATENCY_MS", 300)
	if err != nil {
		return r, err
	}
	r.LatencyMillis = int(ms)
	if r.Enabled && len(r.Secret) < 16 {
		return r, fmt.Errorf("HG_RELAY_SECRET must be at least 16 characters when the relay is enabled")
	}
	return r, nil
}
