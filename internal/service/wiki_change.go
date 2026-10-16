package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// WikiAppendParams is the input of WikiAppend.
type WikiAppendParams struct {
	// Path is the path of a page, or log.md.
	Path string
	// Text goes after the last line of the page.
	Text string
	// Actor is who adds the text. It goes into the event.
	Actor Source
}

// WikiAppend adds text at the end of one page or of the log, and returns
// the stored page. It needs no hash: an append does not remove the change
// of a different session. Text that starts with a heading comes after one
// empty line. The log is made when it is not there.
func (s *Service) WikiAppend(ctx context.Context, p WikiAppendParams) (WikiPage, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return WikiPage{}, err
	}
	if p.Path == wikiIndexPath {
		return WikiPage{}, invalid("path", "must not be index.md: LifeOS writes the index")
	}
	if _, err := wikiFileType(p.Path); err != nil {
		return WikiPage{}, err
	}
	text := strings.Trim(p.Text, "\n")
	if strings.TrimSpace(text) == "" {
		return WikiPage{}, invalid("text", "must not be empty")
	}
	if err := checkText("text", text); err != nil {
		return WikiPage{}, err
	}

	s.wikiMu.Lock()
	defer s.wikiMu.Unlock()
	root, err := s.openWiki()
	if err != nil {
		return WikiPage{}, err
	}
	defer root.Close()

	day := today(s.now()).Format(time.DateOnly)
	var page WikiPage
	oldRaw, err := root.ReadFile(p.Path)
	switch {
	case err == nil:
		if page, err = parseWikiFile(p.Path, oldRaw); err != nil {
			return WikiPage{}, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return WikiPage{}, fmt.Errorf("wiki: read %s: %w", p.Path, err)
	case p.Path == wikiLogPath:
		page = WikiPage{Path: wikiLogPath, Type: "log", Title: "Log", Summary: "Each ingest, saved answer and lint run, oldest first.", Created: day, Body: "# Log"}
	default:
		return WikiPage{}, ErrNotFound
	}

	if len(page.Body)+len(text) > maxWikiBodyBytes {
		return WikiPage{}, invalid("text", "makes the page larger than "+sizeText(maxWikiBodyBytes)+": split the page")
	}
	if err := checkNewCitations(ctx, store.New(s.pool), "text", page.Body, text); err != nil {
		return WikiPage{}, err
	}
	if err := checkStated(p.Path, "text", page.Body, text); err != nil {
		return WikiPage{}, err
	}
	separator := "\n"
	if strings.HasPrefix(text, "#") {
		separator = "\n\n"
	}
	if err := checkMeLength(p.Path, "text", page.Body+separator+text); err != nil {
		return WikiPage{}, err
	}
	page.Body += separator + text
	page.Updated = day

	newRaw, err := composeWikiFile(page)
	if err != nil {
		return WikiPage{}, err
	}
	if err := s.changeWikiFile(ctx, root, "append", p.Actor, p.Path, oldRaw, newRaw); err != nil {
		return WikiPage{}, err
	}
	page.Hash = hashHex(newRaw)
	return page, nil
}

// WikiDelete removes one page, writes index.md again and makes one commit.
// The service refuses a delete from an agent unless the call says that
// Gautam confirmed it. The git history of the wiki keeps the page, so a
// person can get it back.
func (s *Service) WikiDelete(ctx context.Context, path string, p DeleteParams) error {
	if err := p.check(); err != nil {
		return err
	}
	if path == wikiIndexPath || path == wikiLogPath {
		return invalid("path", "must be the path of a page: the index and the log stay")
	}
	if _, err := wikiPageType(path); err != nil {
		return err
	}

	s.wikiMu.Lock()
	defer s.wikiMu.Unlock()
	root, err := s.openWiki()
	if err != nil {
		return err
	}
	defer root.Close()

	oldRaw, err := root.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("wiki: read %s: %w", path, err)
	}
	return s.changeWikiFile(ctx, root, "delete", p.Actor, path, oldRaw, nil)
}
