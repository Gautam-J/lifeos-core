package service

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

func addEntity(t *testing.T, s *Service, kind, name string, aliases ...string) Entity {
	t.Helper()
	e, err := s.AddEntity(context.Background(), AddEntityParams{Kind: kind, Name: name, Aliases: aliases, Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddEntity(%s, %q): %v", kind, name, err)
	}
	return e
}

func TestAddEntityStoresAnEntityAndAppendsAnEvent(t *testing.T) {
	s, pool := newTestService(t)

	e, err := s.AddEntity(context.Background(), AddEntityParams{
		Kind: "person", Name: "  Ravi Kumar ", Aliases: []string{"R", "ravi", " R "}, WikiPath: "people/ravi-kumar.md",
		Provenance: Provenance{Source: SourceAgent},
	})

	if err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	if e.ID == 0 || e.Kind != "person" || e.Name != "Ravi Kumar" || e.WikiPath != "people/ravi-kumar.md" || e.Source != SourceAgent {
		t.Errorf("entity = %+v", e)
	}
	if got := strings.Join(e.Aliases, ","); got != "R,ravi" {
		t.Errorf("aliases = %q, want R,ravi: trimmed, no repeat", got)
	}
	events := allEvents(t, pool)
	if len(events) != 1 || events[0].Action != "create" || events[0].RecordType != "entities" || events[0].RecordID != e.ID || events[0].New["name"] != "Ravi Kumar" {
		t.Errorf("events = %+v, want one create of the entity", events)
	}
}

func TestAddEntityRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	addEntity(t, s, "person", "Ravi", "R")
	addEntity(t, s, "place", "Goa")
	tests := []struct {
		name      string
		params    AddEntityParams
		wantField string
	}{
		{"unknown kind", AddEntityParams{Kind: "animal", Name: "Rex"}, "kind"},
		{"empty name", AddEntityParams{Kind: "person", Name: " "}, "name"},
		{"long name", AddEntityParams{Kind: "person", Name: strings.Repeat("a", 101)}, "name"},
		{"the name of a person that exists", AddEntityParams{Kind: "person", Name: "ravi"}, "name"},
		{"the alias of a person that exists", AddEntityParams{Kind: "person", Name: "r"}, "name"},
		{"an alias that is the name of a person that exists", AddEntityParams{Kind: "person", Name: "Sam", Aliases: []string{"RAVI"}}, "aliases"},
		{"an empty alias", AddEntityParams{Kind: "person", Name: "Sam", Aliases: []string{""}}, "aliases"},
		{"a secret in the name", AddEntityParams{Kind: "person", Name: "AKIA is not here; 4111 1111 1111 1111"}, "name"},
		{"a wiki path above the wiki", AddEntityParams{Kind: "person", Name: "Sam", WikiPath: "../x.md"}, "wiki_path"},
		{"a wiki path that is not a page", AddEntityParams{Kind: "person", Name: "Sam", WikiPath: "people/sam.txt"}, "wiki_path"},
		{"unknown source", AddEntityParams{Kind: "person", Name: "Sam", Provenance: Provenance{Source: "robot"}}, "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.params
			if p.Source == "" {
				p.Source = SourceAgent
			}
			_, err := s.AddEntity(context.Background(), p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "entities"); n != 2 {
		t.Errorf("entities = %d, want 2: refused input stored a row", n)
	}
}

func TestTheSameNameInTwoKindsIsAllowed(t *testing.T) {
	s, _ := newTestService(t)
	addEntity(t, s, "person", "Sam")
	if _, err := s.AddEntity(context.Background(), AddEntityParams{Kind: "place", Name: "Sam", Provenance: Provenance{Source: SourceAgent}}); err != nil {
		t.Errorf("a place named Sam: %v", err)
	}
}

func TestUpdateEntity(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi")
	addEntity(t, s, "person", "Sam", "S")

	got, err := s.UpdateEntity(ctx, ravi.ID, UpdateEntityParams{Name: ptr("Ravi Kumar"), AddAliases: []string{"R", "Ravi"}, WikiPath: ptr("people/ravi.md"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if got.Name != "Ravi Kumar" || strings.Join(got.Aliases, ",") != "R,Ravi" || got.WikiPath != "people/ravi.md" || got.Source != SourceAgent {
		t.Errorf("entity = %+v, want the new fields and the old source", got)
	}
	events := allEvents(t, pool)
	last := events[len(events)-1]
	if last.Action != "update" || last.Source != "manual" || last.Old["name"] != "Ravi" || last.New["name"] != "Ravi Kumar" {
		t.Errorf("last event = %+v, want an update by manual with the old name", last)
	}

	// No value changes: no write and no event.
	before := countRows(t, pool, "events")
	same, err := s.UpdateEntity(ctx, ravi.ID, UpdateEntityParams{Name: ptr("Ravi Kumar"), AddAliases: []string{"r"}, Actor: SourceManual})
	if err != nil || same.Name != "Ravi Kumar" {
		t.Fatalf("UpdateEntity (same) = %+v, %v", same, err)
	}
	if n := countRows(t, pool, "events"); n != before {
		t.Errorf("events = %d, want %d: no value changed", n, before)
	}

	archived, err := s.UpdateEntity(ctx, ravi.ID, UpdateEntityParams{Archived: ptr(true), Actor: SourceManual})
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive = %+v, %v", archived, err)
	}
	back, err := s.UpdateEntity(ctx, ravi.ID, UpdateEntityParams{Archived: ptr(false), Clear: []string{"wiki_path"}, Actor: SourceManual})
	if err != nil || back.ArchivedAt != nil || back.WikiPath != "" {
		t.Fatalf("unarchive and clear = %+v, %v", back, err)
	}

	for name, p := range map[string]UpdateEntityParams{
		"no field":                  {Actor: SourceManual},
		"the name of a different":   {Name: ptr("sam"), Actor: SourceManual},
		"the alias of a different":  {AddAliases: []string{"s"}, Actor: SourceManual},
		"an empty name":             {Name: ptr(" "), Actor: SourceManual},
		"an unknown clear":          {Clear: []string{"kind"}, Actor: SourceManual},
		"a wiki path and its clear": {WikiPath: ptr("a.md"), Clear: []string{"wiki_path"}, Actor: SourceManual},
	} {
		var invalid *ValidationError
		if _, err := s.UpdateEntity(ctx, ravi.ID, p); !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v, want a ValidationError", name, err)
		}
	}
	if _, err := s.UpdateEntity(ctx, 9999, UpdateEntityParams{Name: ptr("x"), Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent entity: error = %v, want ErrNotFound", err)
	}
}

func TestListEntitiesLeavesOutArchivedAndDeleted(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addEntity(t, s, "person", "Ravi")
	old := addEntity(t, s, "person", "Old")
	gone := addEntity(t, s, "place", "Gone")
	addEntity(t, s, "project", "lifeos")
	if _, err := s.UpdateEntity(ctx, old.ID, UpdateEntityParams{Archived: ptr(true), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntity(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListEntities(ctx, ListEntitiesParams{})
	if err != nil || len(all) != 2 {
		t.Fatalf("ListEntities = %+v, %v, want Ravi and lifeos", all, err)
	}
	withArchived, _ := s.ListEntities(ctx, ListEntitiesParams{IncludeArchived: true})
	if len(withArchived) != 3 {
		t.Errorf("with archived = %d, want 3", len(withArchived))
	}
	people, _ := s.ListEntities(ctx, ListEntitiesParams{Kind: "person"})
	if len(people) != 1 || people[0].Name != "Ravi" {
		t.Errorf("people = %+v, want Ravi", people)
	}
	var invalid *ValidationError
	if _, err := s.ListEntities(ctx, ListEntitiesParams{Kind: "animal"}); !errors.As(err, &invalid) {
		t.Errorf("unknown kind: error = %v, want a ValidationError", err)
	}
}

func TestDeleteEntityIsSoftAndAgentsNeedAConfirmation(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	e := addEntity(t, s, "topic", "chess")

	var invalid *ValidationError
	if err := s.DeleteEntity(ctx, e.ID, DeleteParams{Actor: SourceAgent}); !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v", err)
	}
	if err := s.DeleteEntity(ctx, e.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if _, err := s.GetEntity(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "entities"); n != 1 {
		t.Errorf("entities = %d, want 1: a delete keeps the row", n)
	}
	// The name is free again.
	if _, err := s.AddEntity(ctx, AddEntityParams{Kind: "topic", Name: "chess", Provenance: Provenance{Source: SourceAgent}}); err != nil {
		t.Errorf("the name of a deleted entity: %v", err)
	}
}

func TestEntityIsARecordType(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	e := addEntity(t, s, "person", "Ravi", "R")

	got, err := s.Get(ctx, "entity", e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail, ok := got.(EntityDetail); !ok || detail.Name != "Ravi" {
		t.Errorf("Get = %#v, want an EntityDetail", got)
	}
	upd, err := s.Update(ctx, "entity", e.ID, UpdateParams{Fields: map[string]any{"add_aliases": []any{"Rav"}}, Actor: SourceAgent})
	if ent, ok := upd.(Entity); err != nil || !ok || len(ent.Aliases) != 2 {
		t.Errorf("Update = %#v, %v, want the entity with two aliases", upd, err)
	}
	hits, err := s.Search(ctx, SearchParams{Query: "rav", Types: []string{"entity"}})
	if err != nil || len(hits) != 1 || hits[0].Type != "entity" || hits[0].ID != e.ID || !strings.Contains(hits[0].Summary, "Ravi") {
		t.Errorf("Search = %+v, %v, want the entity by alias", hits, err)
	}
	if err := s.Delete(ctx, "entity", e.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deleted, err := s.ListDeleted(ctx, 0)
	if err != nil || len(deleted) != 1 || deleted[0].Type != "entity" || !strings.Contains(deleted[0].Summary, "Ravi") {
		t.Errorf("ListDeleted = %+v, %v", deleted, err)
	}
	if err := s.Purge(ctx, "entity", e.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n := countRows(t, pool, "entities"); n != 0 {
		t.Errorf("entities = %d after Purge, want 0", n)
	}
}

func TestAWikiPageCanCiteAnEntity(t *testing.T) {
	s, pool := newTestService(t)
	e := addEntity(t, s, "person", "Ravi")
	cite := "(entity:" + strconv.FormatInt(e.ID, 10) + ")"

	live, err := citationIsLive(context.Background(), store.New(pool), cite)
	if err != nil || !live {
		t.Errorf("citationIsLive(%s) = %v, %v, want true", cite, live, err)
	}
	if !slices.Contains(WikiCitationPrefixes, "entity") {
		t.Errorf("WikiCitationPrefixes = %v, want entity in it", WikiCitationPrefixes)
	}
}
