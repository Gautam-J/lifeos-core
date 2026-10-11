package service

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// A wiki page cites the row of a fact as (prefix:id): (journal:57),
// (src:12), (movie:3). The prefix of a list item is its kind.
var wikiCitationPattern = regexp.MustCompile(`\(([a-z]+):([0-9]+)\)`)

// WikiCitationPrefixes are the prefixes that a citation can have. The
// description of the wiki_write tool lists them.
var WikiCitationPrefixes = append([]string{"src", "journal", "todo", "habitlog"}, ItemKinds...)

// wikiCitations returns each citation in text one time, as it is written.
func wikiCitations(text string) []string {
	var cites []string
	for _, cite := range wikiCitationPattern.FindAllString(text, -1) {
		if !slices.Contains(cites, cite) {
			cites = append(cites, cite)
		}
	}
	return cites
}

// checkNewCitations refuses text that cites a row that is not there. It
// checks only the citations that oldText does not have: a page keeps the
// citation of a row that was deleted later, and lint reports that
// citation. field is the name of the input field, for the error.
func checkNewCitations(ctx context.Context, q *store.Queries, field, oldText, newText string) error {
	old := wikiCitations(oldText)
	for _, cite := range wikiCitations(newText) {
		if slices.Contains(old, cite) {
			continue
		}
		live, err := citationIsLive(ctx, q, cite)
		if err != nil {
			return err
		}
		if !live {
			return invalid(field, "cites "+cite+", and no live record has this prefix and ID. Cite only an ID that a tool returned to you. The prefixes are: "+strings.Join(WikiCitationPrefixes, ", "))
		}
	}
	return nil
}

// citationIsLive reports whether cite names a row that is not deleted. A
// prefix that is not in WikiCitationPrefixes names no row.
func citationIsLive(ctx context.Context, q *store.Queries, cite string) (bool, error) {
	m := wikiCitationPattern.FindStringSubmatch(cite)
	if m == nil {
		return false, nil
	}
	prefix := m[1]
	// A number that is too large for an ID names no row.
	id, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return false, nil
	}
	var live bool
	switch {
	case prefix == "src":
		live, err = q.LiveSourceExists(ctx, id)
	case prefix == "journal":
		live, err = q.LiveJournalEntryExists(ctx, id)
	case prefix == "todo":
		live, err = q.LiveTodoExists(ctx, id)
	case prefix == "habitlog":
		live, err = q.LiveHabitLogExists(ctx, id)
	case slices.Contains(ItemKinds, prefix):
		live, err = q.LiveItemOfKindExists(ctx, store.LiveItemOfKindExistsParams{ID: id, Kind: prefix})
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("wiki: check the citation %s: %w", cite, err)
	}
	return live, nil
}
