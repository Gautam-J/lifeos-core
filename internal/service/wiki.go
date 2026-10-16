package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Wiki says where the service keeps the wiki pages.
type Wiki struct {
	// Dir is the folder of the pages. It is in a git repository: each
	// write of a page is a commit there.
	Dir string
}

// WithWiki sets the wiki and returns s. Without it, each wiki function
// returns an error.
func (s *Service) WithWiki(wiki Wiki) *Service {
	s.wiki = wiki
	return s
}

// The three files of the wiki that are not in a page folder. WikiWrite
// keeps the index. The log gets a new entry at its end and no other change.
// me.md is a page that an agent writes with a yes of Gautam.
const (
	wikiIndexPath = "index.md"
	wikiLogPath   = "log.md"
	// wikiMePath is the short page about Gautam that each session starts
	// with (#35). It is at the top of the wiki and not in the index.
	wikiMePath = "me.md"
)

// WikiPage is one file of the wiki: a page, the index or the log.
type WikiPage struct {
	// Path is the path below the wiki folder, such as projects/lifeos.md.
	Path string `json:"path"`
	// Type is person, project, topic or review for a page. The folder
	// gives the type. The index has the type index, the log has log.
	Type    string `json:"type"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// Created and Updated are IST days as YYYY-MM-DD.
	Created string `json:"created"`
	Updated string `json:"updated"`
	// Status is active, paused or done. Only a project has it.
	Status string `json:"status,omitempty"`
	// Period is the week or the month of a review, such as 2026-W41.
	Period string `json:"period,omitempty"`
	// Contested is true when the page holds two statements that
	// contradict each other.
	Contested bool   `json:"contested,omitempty"`
	Body      string `json:"body"`
	// Hash is the SHA-256 of the file, in hex. WikiWrite takes it as
	// proof that the caller read this version.
	Hash string `json:"hash"`
}

// wikiHeader is the frontmatter of a wiki file.
type wikiHeader struct {
	Type      string `yaml:"type"`
	Title     string `yaml:"title"`
	Summary   string `yaml:"summary"`
	Created   string `yaml:"created"`
	Updated   string `yaml:"updated"`
	Status    string `yaml:"status,omitempty"`
	Period    string `yaml:"period,omitempty"`
	Contested bool   `yaml:"contested,omitempty"`
}

// wikiFolders gives the page type of each folder of the wiki.
var wikiFolders = map[string]string{
	"people":   "person",
	"projects": "project",
	"topics":   "topic",
	"reviews":  "review",
}

var (
	wikiPagePattern   = regexp.MustCompile(`^(people|projects|topics)/[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	wikiReviewPattern = regexp.MustCompile(`^reviews/([0-9]{4}-(W[0-9]{2}|[0-9]{2}))\.md$`)
)

const wikiPathRule = "must be the path of a page below the wiki folder, such as people/asha-rao.md, projects/lifeos.md, topics/agent-design.md, reviews/2026-W41.md or reviews/2026-10.md: lowercase letters, digits and hyphens"

// wikiPageType returns the page type that the folder of path gives. It
// refuses each path that is not the path of a page.
func wikiPageType(path string) (string, error) {
	if !wikiPagePattern.MatchString(path) && !wikiReviewPattern.MatchString(path) {
		return "", invalid("path", wikiPathRule)
	}
	folder, _, _ := strings.Cut(path, "/")
	return wikiFolders[folder], nil
}

// wikiFileType is wikiPageType, and it accepts the index and the log too.
func wikiFileType(path string) (string, error) {
	switch path {
	case wikiIndexPath:
		return "index", nil
	case wikiLogPath:
		return "log", nil
	case wikiMePath:
		return "me", nil
	}
	pageType, err := wikiPageType(path)
	if err != nil {
		return "", invalid("path", wikiPathRule+"; or index.md, log.md or me.md")
	}
	return pageType, nil
}

// openWiki opens the wiki folder. os.Root refuses a path that leaves the
// folder, also through a symbolic link.
func (s *Service) openWiki() (*os.Root, error) {
	if s.wiki.Dir == "" {
		return nil, errors.New("wiki: the service has no wiki folder")
	}
	root, err := os.OpenRoot(s.wiki.Dir)
	if err != nil {
		return nil, fmt.Errorf("wiki: open the wiki folder: %w", err)
	}
	return root, nil
}

// WikiRead returns one file of the wiki: a page, index.md or log.md.
func (s *Service) WikiRead(ctx context.Context, path string) (WikiPage, error) {
	if _, err := wikiFileType(path); err != nil {
		return WikiPage{}, err
	}
	root, err := s.openWiki()
	if err != nil {
		return WikiPage{}, err
	}
	defer root.Close()
	return readWikiFile(root, path)
}

// readWikiFile reads and parses one file. path is a valid path already.
func readWikiFile(root *os.Root, path string) (WikiPage, error) {
	raw, err := root.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return WikiPage{}, ErrNotFound
	}
	if err != nil {
		return WikiPage{}, fmt.Errorf("wiki: read %s: %w", path, err)
	}
	return parseWikiFile(path, raw)
}

// parseWikiFile reads the frontmatter and the body of one file. A file
// that a person changed by hand can be wrong: the error says what is wrong.
func parseWikiFile(path string, raw []byte) (WikiPage, error) {
	notAPage := func(reason string) (WikiPage, error) {
		return WikiPage{}, invalid("path", "the file "+path+" is not a valid wiki page: "+reason)
	}
	fileType, err := wikiFileType(path)
	if err != nil {
		return WikiPage{}, err
	}
	rest, ok := bytes.CutPrefix(raw, []byte("---\n"))
	if !ok {
		return notAPage("it does not start with a frontmatter block")
	}
	head, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return notAPage("its frontmatter block has no end line")
	}
	var header wikiHeader
	if err := yaml.UnmarshalWithOptions(head, &header, yaml.Strict()); err != nil {
		return notAPage("its frontmatter is not valid: " + firstLine(err.Error()))
	}
	if header.Type != fileType {
		return notAPage(fmt.Sprintf("its type is %q, and the folder needs %q", header.Type, fileType))
	}
	if strings.TrimSpace(header.Title) == "" || strings.TrimSpace(header.Summary) == "" {
		return notAPage("it has no title or no summary")
	}
	for _, day := range []string{header.Created, header.Updated} {
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			return notAPage("created and updated must be dates as YYYY-MM-DD")
		}
	}
	return WikiPage{
		Path:      path,
		Type:      header.Type,
		Title:     header.Title,
		Summary:   header.Summary,
		Created:   header.Created,
		Updated:   header.Updated,
		Status:    header.Status,
		Period:    header.Period,
		Contested: header.Contested,
		Body:      strings.Trim(string(body), "\n"),
		Hash:      hashHex(raw),
	}, nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
