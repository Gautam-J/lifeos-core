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
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// startServer runs serveOn on free ports with a new database. The servers
// stop when the test ends.
func startServer(t *testing.T) (config.Config, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	cfg := freeLoopbackConfig(t)
	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, service.New(pool)) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return cfg, pool
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
	cfg, pool := startServer(t)
	ctx := context.Background()
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
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

	// 2. REST: the review queue shows it.
	if status, body := send(t, "GET", rest+"/api/review", "", ""); status != 200 || !strings.Contains(body, "book: Inferred through MCP, rating 4") {
		t.Fatalf("GET /api/review: status %d, body %s", status, body)
	}

	// 3. REST: a correction of one field.
	if status, body := send(t, "PATCH", item, "application/json", `{"rating":3}`); status != 200 {
		t.Fatalf("PATCH: status %d, body %s", status, body)
	}

	// 4. Admin: the review page shows the corrected record, and Approve
	// takes it out of the queue.
	if status, body := send(t, "GET", rest+"/admin/review", "", ""); status != 200 || !strings.Contains(body, "book: Inferred through MCP, rating 3") {
		t.Fatalf("GET /admin/review: status %d, body %s", status, body)
	}
	status, body := send(t, "POST", rest+"/admin/review/verify", form, fmt.Sprintf("type=item&id=%d", id))
	if status != 200 || !strings.Contains(body, "No record waits for review.") {
		t.Fatalf("POST /admin/review/verify: status %d, body %s", status, body)
	}

	// 5. MCP: get shows the correction and the approval.
	got := tool("get", map[string]any{"type": "item", "id": id})
	if record := got.StructuredContent.(map[string]any); got.IsError || record["rating"] != float64(3) || record["verified"] != true {
		t.Fatalf("MCP get after the approval: %+v", got.StructuredContent)
	}

	// 6. MCP: a change that the agent inferred sends the record back to
	// the queue.
	updated := tool("update", map[string]any{"type": "item", "id": id, "fields": map[string]any{"title": "Changed through MCP"}})
	if updated.IsError || updated.StructuredContent.(map[string]any)["verified"] != false {
		t.Fatalf("MCP update: %+v", updated.StructuredContent)
	}
	if _, body := send(t, "GET", rest+"/api/review", "", ""); !strings.Contains(body, "Changed through MCP") {
		t.Errorf("the review queue does not show the changed record: %s", body)
	}

	// 7. Admin: delete. REST and MCP must not find the record after that.
	if status, body := send(t, "POST", fmt.Sprintf("%s/admin/items/%d/delete", rest, id), form, ""); status != 200 || strings.Contains(body, "Changed through MCP") {
		t.Fatalf("POST delete: status %d, body %s", status, body)
	}
	if status, _ := send(t, "GET", item, "", ""); status != 404 {
		t.Errorf("GET after delete: status %d, want 404", status)
	}
	if gone := tool("get", map[string]any{"type": "item", "id": id}); !gone.IsError {
		t.Errorf("MCP get after delete: %+v, want an error", gone.StructuredContent)
	}

	// 8. Each write has its event, with the adapter that made it.
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
	want := "create by agent, update by manual, verify by manual, update by agent, delete by manual"
	if got := strings.Join(events, ", "); got != want {
		t.Errorf("events = %s\nwant     %s", got, want)
	}
}
