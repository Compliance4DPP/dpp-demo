// Command publish serves an HTTP/JSON API that signs the credentials of a template with the
// gaia-x-signer and publishes the template's offering through the Ecosystems Gateway, both
// reached over their HTTP/JSON APIs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	p := &publisher{
		dppDir:      cfg.DPPDir,
		templateDir: cfg.TemplateDir,
		issuer:      cfg.IssuerDID,
		providerURL: cfg.ProviderCredentialURL,
		timeout:     cfg.PublishTimeout,
		maxBody:     cfg.MaxRequestBytes,
		signer:      &apiClient{name: "signer", base: cfg.SignerURL, auth: cfg.SignerAuthorization},
		gateway:     &apiClient{name: "gateway", base: cfg.GatewayURL, auth: bearer(cfg.GatewayToken)},
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           p.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", cfg.Addr, "signer", cfg.SignerURL, "gateway", cfg.GatewayURL)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func bearer(token string) string {
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

type config struct {
	Addr        string
	DPPDir      string
	TemplateDir string
	IssuerDID   string
	// ProviderCredentialURL is the provider's Gaia-X participant credential
	ProviderCredentialURL string
	MaxRequestBytes       int64
	PublishTimeout        time.Duration
	SignerURL             string
	SignerAuthorization   string
	GatewayURL            string
	GatewayToken          string
}

func loadConfig() (config, error) {
	e := envReader{}
	c := config{
		Addr:                  e.str("HTTP_ADDR", ":8080"),
		DPPDir:                e.str("DPP_DIR", "/data/dpps"),
		TemplateDir:           e.str("TEMPLATE_DIR", "/data/templates"),
		IssuerDID:             e.str("ISSUER_DID", ""),
		ProviderCredentialURL: e.str("PROVIDER_CREDENTIAL_URL", ""),
		MaxRequestBytes:       int64(e.int("MAX_REQUEST_BYTES", 16<<20)),
		PublishTimeout:        e.duration("PUBLISH_TIMEOUT", 10*time.Minute),
		SignerURL:             e.url("SIGNER_URL"),
		SignerAuthorization:   e.str("SIGNER_AUTHORIZATION", ""),
		GatewayURL:            e.url("GATEWAY_URL"),
		GatewayToken:          e.str("GATEWAY_TOKEN", ""),
	}
	if e.err != nil {
		return config{}, e.err
	}
	return c, nil
}

// envReader reads environment variables and keeps the first error.
type envReader struct{ err error }

func (e *envReader) fail(key string, err error) {
	if e.err == nil {
		e.err = fmt.Errorf("%s: %w", key, err)
	}
}

func (e *envReader) str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// url reads a required http(s) base URL.
func (e *envReader) url(key string) string {
	v := os.Getenv(key)
	u, err := url.Parse(v)
	switch {
	case v == "":
		e.fail(key, errors.New("required"))
	case err != nil:
		e.fail(key, err)
	case (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
		e.fail(key, errors.New("want an http(s) URL"))
	}
	return v
}

func (e *envReader) int(key string, def int) int {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		e.fail(key, err)
	}
	return i
}

func (e *envReader) duration(key string, def time.Duration) time.Duration {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.fail(key, err)
	}
	return d
}
