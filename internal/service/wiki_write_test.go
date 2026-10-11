package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// agentDesign is a valid input for a new page. Tests copy it and change
// one field.
func agentDesign() WikiWriteParams {
	return WikiWriteParams{
		Path:    "topics/agent-design.md",
		Title:   "Agent design",
		Summary: "What works when an agent uses tools.",
		Body:    "An agent needs one write path.\n\n- 2026-10-11: Tools carry the rules.",
		Actor:   SourceAgent,
	}
}

func writePage(t *testing.T, s *Service, p WikiWriteParams) WikiPage {
	t.Helper()
	page, err := s.WikiWrite(context.Background(), p)
	if err != nil {
		t.Fatalf("WikiWrite(%s): %v", p.Path, err)
	}
	return page
}

// gitOut runs git in the wiki folder and returns its output.
func gitOut(t *testing.T, s *Service, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", s.wiki.Dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// commits returns the subject of each commit, oldest first.
func commits(t *testing.T, s *Service) []string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.wiki.Dir), ".git", "refs", "heads")); err != nil {
		t.Fatalf("no git repository: %v", err)
	}
	out, err := exec.Command("git", "-C", s.wiki.Dir, "log", "--reverse", "--format=%s").CombinedOutput()
	if err != nil {
		// A repository with no commit has no log.
		return nil
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

type wikiEvent struct {
	Action, Source string
	Old, New       map[string]any
}

// wikiEvents returns each event of the wiki, oldest first. It also checks
// that a wiki event names no row.
func wikiEvents(t *testing.T, pool *pgxpool.Pool) []wikiEvent {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT action, source, record_id, old_value, new_value FROM events WHERE record_type = 'wiki' ORDER BY id")
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	defer rows.Close()
	var events []wikiEvent
	for rows.Next() {
		var e wikiEvent
		var recordID *int64
		var oldJSON, newJSON []byte
		if err := rows.Scan(&e.Action, &e.Source, &recordID, &oldJSON, &newJSON); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		if recordID != nil {
			t.Errorf("a wiki event has the record ID %d, want none", *recordID)
		}
		for raw, target := range map[*[]byte]*map[string]any{&oldJSON: &e.Old, &newJSON: &e.New} {
			if *raw != nil {
				if err := json.Unmarshal(*raw, target); err != nil {
					t.Fatalf("event value: %v", err)
				}
			}
		}
		events = append(events, e)
	}
	return events
}

func TestWikiWriteCreatesAPage(t *testing.T) {
	s, pool := newWikiService(t)

	page := writePage(t, s, agentDesign())

	// testNow is 2026-10-11 in IST.
	if page.Type != "topic" || page.Created != "2026-10-11" || page.Updated != "2026-10-11" {
		t.Errorf("type, created, updated = %q, %q, %q, want topic, 2026-10-11, 2026-10-11", page.Type, page.Created, page.Updated)
	}
	stored, err := s.WikiRead(context.Background(), "topics/agent-design.md")
	if err != nil {
		t.Fatalf("WikiRead: %v", err)
	}
	if stored != page {
		t.Errorf("stored page = %+v\nreturned    = %+v", stored, page)
	}
	if stored.Title != "Agent design" || stored.Body != agentDesign().Body {
		t.Errorf("title, body = %q, %q", stored.Title, stored.Body)
	}

	if got := commits(t, s); len(got) != 1 || got[0] != "wiki: create topics/agent-design.md" {
		t.Errorf("commits = %q, want one commit that creates the page", got)
	}
	if status := gitOut(t, s, "status", "--porcelain"); status != "" {
		t.Errorf("git status = %q, want a clean tree", status)
	}

	events := wikiEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("%d wiki events, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.Source != "agent" || e.Old != nil {
		t.Errorf("event = %+v, want a create by agent with no old value", e)
	}
	if e.New["path"] != "topics/agent-design.md" || e.New["hash"] != page.Hash {
		t.Errorf("event new value = %v, want the path and the hash", e.New)
	}
}

func TestWikiWriteKeepsTheIndex(t *testing.T) {
	s, _ := newWikiService(t)
	writePage(t, s, agentDesign())
	asha := WikiWriteParams{Path: "people/asha-rao.md", Title: "Asha Rao", Summary: "Works on the tunnel with Gautam.", Body: "Asha knows Cloudflare.", Actor: SourceAgent}
	page := writePage(t, s, asha)

	index, err := s.WikiRead(context.Background(), "index.md")
	if err != nil {
		t.Fatalf("WikiRead(index.md): %v", err)
	}
	want := `# Index

Each page of the wiki has one line here. LifeOS writes this file: do not change it.

## People

- [Asha Rao](people/asha-rao.md): Works on the tunnel with Gautam.

## Projects

## Topics

- [Agent design](topics/agent-design.md): What works when an agent uses tools.

## Reviews`
	if index.Body != want {
		t.Errorf("index body =\n%s\nwant\n%s", index.Body, want)
	}
	if status := gitOut(t, s, "status", "--porcelain"); status != "" {
		t.Errorf("git status = %q, want the index in the commit", status)
	}

	// A new summary changes the line of the page.
	asha.Summary, asha.BaseHash = "Knows Cloudflare.", page.Hash
	writePage(t, s, asha)
	index, _ = s.WikiRead(context.Background(), "index.md")
	if !strings.Contains(index.Body, "- [Asha Rao](people/asha-rao.md): Knows Cloudflare.\n") {
		t.Errorf("index body does not have the new summary:\n%s", index.Body)
	}
	if status := gitOut(t, s, "status", "--porcelain"); status != "" {
		t.Errorf("git status = %q after the second write", status)
	}
}

func TestWikiWriteReplacesAPageThatTheCallerRead(t *testing.T) {
	s, pool := newWikiService(t)
	first := writePage(t, s, agentDesign())

	s.now = func() time.Time { return testNow.Add(48 * time.Hour) }
	p := agentDesign()
	p.Body, p.BaseHash, p.Contested = "A new body.", first.Hash, ptr(true)
	second := writePage(t, s, p)

	if second.Body != "A new body." || !second.Contested {
		t.Errorf("body, contested = %q, %t", second.Body, second.Contested)
	}
	if second.Created != "2026-10-11" || second.Updated != "2026-10-13" {
		t.Errorf("created, updated = %q, %q, want 2026-10-11, 2026-10-13", second.Created, second.Updated)
	}
	if second.Hash == first.Hash {
		t.Error("the hash did not change")
	}
	if got := commits(t, s); len(got) != 2 || got[1] != "wiki: update topics/agent-design.md" {
		t.Errorf("commits = %q, want a second commit that updates the page", got)
	}
	events := wikiEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("%d wiki events, want 2", len(events))
	}
	e := events[1]
	if e.Action != "update" || e.Old["hash"] != first.Hash || e.New["hash"] != second.Hash {
		t.Errorf("event = %+v, want an update from the first hash to the second", e)
	}
}

func TestWikiWriteRefusesAHashThatIsNotTheStoredOne(t *testing.T) {
	s, pool := newWikiService(t)
	first := writePage(t, s, agentDesign())
	p := agentDesign()
	p.Body, p.BaseHash = "The change of session A.", first.Hash
	second := writePage(t, s, p)

	for name, baseHash := range map[string]string{
		"the hash of an old version":     first.Hash,
		"no hash for a page that exists": "",
	} {
		stale := agentDesign()
		stale.Body, stale.BaseHash = "The change of session B.", baseHash
		_, err := s.WikiWrite(context.Background(), stale)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "base_hash" {
			t.Errorf("%s: error = %v, want a ValidationError for base_hash", name, err)
		}
	}
	stored, _ := s.WikiRead(context.Background(), "topics/agent-design.md")
	if stored != second {
		t.Errorf("the page changed: %+v", stored)
	}
	if n := len(wikiEvents(t, pool)); n != 2 {
		t.Errorf("%d wiki events, want 2", n)
	}

	// A hash for a page that does not exist is also wrong.
	fresh := agentDesign()
	fresh.Path, fresh.BaseHash = "topics/new-page.md", first.Hash
	_, err := s.WikiWrite(context.Background(), fresh)
	wantInvalid(t, err, "base_hash")
}

func TestWikiWriteOfTheSameContentIsNotAWrite(t *testing.T) {
	s, pool := newWikiService(t)
	first := writePage(t, s, agentDesign())

	s.now = func() time.Time { return testNow.Add(48 * time.Hour) }
	p := agentDesign()
	p.BaseHash = first.Hash
	second := writePage(t, s, p)

	if second != first {
		t.Errorf("page = %+v\nwant the stored page %+v", second, first)
	}
	if n := len(commits(t, s)); n != 1 {
		t.Errorf("%d commits, want 1", n)
	}
	if n := len(wikiEvents(t, pool)); n != 1 {
		t.Errorf("%d wiki events, want 1", n)
	}
}

func TestWikiWriteRefusesBadInput(t *testing.T) {
	s, pool := newWikiService(t)
	cases := []struct {
		name   string
		change func(*WikiWriteParams)
		field  string
	}{
		{"the index", func(p *WikiWriteParams) { p.Path = "index.md" }, "path"},
		{"the log", func(p *WikiWriteParams) { p.Path = "log.md" }, "path"},
		{"a path outside the pages", func(p *WikiWriteParams) { p.Path = "../notes.md" }, "path"},
		{"a folder that is no page type", func(p *WikiWriteParams) { p.Path = "notes/agent-design.md" }, "path"},
		{"no title", func(p *WikiWriteParams) { p.Title = "  " }, "title"},
		{"a title of two lines", func(p *WikiWriteParams) { p.Title = "Agent\ndesign" }, "title"},
		{"a title that is too long", func(p *WikiWriteParams) { p.Title = strings.Repeat("a", 201) }, "title"},
		{"no summary", func(p *WikiWriteParams) { p.Summary = "" }, "summary"},
		{"a summary of two lines", func(p *WikiWriteParams) { p.Summary = "One.\nTwo." }, "summary"},
		{"a summary that is too long", func(p *WikiWriteParams) { p.Summary = strings.Repeat("a", 301) }, "summary"},
		{"no body", func(p *WikiWriteParams) { p.Body = "\n\n" }, "body"},
		{"a body that is too large", func(p *WikiWriteParams) { p.Body = strings.Repeat("a", maxWikiBodyBytes+1) }, "body"},
		{"a secret in the body", func(p *WikiWriteParams) { p.Body = "The key is " + awsKey }, "body"},
		{"a secret in the summary", func(p *WikiWriteParams) { p.Summary = "Key " + awsKey }, "summary"},
		{"a secret in the title", func(p *WikiWriteParams) { p.Title = awsKey }, "title"},
		{"a status on a topic", func(p *WikiWriteParams) { p.Status = "active" }, "status"},
		{"an unknown status", func(p *WikiWriteParams) { p.Path, p.Status = "projects/lifeos.md", "started" }, "status"},
		{"an unknown actor", func(p *WikiWriteParams) { p.Actor = "robot" }, "source"},
	}
	for _, c := range cases {
		p := agentDesign()
		c.change(&p)
		_, err := s.WikiWrite(context.Background(), p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != c.field {
			t.Errorf("%s: error = %v, want a ValidationError for %s", c.name, err, c.field)
		}
		if err != nil && strings.Contains(err.Error(), awsKey) {
			t.Errorf("%s: the error repeats the secret", c.name)
		}
	}
	if n := len(commits(t, s)); n != 0 {
		t.Errorf("%d commits, want none", n)
	}
	if n := len(wikiEvents(t, pool)); n != 0 {
		t.Errorf("%d wiki events, want none", n)
	}
}

func TestWikiWriteGivesAProjectItsStatusAndAReviewItsPeriod(t *testing.T) {
	s, _ := newWikiService(t)

	project := agentDesign()
	project.Path = "projects/lifeos.md"
	if page := writePage(t, s, project); page.Type != "project" || page.Status != "active" {
		t.Errorf("type, status = %q, %q, want project, active", page.Type, page.Status)
	}
	project.Path, project.Status = "projects/kindle.md", "paused"
	if page := writePage(t, s, project); page.Status != "paused" {
		t.Errorf("status = %q, want paused", page.Status)
	}

	review := agentDesign()
	review.Path = "reviews/2026-W41.md"
	if page := writePage(t, s, review); page.Type != "review" || page.Period != "2026-W41" || page.Status != "" {
		t.Errorf("type, period, status = %q, %q, %q, want review, 2026-W41 and no status", page.Type, page.Period, page.Status)
	}
}

func TestWikiWriteKeepsATitleWithPunctuation(t *testing.T) {
	s, _ := newWikiService(t)
	p := agentDesign()
	p.Title = `Dune: "Part Two" #1 - yes: no`
	p.Summary = `A film: it has 'quotes', a # and a trailing colon:`
	p.Body = "---\n\nA body that starts with a rule.\n\n---\n\nAnd has one more."

	written := writePage(t, s, p)

	stored, err := s.WikiRead(context.Background(), p.Path)
	if err != nil {
		t.Fatalf("WikiRead: %v", err)
	}
	if stored.Title != p.Title || stored.Summary != p.Summary || stored.Body != p.Body {
		t.Errorf("title, summary, body = %q, %q, %q", stored.Title, stored.Summary, stored.Body)
	}
	if stored != written {
		t.Errorf("stored page = %+v\nreturned    = %+v", stored, written)
	}
}

func TestWikiWriteThatCannotCommitLeavesNoChange(t *testing.T) {
	s, pool := newWikiService(t)
	first := writePage(t, s, agentDesign())
	index, _ := s.WikiRead(context.Background(), "index.md")

	// Without its git folder, the wiki cannot take a commit.
	repo := filepath.Dir(s.wiki.Dir)
	if err := os.Rename(filepath.Join(repo, ".git"), filepath.Join(repo, "git-away")); err != nil {
		t.Fatal(err)
	}

	update := agentDesign()
	update.Body, update.BaseHash = "A new body.", first.Hash
	if _, err := s.WikiWrite(context.Background(), update); err == nil {
		t.Error("the update returned no error")
	}
	fresh := agentDesign()
	fresh.Path = "topics/new-page.md"
	if _, err := s.WikiWrite(context.Background(), fresh); err == nil {
		t.Error("the create returned no error")
	}

	if stored, _ := s.WikiRead(context.Background(), "topics/agent-design.md"); stored != first {
		t.Errorf("the page changed: %+v", stored)
	}
	if _, err := s.WikiRead(context.Background(), "topics/new-page.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the new page: error = %v, want ErrNotFound", err)
	}
	if now, _ := s.WikiRead(context.Background(), "index.md"); now != index {
		t.Errorf("the index changed: %+v", now)
	}
	if n := len(wikiEvents(t, pool)); n != 1 {
		t.Errorf("%d wiki events, want 1", n)
	}
}

func TestWikiWriteOverAFileThatIsNotAValidPage(t *testing.T) {
	s, _ := newWikiService(t)
	broken := "Only text, from an edit by hand.\n"
	putPage(t, s, "topics/agent-design.md", broken)

	// Without the hash of the file, the write is refused, as for each
	// page that exists.
	_, err := s.WikiWrite(context.Background(), agentDesign())
	wantInvalid(t, err, "base_hash")

	p := agentDesign()
	p.BaseHash = hashOfFile(broken)
	page := writePage(t, s, p)
	if page.Body != agentDesign().Body || page.Created != "2026-10-11" {
		t.Errorf("body, created = %q, %q", page.Body, page.Created)
	}
}

func TestWikiWriteOfTwoCallersAtTheSameTime(t *testing.T) {
	s, _ := newWikiService(t)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := agentDesign()
			p.Body = strings.Repeat("Caller text. ", i+1)
			_, errs[i] = s.WikiWrite(context.Background(), p)
		}()
	}
	wg.Wait()

	refused := 0
	for _, err := range errs {
		var invalid *ValidationError
		if errors.As(err, &invalid) && invalid.Field == "base_hash" {
			refused++
		} else if err != nil {
			t.Errorf("error = %v", err)
		}
	}
	if refused != 1 {
		t.Errorf("%d callers were refused, want 1: the second one must read the page first", refused)
	}
	if n := len(commits(t, s)); n != 1 {
		t.Errorf("%d commits, want 1", n)
	}
}

func TestWikiWriteKeepsTheStatusThatTheCallLeavesOut(t *testing.T) {
	s, _ := newWikiService(t)
	p := agentDesign()
	p.Path, p.Status = "projects/kindle.md", "done"
	first := writePage(t, s, p)

	// An agent corrects one sentence and sends no status.
	p.Body, p.Status, p.BaseHash = "A corrected body.", "", first.Hash
	second := writePage(t, s, p)
	if second.Status != "done" {
		t.Errorf("status = %q, want done: the call did not send a status", second.Status)
	}

	// A status in the call changes the page.
	p.Status, p.BaseHash = "active", second.Hash
	if third := writePage(t, s, p); third.Status != "active" {
		t.Errorf("status = %q, want active", third.Status)
	}
}

func TestWikiWriteKeepsTheContestedMarkThatTheCallLeavesOut(t *testing.T) {
	s, _ := newWikiService(t)
	p := agentDesign()
	p.Contested = ptr(true)
	first := writePage(t, s, p)

	// An agent corrects one sentence and does not send the mark.
	p.Body, p.Contested, p.BaseHash = "A corrected body.", nil, first.Hash
	second := writePage(t, s, p)
	if !second.Contested {
		t.Error("the page lost its contested mark: the call did not send the mark")
	}

	// Only a call that sends false takes the mark away.
	p.Contested, p.BaseHash = ptr(false), second.Hash
	if third := writePage(t, s, p); third.Contested {
		t.Error("the page is still contested after a write with contested false")
	}
}
