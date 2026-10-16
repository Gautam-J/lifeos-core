package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Files says where the service keeps the files of file captures.
type Files struct {
	// Dir is the file store. A file is at Dir/<the first two characters
	// of its SHA-256>/<its SHA-256>, so the same bytes are one file.
	Dir string
	// Inbox is the folder where a file waits for its capture.
	Inbox string
	// MaxBytes is the largest file that Capture accepts.
	MaxBytes int64
}

// WithFiles sets the file store and returns s. Without it, Capture stores a
// text and a link, and it refuses a file.
func (s *Service) WithFiles(files Files) *Service {
	s.files = files
	return s
}

// MaxFileBytes returns the largest file that Capture accepts.
func (s *Service) MaxFileBytes() int64 {
	return s.files.MaxBytes
}

// captureFile stores the file of p in the file store and its row in
// sources. A file from the inbox leaves the inbox after the row is stored.
func (s *Service) captureFile(ctx context.Context, p CaptureParams) (Capture, error) {
	if err := (Provenance{Source: p.Source}).validate(); err != nil {
		return Capture{}, err
	}
	if s.files.Dir == "" {
		return Capture{}, errors.New("capture: the service has no file store")
	}

	field, name, reader := "file", p.FileName, p.Upload
	var inbox *os.Root
	var inboxPath string
	if p.InboxPath != "" {
		root, rel, file, err := s.openInboxFile(p.InboxPath)
		if err != nil {
			return Capture{}, err
		}
		defer root.Close()
		defer file.Close()
		field, name, reader = "path", filepath.Base(rel), file
		inbox, inboxPath = root, rel
	}

	// The file name is the title when the caller gives no title.
	title := p.Title
	if strings.TrimSpace(title) == "" {
		title = name
	}
	titleOrNil, err := captureTitle(title)
	if err != nil {
		return Capture{}, err
	}
	stored, err := s.storeFile(reader, field)
	if err != nil {
		return Capture{}, err
	}
	capture, err := s.storeSource(ctx, store.InsertSourceParams{
		Kind:      "file",
		Title:     titleOrNil,
		FilePath:  &stored.path,
		MediaType: &stored.mediaType,
		SizeBytes: &stored.size,
		Sha256:    stored.sha256,
		Source:    string(p.Source),
	})
	if err != nil {
		return Capture{}, err
	}
	// The file store has the bytes now. This is also true when the source
	// existed already. A second request for the same file can come here
	// after the first one removed it: that is not an error.
	if inbox != nil {
		if err := inbox.Remove(inboxPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Capture{}, fmt.Errorf("capture: remove the file from the inbox: %w", err)
		}
	}
	return capture, nil
}

// openInboxFile opens one file of the inbox. path is a path below the
// inbox, or a full path that is in the inbox. os.Root refuses a path that
// leaves the inbox, also through a symbolic link.
func (s *Service) openInboxFile(path string) (root *os.Root, rel string, file *os.File, err error) {
	rel = path
	if filepath.IsAbs(path) {
		rel, err = filepath.Rel(s.files.Inbox, path)
		if err != nil {
			return nil, "", nil, invalid("path", "must name a file in the inbox folder")
		}
	}
	root, err = os.OpenRoot(s.files.Inbox)
	if err != nil {
		return nil, "", nil, fmt.Errorf("capture: open the inbox: %w", err)
	}
	file, err = root.Open(rel)
	if err != nil {
		root.Close()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", nil, invalid("path", "no file has this path in the inbox folder")
		}
		return nil, "", nil, invalid("path", "must name a file in the inbox folder")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		root.Close()
		return nil, "", nil, invalid("path", "must name a file, not a folder")
	}
	return root, rel, file, nil
}

// storedFile is one file in the file store.
type storedFile struct {
	// path is the path below Files.Dir.
	path      string
	sha256    string
	mediaType string
	size      int64
}

// storeFile copies r into the file store and returns where the file is.
// field is the name of the input field, for an error. It refuses an empty
// file, a file over the limit and a text file that holds a secret; a
// refused file leaves no copy.
func (s *Service) storeFile(r io.Reader, field string) (storedFile, error) {
	tmp, err := os.CreateTemp(s.files.Dir, "incoming-*")
	if err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	// After the rename, this removes nothing.
	defer os.Remove(tmp.Name())

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(r, s.files.MaxBytes+1))
	// The bytes must be on the disk before the file gets its name: after
	// the capture, the file in the inbox is removed.
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	if size == 0 {
		return storedFile{}, invalid(field, "must not be an empty file")
	}
	if size > s.files.MaxBytes {
		return storedFile{}, invalid(field, "must be a file of at most "+sizeText(s.files.MaxBytes))
	}

	content, err := os.ReadFile(tmp.Name())
	if err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	mediaType := http.DetectContentType(content)
	// The service can read a text file, so the rule on secrets applies.
	if strings.HasPrefix(mediaType, "text/") {
		if kind := secretKind(string(content)); kind != "" {
			return storedFile{}, invalid(field, "must not hold "+kind+": LifeOS does not store one")
		}
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	stored := storedFile{path: sum[:2] + "/" + sum, sha256: sum, mediaType: mediaType, size: size}
	final := filepath.Join(s.files.Dir, stored.path)
	// The file store has these bytes already when a file of this size is
	// there. A file of a different size is what a power cut left: the new
	// file takes its place.
	if info, err := os.Stat(final); err == nil && info.Size() == size {
		return stored, nil
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	// A new name is safe on the disk only after a sync of its folder.
	for _, dir := range []string{filepath.Dir(final), s.files.Dir} {
		if err := syncDir(dir); err != nil {
			return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
		}
	}
	return stored, nil
}

// syncDir writes the entries of a folder to the disk.
func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// sizeText returns a number of bytes in words: "50 MB" or "4096 bytes".
func sizeText(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d bytes", n)
}
