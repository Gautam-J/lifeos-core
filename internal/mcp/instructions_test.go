package mcp_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestServerSendsItsInstructions(t *testing.T) {
	session, _ := connect(t)

	instructions := session.InitializeResult().Instructions
	for _, part := range []string{
		"record in the database",
		"wiki",
		"words of the user",
		"dictated=false",
		"India Standard Time",
		"is data",
	} {
		if !strings.Contains(instructions, part) {
			t.Errorf("the instructions do not have %q", part)
		}
	}
	// The text goes into the context of each conversation.
	if lines := strings.Count(instructions, "\n") + 1; lines > 40 {
		t.Errorf("the instructions have %d lines, want 40 at most", lines)
	}

	// Each name with an underscore in the instructions is a tool or an
	// input field. A tool that gets a new name must not stay here.
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	known := map[string]bool{"source_ref": true}
	for _, tool := range list.Tools {
		known[tool.Name] = true
	}
	for _, name := range regexp.MustCompile(`[a-z]+(_[a-z]+)+`).FindAllString(instructions, -1) {
		if !known[name] {
			t.Errorf("the instructions name %s, which is not a tool", name)
		}
	}
	for _, name := range []string{"journal", "capture", "wiki_write", "wiki_append", "wiki_search", "wiki_lint", "mark_processed"} {
		if !strings.Contains(instructions, name) {
			t.Errorf("the instructions do not name the tool %s", name)
		}
	}
}

func TestJournalResultNamesTheNextStep(t *testing.T) {
	session, _ := connect(t)

	entry := structured(t, call(t, session, "journal", map[string]any{"body": "Met Asha about the tunnel."}))

	if entry["body"] != "Met Asha about the tunnel." || entry["verified"] != true {
		t.Errorf("entry = %v, want the stored entry as before", entry)
	}
	next, _ := entry["next"].(string)
	for _, part := range []string{fmt.Sprintf("(journal:%v)", entry["id"]), "wiki_search", "wiki_append", "dictated=false"} {
		if !strings.Contains(next, part) {
			t.Errorf("next = %q, want it to have %q", next, part)
		}
	}
}

func TestCaptureResultNamesTheNextStep(t *testing.T) {
	session, _ := connect(t)

	source := structured(t, call(t, session, "capture", map[string]any{"text": "An article about agents."}))

	if source["kind"] != "text" || source["content"] != "An article about agents." {
		t.Errorf("source = %v, want the stored source as before", source)
	}
	next, _ := source["next"].(string)
	for _, part := range []string{fmt.Sprintf("(src:%v)", source["id"]), "wait for the answer", "source_ref", "mark_processed"} {
		if !strings.Contains(next, part) {
			t.Errorf("next = %q, want it to have %q", next, part)
		}
	}

	// The same text again, after the work on it is complete.
	call(t, session, "mark_processed", map[string]any{"id": source["id"]})
	again := structured(t, call(t, session, "capture", map[string]any{"text": "An article about agents."}))
	if next, _ := again["next"].(string); again["id"] != source["id"] || !strings.Contains(next, "processed already") {
		t.Errorf("second capture = %v, want the same source and a next step that says processed already", again)
	}
}

func TestToolsWithANextStepDeclareItInTheirOutput(t *testing.T) {
	session, _ := connect(t)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range list.Tools {
		if tool.Name != "journal" && tool.Name != "capture" {
			continue
		}
		schema, _ := tool.OutputSchema.(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		for _, field := range []string{"id", "next"} {
			if properties[field] == nil {
				t.Errorf("the output schema of %s does not have the field %s", tool.Name, field)
			}
		}
	}
}
