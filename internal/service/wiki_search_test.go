package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

// searchWiki returns the paths of the hits, in the order of the result.
func searchWiki(t *testing.T, s *Service, query string) []string {
	t.Helper()
	hits, err := s.WikiSearch(context.Background(), query, 0)
	if err != nil {
		t.Fatalf("WikiSearch(%q): %v", query, err)
	}
	paths := []string{}
	for _, hit := range hits {
		paths = append(paths, hit.Path)
	}
	return paths
}

// threePages writes a topic, a project and a person.
func threePages(t *testing.T, s *Service) {
	t.Helper()
	writePage(t, s, agentDesign())
	writePage(t, s, WikiWriteParams{
		Path: "projects/lifeos.md", Title: "LifeOS", Summary: "A second brain on a Raspberry Pi.",
		Body: "LifeOS has one write path.\n\n- 2026-10-11: The design of each Agent tool is complete.", Actor: SourceAgent,
	})
	writePage(t, s, WikiWriteParams{
		Path: "people/asha-rao.md", Title: "Asha Rao", Summary: "Knows Cloudflare.",
		Body: "Asha helped with the tunnel.", Actor: SourceAgent,
	})
}

func TestWikiSearchWithNoQueryListsEachPage(t *testing.T) {
	s, _ := newWikiService(t)
	threePages(t, s)
	putPage(t, s, "log.md", logPage)

	hits, err := s.WikiSearch(context.Background(), "  ", 0)
	if err != nil {
		t.Fatalf("WikiSearch: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("%d hits, want the 3 pages and not the index or the log", len(hits))
	}
	want := WikiHit{Path: "people/asha-rao.md", Type: "person", Title: "Asha Rao", Summary: "Knows Cloudflare.", Updated: "2026-10-11"}
	if got := hits[0]; got.Path != want.Path || got.Type != want.Type || got.Title != want.Title || got.Summary != want.Summary || got.Updated != want.Updated || got.Lines != nil {
		t.Errorf("first hit = %+v, want %+v", got, want)
	}
	if got := searchWiki(t, s, ""); !equal(got, []string{"people/asha-rao.md", "projects/lifeos.md", "topics/agent-design.md"}) {
		t.Errorf("paths = %q, want the order of the path", got)
	}
}

func TestWikiSearchFindsAWordInTheTitleTheSummaryAndTheBody(t *testing.T) {
	s, _ := newWikiService(t)
	threePages(t, s)

	for query, want := range map[string][]string{
		"asha":            {"people/asha-rao.md"},
		"CLOUDFLARE":      {"people/asha-rao.md"},
		"tunnel":          {"people/asha-rao.md"},
		"raspberry brain": {"projects/lifeos.md"},
		// The word is in the title of one page and in the body of one more.
		"agent":       {"topics/agent-design.md", "projects/lifeos.md"},
		"agent tools": {"topics/agent-design.md"},
		"write path":  {"projects/lifeos.md", "topics/agent-design.md"},
		"kubernetes":  {},
		"asha lifeos": {},
	} {
		if got := searchWiki(t, s, query); !equal(got, want) {
			t.Errorf("WikiSearch(%q) = %q, want %q", query, got, want)
		}
	}
}

func TestWikiSearchPutsTheLastChangeFirstWhenTheScoreIsEqual(t *testing.T) {
	s, _ := newWikiService(t)
	writePage(t, s, WikiWriteParams{Path: "topics/a-first.md", Title: "First", Summary: "S.", Body: "About sleep.", Actor: SourceAgent})
	s.now = func() time.Time { return testNow.Add(48 * time.Hour) }
	writePage(t, s, WikiWriteParams{Path: "topics/b-second.md", Title: "Second", Summary: "S.", Body: "About sleep.", Actor: SourceAgent})

	if got := searchWiki(t, s, "sleep"); !equal(got, []string{"topics/b-second.md", "topics/a-first.md"}) {
		t.Errorf("paths = %q, want the page of the later day first", got)
	}
}

func TestWikiSearchReturnsTheLinesThatMatch(t *testing.T) {
	s, _ := newWikiService(t)
	writePage(t, s, WikiWriteParams{
		Path: "topics/sleep.md", Title: "Rest", Summary: "Notes.", Actor: SourceAgent,
		Body: "Intro.\n\n- 2026-10-01: Sleep was short.\n- 2026-10-02: A walk.\n- 2026-10-03: Sleep was long.\n- 2026-10-04: sleep again.\n- 2026-10-05: More sleep.\n\n" + strings.Repeat("sleep ", 80),
	})

	hits, err := s.WikiSearch(context.Background(), "sleep", 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("WikiSearch: %d hits, error %v", len(hits), err)
	}
	want := []string{"- 2026-10-01: Sleep was short.", "- 2026-10-03: Sleep was long.", "- 2026-10-04: sleep again."}
	if !equal(hits[0].Lines, want) {
		t.Errorf("lines = %q, want the first three lines that match: %q", hits[0].Lines, want)
	}

	// A long line is cut.
	writePage(t, s, WikiWriteParams{Path: "topics/long.md", Title: "Long", Summary: "S.", Body: strings.Repeat("walrus ", 80), Actor: SourceAgent})
	hits, _ = s.WikiSearch(context.Background(), "walrus", 0)
	if len(hits) != 1 || len(hits[0].Lines) != 1 || len([]rune(hits[0].Lines[0])) > 201 {
		t.Errorf("hits = %+v, want one line of 200 characters at most", hits)
	}
}

func TestWikiSearchLeavesOutAFileThatIsNotAValidPage(t *testing.T) {
	s, _ := newWikiService(t)
	threePages(t, s)
	putPage(t, s, "topics/broken.md", "Only text about the tunnel.\n")
	putPage(t, s, "topics/Bad Name.md", lifeosPage)
	putPage(t, s, "topics/notes.txt", "tunnel")

	if got := searchWiki(t, s, "tunnel"); !equal(got, []string{"people/asha-rao.md"}) {
		t.Errorf("paths = %q, want only the valid page", got)
	}
}

func TestWikiSearchChecksTheLimit(t *testing.T) {
	s, _ := newWikiService(t)
	threePages(t, s)

	hits, err := s.WikiSearch(context.Background(), "", 2)
	if err != nil || len(hits) != 2 {
		t.Errorf("limit 2: %d hits, error %v", len(hits), err)
	}
	for _, limit := range []int{-1, 201} {
		_, err := s.WikiSearch(context.Background(), "", limit)
		wantInvalid(t, err, "limit")
	}
}

func TestWikiSearchOfAnEmptyWiki(t *testing.T) {
	s, _ := newWikiService(t)

	hits, err := s.WikiSearch(context.Background(), "anything", 0)
	if err != nil || len(hits) != 0 {
		t.Errorf("%d hits, error %v, want no hit and no error", len(hits), err)
	}
}
