package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWikiWriteAcceptsACitationOfEachRecordKind(t *testing.T) {
	s, _ := newWikiService(t)
	addHabit(t, s, "gym")
	cites := []string{
		fmt.Sprintf("(src:%d)", captureText(t, s, "An article about agents.").ID),
		fmt.Sprintf("(journal:%d)", addJournal(t, s, "Planned the wiki.", "").ID),
		fmt.Sprintf("(movie:%d)", addItem(t, s, "movie", "Dune").ID),
		fmt.Sprintf("(book:%d)", addItem(t, s, "book", "Dune").ID),
		fmt.Sprintf("(place:%d)", addItem(t, s, "place", "Lonavala").ID),
		fmt.Sprintf("(game:%d)", addItem(t, s, "game", "Hades").ID),
		fmt.Sprintf("(todo:%d)", addTodo(t, s, "Buy a domain").ID),
		fmt.Sprintf("(habitlog:%d)", logHabit(t, s, "gym", "").ID),
	}

	p := agentDesign()
	p.Body = "Each fact has its row. " + strings.Join(cites, " ") + "\n\n- 2026-10-11: Two in one line " + cites[0] + cites[1]
	writePage(t, s, p)
}

func TestWikiWriteRefusesACitationThatNamesNoLiveRecord(t *testing.T) {
	s, pool := newWikiService(t)
	book := addItem(t, s, "book", "Dune")
	gone := addItem(t, s, "movie", "Tenet")
	if err := s.DeleteItem(context.Background(), gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	for name, cite := range map[string]string{
		"a source that does not exist": "(src:999)",
		"a journal entry of no row":    "(journal:999)",
		"a todo of no row":             "(todo:999)",
		"a habit log of no row":        "(habitlog:999)",
		"a book cited as a movie":      fmt.Sprintf("(movie:%d)", book.ID),
		"a deleted record":             fmt.Sprintf("(movie:%d)", gone.ID),
		"a prefix that LifeOS has not": "(note:1)",
		"a table that is not built":    "(txn:1)",
		"a record type as the prefix":  fmt.Sprintf("(item:%d)", book.ID),
		"a number that is too large":   "(movie:99999999999999999999)",
		"the ID zero":                  "(movie:0)",
	} {
		p := agentDesign()
		p.Body = "A statement of fact " + cite + "."
		_, err := s.WikiWrite(context.Background(), p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "body" {
			t.Errorf("%s: error = %v, want a ValidationError for body", name, err)
			continue
		}
		if !strings.Contains(invalid.Message, cite) {
			t.Errorf("%s: the message %q does not name %s", name, invalid.Message, cite)
		}
	}
	if n := len(wikiEvents(t, pool)); n != 0 {
		t.Errorf("%d wiki events, want none", n)
	}
}

func TestWikiWriteKeepsACitationThatThePageHadAlready(t *testing.T) {
	s, _ := newWikiService(t)
	movie := addItem(t, s, "movie", "Tenet")
	cite := fmt.Sprintf("(movie:%d)", movie.ID)
	p := agentDesign()
	p.Body = "Gautam liked Tenet " + cite + "."
	first := writePage(t, s, p)

	if err := s.DeleteItem(context.Background(), movie.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	// The page can change while the old citation stays. Lint reports it.
	p.Body, p.BaseHash = "Gautam liked Tenet "+cite+". He saw it two times.", first.Hash
	second := writePage(t, s, p)

	// A new citation of no record is refused as on a new page.
	p.Body, p.BaseHash = p.Body+" And one more (movie:999).", second.Hash
	_, err := s.WikiWrite(context.Background(), p)
	wantInvalid(t, err, "body")
}

func TestWikiWriteAcceptsParenthesesThatAreNotCitations(t *testing.T) {
	s, _ := newWikiService(t)

	p := agentDesign()
	p.Body = "The call was late (at 5:30). The ratio is good (3:2). See (a:b) and (Note: 4) and (movie: 3) and ( movie:3 )."
	writePage(t, s, p)
}
