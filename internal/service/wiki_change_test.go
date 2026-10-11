package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const logPage = `---
type: log
title: Log
summary: Append-only timeline of ingests, saved answers and lint runs.
created: 2026-10-10
updated: 2026-10-10
---

# Log

## [2026-10-10] init | Wiki skeleton created
- pages: index.md, log.md
`

func appendTo(t *testing.T, s *Service, path, text string) WikiPage {
	t.Helper()
	page, err := s.WikiAppend(context.Background(), WikiAppendParams{Path: path, Text: text, Actor: SourceAgent})
	if err != nil {
		t.Fatalf("WikiAppend(%s): %v", path, err)
	}
	return page
}

func TestWikiAppendAddsTheTextAtTheEndOfThePage(t *testing.T) {
	s, pool := newWikiService(t)
	first := writePage(t, s, agentDesign())

	s.now = func() time.Time { return testNow.Add(48 * time.Hour) }
	page := appendTo(t, s, "topics/agent-design.md", "- 2026-10-13: A result names the next step.\n")

	want := agentDesign().Body + "\n- 2026-10-13: A result names the next step."
	if page.Body != want {
		t.Errorf("body = %q\nwant   %q", page.Body, want)
	}
	if page.Created != "2026-10-11" || page.Updated != "2026-10-13" || page.Title != first.Title {
		t.Errorf("created, updated, title = %q, %q, %q", page.Created, page.Updated, page.Title)
	}
	if stored, _ := s.WikiRead(context.Background(), "topics/agent-design.md"); stored != page {
		t.Errorf("stored page = %+v\nreturned    = %+v", stored, page)
	}

	if got := commits(t, s); len(got) != 2 || got[1] != "wiki: append topics/agent-design.md" {
		t.Errorf("commits = %q, want a second commit that appends", got)
	}
	// The line of the page in the index did not change, so the commit
	// holds the page only.
	if files := gitOut(t, s, "show", "--name-only", "--format=", "HEAD"); files != "wiki/topics/agent-design.md" {
		t.Errorf("files of the commit = %q, want the page only", files)
	}
	events := wikiEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("%d wiki events, want 2", len(events))
	}
	if e := events[1]; e.Action != "append" || e.Old["hash"] != first.Hash || e.New["hash"] != page.Hash {
		t.Errorf("event = %+v, want an append from the first hash to the second", e)
	}
}

func TestWikiAppendPutsAnEmptyLineBeforeAHeading(t *testing.T) {
	s, _ := newWikiService(t)
	putPage(t, s, "log.md", logPage)

	entry := "## [2026-10-11] ingest | An article about agents\n- pages: topics/agent-design.md"
	page := appendTo(t, s, "log.md", entry)
	if !strings.HasSuffix(page.Body, "- pages: index.md, log.md\n\n"+entry) {
		t.Errorf("the entry is not after one empty line:\n%s", page.Body)
	}

	page = appendTo(t, s, "log.md", "- rows: (none)")
	if !strings.HasSuffix(page.Body, entry+"\n- rows: (none)") {
		t.Errorf("the line is not directly after the entry:\n%s", page.Body)
	}
	if page.Type != "log" || page.Created != "2026-10-10" || page.Updated != "2026-10-11" {
		t.Errorf("type, created, updated = %q, %q, %q", page.Type, page.Created, page.Updated)
	}
}

func TestWikiAppendMakesTheLogWhenThereIsNone(t *testing.T) {
	s, _ := newWikiService(t)

	page := appendTo(t, s, "log.md", "## [2026-10-11] init | First entry")

	if page.Type != "log" || page.Title != "Log" || page.Body != "# Log\n\n## [2026-10-11] init | First entry" {
		t.Errorf("type, title, body = %q, %q, %q", page.Type, page.Title, page.Body)
	}
	if got := commits(t, s); len(got) != 1 || got[0] != "wiki: append log.md" {
		t.Errorf("commits = %q", got)
	}
}

func TestWikiAppendRefusesBadInput(t *testing.T) {
	s, pool := newWikiService(t)
	writePage(t, s, agentDesign())
	before := len(wikiEvents(t, pool))

	cases := []struct {
		name  string
		p     WikiAppendParams
		field string
	}{
		{"the index", WikiAppendParams{Path: "index.md", Text: "- a line", Actor: SourceAgent}, "path"},
		{"a path outside the pages", WikiAppendParams{Path: "../notes.md", Text: "- a line", Actor: SourceAgent}, "path"},
		{"no text", WikiAppendParams{Path: "topics/agent-design.md", Text: " \n", Actor: SourceAgent}, "text"},
		{"a secret", WikiAppendParams{Path: "topics/agent-design.md", Text: "- key " + awsKey, Actor: SourceAgent}, "text"},
		{"a citation of no record", WikiAppendParams{Path: "topics/agent-design.md", Text: "- seen (movie:999)", Actor: SourceAgent}, "text"},
		{"a text that makes the page too large", WikiAppendParams{Path: "topics/agent-design.md", Text: strings.Repeat("a", maxWikiBodyBytes), Actor: SourceAgent}, "text"},
		{"an unknown actor", WikiAppendParams{Path: "topics/agent-design.md", Text: "- a line", Actor: "robot"}, "source"},
	}
	for _, c := range cases {
		_, err := s.WikiAppend(context.Background(), c.p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != c.field {
			t.Errorf("%s: error = %v, want a ValidationError for %s", c.name, err, c.field)
		}
	}

	_, err := s.WikiAppend(context.Background(), WikiAppendParams{Path: "topics/absent.md", Text: "- a line", Actor: SourceAgent})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("a page that is absent: error = %v, want ErrNotFound", err)
	}

	putPage(t, s, "topics/broken.md", "Only text.\n")
	_, err = s.WikiAppend(context.Background(), WikiAppendParams{Path: "topics/broken.md", Text: "- a line", Actor: SourceAgent})
	wantInvalid(t, err, "path")

	if n := len(wikiEvents(t, pool)); n != before {
		t.Errorf("%d wiki events, want %d", n, before)
	}
}

func TestWikiDeleteRemovesThePage(t *testing.T) {
	s, pool := newWikiService(t)
	page := writePage(t, s, agentDesign())
	asha := writePage(t, s, WikiWriteParams{Path: "people/asha-rao.md", Title: "Asha Rao", Summary: "Knows Cloudflare.", Body: "Asha.", Actor: SourceAgent})

	err := s.WikiDelete(context.Background(), "topics/agent-design.md", DeleteParams{Actor: SourceAgent, Confirmed: true})
	if err != nil {
		t.Fatalf("WikiDelete: %v", err)
	}

	if _, err := s.WikiRead(context.Background(), "topics/agent-design.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("read of the deleted page: error = %v, want ErrNotFound", err)
	}
	if stored, _ := s.WikiRead(context.Background(), "people/asha-rao.md"); stored != asha {
		t.Errorf("the other page changed: %+v", stored)
	}
	index, _ := s.WikiRead(context.Background(), "index.md")
	if strings.Contains(index.Body, "agent-design") || !strings.Contains(index.Body, "asha-rao") {
		t.Errorf("index body:\n%s", index.Body)
	}
	if got := commits(t, s); len(got) != 3 || got[2] != "wiki: delete topics/agent-design.md" {
		t.Errorf("commits = %q, want a third commit that deletes", got)
	}
	if status := gitOut(t, s, "status", "--porcelain"); status != "" {
		t.Errorf("git status = %q, want a clean tree", status)
	}
	events := wikiEvents(t, pool)
	e := events[len(events)-1]
	if e.Action != "delete" || e.Old["hash"] != page.Hash || e.Old["path"] != "topics/agent-design.md" || e.New != nil {
		t.Errorf("event = %+v, want a delete with the old hash and no new value", e)
	}
}

func TestWikiDeleteRefuses(t *testing.T) {
	s, pool := newWikiService(t)
	page := writePage(t, s, agentDesign())
	putPage(t, s, "log.md", logPage)
	yes := DeleteParams{Actor: SourceAgent, Confirmed: true}

	err := s.WikiDelete(context.Background(), "topics/agent-design.md", DeleteParams{Actor: SourceAgent})
	wantInvalid(t, err, "confirmed")
	for _, path := range []string{"index.md", "log.md", "../notes.md", "topics"} {
		err := s.WikiDelete(context.Background(), path, yes)
		wantInvalid(t, err, "path")
	}
	if err := s.WikiDelete(context.Background(), "topics/absent.md", yes); !errors.Is(err, ErrNotFound) {
		t.Errorf("a page that is absent: error = %v, want ErrNotFound", err)
	}

	if stored, _ := s.WikiRead(context.Background(), "topics/agent-design.md"); stored != page {
		t.Errorf("the page changed: %+v", stored)
	}
	if n := len(wikiEvents(t, pool)); n != 1 {
		t.Errorf("%d wiki events, want 1", n)
	}
}
