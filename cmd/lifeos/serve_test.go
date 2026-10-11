package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/config"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// freeLoopbackConfig returns a config whose two addresses are free ports.
func freeLoopbackConfig(t *testing.T) config.Config {
	t.Helper()
	addrs := make([]string, 2)
	for i := range addrs {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addrs[i] = l.Addr().String()
		l.Close()
	}
	return config.Config{HTTPAddr: addrs[0], MCPAddr: addrs[1]}
}

// One item goes in through each adapter. Each adapter must then see the
// two items, and each write must have its event: the three adapters use
// one service and one database.
func TestServeRunsOneFlowThroughEveryAdapter(t *testing.T) {
	pool := testdb.New(t)
	cfg := freeLoopbackConfig(t)
	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, service.New(pool)) }()

	// 1. REST: add an item.
	resp, err := http.Post("http://"+cfg.HTTPAddr+"/api/items", "application/json",
		strings.NewReader(`{"kind":"movie","title":"Added through REST","rating":4}`))
	if err != nil {
		t.Fatalf("POST /api/items: %v", err)
	}
	var added struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&added); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || added.ID == 0 {
		t.Fatalf("POST /api/items: status %d, id %d", resp.StatusCode, added.ID)
	}

	// 2. MCP over HTTP: read that item, then add a second one.
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	// The session stays open until the test ends: an MCP client is still
	// connected when the servers stop.
	defer session.Close()
	got, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get", Arguments: map[string]any{"type": "item", "id": added.ID}})
	if err != nil || got.IsError {
		t.Fatalf("MCP get: err %v, result %+v", err, got)
	}
	if title := got.StructuredContent.(map[string]any)["title"]; title != "Added through REST" {
		t.Errorf("MCP get title = %v, want the item that REST added", title)
	}
	second, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "add_item", Arguments: map[string]any{
		"kind": "book", "title": "Added through MCP", "dictated": true,
	}})
	if err != nil || second.IsError {
		t.Fatalf("MCP add_item: err %v, result %+v", err, second)
	}

	// 3. Admin page: the two items show.
	page, err := http.Get("http://" + cfg.HTTPAddr + "/admin/items")
	if err != nil {
		t.Fatalf("GET /admin/items: %v", err)
	}
	html, _ := io.ReadAll(page.Body)
	page.Body.Close()
	for _, title := range []string{"Added through REST", "Added through MCP"} {
		if !strings.Contains(string(html), title) {
			t.Errorf("the admin page does not show %q", title)
		}
	}

	// 4. Each write has its event, with the source of its adapter.
	rows, err := pool.Query(ctx, "SELECT source FROM events ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	if fmt.Sprint(sources) != "[manual agent]" {
		t.Errorf("event sources = %v, want [manual agent]", sources)
	}

	// 5. A cancelled context stops the two servers.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serveOn returned %v after cancel, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serveOn did not return within 15 s after cancel")
	}
}

func TestListenNamesTheConfigKeyOfABusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	for _, key := range []string{"http_addr", "mcp_addr"} {
		t.Run(key, func(t *testing.T) {
			cfg := freeLoopbackConfig(t)
			if key == "http_addr" {
				cfg.HTTPAddr = busy.Addr().String()
			} else {
				cfg.MCPAddr = busy.Addr().String()
			}

			_, _, err := listen(cfg)

			if err == nil {
				t.Fatal("listen returned no error for a port that is in use")
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error %q does not name %s", err, key)
			}
			// The free port of this config must be released again.
			free := cfg.MCPAddr
			if key == "mcp_addr" {
				free = cfg.HTTPAddr
			}
			l, err := net.Listen("tcp", free)
			if err != nil {
				t.Errorf("the other listener stayed open: %v", err)
			} else {
				l.Close()
			}
		})
	}
}

func TestFileStoreNeedsTheTwoFolders(t *testing.T) {
	dir := t.TempDir()
	notAFolder := filepath.Join(dir, "a-file")
	if err := os.WriteFile(notAFolder, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := fileStore(config.Config{FilesDir: dir, InboxDir: t.TempDir(), MaxFileMB: 50})
	if err != nil {
		t.Fatalf("fileStore: %v", err)
	}
	if files.Dir != dir || files.MaxBytes != 50<<20 {
		t.Errorf("files = %+v, want the folder and 50 MB in bytes", files)
	}

	for key, cfg := range map[string]config.Config{
		"files_dir": {FilesDir: filepath.Join(dir, "absent"), InboxDir: dir, MaxFileMB: 50},
		"inbox_dir": {FilesDir: dir, InboxDir: notAFolder, MaxFileMB: 50},
	} {
		_, err := fileStore(cfg)
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("error = %v, want an error that names %s", err, key)
		}
	}
}

// gitRepo makes a new git repository in a temporary folder and returns the
// folder wiki in it.
func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	wiki := filepath.Join(repo, "wiki")
	if err := os.Mkdir(wiki, 0o700); err != nil {
		t.Fatal(err)
	}
	return wiki
}

func TestWikiStoreNeedsAFolderInAGitRepository(t *testing.T) {
	dir := gitRepo(t)

	wiki, err := wikiStore(config.Config{WikiDir: dir})
	if err != nil {
		t.Fatalf("wikiStore: %v", err)
	}
	if wiki.Dir != dir {
		t.Errorf("wiki.Dir = %q, want %q", wiki.Dir, dir)
	}

	for name, dir := range map[string]string{
		"a folder that is absent":                  filepath.Join(dir, "absent"),
		"a folder that is not in a git repository": t.TempDir(),
	} {
		_, err := wikiStore(config.Config{WikiDir: dir})
		if err == nil || !strings.Contains(err.Error(), "wiki_dir") {
			t.Errorf("%s: error = %v, want an error that names wiki_dir", name, err)
		}
	}
}

func TestQueryPoolConfigTakesThePasswordFromTheEnvironment(t *testing.T) {
	t.Setenv("LIFEOS_QUERY_PASSWORD", "from-the-environment")

	pc, err := queryPoolConfig(config.Config{QueryDatabaseURL: "postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable"})
	if err != nil {
		t.Fatalf("queryPoolConfig: %v", err)
	}
	if pc.ConnConfig.User != "lifeos_query" || pc.ConnConfig.Password != "from-the-environment" || pc.MaxConns != 2 {
		t.Errorf("user, max connections = %q, %d; the password is from the environment: %v",
			pc.ConnConfig.User, pc.MaxConns, pc.ConnConfig.Password == "from-the-environment")
	}

	_, err = queryPoolConfig(config.Config{QueryDatabaseURL: "not a url"})
	if err == nil || !strings.Contains(err.Error(), "query_database_url") {
		t.Errorf("error = %v, want an error that names query_database_url", err)
	}
}
