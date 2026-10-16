package mcp_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/mcp"
)

// A plain message about the user's life must reach the right tool with no
// keyword (#31). Each client sends the tool descriptions to the model, so
// each description says when to act. "only" in the description of journal
// taught the model to wait for the word "journal".
func TestDailyToolsSayWhenToActWithNoKeyword(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	descriptions := map[string]string{}
	for _, tool := range list.Tools {
		descriptions[tool.Name] = tool.Description
	}
	for _, name := range []string{"journal", "todo", "add_item", "log_habit"} {
		d := descriptions[name]
		for _, part := range []string{"own life", "no keyword"} {
			if !strings.Contains(d, part) {
				t.Errorf("the description of %s does not have %q:\n%s", name, part, d)
			}
		}
	}
	if regexp.MustCompile(`\bonly\b`).MatchString(descriptions["journal"]) {
		t.Errorf("the description of journal has the word only:\n%s", descriptions["journal"])
	}
	for _, part := range []string{"search", "rating"} {
		if !strings.Contains(descriptions["add_item"], part) {
			t.Errorf("the description of add_item does not have %q", part)
		}
	}
}

// The routing block sends a plain message about the user's life to LifeOS,
// and keeps a message about other work, such as code, out of it.
func TestInstructionsRouteMessagesAboutTheUsersLife(t *testing.T) {
	session, _ := connect(t)

	instructions := session.InitializeResult().Instructions
	for _, part := range []string{
		"Routing:",
		"own life",
		"no keyword",
		"such as code",
		"one call for each kind",
		"one short question",
	} {
		if !strings.Contains(instructions, part) {
			t.Errorf("the instructions do not have %q", part)
		}
	}
	if strings.Index(instructions, "Routing:") > strings.Index(instructions, "Rules:") {
		t.Error("the routing block is not before the rules")
	}
}

// lifeos instructions prints this text, for a client that does not give
// the server instructions to the model. It must be the text that the
// server sends.
func TestInstructionsIsTheTextThatTheServerSends(t *testing.T) {
	session, _ := connect(t)

	if got := session.InitializeResult().Instructions; got != mcp.Instructions() {
		t.Errorf("the server sends instructions that are not mcp.Instructions()")
	}
}

// #40: no approval step, so no tool lists a queue or approves a record.
func TestNoToolForTheReviewQueue(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "review_queue" || tool.Name == "verify" {
			t.Errorf("the server has the tool %s", tool.Name)
		}
	}
	instructions := session.InitializeResult().Instructions
	for _, word := range []string{"review_queue", "verify", "Review:"} {
		if strings.Contains(instructions, word) {
			t.Errorf("the instructions name %q", word)
		}
	}
}

// #40: no tool input asks the agent whether the user dictated the row,
// and no text tells the agent to send dictated.
func TestNoToolTakesDictated(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range list.Tools {
		schema, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(string(schema), `"dictated"`) {
			t.Errorf("the input of %s has dictated", tool.Name)
		}
		if strings.Contains(tool.Description, "dictated") || strings.Contains(tool.Description, "verified") {
			t.Errorf("the description of %s names dictated or verified", tool.Name)
		}
	}
	if strings.Contains(session.InitializeResult().Instructions, "dictated") {
		t.Error("the instructions name dictated")
	}
}

// #39: no record comes from a capture, so no tool takes source_ref, and
// the next step of capture does not ask for records.
func TestNoToolTakesSourceRef(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range list.Tools {
		schema, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(string(schema), "source_ref") || strings.Contains(tool.Description, "source_ref") {
			t.Errorf("%s names source_ref", tool.Name)
		}
	}
	if strings.Contains(session.InitializeResult().Instructions, "source_ref") {
		t.Error("the instructions name source_ref")
	}
	captured := structured(t, call(t, session, "capture", map[string]any{"text": "A note to keep."}))
	if next, _ := captured["next"].(string); strings.Contains(next, "source_ref") || strings.Contains(next, "record") {
		t.Errorf("the next step of capture asks for records: %s", next)
	}
}
