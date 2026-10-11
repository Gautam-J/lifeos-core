package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	// maxCaptureTextBytes is the largest text of one capture: 1 MB.
	maxCaptureTextBytes = 1 << 20
	maxURLLength        = 2000
)

// Capture is one row of sources: a text, a link or a file, stored exactly
// as it came. Its record type is source.
type Capture struct {
	ID int64 `json:"id"`
	// Kind is text, link or file.
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	// Content is the text of a text capture.
	Content string `json:"content,omitempty"`
	// URL is the link of a link capture.
	URL string `json:"url,omitempty"`
	// FilePath is the path of a file capture, below the file store.
	FilePath  string `json:"file_path,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	// SHA256 is the hash of the text, the URL or the file bytes, in hex.
	SHA256 string `json:"sha256"`
	// ProcessedAt is nil until the inbox job reads the capture.
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	Source      Source     `json:"source"`
	SourceRef   *int64     `json:"source_ref,omitempty"`
	Verified    bool       `json:"verified"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CaptureParams is the input of Capture. Exactly one of Text and URL is
// set.
type CaptureParams struct {
	// Text is stored as it is. The service does not trim it.
	Text string
	// URL is a link with the scheme http or https. The service stores the
	// link; it does not get the page.
	URL string
	// Title is optional: a label for the capture.
	Title string
	// Source says who captures: manual for Gautam, agent for an MCP tool.
	Source Source
}

// Capture validates p, stores one source and appends a create event, in one
// transaction. The same content captured two times is one source: the
// second call returns the stored source and writes nothing. A source is
// always verified, because it holds what came in and no agent inferred it.
func (s *Service) Capture(ctx context.Context, p CaptureParams) (Capture, error) {
	row, err := p.toRow()
	if err != nil {
		return Capture{}, err
	}
	return s.storeSource(ctx, row)
}

// storeSource inserts row, or returns the live source that has the same
// kind and hash.
func (s *Service) storeSource(ctx context.Context, row store.InsertSourceParams) (Capture, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertSource(ctx, row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := q.GetSourceByHash(ctx, store.GetSourceByHashParams{Kind: row.Kind, Sha256: row.Sha256})
		if err != nil {
			return Capture{}, fmt.Errorf("capture: %w", err)
		}
		return captureFromRow(existing), nil
	}
	if err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	capture := captureFromRow(stored)
	if err := recordEvent(ctx, q, "create", "sources", capture.ID, Source(row.Source), nil, capture); err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	return capture, nil
}

// toRow checks every field and returns the row to insert.
func (p CaptureParams) toRow() (store.InsertSourceParams, error) {
	row := store.InsertSourceParams{Source: string(p.Source), Verified: true}

	link := strings.TrimSpace(p.URL)
	if (p.Text == "") == (link == "") {
		return row, invalid("text", "give a text or a url, and not the two together")
	}
	title, err := captureTitle(p.Title)
	if err != nil {
		return row, err
	}
	row.Title = title
	if err := (Provenance{Source: p.Source}).validate(); err != nil {
		return row, err
	}

	if link != "" {
		if err := checkLink(link); err != nil {
			return row, err
		}
		row.Kind, row.Url, row.Sha256 = "link", &link, hashHex([]byte(link))
		return row, nil
	}

	if strings.TrimSpace(p.Text) == "" {
		return row, invalid("text", "must not be empty")
	}
	if len(p.Text) > maxCaptureTextBytes {
		return row, invalid("text", "must be at most 1 MB")
	}
	if err := checkText("text", p.Text); err != nil {
		return row, err
	}
	text := p.Text
	row.Kind, row.Content, row.Sha256 = "text", &text, hashHex([]byte(text))
	return row, nil
}

// captureTitle returns the optional title as the table holds it: nil when
// the caller gave none.
func captureTitle(title string) (*string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return nil, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLength))
	}
	if err := checkText("title", title); err != nil {
		return nil, err
	}
	return &title, nil
}

// checkLink accepts an absolute http or https URL.
func checkLink(link string) error {
	if utf8.RuneCountInString(link) > maxURLLength {
		return invalid("url", fmt.Sprintf("must be at most %d characters", maxURLLength))
	}
	if err := checkText("url", link); err != nil {
		return err
	}
	parsed, err := url.Parse(link)
	if err != nil || strings.ContainsAny(link, " \t\r\n") {
		return invalid("url", "must be a link, such as https://example.com/page")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return invalid("url", "must start with http:// or https:// and name a host")
	}
	// https://name:password@host has a password in it (AGENTS.md rule 7).
	if parsed.User != nil {
		return invalid("url", "must not hold a user name or a password")
	}
	return nil
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func captureFromRow(row store.Source) Capture {
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	c := Capture{
		ID:        row.ID,
		Kind:      row.Kind,
		Title:     text(row.Title),
		Content:   text(row.Content),
		URL:       text(row.Url),
		FilePath:  text(row.FilePath),
		MediaType: text(row.MediaType),
		SHA256:    row.Sha256,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.SizeBytes != nil {
		c.SizeBytes = *row.SizeBytes
	}
	if row.ProcessedAt != nil {
		at := row.ProcessedAt.UTC()
		c.ProcessedAt = &at
	}
	return c
}

// GetCapture returns the source with this ID. It returns ErrNotFound when
// there is no such source or the source is deleted.
func (s *Service) GetCapture(ctx context.Context, id int64) (Capture, error) {
	row, err := store.New(s.pool).GetSource(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Capture{}, ErrNotFound
	}
	if err != nil {
		return Capture{}, fmt.Errorf("get source: %w", err)
	}
	return captureFromRow(row), nil
}

// ListCaptures returns the sources that are not deleted, the newest first.
// limit 0 means 50; the maximum is 200.
func (s *Service) ListCaptures(ctx context.Context, limit int) ([]Capture, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListSources(ctx, maxRows)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	captures := make([]Capture, 0, len(rows))
	for _, row := range rows {
		captures = append(captures, captureFromRow(row))
	}
	return captures, nil
}
