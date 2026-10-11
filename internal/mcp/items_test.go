package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// connect starts the lifeos MCP server in this process and returns a client
// session that talks to it.
func connect(t *testing.T) (*sdk.ClientSession, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	return connectTo(t, service.New(pool)), pool
}

// connectTo starts the lifeos MCP server for svc in this process and
// returns a client session that talks to it.
func connectTo(t *testing.T, svc *service.Service) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := lifeosmcp.NewServer(svc)

	clientSide, serverSide := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func call(t *testing.T, session *sdk.ClientSession, tool string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", tool, err)
	}
	return result
}

// text joins the text content of a tool result.
func text(result *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func structured(t *testing.T, result *sdk.CallToolResult) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool returned an error: %s", text(result))
	}
	m, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want a JSON object", result.StructuredContent)
	}
	return m
}

func TestServerListsTheTools(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := map[string]bool{}
	for _, tool := range list.Tools {
		found[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has no description", tool.Name)
		}
	}
	for _, name := range []string{"add_item", "get", "capture"} {
		if !found[name] {
			t.Errorf("tool %s is not registered", name)
		}
	}
}

func TestAddItemThenGet(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "add_item", map[string]any{
		"kind": "movie", "title": "Dune: Part Two", "rating": 4, "occurred_on": "2024-03-01", "dictated": true,
	}))
	if added["source"] != "agent" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", added["source"], added["verified"])
	}

	got := structured(t, call(t, session, "get", map[string]any{"type": "item", "id": added["id"]}))
	if got["title"] != "Dune: Part Two" || got["rating"] != float64(4) {
		t.Errorf("get returned %v", got)
	}
}

// A row that the agent inferred must wait for review. The safe default when
// the model leaves out "dictated" is not verified.
func TestAddItemIsNotVerifiedUnlessDictated(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "add_item", map[string]any{"kind": "book", "title": "Dune"}))

	if added["verified"] != false {
		t.Errorf("verified = %v, want false when dictated is absent", added["verified"])
	}
}

func TestToolsReturnErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantText string
	}{
		{"rating the service refuses", "add_item", map[string]any{"kind": "movie", "title": "Dune", "rating": 7}, "rating"},
		{"kind the service refuses", "add_item", map[string]any{"kind": "film", "title": "Dune"}, "kind"},
		{"misspelled argument", "add_item", map[string]any{"kind": "movie", "title": "Dune", "ratting": 4}, "ratting"},
		{"missing title", "add_item", map[string]any{"kind": "movie"}, "title"},
		{"rating of the wrong type", "add_item", map[string]any{"kind": "movie", "title": "Dune", "rating": "four"}, "rating"},
		{"unknown ID", "get", map[string]any{"type": "item", "id": 9999}, "not found"},
		{"unknown type", "get", map[string]any{"type": "song", "id": 1}, "type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, tt.tool, tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("catalog_items has %d rows after rejected calls, want 0", n)
	}
}

func TestDatabaseFailureGivesAPlainError(t *testing.T) {
	session, pool := connect(t)
	pool.Close()

	result := call(t, session, "add_item", map[string]any{"kind": "movie", "title": "Dune"})

	if !result.IsError {
		t.Fatal("IsError = false, want an error result")
	}
	if got := text(result); got != "internal error" {
		t.Errorf("error text = %q, want only \"internal error\"", got)
	}
}
