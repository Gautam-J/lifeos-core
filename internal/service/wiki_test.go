package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lifeosPage is a page as a person writes it by hand: the dates have no
// quotes.
const lifeosPage = `---
type: project
title: LifeOS
summary: Self-hosted second brain on a Raspberry Pi.
created: 2026-10-01
updated: 2026-10-09
status: active
---

LifeOS is the second brain of Gautam.

- 2026-10-09: The data model is complete.
`

// newWikiService returns a service with an empty wiki in a new git
// repository in a temporary folder.
func newWikiService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	s, pool := newTestService(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	dir := filepath.Join(repo, "wiki")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s.WithWiki(Wiki{Dir: dir})
	return s, pool
}

// putPage writes one file into the wiki folder, as a person does by hand.
func putPage(t *testing.T, s *Service, path, content string) {
	t.Helper()
	full := filepath.Join(s.wiki.Dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatalf("make the folder of %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hashOfFile(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// wantInvalid fails the test unless err is a ValidationError for field.
func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != field {
		t.Errorf("error = %v, want a ValidationError for %s", err, field)
	}
}

func TestWikiReadReturnsThePageAndItsHash(t *testing.T) {
	s, _ := newWikiService(t)
	putPage(t, s, "projects/lifeos.md", lifeosPage)

	page, err := s.WikiRead(context.Background(), "projects/lifeos.md")
	if err != nil {
		t.Fatalf("WikiRead: %v", err)
	}
	want := WikiPage{
		Path:    "projects/lifeos.md",
		Type:    "project",
		Title:   "LifeOS",
		Summary: "Self-hosted second brain on a Raspberry Pi.",
		Created: "2026-10-01",
		Updated: "2026-10-09",
		Status:  "active",
		Body:    "LifeOS is the second brain of Gautam.\n\n- 2026-10-09: The data model is complete.",
		Hash:    hashOfFile(lifeosPage),
	}
	if page != want {
		t.Errorf("page = %+v\nwant   %+v", page, want)
	}
}

func TestWikiReadReturnsTheIndexAndTheLog(t *testing.T) {
	s, _ := newWikiService(t)
	for path, pageType := range map[string]string{"index.md": "index", "log.md": "log"} {
		putPage(t, s, path, "---\ntype: "+pageType+"\ntitle: T\nsummary: S\ncreated: 2026-10-10\nupdated: 2026-10-10\n---\n\n# T\n")

		page, err := s.WikiRead(context.Background(), path)
		if err != nil {
			t.Fatalf("WikiRead(%s): %v", path, err)
		}
		if page.Type != pageType || page.Body != "# T" {
			t.Errorf("%s: type, body = %q, %q, want %q, %q", path, page.Type, page.Body, pageType, "# T")
		}
	}
}

func TestWikiReadAcceptsThePathOfEachPageType(t *testing.T) {
	s, _ := newWikiService(t)
	for path, pageType := range map[string]string{
		"people/asha-rao.md":     "person",
		"projects/lifeos.md":     "project",
		"topics/agent-design.md": "topic",
		"topics/2026-plans.md":   "topic",
		"reviews/2026-W41.md":    "review",
		"reviews/2026-10.md":     "review",
	} {
		putPage(t, s, path, "---\ntype: "+pageType+"\ntitle: T\nsummary: S\ncreated: 2026-10-10\nupdated: 2026-10-10\n---\n\nText.\n")

		page, err := s.WikiRead(context.Background(), path)
		if err != nil {
			t.Errorf("WikiRead(%s): %v", path, err)
		}
		if page.Type != pageType {
			t.Errorf("%s: type = %q, want %q", path, page.Type, pageType)
		}
	}
}

func TestWikiReadRefusesAPathThatIsNotAPage(t *testing.T) {
	s, _ := newWikiService(t)
	outside := filepath.Join(filepath.Dir(s.wiki.Dir), "secret.md")
	if err := os.WriteFile(outside, []byte(lifeosPage), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"",
		"../secret.md",
		outside,
		"projects/../../secret.md",
		"notes/lifeos.md",
		"projects/Life OS.md",
		"projects/LifeOS.md",
		"projects/lifeos.txt",
		"projects/lifeos",
		"projects/a/b.md",
		"projects/-lifeos.md",
		"reviews/notes.md",
		"reviews/2026-w41.md",
		"README.md",
	} {
		_, err := s.WikiRead(context.Background(), path)
		wantInvalid(t, err, "path")
	}
}

func TestWikiReadOfAPageThatIsAbsent(t *testing.T) {
	s, _ := newWikiService(t)

	_, err := s.WikiRead(context.Background(), "topics/agent-design.md")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestWikiReadRefusesAFileThatIsNotAValidPage(t *testing.T) {
	s, _ := newWikiService(t)
	header := "type: topic\ntitle: T\nsummary: S\ncreated: 2026-10-10\nupdated: 2026-10-10\n"
	for name, content := range map[string]string{
		"no frontmatter":             "Only text.\n",
		"frontmatter with no end":    "---\n" + header + "\nText.\n",
		"type of a different folder": "---\n" + strings.Replace(header, "topic", "person", 1) + "---\n\nText.\n",
		"no title":                   "---\n" + strings.Replace(header, "title: T\n", "", 1) + "---\n\nText.\n",
		"no summary":                 "---\n" + strings.Replace(header, "summary: S\n", "", 1) + "---\n\nText.\n",
		"a date that is no date":     "---\n" + strings.Replace(header, "created: 2026-10-10", "created: yesterday", 1) + "---\n\nText.\n",
		"an unknown key":             "---\n" + header + "owner: me\n---\n\nText.\n",
	} {
		putPage(t, s, "topics/broken.md", content)

		_, err := s.WikiRead(context.Background(), "topics/broken.md")
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "path" {
			t.Errorf("%s: error = %v, want a ValidationError for path", name, err)
		}
	}
}

func TestWikiReadFollowsNoLinkOutOfTheWiki(t *testing.T) {
	s, _ := newWikiService(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(strings.Replace(lifeosPage, "project", "topic", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.wiki.Dir, "topics"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.wiki.Dir, "topics", "leak.md")); err != nil {
		t.Fatal(err)
	}

	page, err := s.WikiRead(context.Background(), "topics/leak.md")
	if err == nil {
		t.Errorf("WikiRead returned the file outside the wiki: %+v", page)
	}
}

func TestWikiReadNeedsAWiki(t *testing.T) {
	s, _ := newTestService(t)

	if _, err := s.WikiRead(context.Background(), "index.md"); err == nil {
		t.Error("WikiRead returned no error for a service that has no wiki")
	}
}
