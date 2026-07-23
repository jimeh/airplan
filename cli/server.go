package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/jimeh/airplan/internal/httpapi"
	"github.com/jimeh/airplan/internal/serverlog"
	"github.com/spf13/cobra"
)

type serveOptions struct {
	config           string
	profile          string
	listen           string
	tokenFile        string
	allowedOrigins   []string
	tempDir          string
	logLevel         string
	allowNonLoopback bool
}

func newServeCmd() *cobra.Command {
	opts := &serveOptions{}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Airplan REST API and Streamable HTTP MCP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	flags.StringVarP(&opts.profile, "profile", "p", "",
		"S3 config profile used by the server")
	flags.StringVar(&opts.listen, "listen", "127.0.0.1:8080",
		"HTTP listen address")
	flags.StringVar(&opts.tokenFile, "token-file", "",
		"file containing the static server bearer token")
	flags.StringSliceVar(&opts.allowedOrigins, "allowed-origin", nil,
		"allowed browser Origin for Streamable HTTP MCP (repeatable)")
	flags.StringVar(&opts.tempDir, "temp-dir", "",
		"directory for bounded collection upload spooling")
	flags.StringVar(&opts.logLevel, "log-level", "",
		"server log level: error, warn, info, debug, or trace (default: info)")
	flags.BoolVar(&opts.allowNonLoopback, "allow-non-loopback", false,
		"acknowledge non-loopback serving requires trusted reverse-proxy TLS")
	return cmd
}

func runServe(cmd *cobra.Command, opts *serveOptions) error {
	level, err := serveLogLevel(cmd, opts.logLevel)
	if err != nil {
		return err
	}
	logger := serverlog.New(cmd.ErrOrStderr(), level)
	if !isLoopbackListen(opts.listen) && !opts.allowNonLoopback {
		return errors.New(
			"non-loopback --listen requires --allow-non-loopback; " +
				"terminate TLS at a trusted reverse proxy",
		)
	}
	token, err := loadServerToken(opts.tokenFile, os.Getenv("AIRPLAN_SERVER_TOKEN"))
	if err != nil {
		return err
	}
	cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}
	if cfg.EffectiveBackend() != airplan.BackendS3 {
		return errors.New("airplan serve requires an s3 backend profile")
	}
	client, err := airplan.New(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	if err := client.StorageReady(cmd.Context()); err != nil {
		return err
	}

	operations := &airplan.HTTPOperations{
		Client: client, ServerVersion: buildVersion(),
	}
	rest, err := httpapi.NewHandler(operations, httpapi.Options{
		Token: token, TempDir: opts.tempDir, Logger: logger,
		MaxDocumentBytes:        DefaultServerDocumentBytes(),
		MaxCollectionFileBytes:  airplan.DefaultMaxCollectionFileSize,
		MaxCollectionTotalBytes: airplan.DefaultMaxCollectionTotalSize,
		MaxCollectionFiles:      airplan.MaxCollectionFiles,
	})
	if err != nil {
		return fmt.Errorf("airplan: construct HTTP API: %w", err)
	}
	mcpHandler, err := airplan.NewMCPHTTPHandlerWithOptions(
		client, buildVersion(), airplan.MCPHTTPOptions{
			AllowedOrigins: opts.allowedOrigins,
			Logger:         logger,
		},
	)
	if err != nil {
		return err
	}
	auth, err := httpapi.NewBearerAuth(token)
	if err != nil {
		return fmt.Errorf("airplan: construct MCP authentication: %w", err)
	}
	handler := newServeHandler(rest, mcpHandler, auth, logger)

	listener, err := net.Listen("tcp", opts.listen)
	if err != nil {
		return fmt.Errorf("airplan: listen on %s: %w", opts.listen, err)
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20,
	}
	ctx, stop := signal.NotifyContext(
		cmd.Context(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	fmt.Fprintf(cmd.ErrOrStderr(), "airplan: serving on http://%s\n",
		listener.Addr())
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("airplan: HTTP server: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("airplan: HTTP server shutdown: %w", err)
	}
	return nil
}

func serveLogLevel(cmd *cobra.Command, flagValue string) (slog.Level, error) {
	value := flagValue
	if !cmd.Flags().Changed("log-level") {
		value = os.Getenv("AIRPLAN_SERVER_LOG_LEVEL")
	}
	return serverlog.ParseLevel(value)
}

func newServeHandler(
	rest, mcpHandler http.Handler,
	auth *httpapi.BearerAuth,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()
	mcpRoute := auth.WrapWithLogger(mcpHandler, logger, "mcp")
	mux.Handle("/mcp", serverlog.HTTPMiddleware(
		logger, "mcp", func(*http.Request) string { return "/mcp" }, mcpRoute,
	))
	mux.Handle("/mcp/", serverlog.HTTPMiddleware(
		logger, "mcp", func(*http.Request) string { return "/mcp" }, mcpRoute,
	))
	mux.Handle("/", serverlog.HTTPMiddleware(
		logger, "rest", safeRESTLogPath, rest,
	))
	return mux
}

func safeRESTLogPath(request *http.Request) string {
	switch request.URL.Path {
	case "/healthz", "/openapi.yaml",
		"/api/v1/capabilities",
		"/api/v1/uploads",
		"/api/v1/storage/uploads",
		"/api/v1/uploads/documents",
		"/api/v1/uploads/collections",
		"/api/v1/uploads/get",
		"/api/v1/uploads/inspect",
		"/api/v1/uploads/delete",
		"/api/v1/sync",
		"/api/v1/purge/preview",
		"/api/v1/purge":
		return request.URL.Path
	default:
		return "unmatched"
	}
}

// DefaultServerDocumentBytes keeps CLI server construction explicit while the
// transport enforces the same documented library default.
func DefaultServerDocumentBytes() int64 { return airplan.DefaultMaxInputSize }

func loadServerToken(tokenFile, environment string) (string, error) {
	if tokenFile != "" && environment != "" {
		return "", errors.New(
			"airplan: set either --token-file or AIRPLAN_SERVER_TOKEN, not both",
		)
	}
	token := environment
	if tokenFile != "" {
		body, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("airplan: read server token file: %w", err)
		}
		token = strings.TrimSpace(string(body))
	}
	if len(token) < 32 {
		return "", errors.New(
			"airplan: server bearer token must be at least 32 bytes",
		)
	}
	return token, nil
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type mcpOptions struct {
	config  string
	profile string
}

func newMCPCmd() *cobra.Command {
	opts := &mcpOptions{}
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve Airplan tools over MCP stdio",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
			if err != nil {
				return err
			}
			client, err := airplan.New(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			return airplan.RunMCPStdio(cmd.Context(), client, buildVersion())
		},
	}
	cmd.Flags().StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	cmd.Flags().StringVarP(&opts.profile, "profile", "p", "",
		"config profile (s3 or airplan backend)")
	return cmd
}
