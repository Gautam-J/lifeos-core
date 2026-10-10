// Package config loads config/lifeos.yaml, the one configuration file.
package config

import (
	"fmt"
	"net"
	"os"

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
}

// Load reads and checks the file at path. It rejects unknown keys, a missing
// value, and a listen address that is not on the loopback interface.
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
	for key, addr := range map[string]string{"http_addr": cfg.HTTPAddr, "mcp_addr": cfg.MCPAddr} {
		if err := checkLoopback(addr); err != nil {
			return Config{}, fmt.Errorf("config: %s: %s: %w", path, key, err)
		}
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
