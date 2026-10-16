package service

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	// maxWikiPageLines is the length above which lint reports a page.
	maxWikiPageLines = 150
	// wikiStaleDays is the age above which lint reports a page.
	wikiStaleDays = 90
)

// WikiFinding is one problem that WikiLint found.
type WikiFinding struct {
	Path string `json:"path"`
	// Check is the kind of the problem: not_a_page, dead_citation,
	// broken_link, contested, orphan, stale, long, stray_stated or
	// expired_line.
	Check  string `json:"check"`
	Detail string `json:"detail"`
}

// wikiLinkPattern finds the target of a Markdown link.
var wikiLinkPattern = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// WikiLint returns the problems of the wiki that code can find. It changes
// nothing. The checks:
//
//   - not_a_page: a Markdown file in a page folder that is not a valid page
//   - dead_citation: a citation of a record that is deleted or not there
//   - broken_link: a link to a page that is not there
//   - contested: the page is marked as one that holds two statements that
//     contradict each other, so a person must say which one is true
//   - orphan: a page that no other page links to; the index does not count
//   - stale: no change for more than 90 days; not for a review and not
//     for a project that is done
//   - long: more than 150 lines
//
// me.md is checked for dead_citation and for expired_line: a line with
// (until YYYY-MM-DD) whose day is past.
//
// A statement with no citation and two pages that contradict each other
// need a reader: the agent looks for them.
func (s *Service) WikiLint(ctx context.Context) ([]WikiFinding, error) {
	root, err := s.openWiki()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	pages, bad, err := listWikiPages(root)
	if err != nil {
		return nil, err
	}

	findings := []WikiFinding{}
	for _, file := range bad {
		findings = append(findings, WikiFinding{Path: file.Path, Check: "not_a_page", Detail: file.Reason})
	}
	isPage := map[string]bool{wikiIndexPath: true, wikiLogPath: true, wikiMePath: true}
	for _, page := range pages {
		isPage[page.Path] = true
	}
	inbound := map[string]int{}
	q := store.New(s.pool)
	day := today(s.now())
	for _, page := range pages {
		for _, cite := range wikiCitations(page.Body) {
			live, err := citationIsLive(ctx, q, cite)
			if err != nil {
				return nil, err
			}
			if !live {
				findings = append(findings, WikiFinding{Path: page.Path, Check: "dead_citation", Detail: cite + " names no live record"})
			}
		}
		if strings.Contains(page.Body, statedMark) {
			findings = append(findings, WikiFinding{Path: page.Path, Check: "stray_stated", Detail: statedMark + " is for me.md only: cite the record of the fact, or move the line to me.md"})
		}
		for _, target := range wikiLinks(page) {
			switch {
			case !isPage[target]:
				findings = append(findings, WikiFinding{Path: page.Path, Check: "broken_link", Detail: "the link to " + target + " names no page"})
			case target != page.Path:
				inbound[target]++
			}
		}
		if page.Contested {
			findings = append(findings, WikiFinding{Path: page.Path, Check: "contested", Detail: "the page holds two statements that contradict each other: ask the user which one is true now, then write the page with contested set to false"})
		}
		if lines := strings.Count(page.Body, "\n") + 1; lines > maxWikiPageLines {
			findings = append(findings, WikiFinding{Path: page.Path, Check: "long", Detail: fmt.Sprintf("%d lines: split the page, or make it shorter, to %d lines at most", lines, maxWikiPageLines)})
		}
		// parseWikiFile accepted the date.
		updated, _ := time.Parse(time.DateOnly, page.Updated)
		closed := page.Type == "review" || page.Type == "project" && page.Status == "done"
		if !closed && day.Sub(updated) > wikiStaleDays*24*time.Hour {
			findings = append(findings, WikiFinding{Path: page.Path, Check: "stale", Detail: fmt.Sprintf("the last change was on %s, more than %d days ago", page.Updated, wikiStaleDays)})
		}
	}
	meFindings, err := s.lintMe(ctx, root, q, day)
	if err != nil {
		return nil, err
	}
	findings = append(findings, meFindings...)
	for _, page := range pages {
		if inbound[page.Path] == 0 {
			findings = append(findings, WikiFinding{Path: page.Path, Check: "orphan", Detail: "no other page links to this page"})
		}
	}
	slices.SortStableFunc(findings, func(a, b WikiFinding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Check, b.Check))
	})
	return findings, nil
}

// wikiLinks returns the path, below the wiki folder, of each Markdown file
// that page links to. A link to a different site is not in the result.
func wikiLinks(page WikiPage) []string {
	var targets []string
	for _, m := range wikiLinkPattern.FindAllStringSubmatch(page.Body, -1) {
		link, _, _ := strings.Cut(m[1], "#")
		if strings.Contains(link, ":") || !strings.HasSuffix(link, ".md") {
			continue
		}
		targets = append(targets, path.Join(path.Dir(page.Path), link))
	}
	return targets
}
