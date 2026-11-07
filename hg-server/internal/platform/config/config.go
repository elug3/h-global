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
