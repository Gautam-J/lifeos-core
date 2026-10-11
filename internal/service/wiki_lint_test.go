package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// lintWiki returns each finding as "path check", in the order of the result.
func lintWiki(t *testing.T, s *Service) []string {
	t.Helper()
	findings, err := s.WikiLint(context.Background())
	if err != nil {
		t.Fatalf("WikiLint: %v", err)
	}
	got := []string{}
	for _, f := range findings {
		if f.Detail == "" {
			t.Errorf("the finding %s %s has no detail", f.Path, f.Check)
		}
		got = append(got, f.Path+" "+f.Check)
	}
	return got
}

// linked writes a page whose body links to each of links. The links are
// paths below the wiki folder.
func linked(t *testing.T, s *Service, pagePath, title string, links ...string) WikiPage {
	t.Helper()
	body := "About " + title + "."
	for _, link := range links {
		body += " See [a page](../" + link + ")."
	}
	return writePage(t, s, WikiWriteParams{Path: pagePath, Title: title, Summary: "S.", Body: body, Actor: SourceAgent})
}

func TestWikiLintOfACleanWikiFindsNothing(t *testing.T) {
	s, pool := newWikiService(t)
	linked(t, s, "topics/agent-design.md", "Agent design", "projects/lifeos.md", "index.md", "log.md")
	movie := addItem(t, s, "movie", "Dune")
	writePage(t, s, WikiWriteParams{
		Path: "projects/lifeos.md", Title: "LifeOS", Summary: "S.", Actor: SourceAgent,
		Body: fmt.Sprintf("See [the design](../topics/agent-design.md#rules), [a site](https://example.com/a.md), [mail](mailto:a@example.com) and [this part](#part). A film (movie:%d).", movie.ID),
	})
	events := len(wikiEvents(t, pool))

	if got := lintWiki(t, s); len(got) != 0 {
		t.Errorf("findings = %q, want none", got)
	}
	if n := len(commits(t, s)); n != 2 {
		t.Errorf("%d commits, want 2: lint writes nothing", n)
	}
	if n := len(wikiEvents(t, pool)); n != events {
		t.Errorf("%d wiki events, want %d: lint writes nothing", n, events)
	}
}

func TestWikiLintReportsEachKindOfFinding(t *testing.T) {
	s, _ := newWikiService(t)
	now := s.now

	// Three old pages. Only the topic is stale: a review and a project
	// that is done do not change.
	s.now = func() time.Time { return testNow.AddDate(0, 0, -91) }
	linked(t, s, "topics/old-topic.md", "Old topic", "reviews/2026-06.md", "projects/done.md")
	linked(t, s, "reviews/2026-06.md", "June", "topics/old-topic.md")
	writePage(t, s, WikiWriteParams{Path: "projects/done.md", Title: "Done", Summary: "S.", Body: "See [old](../topics/old-topic.md).", Status: "done", Actor: SourceAgent})
	s.now = now

	// A page that cites a record that is deleted later.
	movie := addItem(t, s, "movie", "Tenet")
	writePage(t, s, WikiWriteParams{
		Path: "topics/films.md", Title: "Films", Summary: "S.", Actor: SourceAgent,
		Body: fmt.Sprintf("Tenet (movie:%d). See [old](../topics/old-topic.md).", movie.ID),
	})
	if err := s.DeleteItem(context.Background(), movie.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	// A page with a link to a page that is not there.
	linked(t, s, "topics/lonely.md", "Lonely", "people/nobody.md", "topics/films.md")

	// A page of 151 lines.
	writePage(t, s, WikiWriteParams{
		Path: "topics/long.md", Title: "Long", Summary: "S.", Actor: SourceAgent,
		Body: "See [lonely](lonely.md)." + strings.Repeat("\nA line.", 150),
	})
	linked(t, s, "people/asha-rao.md", "Asha Rao", "topics/long.md", "projects/kindle.md")

	// A page that holds two statements that contradict each other.
	writePage(t, s, WikiWriteParams{
		Path: "projects/kindle.md", Title: "Kindle", Summary: "S.", Contested: ptr(true), Actor: SourceAgent,
		Body: "- 2026-09-01: The screen is 6 inches.\n- 2026-10-01: The screen is 7 inches. See [long](../topics/long.md).",
	})

	// Two files that a person put there by hand.
	putPage(t, s, "topics/broken.md", "Only text.\n")
	putPage(t, s, "topics/Bad Name.md", lifeosPage)
	putPage(t, s, "topics/notes.txt", "Not a Markdown file: lint does not look at it.")

	want := []string{
		"people/asha-rao.md orphan",
		"projects/kindle.md contested",
		"topics/Bad Name.md not_a_page",
		"topics/broken.md not_a_page",
		"topics/films.md dead_citation",
		"topics/lonely.md broken_link",
		"topics/long.md long",
		"topics/old-topic.md stale",
	}
	got := lintWiki(t, s)
	if !equal(got, want) {
		t.Errorf("findings =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	findings, _ := s.WikiLint(context.Background())
	details := map[string]string{}
	for _, f := range findings {
		details[f.Check] = f.Detail
	}
	for check, part := range map[string]string{
		"dead_citation": fmt.Sprintf("(movie:%d)", movie.ID),
		"broken_link":   "people/nobody.md",
		"long":          "151",
		"stale":         "2026-07-12",
	} {
		if !strings.Contains(details[check], part) {
			t.Errorf("the detail of %s is %q, want it to name %s", check, details[check], part)
		}
	}
}

func TestWikiLintOfAnEmptyWiki(t *testing.T) {
	s, _ := newWikiService(t)

	findings, err := s.WikiLint(context.Background())
	if err != nil || findings == nil || len(findings) != 0 {
		t.Errorf("findings = %v, error %v, want an empty list and no error", findings, err)
	}
}

func TestWikiLintReportsEachDeadCitationAndEachBrokenLinkOfAPage(t *testing.T) {
	s, _ := newWikiService(t)
	a, b := addItem(t, s, "book", "A"), addItem(t, s, "book", "B")
	linked(t, s, "topics/one.md", "One", "topics/two.md")
	writePage(t, s, WikiWriteParams{
		Path: "topics/two.md", Title: "Two", Summary: "S.", Actor: SourceAgent,
		Body: fmt.Sprintf("A (book:%d) and B (book:%d), and A again (book:%d). See [one](one.md), [x](x.md) and [y](../people/y.md).", a.ID, b.ID, a.ID),
	})
	for _, id := range []int64{a.ID, b.ID} {
		if err := s.DeleteItem(context.Background(), id, DeleteParams{Actor: SourceManual}); err != nil {
			t.Fatalf("DeleteItem: %v", err)
		}
	}

	got := lintWiki(t, s)
	want := []string{"topics/two.md broken_link", "topics/two.md broken_link", "topics/two.md dead_citation", "topics/two.md dead_citation"}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %q, want %q", got, want)
	}
}
