package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// A link, a receipt and a note come in through three adapters. The agent
// finds the note, takes a book from it and names the note as its source.
// Each adapter must find what the others stored.
func TestServeTakesACaptureThroughEveryAdapter(t *testing.T) {
	cfg, pool := startServer(t)
	ctx := context.Background()
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"
	const json = "application/json"

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	defer session.Close()
	call := func(name string, args map[string]any) *sdk.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("MCP %s: %v", name, err)
		}
		return result
	}
	tool := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result := call(name, args)
		if result.IsError {
			t.Fatalf("MCP %s returned an error: %+v", name, result.Content[0])
		}
		return result.StructuredContent.(map[string]any)
	}

	// 1. REST: the share sheet sends a link, then a file.
	status, body := send(t, "POST", rest+"/api/ingest", json, `{"url":"https://example.com/lonavala-guide"}`)
	if status != 200 || !strings.Contains(body, `"kind":"link"`) {
		t.Fatalf("POST /api/ingest: status %d, body %s", status, body)
	}
	status, body = send(t, "POST", rest+"/api/ingest/file?name=receipt.pdf", "application/pdf", "%PDF-1.7\n1 0 obj\nendobj\n")
	if status != 200 || !strings.Contains(body, `"media_type":"application/pdf"`) || !strings.Contains(body, `"title":"receipt.pdf"`) {
		t.Fatalf("POST /api/ingest/file: status %d, body %s", status, body)
	}

	// 2. Admin: Gautam types a note.
	status, body = send(t, "POST", rest+"/admin/sources", form, "text=Read+Dune+on+the+train+to+Lonavala.&title=Trip+note")
	if status != 200 || !strings.Contains(body, "Trip note") || !strings.Contains(body, "receipt.pdf") || !strings.Contains(body, "lonavala-guide") {
		t.Fatalf("POST /admin/sources: status %d, body %s", status, body)
	}

	// 3. MCP: the agent looks for Lonavala. The newest source is first.
	hits := tool("search", map[string]any{"query": "lonavala"})["hits"].([]any)
	if len(hits) != 2 {
		t.Fatalf("search: hits = %v, want the note and the link", hits)
	}
	note := hits[0].(map[string]any)
	if note["type"] != "source" || note["summary"] != "text: Trip note: Read Dune on the train to Lonavala." {
		t.Fatalf("first hit = %v, want the note", note)
	}
	link := hits[1].(map[string]any)

	// 4. MCP: the agent reads the note, and proposes a book that names
	// the note as its source. REST shows the book with its source.
	if got := tool("get", map[string]any{"type": "source", "id": note["id"]}); got["content"] != "Read Dune on the train to Lonavala." {
		t.Fatalf("get of the note: %v", got)
	}
	book := tool("add_item", map[string]any{"kind": "book", "title": "Dune", "source_ref": note["id"]})
	if book["source_ref"] != note["id"] || book["verified"] != false {
		t.Fatalf("add_item: %v, want a proposed book with the source_ref of the note", book)
	}
	status, body = send(t, "GET", rest+"/api/search?query=dune", "", "")
	if status != 200 || !strings.Contains(body, `"summary":"book: Dune"`) || !strings.Contains(body, "Trip note") {
		t.Fatalf("GET /api/search: status %d, body %s", status, body)
	}

	// 5. MCP: the same link a second time is the same source.
	if again := tool("capture", map[string]any{"url": "https://example.com/lonavala-guide"}); again["id"] != link["id"] || again["source"] != "manual" {
		t.Fatalf("second capture of the link: %v, want the source %v as it was", again, link["id"])
	}

	// 6. MCP: a question that needs a count. The query cannot change data.
	counts := tool("query", map[string]any{"sql": "SELECT kind, count(*) FROM sources WHERE deleted_at IS NULL GROUP BY kind ORDER BY kind"})
	if got := fmt.Sprint(counts["rows"]); got != "[[file 1] [link 1] [text 1]]" {
		t.Fatalf("query: rows = %s, want one source of each kind", got)
	}
	if refused := call("query", map[string]any{"sql": "DELETE FROM sources"}); !refused.IsError {
		t.Fatal("query: a DELETE gave no error")
	}

	// 7. Each adapter refuses a card number, and none repeats it.
	const card = "4111 1111 1111 1111"
	status, body = send(t, "POST", rest+"/api/ingest", json, `{"text":"my card is `+card+`"}`)
	if status != 422 || !strings.Contains(body, "a card number") || strings.Contains(body, card) {
		t.Fatalf("REST with a card number: status %d, body %s", status, body)
	}
	refused := call("journal", map[string]any{"body": "new card: " + card})
	if text := refused.Content[0].(*sdk.TextContent).Text; !refused.IsError || !strings.Contains(text, "a card number") || strings.Contains(text, card) {
		t.Fatalf("MCP journal with a card number: %q", text)
	}
	status, body = send(t, "POST", rest+"/admin/todos", form, "title=pay+with+"+strings.ReplaceAll(card, " ", "+"))
	if status != 200 || !strings.Contains(body, "title: must not hold a card number") || strings.Contains(body, card) {
		t.Fatalf("admin todo with a card number: status %d, body %s", status, body)
	}

	// 8. The log has one event for each write: three sources and a book.
	var events, withCard int
	err = pool.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE new_value::text LIKE '%4111%') FROM events").Scan(&events, &withCard)
	if err != nil || events != 4 || withCard != 0 {
		t.Fatalf("events = %d, with the card number = %d, %v, want 4, 0", events, withCard, err)
	}
}
