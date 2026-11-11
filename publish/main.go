// Command publish serves the dppdemo.publish.v1.Publisher gRPC API: it signs the credentials of
// a template with the gaia-x-signer and publishes the template's offering through the Ecosystems
// Gateway.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"dpp-demo/publish/gen/gatewayv3"
	"dpp-demo/publish/gen/publishv1"
	"dpp-demo/publish/gen/signerpb"
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

	signerConn, err := dial(cfg.SignerAddr, cfg.SignerTLS)
	if err != nil {
		return fmt.Errorf("signer: %w", err)
	}
	defer signerConn.Close()
	gatewayConn, err := dial(cfg.GatewayAddr, cfg.GatewayTLS)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	defer gatewayConn.Close()

	svc := &publisher{
		dppDir:      cfg.DPPDir,
		templateDir: cfg.TemplateDir,
		issuer:      cfg.IssuerDID,
		signerAuth:  cfg.SignerAuthorization,
		timeout:     cfg.PublishTimeout,
		signer:      signerpb.NewSignerClient(signerConn),
		gateway:     gatewayv3.NewEcosystemsgatewayClient(gatewayConn),
	}

	srv := grpc.NewServer(grpc.MaxRecvMsgSize(cfg.MaxMessageBytes))
	publishv1.RegisterPublisherServer(srv, svc)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		srv.GracefulStop()
	}()
	slog.Info("listening", "addr", cfg.Addr, "signer", cfg.SignerAddr, "gateway", cfg.GatewayAddr)
	return srv.Serve(lis)
}

func dial(addr string, useTLS bool) (*grpc.ClientConn, error) {
	creds := insecure.NewCredentials()
	if useTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	return grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
}

type config struct {
	Addr                string
	DPPDir              string
	TemplateDir         string
	IssuerDID           string
	MaxMessageBytes     int
	PublishTimeout      time.Duration
	SignerAddr          string
	SignerTLS           bool
	SignerAuthorization string
	GatewayAddr         string
	GatewayTLS          bool
}

func loadConfig() (config, error) {
	e := envReader{}
	c := config{
		Addr:                e.str("GRPC_ADDR", ":50051"),
		DPPDir:              e.str("DPP_DIR", "/data/dpps"),
		TemplateDir:         e.str("TEMPLATE_DIR", "/data/templates"),
		IssuerDID:           e.str("ISSUER_DID", ""),
		MaxMessageBytes:     e.int("MAX_MESSAGE_BYTES", 16<<20),
		PublishTimeout:      e.duration("PUBLISH_TIMEOUT", 10*time.Minute),
		SignerAddr:          e.required("SIGNER_ADDR"),
		SignerTLS:           e.bool("SIGNER_TLS", false),
		SignerAuthorization: e.str("SIGNER_AUTHORIZATION", ""),
		GatewayAddr:         e.required("GATEWAY_ADDR"),
		GatewayTLS:          e.bool("GATEWAY_TLS", false),
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

func (e *envReader) required(key string) string {
	v := os.Getenv(key)
	if v == "" {
		e.fail(key, fmt.Errorf("required"))
	}
	return v
}

func (e *envReader) bool(key string, def bool) bool {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.fail(key, err)
	}
	return b
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
