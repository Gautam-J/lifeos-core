package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func TestCaptureATextThenGetAndDeleteIt(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "capture", map[string]any{"text": "  Renewal on 12 March.\n", "title": "Insurance"}))
	if added["kind"] != "text" || added["content"] != "  Renewal on 12 March.\n" || added["title"] != "Insurance" {
		t.Errorf("source = %v, want the text exactly as it was sent", added)
	}
	// A capture is what came in. No agent inferred it, so it is verified.
	if added["source"] != "agent" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", added["source"], added["verified"])
	}

	got := structured(t, call(t, session, "get", map[string]any{"type": "source", "id": added["id"]}))
	if got["content"] != added["content"] || got["sha256"] != added["sha256"] {
		t.Errorf("get = %v, want the source", got)
	}

	again := structured(t, call(t, session, "capture", map[string]any{"text": "  Renewal on 12 March.\n"}))
	if again["id"] != added["id"] {
		t.Errorf("second capture has the ID %v, want %v", again["id"], added["id"])
	}

	update := call(t, session, "update", map[string]any{"type": "source", "id": added["id"], "fields": map[string]any{"content": "x"}})
	if !update.IsError || !strings.Contains(text(update), "a source has no field that you can change") {
		t.Errorf("update of a source = %q, want it refused", text(update))
	}
	refused := call(t, session, "delete", map[string]any{"type": "source", "id": added["id"]})
	if !refused.IsError || !strings.Contains(text(refused), "confirmed") {
		t.Errorf("delete without confirmed = %q, want it refused", text(refused))
	}
	deleted := structured(t, call(t, session, "delete", map[string]any{"type": "source", "id": added["id"], "confirmed": true}))
	if deleted["deleted"] != true {
		t.Errorf("delete = %v", deleted)
	}
}

func TestCaptureALink(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "capture", map[string]any{"url": "https://example.com/article"}))
	if added["kind"] != "link" || added["url"] != "https://example.com/article" {
		t.Errorf("source = %v", added)
	}
}

func TestCaptureAFileFromTheInbox(t *testing.T) {
	inbox, files := t.TempDir(), t.TempDir()
	svc := service.New(testdb.New(t)).WithFiles(service.Files{Dir: files, Inbox: inbox, MaxBytes: 4096})
	session := connectTo(t, svc)
	pdf := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	if err := os.WriteFile(filepath.Join(inbox, "policy.pdf"), pdf, 0o600); err != nil {
		t.Fatal(err)
	}

	added := structured(t, call(t, session, "capture", map[string]any{"path": "policy.pdf"}))
	if added["kind"] != "file" || added["title"] != "policy.pdf" || added["media_type"] != "application/pdf" {
		t.Errorf("source = %v", added)
	}
	if _, err := os.Stat(filepath.Join(files, added["file_path"].(string))); err != nil {
		t.Errorf("the file is not in the file store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inbox, "policy.pdf")); err == nil {
		t.Error("the file is still in the inbox")
	}

	outside := call(t, session, "capture", map[string]any{"path": "/etc/hostname"})
	if !outside.IsError || !strings.Contains(text(outside), "path: must name a file in the inbox folder") {
		t.Errorf("capture of a file outside the inbox = %q, want it refused", text(outside))
	}
}

func TestCaptureRejectsBadInput(t *testing.T) {
	session, pool := connect(t)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"nothing to capture", map[string]any{}, "text: give one of these"},
		{"a text and a link", map[string]any{"text": "a note", "url": "https://example.com"}, "text: give one of these"},
		{"a card number", map[string]any{"text": "card 4111 1111 1111 1111"}, "text: must not hold a card number"},
		{"a link with no scheme", map[string]any{"url": "example.com"}, "url: must start with http:// or https://"},
		{"an argument that the tool does not have", map[string]any{"text": "a note", "dictated": true}, "dictated"},
	}
	for _, tt := range tests {
		result := call(t, session, "capture", tt.args)
		if !result.IsError || !strings.Contains(text(result), tt.want) {
			t.Errorf("%s: result = %q, want an error with %q", tt.name, text(result), tt.want)
		}
		if strings.Contains(text(result), "4111 1111") {
			t.Errorf("%s: the error repeats the card number: %q", tt.name, text(result))
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM sources").Scan(&n); err != nil || n != 0 {
		t.Errorf("sources = %d, %v, want 0", n, err)
	}
}

func TestARecordNamesTheSourceThatItComesFrom(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "gym")
	source := structured(t, call(t, session, "capture", map[string]any{"text": "Read Dune. Renew the policy. Went to the gym."}))

	item := structured(t, call(t, session, "add_item", map[string]any{"kind": "book", "title": "Dune", "source_ref": source["id"]}))
	todo := structured(t, call(t, session, "todo", map[string]any{"title": "Renew the policy", "source_ref": source["id"]}))
	log := structured(t, call(t, session, "log_habit", map[string]any{"habit": "gym", "source_ref": source["id"]}))
	for name, record := range map[string]map[string]any{"add_item": item, "todo": todo, "log_habit": log} {
		if record["source_ref"] != source["id"] || record["verified"] != false {
			t.Errorf("%s: source_ref, verified = %v, %v, want %v, false", name, record["source_ref"], record["verified"], source["id"])
		}
	}

	unknown := call(t, session, "todo", map[string]any{"title": "Call the agent", "source_ref": 9999})
	if !unknown.IsError || !strings.Contains(text(unknown), "source_ref: must be the ID of a source that exists") {
		t.Errorf("a todo with an unknown source_ref = %q, want it refused", text(unknown))
	}
}
