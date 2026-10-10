package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lifeos.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const valid = `
http_addr: 127.0.0.1:8200
mcp_addr: 127.0.0.1:8100
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
`

func TestLoadReadsAllKeys(t *testing.T) {
	cfg, err := Load(writeConfig(t, valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		HTTPAddr:    "127.0.0.1:8200",
		MCPAddr:     "127.0.0.1:8100",
		DatabaseURL: "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
	}
	if cfg != want {
		t.Errorf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "address open to the network",
			content: strings.Replace(valid, "127.0.0.1:8200", "0.0.0.0:8200", 1),
			wantErr: "http_addr",
		},
		{
			name:    "address with no host",
			content: strings.Replace(valid, "127.0.0.1:8100", ":8100", 1),
			wantErr: "mcp_addr",
		},
		{
			name:    "missing address",
			content: strings.Replace(valid, "mcp_addr: 127.0.0.1:8100", "", 1),
			wantErr: "mcp_addr",
		},
		{
			name:    "missing database url",
			content: strings.Replace(valid, "database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable", "", 1),
			wantErr: "database_url",
		},
		{
			name:    "unknown key",
			content: valid + "htp_addr: 127.0.0.1:1\n",
			wantErr: "htp_addr",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.content))
			if err == nil {
				t.Fatal("Load returned no error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not name %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadReportsAMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("Load returned no error for a missing file")
	}
}
