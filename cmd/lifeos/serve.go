package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/admin"
	"github.com/Gautam-J/lifeos-core/internal/api"
	"github.com/Gautam-J/lifeos-core/internal/config"
	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
)

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "config/lifeos.yaml", "path to the config file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	pc, err := poolConfig(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}

	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		return err
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, service.New(pool))
}

// listen opens the two listeners before any request is served, so that a
// port that is in use stops the start with an error that names the key.
func listen(cfg config.Config) (httpListener, mcpListener net.Listener, err error) {
	httpListener, err = net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("http_addr: %w", err)
	}
	mcpListener, err = net.Listen("tcp", cfg.MCPAddr)
	if err != nil {
		httpListener.Close()
		return nil, nil, fmt.Errorf("mcp_addr: %w", err)
	}
	return httpListener, mcpListener, nil
}

// serveOn serves the REST API and the admin pages on httpListener and the
// MCP server on mcpListener. It returns nil after ctx is cancelled and the
// servers have shut down.
func serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service) error {
	router := chi.NewRouter()
	api.Register(router, svc)
	admin.Register(router, svc)

	mcpMux := http.NewServeMux()
	mcpMux.Handle("/mcp", lifeosmcp.Handler(svc))

	servers := []*http.Server{
		{Handler: router, ReadHeaderTimeout: 10 * time.Second},
		{Handler: mcpMux, ReadHeaderTimeout: 10 * time.Second},
	}
	failed := make(chan error, len(servers))
	for i, listener := range []net.Listener{httpListener, mcpListener} {
		go func() { failed <- servers[i].Serve(listener) }()
	}

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var errs []error
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
