package mcp_test

import (
	"context"
	"strings"
	"testing"
)

// entityNames returns the name of each entity in the "entities" array of a
// tool result, and fails when the result has none.
func entityNames(t *testing.T, result map[string]any, key string) []string {
	t.Helper()
	list, ok := result[key].([]any)
	if !ok {
		t.Fatalf("result has no %q list: %v", key, result)
	}
	var names []string
	for _, v := range list {
		names = append(names, v.(map[string]any)["name"].(string))
	}
	return names
}

func TestEveryWriteToolTakesEntitiesAndListsTheLinksItMade(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "gym")
	person := map[string]any{"kind": "person", "name": "Ravi"}
	place := map[string]any{"kind": "place", "name": "Goa"}

	for tool, args := range map[string]map[string]any{
		"journal":   {"body": "met Ravi in Goa", "entities": []any{person, place}},
		"todo":      {"title": "Call Ravi", "entities": []any{person}},
		"add_item":  {"kind": "place", "title": "Goa", "entities": []any{place}},
		"log_habit": {"habit": "gym", "entities": []any{person}},
		"capture":   {"text": "A note about Ravi", "entities": []any{person}},
	} {
		got := structured(t, call(t, session, tool, args))
		names := entityNames(t, got, "entities")
		if len(names) == 0 || got["entities_error"] != nil {
			t.Errorf("%s: entities = %v, error = %v", tool, names, got["entities_error"])
		}
	}

	var entities, links int
	pool.QueryRow(context.Background(), "SELECT count(*) FROM entities").Scan(&entities)
	pool.QueryRow(context.Background(), "SELECT count(*) FROM record_entities").Scan(&links)
	if entities != 2 || links != 6 {
		t.Errorf("entities = %d, links = %d, want 2 and 6", entities, links)
	}
}

func TestTheFirstLinkMakesTheEntityAndSaysSo(t *testing.T) {
	session, _ := connect(t)

	first := structured(t, call(t, session, "journal", map[string]any{"body": "met Ravi", "entities": []any{map[string]any{"kind": "person", "name": "Ravi"}}}))
	second := structured(t, call(t, session, "journal", map[string]any{"body": "met Ravi again", "entities": []any{map[string]any{"name": "ravi"}}}))

	a := first["entities"].([]any)[0].(map[string]any)
	b := second["entities"].([]any)[0].(map[string]any)
	if a["new_entity"] != true || b["new_entity"] != nil || a["ref"] != b["ref"] {
		t.Errorf("first = %v, second = %v, want one entity, new in the first result only", a, b)
	}
}

// The record stays when the entities are refused. The result says so and
// names the tool to call.
func TestARefusedEntityDoesNotLoseTheRecord(t *testing.T) {
	session, pool := connect(t)

	got := structured(t, call(t, session, "journal", map[string]any{"body": "met Ravi", "entities": []any{map[string]any{"name": "nobody"}}}))

	msg, _ := got["entities_error"].(string)
	if !strings.Contains(msg, "stored") || !strings.Contains(msg, "tag") || got["id"] == nil {
		t.Errorf("result = %v, want the entry and an entities_error that names tag", got)
	}
	var n int
	pool.QueryRow(context.Background(), "SELECT count(*) FROM journal_entries").Scan(&n)
	if n != 1 {
		t.Errorf("journal entries = %d, want 1", n)
	}
}

func TestTagLinksAndUnlinksARecord(t *testing.T) {
	session, _ := connect(t)
	added := structured(t, call(t, session, "todo", map[string]any{"title": "Call"}))

	linked := structured(t, call(t, session, "tag", map[string]any{
		"type": "todo", "id": added["id"],
		"add": []any{map[string]any{"kind": "person", "name": "Ravi"}, map[string]any{"kind": "topic", "name": "chess"}},
	}))
	if names := entityNames(t, linked, "added"); len(names) != 2 {
		t.Errorf("added = %v, want 2", names)
	}
	removed := structured(t, call(t, session, "tag", map[string]any{"type": "todo", "id": added["id"], "remove": []any{map[string]any{"name": "chess"}}}))
	if names := entityNames(t, removed, "removed"); len(names) != 1 || names[0] != "chess" {
		t.Errorf("removed = %v, want chess", names)
	}
	bad := call(t, session, "tag", map[string]any{"type": "todo", "id": added["id"]})
	if !bad.IsError || !strings.Contains(text(bad), "add") {
		t.Errorf("tag with nothing: IsError = %v, text = %q", bad.IsError, text(bad))
	}
}

func TestEntityToolAddsAnEntityAndGetReturnsItsRecords(t *testing.T) {
	session, _ := connect(t)

	ravi := structured(t, call(t, session, "entity", map[string]any{"kind": "person", "name": "Ravi Kumar", "aliases": []any{"R"}, "wiki_path": "people/ravi-kumar.md"}))
	if ravi["name"] != "Ravi Kumar" || ravi["wiki_path"] != "people/ravi-kumar.md" || ravi["source"] != "agent" {
		t.Fatalf("entity = %v", ravi)
	}
	call(t, session, "journal", map[string]any{"body": "met R", "entities": []any{map[string]any{"id": ravi["id"]}}})
	call(t, session, "todo", map[string]any{"title": "Call R", "entities": []any{map[string]any{"name": "r"}}})

	got := structured(t, call(t, session, "get", map[string]any{"type": "entity", "id": ravi["id"]}))
	records, _ := got["records"].([]any)
	if len(records) != 2 || got["wiki_path"] != "people/ravi-kumar.md" {
		t.Fatalf("get = %v, want two records and the page", got)
	}
	if first := records[0].(map[string]any); first["type"] != "todo" || !strings.HasPrefix(first["ref"].(string), "todo:") {
		t.Errorf("first record = %v, want the newest: the todo", first)
	}

	again := call(t, session, "entity", map[string]any{"kind": "person", "name": "r"})
	if !again.IsError || !strings.Contains(text(again), "update") {
		t.Errorf("a second entity with the same alias: IsError = %v, text = %q", again.IsError, text(again))
	}
	upd := structured(t, call(t, session, "update", map[string]any{"type": "entity", "id": ravi["id"], "fields": map[string]any{"add_aliases": []any{"Rav"}}}))
	if aliases, _ := upd["aliases"].([]any); len(aliases) != 2 {
		t.Errorf("update = %v, want two aliases", upd)
	}
}

func TestServerListsTheEntityTools(t *testing.T) {
	session, _ := connect(t)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, tool := range list.Tools {
		found[tool.Name] = tool.Description
	}
	for _, name := range []string{"tag", "entity"} {
		if found[name] == "" {
			t.Errorf("tool %s is not registered", name)
		}
	}
	for _, name := range []string{"journal", "todo", "add_item", "log_habit", "capture"} {
		if !strings.Contains(found[name], "entities") {
			t.Errorf("the description of %s does not tell the agent about entities", name)
		}
	}
}
