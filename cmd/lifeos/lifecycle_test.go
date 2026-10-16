package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/config"
	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testaccess"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// startServer runs serveOn on free ports with a new database, a file store
// in a temporary folder, the read-only query pool and a stand-in for
// Cloudflare Access, which signs the tokens that the MCP server wants. The
// servers stop when the test ends.
func startServer(t *testing.T) (config.Config, *pgxpool.Pool, *testaccess.Access) {
	t.Helper()
	pool := testdb.New(t)
	cfg := freeLoopbackConfig(t)
	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svc := service.New(pool).WithFiles(service.Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: 1 << 20})
	if err := svc.UseQueryPool(context.Background(), testdb.QueryPool(t, pool)); err != nil {
		t.Fatalf("UseQueryPool: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	access := testaccess.New(t)
	go func() {
		done <- serveOn(ctx, httpListener, mcpListener, svc, lifeosmcp.NewAccessCheck(access.Issuer, access.Audience))
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return cfg, pool, access
}

// send makes one HTTP request and returns the status and the body.
func send(t *testing.T, method, url, contentType, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}

// One record goes through its life: an agent infers it, Gautam corrects
// it, approves it and deletes it. Each step uses a different adapter, and
// each adapter must see what the others did.
func TestServeTakesOneRecordThroughItsLife(t *testing.T) {
	cfg, pool, access := startServer(t)
	ctx := context.Background()
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, mcpTransport(cfg.MCPAddr, access), nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	defer session.Close()
	tool := func(name string, args map[string]any) *sdk.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("MCP %s: %v", name, err)
		}
		return result
	}

	// 1. MCP: an agent infers an item. It is not verified.
	added := tool("add_item", map[string]any{"kind": "book", "title": "Inferred through MCP", "rating": 4})
	if added.IsError {
		t.Fatalf("add_item: %+v", added)
	}
	id := int64(added.StructuredContent.(map[string]any)["id"].(float64))
	item := fmt.Sprintf("%s/api/items/%d", rest, id)

	// 2. REST: the item is there at once; nothing waits for approval.
	if status, body := send(t, "GET", item, "", ""); status != 200 || !strings.Contains(body, "Inferred through MCP") {
		t.Fatalf("GET item: status %d, body %s", status, body)
	}

	// 3. REST: a correction of one field.
	if status, body := send(t, "PATCH", item, "application/json", `{"rating":3}`); status != 200 {
		t.Fatalf("PATCH: status %d, body %s", status, body)
	}

	// 4. MCP: get shows the correction.
	got := tool("get", map[string]any{"type": "item", "id": id})
	if record := got.StructuredContent.(map[string]any); got.IsError || record["rating"] != float64(3) {
		t.Fatalf("MCP get after the correction: %+v", got.StructuredContent)
	}

	// 5. MCP: the agent changes the record that Gautam corrected. The
	// event keeps the old value.
	updated := tool("update", map[string]any{"type": "item", "id": id, "fields": map[string]any{"title": "Changed through MCP"}})
	if updated.IsError {
		t.Fatalf("MCP update: %+v", updated.StructuredContent)
	}
	if _, body := send(t, "GET", rest+"/admin/items", "", ""); !strings.Contains(body, "Changed through MCP") {
		t.Errorf("the items page does not show the changed record")
	}

	// 6. Admin: delete. REST and MCP must not find the record after that.
	if status, body := send(t, "POST", fmt.Sprintf("%s/admin/items/%d/delete", rest, id), form, ""); status != 200 || strings.Contains(body, "Changed through MCP") {
		t.Fatalf("POST delete: status %d, body %s", status, body)
	}
	if status, _ := send(t, "GET", item, "", ""); status != 404 {
		t.Errorf("GET after delete: status %d, want 404", status)
	}
	if gone := tool("get", map[string]any{"type": "item", "id": id}); !gone.IsError {
		t.Errorf("MCP get after delete: %+v, want an error", gone.StructuredContent)
	}

	// 7. Each write has its event, with the adapter that made it.
	rows, err := pool.Query(ctx, "SELECT action || ' by ' || source FROM events ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	want := "create by agent, update by manual, update by agent, delete by manual"
	if got := strings.Join(events, ", "); got != want {
		t.Errorf("events = %s\nwant     %s", got, want)
	}
}
