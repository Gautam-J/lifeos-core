package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	maxWikiTitleRunes   = 200
	maxWikiSummaryRunes = 300
	// maxWikiBodyBytes is far above a page that a person can read. The
	// limit stops a mistake, such as a whole book in one page.
	maxWikiBodyBytes = 256 << 10
)

var wikiStatuses = []string{"active", "paused", "done"}

// WikiWriteParams is the input of WikiWrite.
type WikiWriteParams struct {
	// Path is the path of the page below the wiki folder. Its folder
	// gives the page type.
	Path    string
	Title   string
	Summary string
	// Body is the text of the page, in Markdown, without frontmatter.
	Body string
	// Status is active, paused or done. Only a project takes it; a
	// project with no status is active.
	Status    string
	Contested bool
	// BaseHash is the hash of the version that the caller read. It is
	// empty for a new page.
	BaseHash string
	// Actor is who writes the page. It goes into the event.
	Actor Source
}

// wikiRef is the value of a wiki event. The text of the page is not in the
// event: the git history of the wiki has it.
type wikiRef struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// WikiWrite creates one page or replaces its content, and returns the
// stored page. A page that exists takes a write only with the hash of its
// stored version, so that one session does not overwrite the change of a
// different session. The write sets the two dates, keeps index.md and
// makes one commit in the git repository of the wiki. A write of the
// content that the page has already is not a write.
func (s *Service) WikiWrite(ctx context.Context, p WikiWriteParams) (WikiPage, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return WikiPage{}, err
	}
	switch p.Path {
	case wikiIndexPath:
		return WikiPage{}, invalid("path", "must not be index.md: LifeOS writes the index")
	case wikiLogPath:
		return WikiPage{}, invalid("path", "must not be log.md: the log takes a new entry at its end and no other change")
	}
	pageType, err := wikiPageType(p.Path)
	if err != nil {
		return WikiPage{}, err
	}
	page := WikiPage{
		Path:      p.Path,
		Type:      pageType,
		Title:     strings.TrimSpace(p.Title),
		Summary:   strings.TrimSpace(p.Summary),
		Status:    p.Status,
		Contested: p.Contested,
		Body:      strings.Trim(p.Body, "\n"),
	}
	if err := page.check(); err != nil {
		return WikiPage{}, err
	}

	s.wikiMu.Lock()
	defer s.wikiMu.Unlock()
	root, err := s.openWiki()
	if err != nil {
		return WikiPage{}, err
	}
	defer root.Close()

	oldRaw, err := root.ReadFile(p.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return WikiPage{}, fmt.Errorf("wiki: read %s: %w", p.Path, err)
	}
	day := today(s.now()).Format(time.DateOnly)
	page.Created, page.Updated = day, day
	action := "create"
	oldBody := ""
	if oldRaw != nil {
		action = "update"
		switch p.BaseHash {
		case hashHex(oldRaw):
		case "":
			return WikiPage{}, invalid("base_hash", "the page exists: read it, then send its hash with your change")
		default:
			return WikiPage{}, invalid("base_hash", "is not the hash of the stored page: the page changed after you read it. Read it again, put your change into the new text, then write with the new hash")
		}
		// A file that a person broke by hand has no dates to keep.
		if old, err := parseWikiFile(p.Path, oldRaw); err == nil {
			if old.sameContent(page) {
				return old, nil
			}
			page.Created, oldBody = old.Created, old.Body
		}
	} else if p.BaseHash != "" {
		return WikiPage{}, invalid("base_hash", "no page has this path: leave base_hash out to create the page")
	}
	if err := checkNewCitations(ctx, store.New(s.pool), "body", oldBody, page.Body); err != nil {
		return WikiPage{}, err
	}

	newRaw, err := composeWikiFile(page)
	if err != nil {
		return WikiPage{}, err
	}
	if err := s.changeWikiFile(ctx, root, action, p.Actor, p.Path, oldRaw, newRaw); err != nil {
		return WikiPage{}, err
	}
	page.Hash = hashHex(newRaw)
	return page, nil
}

// check refuses a page whose fields the wiki cannot take. It sets the
// status of a project and the period of a review.
func (page *WikiPage) check() error {
	for _, f := range []struct {
		field, value string
		max          int
	}{{"title", page.Title, maxWikiTitleRunes}, {"summary", page.Summary, maxWikiSummaryRunes}} {
		if f.value == "" {
			return invalid(f.field, "must not be empty")
		}
		if strings.ContainsAny(f.value, "\r\n") {
			return invalid(f.field, "must be one line")
		}
		if utf8.RuneCountInString(f.value) > f.max {
			return invalid(f.field, fmt.Sprintf("must be at most %d characters", f.max))
		}
		if err := checkText(f.field, f.value); err != nil {
			return err
		}
	}
	if strings.TrimSpace(page.Body) == "" {
		return invalid("body", "must not be empty")
	}
	if len(page.Body) > maxWikiBodyBytes {
		return invalid("body", "must be at most "+sizeText(maxWikiBodyBytes)+": split the page")
	}
	if err := checkText("body", page.Body); err != nil {
		return err
	}
	switch page.Type {
	case "project":
		if page.Status == "" {
			page.Status = "active"
		}
		if !slices.Contains(wikiStatuses, page.Status) {
			return invalid("status", "must be one of: "+strings.Join(wikiStatuses, ", "))
		}
	default:
		if page.Status != "" {
			return invalid("status", "must be empty: only a project has a status")
		}
	}
	if m := wikiReviewPattern.FindStringSubmatch(page.Path); m != nil {
		page.Period = m[1]
	}
	return nil
}

// sameContent reports whether next has the content of page. The dates and
// the hash do not count: they follow the content.
func (page WikiPage) sameContent(next WikiPage) bool {
	next.Created, next.Updated, next.Hash = page.Created, page.Updated, page.Hash
	return page == next
}

// composeWikiFile returns the bytes of the file of page: the frontmatter,
// an empty line and the body.
func composeWikiFile(page WikiPage) ([]byte, error) {
	head, err := yaml.Marshal(wikiHeader{
		Type:      page.Type,
		Title:     page.Title,
		Summary:   page.Summary,
		Created:   page.Created,
		Updated:   page.Updated,
		Status:    page.Status,
		Period:    page.Period,
		Contested: page.Contested,
	})
	if err != nil {
		return nil, fmt.Errorf("wiki: frontmatter of %s: %w", page.Path, err)
	}
	return []byte("---\n" + string(head) + "---\n\n" + page.Body + "\n"), nil
}

// changeWikiFile puts newRaw into the file at path, or removes the file
// when newRaw is nil. oldRaw is the content before the change, nil for a
// new file. It also writes index.md again, appends the event and makes the
// commit. When one of these fails, the files are as they were before.
//
// The caller holds wikiMu.
func (s *Service) changeWikiFile(ctx context.Context, root *os.Root, action string, actor Source, filePath string, oldRaw, newRaw []byte) (err error) {
	oldIndex, indexErr := root.ReadFile(wikiIndexPath)
	if indexErr != nil && !errors.Is(indexErr, fs.ErrNotExist) {
		return fmt.Errorf("wiki: read the index: %w", indexErr)
	}
	// A client that goes away must not stop git in the middle of a commit.
	ctx = context.WithoutCancel(ctx)
	paths := []string{filePath, wikiIndexPath}
	defer func() {
		if err == nil {
			return
		}
		// Each step is done on the files that are there: an error of a
		// step here has no better answer than the first error.
		s.wikiGit(ctx, append([]string{"reset", "-q", "--"}, paths...)...)
		putWikiFile(root, filePath, oldRaw)
		putWikiFile(root, wikiIndexPath, oldIndex)
	}()

	if err := putWikiFile(root, filePath, newRaw); err != nil {
		return fmt.Errorf("wiki: write %s: %w", filePath, err)
	}
	if err := s.writeWikiIndex(root, oldIndex); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wiki: %w", err)
	}
	defer tx.Rollback(ctx)
	var oldRef, newRef any
	if oldRaw != nil {
		oldRef = wikiRef{Path: filePath, Hash: hashHex(oldRaw)}
	}
	if newRaw != nil {
		newRef = wikiRef{Path: filePath, Hash: hashHex(newRaw)}
	}
	if err := recordWikiEvent(ctx, store.New(tx), action, actor, oldRef, newRef); err != nil {
		return fmt.Errorf("wiki: %w", err)
	}
	if err := s.wikiGit(ctx, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return err
	}
	if err := s.wikiGit(ctx, append([]string{"commit", "-q", "-m", "wiki: " + action + " " + filePath, "--"}, paths...)...); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("wiki: %w", err)
	}
	return nil
}

// putWikiFile writes raw to the file at filePath, or removes the file when
// raw is nil. A new file gets its name only when all its bytes are there.
func putWikiFile(root *os.Root, filePath string, raw []byte) error {
	if raw == nil {
		if err := root.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if folder := path.Dir(filePath); folder != "." {
		if err := root.Mkdir(folder, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	tmp := filePath + ".tmp"
	if err := root.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := root.Rename(tmp, filePath); err != nil {
		root.Remove(tmp)
		return err
	}
	return nil
}

// wikiGit runs git in the wiki folder. It does not read the git settings
// of the user or of the system, so that a commit is the same on each
// computer: no signature and no editor.
func (s *Service) wikiGit(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.wiki.Dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=LifeOS",
		"GIT_AUTHOR_EMAIL=lifeos@localhost",
		"GIT_COMMITTER_NAME=LifeOS",
		"GIT_COMMITTER_EMAIL=lifeos@localhost",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wiki: git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

// wikiBadFile is a file in a page folder that is not a valid page.
type wikiBadFile struct {
	Path   string
	Reason string
}

// listWikiPages reads each page of the wiki, in the order of the path. A
// file that is not a valid page is in bad.
func listWikiPages(root *os.Root) (pages []WikiPage, bad []wikiBadFile, err error) {
	for _, folder := range []string{"people", "projects", "reviews", "topics"} {
		entries, err := fs.ReadDir(root.FS(), folder)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("wiki: list %s: %w", folder, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			filePath := folder + "/" + entry.Name()
			if _, err := wikiPageType(filePath); err != nil {
				bad = append(bad, wikiBadFile{Path: filePath, Reason: "the name is not the name of a page: use lowercase letters, digits and hyphens"})
				continue
			}
			page, err := readWikiFile(root, filePath)
			if err != nil {
				bad = append(bad, wikiBadFile{Path: filePath, Reason: err.Error()})
				continue
			}
			pages = append(pages, page)
		}
	}
	return pages, bad, nil
}

// wikiIndexSections gives the heading of each page type, in the order of
// the index.
var wikiIndexSections = []struct{ heading, pageType string }{
	{"People", "person"},
	{"Projects", "project"},
	{"Topics", "topic"},
	{"Reviews", "review"},
}

// wikiIndexBody returns the body of index.md: one line for each page,
// below the heading of its type, in the order of the title.
func wikiIndexBody(pages []WikiPage) string {
	pages = slices.Clone(pages)
	slices.SortFunc(pages, func(a, b WikiPage) int {
		if c := strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	var b strings.Builder
	b.WriteString("# Index\n\nEach page of the wiki has one line here. LifeOS writes this file: do not change it.\n")
	for _, section := range wikiIndexSections {
		b.WriteString("\n## " + section.heading + "\n")
		first := true
		for _, page := range pages {
			if page.Type != section.pageType {
				continue
			}
			if first {
				b.WriteString("\n")
				first = false
			}
			b.WriteString("- [" + page.Title + "](" + page.Path + "): " + page.Summary + "\n")
		}
	}
	return strings.Trim(b.String(), "\n")
}

// writeWikiIndex writes index.md from the pages that are in the wiki now.
// It leaves the file alone when its lines are correct already. oldIndex is
// the content of the file, nil when there is none.
func (s *Service) writeWikiIndex(root *os.Root, oldIndex []byte) error {
	pages, _, err := listWikiPages(root)
	if err != nil {
		return err
	}
	day := today(s.now()).Format(time.DateOnly)
	index := WikiPage{
		Path:    wikiIndexPath,
		Type:    "index",
		Title:   "Index",
		Summary: "Each page of the wiki, one line each.",
		Created: day,
		Updated: day,
		Body:    wikiIndexBody(pages),
	}
	if oldIndex != nil {
		if old, err := parseWikiFile(wikiIndexPath, oldIndex); err == nil {
			if old.Body == index.Body {
				return nil
			}
			index.Created = old.Created
		}
	}
	raw, err := composeWikiFile(index)
	if err != nil {
		return err
	}
	if err := putWikiFile(root, wikiIndexPath, raw); err != nil {
		return fmt.Errorf("wiki: write the index: %w", err)
	}
	return nil
}
