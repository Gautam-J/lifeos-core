package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func tag(t *testing.T, s *Service, recordType string, id int64, add ...EntityRef) TagResult {
	t.Helper()
	res, err := s.Tag(context.Background(), TagParams{Type: recordType, ID: id, Add: add, Actor: SourceAgent})
	if err != nil {
		t.Fatalf("Tag(%s %d): %v", recordType, id, err)
	}
	return res
}

// A name with a kind and no match makes the entity and the link in one
// write, and the result names both.
func TestTagMakesAnEntityAndALinkInOneWrite(t *testing.T) {
	s, pool := newTestService(t)
	entry := addJournal(t, s, "met Ravi at the gym", "2026-10-09")

	res := tag(t, s, "journal", entry.ID, EntityRef{Kind: "person", Name: "Ravi"}, EntityRef{Kind: "place", Name: "gym"})

	if len(res.Added) != 2 || !res.Added[0].NewEntity || res.Added[0].Name != "Ravi" || res.Added[0].Kind != "person" || res.Added[0].Ref == "" || res.Added[1].Name != "gym" {
		t.Fatalf("added = %+v, want two new entities", res.Added)
	}
	var created, links int
	for _, e := range allEvents(t, pool) {
		switch {
		case e.Action == "create" && e.RecordType == "entities":
			created++
		case e.Action == "create" && e.RecordType == "record_entities":
			links++
			if e.New["record_type"] != "journal" || e.New["record_id"] != float64(entry.ID) {
				t.Errorf("link event = %+v", e.New)
			}
		}
	}
	if created != 2 || links != 2 {
		t.Errorf("events: %d entity creates, %d link creates, want 2 and 2", created, links)
	}
	got, err := s.EntitiesOf(context.Background(), "journal", entry.ID)
	if err != nil || len(got) != 2 {
		t.Errorf("EntitiesOf = %+v, %v, want 2", got, err)
	}
}

func TestTheSameLinkASecondTimeIsNotAWrite(t *testing.T) {
	s, pool := newTestService(t)
	entry := addJournal(t, s, "met Ravi", "2026-10-09")
	tag(t, s, "journal", entry.ID, EntityRef{Kind: "person", Name: "Ravi"})
	before := countRows(t, pool, "events")

	res := tag(t, s, "journal", entry.ID, EntityRef{Kind: "person", Name: "ravi"}, EntityRef{Name: "Ravi"})

	if len(res.Added) != 1 || !res.Added[0].AlreadyLinked || res.Added[0].NewEntity {
		t.Errorf("added = %+v, want one link that existed", res.Added)
	}
	if n := countRows(t, pool, "events"); n != before {
		t.Errorf("events = %d, want %d: the link existed", n, before)
	}
	if n := countRows(t, pool, "record_entities"); n != 1 {
		t.Errorf("links = %d, want 1", n)
	}
}

func TestTagLinksEveryRecordType(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi")
	capture, err := s.Capture(ctx, CaptureParams{Text: "a note about Ravi", Source: SourceManual})
	if err != nil {
		t.Fatal(err)
	}
	addHabit(t, s, "gym")
	records := map[string]int64{
		"journal":   addJournal(t, s, "a", "2026-10-09").ID,
		"todo":      addTodo(t, s, "call").ID,
		"item":      addItem(t, s, "movie", "Dune").ID,
		"habit_log": logHabit(t, s, "gym", "2026-10-09").ID,
		"source":    capture.ID,
	}
	for recordType, id := range records {
		tag(t, s, recordType, id, EntityRef{ID: ravi.ID})
		got, err := s.EntitiesOf(ctx, recordType, id)
		if err != nil || len(got) != 1 || got[0].ID != ravi.ID {
			t.Errorf("%s: EntitiesOf = %+v, %v, want Ravi", recordType, got, err)
		}
	}
	if _, err := s.EntitiesOf(ctx, "journal", records["todo"]); err != nil {
		t.Errorf("a journal entry with the ID of a todo: %v", err)
	}
}

func TestTagRefusesBadInput(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	entry := addJournal(t, s, "a", "2026-10-09")
	ravi := addEntity(t, s, "person", "Ravi")
	addEntity(t, s, "place", "Ravi")
	gone := addEntity(t, s, "topic", "gone")
	if err := s.DeleteEntity(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, pool, "events")

	tests := []struct {
		name      string
		params    TagParams
		wantField string
	}{
		{"no entity", TagParams{Type: "journal", ID: entry.ID}, "add"},
		{"a record type that no link names", TagParams{Type: "entity", ID: ravi.ID, Add: []EntityRef{{ID: ravi.ID}}}, "type"},
		{"an unknown type", TagParams{Type: "wiki", ID: 1, Add: []EntityRef{{ID: ravi.ID}}}, "type"},
		{"an entity that is not there", TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{ID: 9999}}}, "entities"},
		{"a deleted entity", TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{ID: gone.ID}}}, "entities"},
		{"a name with no match and no kind", TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{Name: "nobody"}}}, "entities"},
		{"an unknown kind", TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{Kind: "animal", Name: "Rex"}}}, "entities"},
		{"an empty ref", TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{}}}, "entities"},
		{"a name to remove that has no match", TagParams{Type: "journal", ID: entry.ID, Remove: []EntityRef{{Kind: "person", Name: "nobody"}}}, "entities"},
		{"an unknown actor", TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{ID: ravi.ID}}, Actor: "robot"}, "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.params
			if p.Actor == "" {
				p.Actor = SourceAgent
			}
			_, err := s.Tag(ctx, p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if _, err := s.Tag(ctx, TagParams{Type: "journal", ID: 9999, Add: []EntityRef{{ID: ravi.ID}}, Actor: SourceAgent}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a record that is not there: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "events"); n != before {
		t.Errorf("events = %d, want %d: refused input appended an event", n, before)
	}
}

// A refused name must not leave an entity that an earlier ref of the same
// call made.
func TestTagIsOneTransaction(t *testing.T) {
	s, pool := newTestService(t)
	entry := addJournal(t, s, "a", "2026-10-09")

	_, err := s.Tag(context.Background(), TagParams{Type: "journal", ID: entry.ID, Add: []EntityRef{{Kind: "person", Name: "Ravi"}, {Name: "nobody"}}, Actor: SourceAgent})

	if err == nil {
		t.Fatal("Tag worked with a name that matches nothing")
	}
	if n := countRows(t, pool, "entities"); n != 0 {
		t.Errorf("entities = %d, want 0: the call is one transaction", n)
	}
}

// A name with no kind that two entities of different kinds have: the one
// that most records link to together with the other entities of the call.
func TestAnAmbiguousNameTakesTheEntityThatIsLinkedNextToTheOthers(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi")
	aloo := addEntity(t, s, "project", "Aloo")
	_ = aloo
	place := addEntity(t, s, "place", "Aloo")
	first := addJournal(t, s, "one", "2026-10-09")
	// Ravi and the place Aloo were together before.
	tag(t, s, "journal", first.ID, EntityRef{ID: ravi.ID}, EntityRef{ID: place.ID})
	second := addJournal(t, s, "two", "2026-10-10")

	res := tag(t, s, "journal", second.ID, EntityRef{ID: ravi.ID}, EntityRef{Name: "aloo"})

	if len(res.Added) != 2 || res.Added[1].Kind != "place" {
		t.Errorf("added = %+v, want the place Aloo", res.Added)
	}
	// With nothing to compare, the oldest wins.
	third := addJournal(t, s, "three", "2026-10-11")
	res, err := s.Tag(ctx, TagParams{Type: "journal", ID: third.ID, Add: []EntityRef{{Name: "ALOO"}}, Actor: SourceAgent})
	if err != nil || res.Added[0].Kind != "project" {
		t.Errorf("added = %+v, %v, want the oldest entity: the project", res.Added, err)
	}
}

func TestTagRemovesALink(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	entry := addJournal(t, s, "met Ravi", "2026-10-09")
	tag(t, s, "journal", entry.ID, EntityRef{Kind: "person", Name: "Ravi"})

	res, err := s.Tag(ctx, TagParams{Type: "journal", ID: entry.ID, Remove: []EntityRef{{Name: "ravi"}}, Actor: SourceManual})

	if err != nil || len(res.Removed) != 1 || res.Removed[0].Name != "Ravi" {
		t.Fatalf("Tag = %+v, %v, want one removed link", res, err)
	}
	events := allEvents(t, pool)
	last := events[len(events)-1]
	if last.Action != "delete" || last.RecordType != "record_entities" || last.Source != "manual" || last.Old["record_type"] != "journal" {
		t.Errorf("last event = %+v, want a delete of the link by manual", last)
	}
	before := len(events)
	again, err := s.Tag(ctx, TagParams{Type: "journal", ID: entry.ID, Remove: []EntityRef{{Name: "ravi"}}, Actor: SourceManual})
	if err != nil || len(again.Removed) != 0 || countRows(t, pool, "events") != before {
		t.Errorf("a second removal = %+v, %v, want no write", again, err)
	}
}

func TestGetOfAnEntityReturnsItsRecordsAcrossTheTables(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi")
	entry := addJournal(t, s, "ran with Ravi", "2026-10-09")
	todo := addTodo(t, s, "call Ravi")
	item := addItem(t, s, "movie", "Dune")
	deleted := addJournal(t, s, "gone", "2026-10-08")
	for recordType, id := range map[string]int64{"journal": entry.ID, "todo": todo.ID, "item": item.ID} {
		tag(t, s, recordType, id, EntityRef{ID: ravi.ID})
	}
	tag(t, s, "journal", deleted.ID, EntityRef{ID: ravi.ID})
	if err := s.DeleteJournalEntry(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	detail, err := s.GetEntityDetail(ctx, ravi.ID)

	if err != nil || len(detail.Records) != 3 {
		t.Fatalf("detail = %+v, %v, want 3 records: the deleted entry is left out", detail, err)
	}
	// Newest link first.
	if r := detail.Records[0]; r.Type != "item" || r.Ref != "movie:"+strconv.FormatInt(item.ID, 10) || !strings.Contains(r.Summary, "Dune") {
		t.Errorf("first record = %+v, want the item, with its kind as the ref prefix", r)
	}
	if r := detail.Records[2]; r.Type != "journal" || r.Ref != "journal:"+strconv.FormatInt(entry.ID, 10) {
		t.Errorf("last record = %+v, want the journal entry", r)
	}
}

func TestPurgeOfARecordRemovesItsLinks(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi")
	todo := addTodo(t, s, "call")
	tag(t, s, "todo", todo.ID, EntityRef{ID: ravi.ID})
	if err := s.DeleteTodo(ctx, todo.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	if err := s.Purge(ctx, "todo", todo.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "record_entities"); n != 0 {
		t.Errorf("links = %d, want 0", n)
	}
	events := allEvents(t, pool)
	var deletes int
	for _, e := range events {
		if e.Action == "delete" && e.RecordType == "record_entities" {
			deletes++
		}
	}
	if deletes != 1 {
		t.Errorf("link delete events = %d, want 1", deletes)
	}
}

func TestPurgeOfAnEntityIsRefusedWhileARecordLinksToIt(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi")
	entry := addJournal(t, s, "met Ravi", "2026-10-09")
	tag(t, s, "journal", entry.ID, EntityRef{ID: ravi.ID})
	if err := s.DeleteEntity(ctx, ravi.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	err := s.Purge(ctx, "entity", ravi.ID)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "record_entities") {
		t.Fatalf("Purge = %v, want a ValidationError that names record_entities", err)
	}
	if _, err := s.Tag(ctx, TagParams{Type: "journal", ID: entry.ID, Remove: []EntityRef{{ID: ravi.ID}}, Actor: SourceManual}); err == nil {
		t.Log("a deleted entity cannot be named; the link goes with the record")
	}
	if err := s.DeleteJournalEntry(ctx, entry.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, "journal", entry.ID); err != nil {
		t.Fatalf("Purge of the entry: %v", err)
	}
	if err := s.Purge(ctx, "entity", ravi.ID); err != nil {
		t.Errorf("Purge of the entity after its records went: %v", err)
	}
}

func TestPurgeOfAProjectIsRefusedWhileATodoNamesIt(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	todo := addTodo(t, s, "a") // project lifeos
	var project int64
	if err := pool.QueryRow(ctx, "SELECT id FROM entities WHERE name = 'lifeos'").Scan(&project); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntity(ctx, project, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	err := s.Purge(ctx, "entity", project)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "todos") {
		t.Errorf("Purge = %v, want a ValidationError that names todos", err)
	}
	got, err := s.GetTodo(ctx, todo.ID)
	if err != nil || got.Project != "lifeos" {
		t.Errorf("todo = %+v, %v, want it to keep the name of its deleted project", got, err)
	}
}
