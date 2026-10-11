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
	"os/exec"
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
	files, err := fileStore(cfg)
	if err != nil {
		return err
	}
	queryConfig, err := queryPoolConfig(cfg)
	if err != nil {
		return err
	}
	wiki, err := wikiStore(cfg)
	if err != nil {
		return err
	}
	access, err := accessCheck()
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

	queryPool, err := pgxpool.NewWithConfig(ctx, queryConfig)
	if err != nil {
		return fmt.Errorf("query_database_url: %w", err)
	}
	defer queryPool.Close()
	svc := service.New(pool).WithFiles(files).WithWiki(wiki)
	if err := svc.UseQueryPool(ctx, queryPool); err != nil {
		return fmt.Errorf("query_database_url: %w", err)
	}

	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		return err
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, svc, access)
}

// fileStore returns the settings of the file store. It checks that the two
// folders exist, so that a wrong path stops the start and not a capture.
func fileStore(cfg config.Config) (service.Files, error) {
	for _, folder := range []struct{ key, dir string }{{"files_dir", cfg.FilesDir}, {"inbox_dir", cfg.InboxDir}} {
		info, err := os.Stat(folder.dir)
		if err != nil {
			return service.Files{}, fmt.Errorf("%s: %w", folder.key, err)
		}
		if !info.IsDir() {
			return service.Files{}, fmt.Errorf("%s: %s is not a folder", folder.key, folder.dir)
		}
	}
	return service.Files{Dir: cfg.FilesDir, Inbox: cfg.InboxDir, MaxBytes: cfg.MaxFileMB << 20}, nil
}

// wikiStore returns the settings of the wiki. It checks that the folder
// exists and that it is in a git repository, so that a wrong path stops the
// start and not the first write of a page.
func wikiStore(cfg config.Config) (service.Wiki, error) {
	info, err := os.Stat(cfg.WikiDir)
	if err != nil {
		return service.Wiki{}, fmt.Errorf("wiki_dir: %w", err)
	}
	if !info.IsDir() {
		return service.Wiki{}, fmt.Errorf("wiki_dir: %s is not a folder", cfg.WikiDir)
	}
	if err := exec.Command("git", "-C", cfg.WikiDir, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return service.Wiki{}, fmt.Errorf("wiki_dir: %s is not in a git repository: %w", cfg.WikiDir, err)
	}
	return service.Wiki{Dir: cfg.WikiDir}, nil
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
// MCP server on mcpListener. The MCP server answers only a request that
// has a token that access accepts. It returns nil after ctx is cancelled
// and the servers have shut down.
func serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service, access *lifeosmcp.AccessCheck) error {
	router := chi.NewRouter()
	api.Register(router, svc)
	admin.Register(router, svc)

	mcpMux := http.NewServeMux()
	mcpMux.Handle("/mcp", lifeosmcp.Handler(svc, access))

	// Cloudflare Access is the login, and the browser sends that login
	// with each request, also with one that a page of a different site
	// makes. This refuses a request that changes data and that a browser
	// sent from a different origin. A program, such as a Shortcut, sends
	// no header about its origin and goes through.
	sameOrigin := http.NewCrossOriginProtection()

	servers := []*http.Server{
		{Handler: sameOrigin.Handler(router), ReadHeaderTimeout: 10 * time.Second},
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
