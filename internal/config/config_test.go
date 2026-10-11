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
query_database_url: postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable
files_dir: /var/lib/lifeos/files
inbox_dir: /var/lib/lifeos/inbox
max_file_mb: 50
`

func TestLoadReadsAllKeys(t *testing.T) {
	cfg, err := Load(writeConfig(t, valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		HTTPAddr:         "127.0.0.1:8200",
		MCPAddr:          "127.0.0.1:8100",
		DatabaseURL:      "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
		QueryDatabaseURL: "postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable",
		FilesDir:         "/var/lib/lifeos/files",
		InboxDir:         "/var/lib/lifeos/inbox",
		MaxFileMB:        50,
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
			name:    "missing query database url",
			content: strings.Replace(valid, "query_database_url: postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable", "", 1),
			wantErr: "query_database_url",
		},
		{
			name:    "file store that is not a full path",
			content: strings.Replace(valid, "files_dir: /var/lib/lifeos/files", "files_dir: files", 1),
			wantErr: "files_dir",
		},
		{
			name:    "missing inbox",
			content: strings.Replace(valid, "inbox_dir: /var/lib/lifeos/inbox", "", 1),
			wantErr: "inbox_dir",
		},
		{
			name:    "inbox that is the file store",
			content: strings.Replace(valid, "inbox_dir: /var/lib/lifeos/inbox", "inbox_dir: /var/lib/lifeos/files", 1),
			wantErr: "inbox_dir",
		},
		{
			name:    "missing file limit",
			content: strings.Replace(valid, "max_file_mb: 50", "", 1),
			wantErr: "max_file_mb",
		},
		{
			name:    "file limit below 1",
			content: strings.Replace(valid, "max_file_mb: 50", "max_file_mb: -1", 1),
			wantErr: "max_file_mb",
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
