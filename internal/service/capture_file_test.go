package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testMaxFileBytes is the file limit of the tests: small, so that a test
// can go over it.
const testMaxFileBytes = 4096

// pdf is the start of a PDF file. The service finds the type in the bytes.
var pdf = []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")

// newFileService returns a service with a file store and an inbox in new
// temporary folders.
func newFileService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	s, pool := newTestService(t)
	s.WithFiles(Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: testMaxFileBytes})
	return s, pool
}

// putInInbox writes one file into the inbox and returns its full path.
func putInInbox(t *testing.T, s *Service, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(s.files.Inbox, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make the folder of %s: %v", name, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// storedFiles returns the paths of the files in the file store, below Dir.
func storedFiles(t *testing.T, s *Service) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(s.files.Dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(s.files.Dir, path)
		paths = append(paths, rel)
		return err
	})
	if err != nil {
		t.Fatalf("walk the file store: %v", err)
	}
	return paths
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestCaptureMovesAFileFromTheInboxIntoTheFileStore(t *testing.T) {
	s, pool := newFileService(t)
	inboxPath := putInInbox(t, s, "policy.pdf", pdf)
	hash := hashHex(pdf)

	got, err := s.Capture(context.Background(), CaptureParams{InboxPath: "policy.pdf", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if got.Kind != "file" || got.Title != "policy.pdf" || got.MediaType != "application/pdf" {
		t.Errorf("kind, title, media_type = %q, %q, %q", got.Kind, got.Title, got.MediaType)
	}
	if got.SizeBytes != int64(len(pdf)) || got.SHA256 != hash {
		t.Errorf("size_bytes, sha256 = %d, %q", got.SizeBytes, got.SHA256)
	}
	if want := hash[:2] + "/" + hash; got.FilePath != want {
		t.Errorf("file_path = %q, want %q", got.FilePath, want)
	}
	if got.Content != "" || got.URL != "" {
		t.Errorf("content, url = %q, %q, want them empty", got.Content, got.URL)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want agent, true", got.Source, got.Verified)
	}

	stored, err := os.ReadFile(filepath.Join(s.files.Dir, got.FilePath))
	if err != nil {
		t.Fatalf("read the stored file: %v", err)
	}
	if !bytes.Equal(stored, pdf) {
		t.Error("the stored file does not have the bytes of the inbox file")
	}
	if exists(inboxPath) {
		t.Error("the file is still in the inbox")
	}
	if files := storedFiles(t, s); len(files) != 1 {
		t.Errorf("files in the file store = %v, want only the stored file", files)
	}
	events := allEvents(t, pool)
	if len(events) != 1 || events[0].Action != "create" || events[0].RecordType != "sources" {
		t.Errorf("events = %+v, want one create event for sources", events)
	}
}

func TestCaptureAcceptsAFullPathAndAFolderInTheInbox(t *testing.T) {
	s, _ := newFileService(t)
	ctx := context.Background()

	full := putInInbox(t, s, "a.pdf", pdf)
	got, err := s.Capture(ctx, CaptureParams{InboxPath: full, Title: "Policy 2026", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture with a full path: %v", err)
	}
	if got.Title != "Policy 2026" {
		t.Errorf("title = %q, want the title of the caller", got.Title)
	}

	putInInbox(t, s, "scans/b.bin", []byte{0, 1, 2, 3})
	got, err = s.Capture(ctx, CaptureParams{InboxPath: "scans/b.bin", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture of a file in a folder: %v", err)
	}
	if got.Title != "b.bin" || got.MediaType != "application/octet-stream" {
		t.Errorf("title, media_type = %q, %q", got.Title, got.MediaType)
	}
}

func TestCaptureStoresAnUpload(t *testing.T) {
	s, _ := newFileService(t)

	got, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(pdf), FileName: "receipt.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if got.Kind != "file" || got.Title != "receipt.pdf" || got.MediaType != "application/pdf" || got.Source != SourceManual {
		t.Errorf("capture = %+v", got)
	}
	if !exists(filepath.Join(s.files.Dir, got.FilePath)) {
		t.Error("the upload is not in the file store")
	}

	// An upload can come without a name.
	noName, err := s.Capture(context.Background(), CaptureParams{Upload: strings.NewReader("\x89PNG\r\n\x1a\n...."), Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture without a name: %v", err)
	}
	if noName.Title != "" || noName.MediaType != "image/png" {
		t.Errorf("title, media_type = %q, %q", noName.Title, noName.MediaType)
	}
}

// The same file captured two times is one source and one stored file (E1).
func TestCaptureOfTheSameFileReturnsTheStoredSource(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()

	first, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), FileName: "receipt.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("first Capture: %v", err)
	}
	inboxPath := putInInbox(t, s, "the-same.pdf", pdf)
	again, err := s.Capture(ctx, CaptureParams{InboxPath: "the-same.pdf", Source: SourceAgent})
	if err != nil {
		t.Fatalf("second Capture: %v", err)
	}

	if again.ID != first.ID || again.Title != "receipt.pdf" {
		t.Errorf("second capture = %+v, want the first source as it was", again)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	if files := storedFiles(t, s); len(files) != 1 {
		t.Errorf("files in the file store = %v, want 1", files)
	}
	// The stored file has the bytes, so the inbox file can go.
	if exists(inboxPath) {
		t.Error("the file of the second capture is still in the inbox")
	}
}

func TestCaptureRejectsABadFile(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()

	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.files.Inbox, "link.pdf")); err != nil {
		t.Fatal(err)
	}
	putInInbox(t, s, "folder/a.pdf", pdf)
	putInInbox(t, s, "empty.pdf", nil)
	putInInbox(t, s, "big.bin", bytes.Repeat([]byte{1}, testMaxFileBytes+1))
	putInInbox(t, s, "card.txt", []byte("my card\n"+testCard+"\n"))
	putInInbox(t, s, "ok.pdf", pdf)
	big := bytes.Repeat([]byte{1}, testMaxFileBytes+1)

	cases := []struct {
		name  string
		p     CaptureParams
		field string
	}{
		{"a path above the inbox", CaptureParams{InboxPath: "../outside.pdf", Source: SourceAgent}, "path"},
		{"a full path that is not in the inbox", CaptureParams{InboxPath: outside, Source: SourceAgent}, "path"},
		{"a link in the inbox to a file outside", CaptureParams{InboxPath: "link.pdf", Source: SourceAgent}, "path"},
		{"a file that does not exist", CaptureParams{InboxPath: "missing.pdf", Source: SourceAgent}, "path"},
		{"a folder", CaptureParams{InboxPath: "folder", Source: SourceAgent}, "path"},
		{"an empty file", CaptureParams{InboxPath: "empty.pdf", Source: SourceAgent}, "path"},
		{"a file over the limit", CaptureParams{InboxPath: "big.bin", Source: SourceAgent}, "path"},
		{"a text file with a card number", CaptureParams{InboxPath: "card.txt", Source: SourceAgent}, "path"},
		{"an empty upload", CaptureParams{Upload: strings.NewReader(""), Source: SourceManual}, "file"},
		{"an upload over the limit", CaptureParams{Upload: bytes.NewReader(big), Source: SourceManual}, "file"},
		{"an upload with a card number", CaptureParams{Upload: strings.NewReader("card " + testCard), Source: SourceManual}, "file"},
		{"an upload with a name that is too long", CaptureParams{Upload: bytes.NewReader(pdf), FileName: strings.Repeat("a", maxTitleLength+1), Source: SourceManual}, "title"},
		{"an upload with a card number in its name", CaptureParams{Upload: bytes.NewReader(pdf), FileName: testCard + ".pdf", Source: SourceManual}, "title"},
		{"a file and a text", CaptureParams{InboxPath: "ok.pdf", Text: "a note", Source: SourceAgent}, "text"},
		{"a file and an upload", CaptureParams{InboxPath: "ok.pdf", Upload: bytes.NewReader(pdf), Source: SourceAgent}, "text"},
	}
	for _, c := range cases {
		_, err := s.Capture(ctx, c.p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v, want a ValidationError", c.name, err)
			continue
		}
		if invalid.Field != c.field {
			t.Errorf("%s: field = %q, want %q", c.name, invalid.Field, c.field)
		}
	}

	if n := countRows(t, pool, "sources") + countRows(t, pool, "events"); n != 0 {
		t.Errorf("rows in sources and events = %d, want 0", n)
	}
	if files := storedFiles(t, s); len(files) != 0 {
		t.Errorf("files in the file store = %v, want none: a refused file leaves no copy", files)
	}
	// A refused file stays in the inbox, so that Gautam can look at it.
	for _, name := range []string{"big.bin", "card.txt", "ok.pdf", "empty.pdf"} {
		if !exists(filepath.Join(s.files.Inbox, name)) {
			t.Errorf("%s is not in the inbox any more", name)
		}
	}
	if !exists(outside) {
		t.Error("the file outside the inbox is gone")
	}
}

// The limit is the largest size that Capture accepts.
func TestCaptureAcceptsAFileOfTheMaximumSize(t *testing.T) {
	s, _ := newFileService(t)
	content := bytes.Repeat([]byte{1}, testMaxFileBytes)

	got, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(content), Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if got.SizeBytes != testMaxFileBytes {
		t.Errorf("size_bytes = %d, want %d", got.SizeBytes, testMaxFileBytes)
	}
}

func TestCaptureOfAFileNeedsTheFileStore(t *testing.T) {
	s, _ := newTestService(t) // no WithFiles

	_, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(pdf), Source: SourceManual})
	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) {
		t.Errorf("error = %v, want an error that is not a ValidationError", err)
	}
}

func TestCaptureKeepsTheInboxFileWhenTheEventFails(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()
	inboxPath := putInInbox(t, s, "policy.pdf", pdf)
	if _, err := pool.Exec(ctx, "ALTER TABLE events RENAME TO events_gone"); err != nil {
		t.Fatalf("rename events: %v", err)
	}

	if _, err := s.Capture(ctx, CaptureParams{InboxPath: "policy.pdf", Source: SourceAgent}); err == nil {
		t.Fatal("Capture gave no error when the event could not be stored")
	}
	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources = %d, want 0", n)
	}
	if !exists(inboxPath) {
		t.Error("the inbox file is gone, and no source holds it")
	}
}
