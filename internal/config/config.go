// Package config loads config/lifeos.yaml, the one configuration file.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// Config is the content of config/lifeos.yaml.
type Config struct {
	// HTTPAddr is the listen address of the REST API and the admin pages.
	HTTPAddr string `yaml:"http_addr"`
	// MCPAddr is the listen address of the MCP server.
	MCPAddr string `yaml:"mcp_addr"`
	// DatabaseURL is the Postgres URL without a password. The password
	// comes from the POSTGRES_PASSWORD environment variable.
	DatabaseURL string `yaml:"database_url"`
	// QueryDatabaseURL is the Postgres URL of the read-only user, for the
	// query tool. The password comes from the LIFEOS_QUERY_PASSWORD
	// environment variable.
	QueryDatabaseURL string `yaml:"query_database_url"`
	// FilesDir is the file store: the folder that holds the captured
	// files.
	FilesDir string `yaml:"files_dir"`
	// InboxDir is the folder where a file waits for its capture.
	InboxDir string `yaml:"inbox_dir"`
	// MaxFileMB is the largest file that a capture accepts, in megabytes.
	MaxFileMB int64 `yaml:"max_file_mb"`
	// WikiDir is the folder of the wiki pages. It is in a git repository:
	// each write of a page is a commit.
	WikiDir string `yaml:"wiki_dir"`
}

// Load reads and checks the file at path. It rejects unknown keys, a missing
// value, a listen address that is not on the loopback interface, and a
// folder that is not a full path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: %s: database_url is empty", path)
	}
	if cfg.QueryDatabaseURL == "" {
		return Config{}, fmt.Errorf("config: %s: query_database_url is empty", path)
	}
	for key, addr := range map[string]string{"http_addr": cfg.HTTPAddr, "mcp_addr": cfg.MCPAddr} {
		if err := checkLoopback(addr); err != nil {
			return Config{}, fmt.Errorf("config: %s: %s: %w", path, key, err)
		}
	}
	for key, dir := range map[string]string{"files_dir": cfg.FilesDir, "inbox_dir": cfg.InboxDir, "wiki_dir": cfg.WikiDir} {
		if !filepath.IsAbs(dir) {
			return Config{}, fmt.Errorf("config: %s: %s: %q is not a full path such as /var/lib/lifeos/files", path, key, dir)
		}
	}
	if filepath.Clean(cfg.InboxDir) == filepath.Clean(cfg.FilesDir) {
		return Config{}, fmt.Errorf("config: %s: inbox_dir: must not be the folder of files_dir", path)
	}
	if cfg.MaxFileMB < 1 {
		return Config{}, fmt.Errorf("config: %s: max_file_mb: must be 1 or more", path)
	}
	return cfg, nil
}

// checkLoopback accepts only host:port with a loopback IP address. Every
// LifeOS service listens on 127.0.0.1; the Cloudflare Tunnel is the way in.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not host:port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%q is not a loopback address such as 127.0.0.1:8200", addr)
	}
	return nil
}
