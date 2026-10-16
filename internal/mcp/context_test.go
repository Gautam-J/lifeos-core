package mcp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func wikiService(t *testing.T) *service.Service {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	dir := filepath.Join(repo, "wiki")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return service.New(testdb.New(t)).WithWiki(service.Wiki{Dir: dir})
}

// #35: the Claude app does not give the instructions to the model, so the
// tool context is the way in, and the routing block says to call it first.
func TestContextToolReturnsMeOrSaysHowToMakeIt(t *testing.T) {
	session := connectTo(t, wikiService(t))

	missing := structured(t, call(t, session, "context", map[string]any{}))
	if note, _ := missing["note"].(string); !strings.Contains(note, "me.md") || !strings.Contains(note, "wiki_write") {
		t.Errorf("with no page: %v, want a note that names me.md and wiki_write", missing)
	}

	call(t, session, "wiki_write", map[string]any{
		"path": "me.md", "title": "About Gautam", "summary": "Who Gautam is.", "body": "## Who\n\nGautam builds LifeOS. (stated)",
	})
	got := structured(t, call(t, session, "context", map[string]any{}))
	if me, _ := got["me"].(string); !strings.Contains(me, "Gautam builds LifeOS.") || got["note"] != nil {
		t.Errorf("with the page: %v, want its text and no note", got)
	}

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "context" {
			if !strings.Contains(tool.Description, "first in each conversation") {
				t.Errorf("the description of context: %s", tool.Description)
			}
			return
		}
	}
	t.Error("no tool context")
}

func TestInstructionsRouteToContextFirst(t *testing.T) {
	session, _ := connect(t)

	instructions := session.InitializeResult().Instructions
	if !strings.Contains(instructions, "Call context first in each conversation") {
		t.Error("the routing block does not tell the model to call context first")
	}
	if strings.Index(instructions, "Call context first") > strings.Index(instructions, "Where each thing goes:") {
		t.Error("the line about context is not in the routing block")
	}
}

// A session starts with the text of me.md as it is now, with no restart.
func TestInstructionsOfANewSessionHoldMe(t *testing.T) {
	svc := wikiService(t)
	before := connectTo(t, svc).InitializeResult().Instructions
	if before != lifeosmcp.Instructions() {
		t.Error("with no me.md, the instructions are not the fixed text")
	}

	call(t, connectTo(t, svc), "wiki_write", map[string]any{
		"path": "me.md", "title": "About Gautam", "summary": "Who Gautam is.",
		"body": "Gautam builds LifeOS.\n- Old goal (until 2020-01-01)",
	})
	after := connectTo(t, svc).InitializeResult().Instructions
	if !strings.HasPrefix(after, lifeosmcp.Instructions()) || !strings.Contains(after, "Gautam builds LifeOS.") {
		t.Errorf("the instructions of a new session do not hold me.md:\n%s", after)
	}
	if strings.Contains(after, "Old goal") {
		t.Error("the instructions hold a line that is past its day")
	}
	// The first session keeps what it got at its start.
	if strings.Contains(before, "Gautam builds LifeOS.") {
		t.Error("an old session changed")
	}
}
