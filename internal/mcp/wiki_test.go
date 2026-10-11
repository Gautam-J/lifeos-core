package mcp_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// connectWithWiki is connect, with an empty wiki in a new git repository
// in a temporary folder.
func connectWithWiki(t *testing.T) (*sdk.ClientSession, *pgxpool.Pool) {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	dir := filepath.Join(repo, "wiki")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	pool := testdb.New(t)
	return connectTo(t, service.New(pool).WithWiki(service.Wiki{Dir: dir})), pool
}

// wantToolError fails the test unless the result is an error whose text
// has part.
func wantToolError(t *testing.T, result *sdk.CallToolResult, part string) {
	t.Helper()
	if !result.IsError || !strings.Contains(text(result), part) {
		t.Errorf("result: error = %t, text = %q, want an error that has %q", result.IsError, text(result), part)
	}
}

func TestWikiToolsWriteReadSearchAppendAndDeleteAPage(t *testing.T) {
	session, _ := connectWithWiki(t)
	entry := structured(t, call(t, session, "journal", map[string]any{"body": "Designed the wiki tools today."}))
	cite := fmt.Sprintf("(journal:%v)", entry["id"])

	written := structured(t, call(t, session, "wiki_write", map[string]any{
		"path": "projects/lifeos.md", "title": "LifeOS", "summary": "A second brain on a Raspberry Pi.",
		"body": "LifeOS keeps facts in rows.\n\n- 2026-10-11: The wiki tools are designed " + cite,
	}))
	if written["type"] != "project" || written["status"] != "active" || written["hash"] == "" {
		t.Errorf("written page = %v, want a project with the status active and a hash", written)
	}

	read := structured(t, call(t, session, "wiki_read", map[string]any{"path": "projects/lifeos.md"}))
	if read["hash"] != written["hash"] || read["title"] != "LifeOS" || !strings.Contains(read["body"].(string), cite) {
		t.Errorf("read page = %v", read)
	}
	index := structured(t, call(t, session, "wiki_read", map[string]any{"path": "index.md"}))
	if !strings.Contains(index["body"].(string), "- [LifeOS](projects/lifeos.md): A second brain on a Raspberry Pi.") {
		t.Errorf("index body = %q", index["body"])
	}

	changed := structured(t, call(t, session, "wiki_write", map[string]any{
		"path": "projects/lifeos.md", "title": "LifeOS", "summary": "A second brain on a Raspberry Pi.",
		"body": "LifeOS keeps each fact in a row " + cite, "status": "paused", "contested": true, "base_hash": written["hash"],
	}))
	if changed["status"] != "paused" || changed["contested"] != true || changed["hash"] == written["hash"] {
		t.Errorf("changed page = %v", changed)
	}

	// A write that sends no status and no mark keeps the two.
	kept := structured(t, call(t, session, "wiki_write", map[string]any{
		"path": "projects/lifeos.md", "title": "LifeOS", "summary": "A second brain on a Raspberry Pi.",
		"body": "LifeOS keeps each fact in one row " + cite, "base_hash": changed["hash"],
	}))
	if kept["status"] != "paused" || kept["contested"] != true {
		t.Errorf("page after a write with no status and no mark = %v, want paused and contested", kept)
	}

	appended := structured(t, call(t, session, "wiki_append", map[string]any{"path": "projects/lifeos.md", "text": "- 2026-10-12: One more line."}))
	if !strings.HasSuffix(appended["body"].(string), cite+"\n- 2026-10-12: One more line.") {
		t.Errorf("appended body = %q", appended["body"])
	}
	logged := structured(t, call(t, session, "wiki_append", map[string]any{"path": "log.md", "text": "## [2026-10-11] journal | " + cite}))
	if logged["type"] != "log" {
		t.Errorf("log = %v", logged)
	}

	found := structured(t, call(t, session, "wiki_search", map[string]any{"query": "raspberry"}))
	pages := found["pages"].([]any)
	if len(pages) != 1 || pages[0].(map[string]any)["path"] != "projects/lifeos.md" {
		t.Errorf("search result = %v, want the one page", found)
	}
	all := structured(t, call(t, session, "wiki_search", map[string]any{}))
	if len(all["pages"].([]any)) != 1 {
		t.Errorf("list of pages = %v, want one page", all)
	}

	lint := structured(t, call(t, session, "wiki_lint", map[string]any{}))
	// The page is marked as contested, and no other page links to it.
	findings := lint["findings"].([]any)
	if len(findings) != 2 || findings[0].(map[string]any)["check"] != "contested" || findings[1].(map[string]any)["check"] != "orphan" {
		t.Errorf("lint findings = %v, want contested and orphan", findings)
	}

	wantToolError(t, call(t, session, "wiki_delete", map[string]any{"path": "projects/lifeos.md"}), "confirmed")
	deleted := structured(t, call(t, session, "wiki_delete", map[string]any{"path": "projects/lifeos.md", "confirmed": true}))
	if deleted["deleted"] != true {
		t.Errorf("delete result = %v", deleted)
	}
	wantToolError(t, call(t, session, "wiki_read", map[string]any{"path": "projects/lifeos.md"}), "not found")
}

func TestWikiToolsReportWhatIsWrong(t *testing.T) {
	session, _ := connectWithWiki(t)
	page := map[string]any{"path": "topics/agent-design.md", "title": "Agent design", "summary": "S.", "body": "Text."}
	written := structured(t, call(t, session, "wiki_write", page))

	// A second write without the hash of the stored page.
	page["body"] = "A change."
	wantToolError(t, call(t, session, "wiki_write", page), "base_hash")
	page["base_hash"] = written["hash"]
	changed := structured(t, call(t, session, "wiki_write", page))
	// The first hash is stale now.
	page["body"] = "The change of a different session."
	wantToolError(t, call(t, session, "wiki_write", page), "Read it again")
	page["base_hash"] = changed["hash"]

	page["body"] = "A fact with no row (movie:999)."
	wantToolError(t, call(t, session, "wiki_write", page), "(movie:999)")
	wantToolError(t, call(t, session, "wiki_append", map[string]any{"path": "topics/agent-design.md", "text": "- (src:999)"}), "(src:999)")

	wantToolError(t, call(t, session, "wiki_read", map[string]any{"path": "../secret.md"}), "path")
	wantToolError(t, call(t, session, "wiki_write", map[string]any{"path": "index.md", "title": "T", "summary": "S.", "body": "B."}), "index.md")
	wantToolError(t, call(t, session, "wiki_append", map[string]any{"path": "topics/absent.md", "text": "- a line"}), "not found")
	wantToolError(t, call(t, session, "wiki_search", map[string]any{"limit": 500}), "limit")
}

func TestWikiToolsWriteAsAnAgent(t *testing.T) {
	session, pool := connectWithWiki(t)
	call(t, session, "wiki_write", map[string]any{"path": "topics/agent-design.md", "title": "Agent design", "summary": "S.", "body": "Text."})

	var action, source string
	err := pool.QueryRow(context.Background(), "SELECT action, source FROM events WHERE record_type = 'wiki'").Scan(&action, &source)
	if err != nil || action != "create" || source != "agent" {
		t.Errorf("event = %q by %q, error %v, want create by agent", action, source, err)
	}
}

func TestWikiToolsAreListedWithTheirRules(t *testing.T) {
	session, _ := connectWithWiki(t)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	descriptions := map[string]string{}
	for _, tool := range list.Tools {
		descriptions[tool.Name] = tool.Description
	}
	for _, name := range []string{"wiki_search", "wiki_read", "wiki_write", "wiki_append", "wiki_delete", "wiki_lint", "mark_processed"} {
		if descriptions[name] == "" {
			t.Errorf("the tool %s is not registered, or it has no description", name)
		}
	}
	// The agent learns the citation prefixes from the description.
	for _, prefix := range service.WikiCitationPrefixes {
		if !strings.Contains(descriptions["wiki_write"], prefix) {
			t.Errorf("the description of wiki_write does not name the prefix %s", prefix)
		}
	}
	for name, part := range map[string]string{
		"wiki_write":  "base_hash",
		"wiki_append": "log.md",
		"wiki_delete": "only after the user says yes",
		"wiki_lint":   "It changes nothing",
		"wiki_read":   "index.md",
	} {
		if !strings.Contains(descriptions[name], part) {
			t.Errorf("the description of %s does not have %q", name, part)
		}
	}
}

func TestMarkProcessedTool(t *testing.T) {
	session, _ := connect(t)
	source := structured(t, call(t, session, "capture", map[string]any{"text": "An article about agents."}))
	if source["processed_at"] != nil {
		t.Fatalf("a new source has processed_at = %v", source["processed_at"])
	}

	marked := structured(t, call(t, session, "mark_processed", map[string]any{"id": source["id"]}))
	if marked["processed_at"] == nil || marked["id"] != source["id"] {
		t.Errorf("marked source = %v, want the source with a time in processed_at", marked)
	}
	wantToolError(t, call(t, session, "mark_processed", map[string]any{"id": 999}), "not found")
}
