package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// maxMeRunes is the length of the body of me.md. The page is in the
// context of each session, so it must stay short.
const maxMeRunes = 2000

// meUntilPattern finds the last day of a line of me.md that expires.
var meUntilPattern = regexp.MustCompile(`\(until ([0-9]{4}-[0-9]{2}-[0-9]{2})\)`)

// checkMeLength refuses a body of me.md that is too long. Another page has
// no such limit here. field is the name of the input field, for the error.
func checkMeLength(path, field, body string) error {
	if path == wikiMePath && utf8.RuneCountInString(body) > maxMeRunes {
		return invalid(field, fmt.Sprintf("makes me.md longer than %d characters: the page is in the context of each session, so shorten it first", maxMeRunes))
	}
	return nil
}

// meLineExpired reports whether line ends with a day that is before day.
func meLineExpired(line string, day time.Time) bool {
	m := meUntilPattern.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	until, err := time.Parse(time.DateOnly, m[1])
	return err == nil && until.Before(day)
}

// lintMe returns the findings for me.md: a citation of a row that is not
// live, and a line that is past its day. A wiki with no me.md has none.
func (s *Service) lintMe(ctx context.Context, root *os.Root, q *store.Queries, day time.Time) ([]WikiFinding, error) {
	raw, err := root.ReadFile(wikiMePath)
	if err != nil {
		return nil, nil
	}
	page, err := parseWikiFile(wikiMePath, raw)
	if err != nil {
		return []WikiFinding{{Path: wikiMePath, Check: "not_a_page", Detail: err.Error()}}, nil
	}
	var findings []WikiFinding
	for _, cite := range wikiCitations(page.Body) {
		live, err := citationIsLive(ctx, q, cite)
		if err != nil {
			return nil, err
		}
		if !live {
			findings = append(findings, WikiFinding{Path: wikiMePath, Check: "dead_citation", Detail: cite + " names no live record"})
		}
	}
	for _, line := range strings.Split(page.Body, "\n") {
		if meLineExpired(line, day) {
			findings = append(findings, WikiFinding{Path: wikiMePath, Check: "expired_line", Detail: "this line is past its day: " + strings.TrimSpace(line)})
		}
	}
	return findings, nil
}

// SessionContext is what each session starts with.
type SessionContext struct {
	// Me is the body of me.md without the lines that are past their day.
	Me string `json:"me,omitempty"`
	// Note says what is missing and how to make it.
	Note string `json:"note,omitempty"`
}

// Context returns the text that each session starts with: the body of
// me.md, without the lines that are past their day. When the page is not
// there, Note says how to make it.
func (s *Service) Context(ctx context.Context) (SessionContext, error) {
	page, err := s.WikiRead(ctx, wikiMePath)
	if errors.Is(err, ErrNotFound) {
		return SessionContext{Note: "The page me.md does not exist yet. Offer to draft it from the wiki and the records, ask the user to approve the text, then call wiki_write with path me.md. Keep it at most 2000 characters. End a line that the user only said with (stated)."}, nil
	}
	if err != nil {
		return SessionContext{}, err
	}
	day := today(s.now())
	var kept []string
	for _, line := range strings.Split(page.Body, "\n") {
		if !meLineExpired(line, day) {
			kept = append(kept, line)
		}
	}
	return SessionContext{Me: strings.Join(kept, "\n")}, nil
}
