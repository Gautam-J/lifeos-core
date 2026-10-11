package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// SearchTypes are the record types that Search looks in. A habit log has no
// text; the query tool answers a question about habits.
var SearchTypes = []string{"item", "journal", "todo", "source"}

const (
	maxQueryLength = 200
	maxQueryWords  = 10
)

// SearchParams is the input of Search.
type SearchParams struct {
	// Query is one or more words. A record matches when its text has
	// every word. Upper and lower case are the same, and a part of a word
	// matches: "lonav" finds "Lonavala".
	Query string
	// Types is optional: the record types to look in. Empty means every
	// type in SearchTypes.
	Types []string
	// Limit is the maximum number of hits. 0 means 50; the maximum is 200.
	Limit int
}

// SearchHit is one record that matches a search.
type SearchHit struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	// Summary is one line that says what the record holds.
	Summary string `json:"summary"`
	// Verified is false for a record that waits in the review queue.
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

// Search returns the records that have every word of the query, the newest
// first. It leaves out a deleted record. It writes nothing.
func (s *Service) Search(ctx context.Context, p SearchParams) ([]SearchHit, error) {
	if !isText(p.Query) {
		return nil, invalid("query", "must be valid text without a NUL character")
	}
	if utf8.RuneCountInString(p.Query) > maxQueryLength {
		return nil, invalid("query", fmt.Sprintf("must be at most %d characters", maxQueryLength))
	}
	words := strings.Fields(p.Query)
	if len(words) == 0 {
		return nil, invalid("query", "must not be empty")
	}
	if len(words) > maxQueryWords {
		return nil, invalid("query", fmt.Sprintf("must have at most %d words", maxQueryWords))
	}
	types := p.Types
	if len(types) == 0 {
		types = SearchTypes
	}
	for _, recordType := range types {
		if !slices.Contains(SearchTypes, recordType) {
			return nil, invalid("types", fmt.Sprintf("%q is not a type that search looks in; the types are: %s", recordType, strings.Join(SearchTypes, ", ")))
		}
	}
	maxRows, err := checkLimit(p.Limit)
	if err != nil {
		return nil, err
	}

	patterns := make([]string, len(words))
	for i, word := range words {
		patterns[i] = "%" + likeEscaper.Replace(word) + "%"
	}
	q := store.New(s.pool)
	hits := []SearchHit{}
	for _, recordType := range SearchTypes {
		if !slices.Contains(types, recordType) {
			continue
		}
		more, err := searchType(ctx, q, recordType, patterns, maxRows)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		hits = append(hits, more...)
	}
	// Each type gave its newest hits. Put them in one order.
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].CreatedAt.After(hits[j].CreatedAt) })
	if len(hits) > int(maxRows) {
		hits = hits[:maxRows]
	}
	return hits, nil
}

// likeEscaper makes a word a plain text in a LIKE pattern: the percent
// sign and the underscore lose their special meaning.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// searchType returns the newest hits of one record type.
func searchType(ctx context.Context, q *store.Queries, recordType string, patterns []string, maxRows int32) ([]SearchHit, error) {
	var hits []SearchHit
	switch recordType {
	case "item":
		rows, err := q.SearchCatalogItems(ctx, store.SearchCatalogItemsParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			item, err := itemFromRow(row)
			if err != nil {
				return nil, err
			}
			hits = append(hits, SearchHit{"item", item.ID, item.summary(), item.Verified, item.CreatedAt})
		}
	case "journal":
		rows, err := q.SearchJournalEntries(ctx, store.SearchJournalEntriesParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			entry := journalEntryFromRow(row)
			hits = append(hits, SearchHit{"journal", entry.ID, entry.summary(), entry.Verified, entry.CreatedAt})
		}
	case "todo":
		rows, err := q.SearchTodos(ctx, store.SearchTodosParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			todo := todoFromRow(row)
			hits = append(hits, SearchHit{"todo", todo.ID, todo.summary(), todo.Verified, todo.CreatedAt})
		}
	case "source":
		rows, err := q.SearchSources(ctx, store.SearchSourcesParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			capture := captureFromRow(row)
			hits = append(hits, SearchHit{"source", capture.ID, capture.summary(), capture.Verified, capture.CreatedAt})
		}
	}
	return hits, nil
}
