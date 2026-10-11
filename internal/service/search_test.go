package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// hitRefs returns the type and the ID of each hit, in order.
func hitRefs(hits []SearchHit) []RecordRef {
	refs := []RecordRef{}
	for _, hit := range hits {
		refs = append(refs, RecordRef{hit.Type, hit.ID})
	}
	return refs
}

func search(t *testing.T, s *Service, query string, types ...string) []SearchHit {
	t.Helper()
	hits, err := s.Search(context.Background(), SearchParams{Query: query, Types: types})
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return hits
}

func wantHits(t *testing.T, name string, got []SearchHit, want ...RecordRef) {
	t.Helper()
	refs := hitRefs(got)
	if len(refs) != len(want) {
		t.Errorf("%s: hits = %v, want %v", name, refs, want)
		return
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("%s: hits = %v, want %v", name, refs, want)
			return
		}
	}
}

func TestSearchFindsARecordOfEachType(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	item, err := s.AddItem(ctx, dune()) // Dune: Part Two, director Denis Villeneuve
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	entry := addJournal(t, s, "Went to Lonavala with Asha.\nIt rained all day.", "2026-10-09")
	p := buyDomain()
	p.Title, p.Project = "Renew the car insurance", "home"
	todo, err := s.AddTodo(ctx, p)
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	text, err := s.Capture(ctx, CaptureParams{Text: "The policy renews on 12 March.", Title: "Insurance note", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	link, err := s.Capture(ctx, CaptureParams{URL: "https://example.com/lonavala-guide", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	wantHits(t, "a word of a title", search(t, s, "dune"), RecordRef{"item", item.ID})
	wantHits(t, "a word of an attr", search(t, s, "villeneuve"), RecordRef{"item", item.ID})
	wantHits(t, "the kind and a word", search(t, s, "movie dune"), RecordRef{"item", item.ID})
	wantHits(t, "a word of a body", search(t, s, "rained"), RecordRef{"journal", entry.ID})
	wantHits(t, "the day of an entry", search(t, s, "2026-10-09"), RecordRef{"journal", entry.ID})
	wantHits(t, "a project", search(t, s, "home"), RecordRef{"todo", todo.ID})
	wantHits(t, "a word of a text", search(t, s, "renews"), RecordRef{"source", text.ID})
	wantHits(t, "a part of a link", search(t, s, "example.com"), RecordRef{"source", link.ID})

	// The newest record is first.
	wantHits(t, "a word in two types", search(t, s, "insurance"), RecordRef{"source", text.ID}, RecordRef{"todo", todo.ID})
	wantHits(t, "a part of a word, in a different case", search(t, s, "LONAV"), RecordRef{"source", link.ID}, RecordRef{"journal", entry.ID})
	wantHits(t, "one type", search(t, s, "lonavala", "journal"), RecordRef{"journal", entry.ID})
	wantHits(t, "two types", search(t, s, "insurance", "todo", "item"), RecordRef{"todo", todo.ID})

	// Every word must be in the record. The words can be in any order.
	wantHits(t, "two words far apart", search(t, s, "rained lonavala"), RecordRef{"journal", entry.ID})
	wantHits(t, "a word that no record has", search(t, s, "lonavala snow"))

	hits := search(t, s, "dune")
	if hits[0].Summary != "movie: Dune: Part Two, rating 4.5, on 2026-10-09" || !hits[0].Verified || hits[0].CreatedAt.IsZero() {
		t.Errorf("hit = %+v, want the summary of the item", hits[0])
	}
	if got := search(t, s, "renews")[0].Summary; got != "text: Insurance note: The policy renews on 12 March." {
		t.Errorf("summary of a text = %q", got)
	}
	if got := search(t, s, "example.com")[0].Summary; got != "link: https://example.com/lonavala-guide" {
		t.Errorf("summary of a link = %q", got)
	}
}

func TestSearchLeavesOutADeletedRecordAndShowsARecordThatWaits(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	deleted := addItem(t, s, "book", "Dune")
	waits := addUnverified(t, s, "Dune Messiah")
	if err := s.DeleteItem(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	hits := search(t, s, "dune")
	wantHits(t, "dune", hits, RecordRef{"item", waits.ID})
	if hits[0].Verified {
		t.Error("the hit says that the record is verified, but it waits for review")
	}
}

// A percent sign and an underscore are plain characters in a search.
func TestSearchTakesTheQueryLiterally(t *testing.T) {
	s, _ := newTestService(t)
	percent := addTodo(t, s, "Get 50% off the domain")
	underscore := addTodo(t, s, "Rename max_file_mb")
	addTodo(t, s, "Buy a domain")

	wantHits(t, "a percent sign", search(t, s, "50%"), RecordRef{"todo", percent.ID})
	wantHits(t, "only a percent sign", search(t, s, "%"), RecordRef{"todo", percent.ID})
	wantHits(t, "an underscore", search(t, s, "max_file"), RecordRef{"todo", underscore.ID})
	wantHits(t, "a backslash", search(t, s, `\`))
	wantHits(t, "a quote", search(t, s, `'; DROP TABLE todos; --`))
	// A search stores nothing, so the rule on secrets does not apply.
	wantHits(t, "a card number", search(t, s, testCard))
}

func TestSearchReturnsTheNewestHitsUpToTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addTodo(t, s, "Call the bank")
	entry := addJournal(t, s, "the bank was closed", "")
	last := addTodo(t, s, "Call the bank again")

	hits, err := s.Search(ctx, SearchParams{Query: "bank", Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	wantHits(t, "limit 2", hits, RecordRef{"todo", last.ID}, RecordRef{"journal", entry.ID})
	wantHits(t, "no limit", search(t, s, "bank"), RecordRef{"todo", last.ID}, RecordRef{"journal", entry.ID}, RecordRef{"todo", first.ID})

	none := search(t, s, "nothing-has-this-word")
	if none == nil || len(none) != 0 {
		t.Errorf("hits = %v, want an empty list, not nil", none)
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		p     SearchParams
		field string
	}{
		{"no query", SearchParams{}, "query"},
		{"a query of spaces", SearchParams{Query: " \t\n"}, "query"},
		{"a query that is too long", SearchParams{Query: strings.Repeat("a", maxQueryLength+1)}, "query"},
		{"too many words", SearchParams{Query: strings.Repeat("word ", maxQueryWords+1)}, "query"},
		{"a NUL", SearchParams{Query: "a\x00b"}, "query"},
		{"a type that search does not cover", SearchParams{Query: "gym", Types: []string{"habit_log"}}, "types"},
		{"an unknown type", SearchParams{Query: "gym", Types: []string{"item", "song"}}, "types"},
		{"a limit over the maximum", SearchParams{Query: "gym", Limit: 201}, "limit"},
	}
	for _, c := range cases {
		_, err := s.Search(ctx, c.p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != c.field {
			t.Errorf("%s: error = %v, want a ValidationError on %s", c.name, err, c.field)
		}
	}
	if _, err := s.Search(ctx, SearchParams{Query: strings.Repeat("word ", maxQueryWords)}); err != nil {
		t.Errorf("a query with the maximum number of words: %v", err)
	}
}

// An agent that looks for a todo must see that it is done already.
func TestSearchShowsTheStatusOfATodoThatIsNotOpen(t *testing.T) {
	s, _ := newTestService(t)
	open := addTodo(t, s, "Call the bank")
	done := addTodo(t, s, "Call the bank about the card")
	setStatus(t, s, done.ID, "done")

	hits := search(t, s, "bank")
	wantHits(t, "bank", hits, RecordRef{"todo", done.ID}, RecordRef{"todo", open.ID})
	// The test helper gives each todo a project and a due day.
	if hits[0].Summary != "todo: Call the bank about the card, done, project lifeos, due 2026-10-20" || hits[1].Summary != "todo: Call the bank, project lifeos, due 2026-10-20" {
		t.Errorf("summaries = %q, %q", hits[0].Summary, hits[1].Summary)
	}
}

func TestSearchShowsAFileByItsName(t *testing.T) {
	s, _ := newFileService(t)
	file, err := s.Capture(context.Background(), CaptureParams{Upload: strings.NewReader("%PDF-1.7\n"), FileName: "policy-2026.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	hits := search(t, s, "policy")
	wantHits(t, "policy", hits, RecordRef{"source", file.ID})
	if hits[0].Summary != "file: policy-2026.pdf (application/pdf)" {
		t.Errorf("summary = %q", hits[0].Summary)
	}
}
