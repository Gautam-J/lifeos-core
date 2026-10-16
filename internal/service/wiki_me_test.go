package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func meParams(body string) WikiWriteParams {
	return WikiWriteParams{Path: "me.md", Title: "About Gautam", Summary: "Who Gautam is and what he does now.", Body: body, Actor: SourceAgent}
}

func TestWikiWriteKeepsMeInTheTopOfTheWikiOutOfTheIndex(t *testing.T) {
	s, _ := newWikiService(t)

	page := writePage(t, s, meParams("## Who\n\nGautam builds LifeOS. (stated)"))

	if page.Path != "me.md" || page.Type != "me" {
		t.Errorf("page = %s, %s, want me.md, me", page.Path, page.Type)
	}
	if _, err := os.Stat(filepath.Join(s.wiki.Dir, "me.md")); err != nil {
		t.Errorf("no file me.md: %v", err)
	}
	index, err := s.WikiRead(context.Background(), "index.md")
	if err != nil || strings.Contains(index.Body, "me.md") {
		t.Errorf("index = %q, %v, want no line for me.md", index.Body, err)
	}
	read, err := s.WikiRead(context.Background(), "me.md")
	if err != nil || read.Hash != page.Hash {
		t.Errorf("WikiRead(me.md) = %+v, %v", read, err)
	}
}

func TestWikiWriteRefusesAMeLongerThan2000Characters(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()

	ok := strings.Repeat("é", 2000)
	page := writePage(t, s, meParams(ok))
	_, err := s.WikiWrite(ctx, WikiWriteParams{Path: "me.md", Title: "About Gautam", Summary: "S.", Body: ok + "x", BaseHash: page.Hash, Actor: SourceAgent})
	wantInvalid(t, err, "body")
	if err == nil || !strings.Contains(err.Error(), "2000") {
		t.Errorf("error = %v, want it to name the limit 2000", err)
	}
	// An append that makes the page longer is refused too.
	_, err = s.WikiAppend(ctx, WikiAppendParams{Path: "me.md", Text: "- one more line", Actor: SourceAgent})
	wantInvalid(t, err, "text")
	stored, _ := s.WikiRead(ctx, "me.md")
	if stored.Hash != page.Hash {
		t.Error("a refused call changed me.md")
	}
	// Another page has no such limit.
	long := agentDesign()
	long.Body = strings.Repeat("a ", 3000)
	writePage(t, s, long)
}

func TestStatedIsACitationOfMeOnly(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()

	writePage(t, s, meParams("Lives in Chennai. (stated)"))
	if _, err := s.WikiAppend(ctx, WikiAppendParams{Path: "me.md", Text: "- Likes tea. (stated)", Actor: SourceAgent}); err != nil {
		t.Errorf("append to me.md: %v", err)
	}

	p := agentDesign()
	p.Body = "Tools carry the rules. (stated)"
	_, err := s.WikiWrite(ctx, p)
	wantInvalid(t, err, "body")
	page := writePage(t, s, agentDesign())
	_, err = s.WikiAppend(ctx, WikiAppendParams{Path: page.Path, Text: "- 2026-10-11: It works. (stated)", Actor: SourceAgent})
	wantInvalid(t, err, "text")
}

func TestWikiLintChecksMe(t *testing.T) {
	s, _ := newWikiService(t)
	// The day is 2026-10-11 in IST.
	putPage(t, s, "me.md", "---\ntype: me\ntitle: About Gautam\nsummary: S.\ncreated: 2026-10-01\nupdated: 2026-10-01\n---\n\n"+
		"## Now\n\n- Ships #31 (until 2026-10-10)\n- Ships #35 (until 2026-10-11)\n- Reads (journal:9999)\n")
	got := lintWiki(t, s)
	want := []string{"me.md dead_citation", "me.md expired_line"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("findings = %v, want %v", got, want)
	}

	// A page outside me.md that holds (stated) was not written by WikiWrite.
	putPage(t, s, "topics/a.md", "---\ntype: topic\ntitle: A\nsummary: S.\ncreated: 2026-10-10\nupdated: 2026-10-10\n---\n\nA fact. (stated) [me](../me.md)\n")
	putPage(t, s, "index.md", "---\ntype: index\ntitle: Index\nsummary: S.\ncreated: 2026-10-10\nupdated: 2026-10-10\n---\n\n# Index\n\n[a](topics/a.md)\n")
	got = lintWiki(t, s)
	found := false
	for _, f := range got {
		if f == "topics/a.md stray_stated" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %v, want topics/a.md stray_stated", got)
	}
}

func TestContext(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()

	missing, err := s.Context(ctx)
	if err != nil || missing.Me != "" || !strings.Contains(missing.Note, "me.md") || !strings.Contains(missing.Note, "wiki_write") {
		t.Errorf("with no page: %+v, %v, want a note that names me.md and wiki_write", missing, err)
	}

	putPage(t, s, "me.md", "---\ntype: me\ntitle: About Gautam\nsummary: S.\ncreated: 2026-10-01\nupdated: 2026-10-01\n---\n\n"+
		"## Now\n\n- Ships #31 (until 2026-10-10)\n- Ships #35 (until 2026-10-11)\n- No expiry\n")
	got, err := s.Context(ctx)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if strings.Contains(got.Me, "#31") || !strings.Contains(got.Me, "#35") || !strings.Contains(got.Me, "No expiry") || got.Note != "" {
		t.Errorf("Context = %+v, want the page without the expired line", got)
	}

	// The service has no wiki folder: the caller gets an error.
	bare, _ := newTestService(t)
	if _, err := bare.Context(ctx); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("with no wiki folder: error = %v, want an error", err)
	}
}
