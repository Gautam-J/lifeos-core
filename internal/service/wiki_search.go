package service

import (
	"context"
	"slices"
	"strings"
)

const (
	// maxWikiHitLines is the number of lines that a hit shows.
	maxWikiHitLines = 3
	maxWikiHitRunes = 200
)

// WikiHit is one page that WikiSearch found.
type WikiHit struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Updated string `json:"updated"`
	// Lines are the first lines of the body that have a word of the
	// query, three at most.
	Lines []string `json:"lines,omitempty"`
}

// WikiSearch returns the pages that have each word of query in the title,
// the summary or the body. A word matches in any case and as a part of a
// longer word. A match in the title counts more than one in the summary,
// and that counts more than one in the body. With no word in query, it
// returns each page in the order of the path. The index and the log are
// not pages. limit 0 means 50; the maximum is 200.
//
// The search reads the files: a wiki of one person is small.
func (s *Service) WikiSearch(ctx context.Context, query string, limit int) ([]WikiHit, error) {
	max, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	root, err := s.openWiki()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	pages, _, err := listWikiPages(root)
	if err != nil {
		return nil, err
	}

	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		hit   WikiHit
		score int
	}
	found := []scored{}
	for _, page := range pages {
		hit := WikiHit{Path: page.Path, Type: page.Type, Title: page.Title, Summary: page.Summary, Updated: page.Updated}
		score, ok := wikiScore(page, words)
		if !ok {
			continue
		}
		hit.Lines = wikiHitLines(page.Body, words)
		found = append(found, scored{hit, score})
	}
	// The pages are in the order of the path. With no word there is no
	// score, and that order stays.
	if len(words) > 0 {
		slices.SortStableFunc(found, func(a, b scored) int {
			if a.score != b.score {
				return b.score - a.score
			}
			return strings.Compare(b.hit.Updated, a.hit.Updated)
		})
	}
	if len(found) > int(max) {
		found = found[:max]
	}
	hits := make([]WikiHit, len(found))
	for i, f := range found {
		hits[i] = f.hit
	}
	return hits, nil
}

// wikiScore returns the score of page for words. ok is false when one of
// the words is not in the page.
func wikiScore(page WikiPage, words []string) (score int, ok bool) {
	title, summary, body := strings.ToLower(page.Title), strings.ToLower(page.Summary), strings.ToLower(page.Body)
	for _, word := range words {
		wordScore := 0
		if strings.Contains(title, word) {
			wordScore += 3
		}
		if strings.Contains(summary, word) {
			wordScore += 2
		}
		if strings.Contains(body, word) {
			wordScore++
		}
		if wordScore == 0 {
			return 0, false
		}
		score += wordScore
	}
	return score, true
}

// wikiHitLines returns the first lines of body that have one of words.
func wikiHitLines(body string, words []string) []string {
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		lower := strings.ToLower(line)
		if !slices.ContainsFunc(words, func(word string) bool { return strings.Contains(lower, word) }) {
			continue
		}
		lines = append(lines, shorten(line, maxWikiHitRunes))
		if len(lines) == maxWikiHitLines {
			break
		}
	}
	return lines
}
