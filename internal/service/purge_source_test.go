package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureUpload stores pdf as a file capture and returns the source.
func captureUpload(t *testing.T, s *Service, name string) Capture {
	t.Helper()
	capture, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(pdf), FileName: name, Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture %s: %v", name, err)
	}
	return capture
}

func TestPurgeRemovesADeletedSourceAndItsFile(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	if got := storedFiles(t, s); len(got) != 1 {
		t.Fatalf("the file store has %v after the delete, want 1 file: a delete keeps the file", got)
	}

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
	if got := storedFiles(t, s); len(got) != 0 {
		t.Errorf("the file store has %v, want no file", got)
	}
	for _, e := range allEvents(t, pool) {
		if e.Old != nil || e.New != nil {
			t.Errorf("the %s event keeps a value: old %v, new %v", e.Action, e.Old, e.New)
		}
	}
}

func TestPurgeKeepsAFileThatAnotherSourceUses(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()
	first := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", first.ID)
	// The same bytes again: a new source that uses the same file.
	second := captureUpload(t, s, "policy-copy.pdf")
	if second.ID == first.ID {
		t.Fatalf("the second capture gave the deleted source %d", first.ID)
	}

	if err := s.Purge(ctx, "source", first.ID); err != nil {
		t.Fatalf("Purge the first source: %v", err)
	}
	if got := storedFiles(t, s); len(got) != 1 {
		t.Fatalf("the file store has %v, want the file of the second source", got)
	}

	// A deleted source also keeps the file for a later purge of its own.
	deleteByHand(t, s, "source", second.ID)
	third := captureUpload(t, s, "policy-third.pdf")
	if err := s.Purge(ctx, "source", second.ID); err != nil {
		t.Fatalf("Purge the second source: %v", err)
	}
	if got := storedFiles(t, s); len(got) != 1 {
		t.Errorf("the file store has %v, want the file of the third source", got)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1", n)
	}
	if _, err := s.GetCapture(ctx, third.ID); err != nil {
		t.Errorf("GetCapture of the third source: %v", err)
	}
}

func TestPurgeRemovesATextSourceFromTheTableAndFromTheEvents(t *testing.T) {
	s, pool := newTestService(t)
	source := captureText(t, s, "a private note that must go")
	deleteByHand(t, s, "source", source.ID)

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
	var left int
	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE old_value::text LIKE '%private note%' OR new_value::text LIKE '%private note%'").Scan(&left)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if left != 0 {
		t.Errorf("%d events keep the text of the source", left)
	}
}

func TestPurgeRefusesASourceThatARecordNames(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	source := captureText(t, s, "Renew the policy before March.")
	todo, err := s.AddTodo(ctx, AddTodoParams{Title: "Renew the policy", Provenance: Provenance{Source: SourceAgent, SourceRef: &source.ID}})
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	deleteByHand(t, s, "source", source.ID)

	err = s.Purge(ctx, "source", source.ID)

	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "id" {
		t.Fatalf("error = %v, want a ValidationError for id", err)
	}
	if !strings.Contains(invalid.Message, "todos") {
		t.Errorf("the message %q does not name the table todos", invalid.Message)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1: a refused purge removes nothing", n)
	}
	if events := allEvents(t, pool); events[0].New["content"] != "Renew the policy before March." {
		t.Errorf("a refused purge emptied the create event: %v", events[0].New)
	}

	// After the record is gone, the source can go.
	deleteByHand(t, s, "todo", todo.ID)
	if err := s.Purge(ctx, "todo", todo.ID); err != nil {
		t.Fatalf("Purge the todo: %v", err)
	}
	if err := s.Purge(ctx, "source", source.ID); err != nil {
		t.Errorf("Purge the source after the todo: %v", err)
	}
}

func TestPurgeOfASourceWhoseFileIsGoneAlreadyRemovesTheRow(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	for _, path := range storedFiles(t, s) {
		if err := os.Remove(filepath.Join(s.files.Dir, path)); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
}

func TestPurgeKeepsTheRowWhenTheFileCannotBeRemoved(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	// The service loses its file store, as after a wrong config change.
	dir := s.files.Dir
	s.files.Dir = filepath.Join(dir, "no-such-folder")

	err := s.Purge(context.Background(), "source", source.ID)

	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) || errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want an internal error", err)
	}
	if !strings.Contains(err.Error(), "remove the file") {
		t.Errorf("error = %v, want one that says that the file was not removed", err)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1: the row stays when the file stays", n)
	}
	s.files.Dir = dir
	if got := storedFiles(t, s); len(got) != 1 {
		t.Errorf("the file store has %v, want the file", got)
	}
}

// The file store keeps a file by its content. After a purge took the file
// away, the same file can come in again.
func TestCaptureAfterAPurgeStoresTheFileAgain(t *testing.T) {
	s, _ := newFileService(t)
	first := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", first.ID)
	if err := s.Purge(context.Background(), "source", first.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	second := captureUpload(t, s, "policy.pdf")

	if second.ID == first.ID {
		t.Errorf("the capture gave the removed source %d", first.ID)
	}
	if got := storedFiles(t, s); len(got) != 1 {
		t.Errorf("the file store has %v, want the file of the new source", got)
	}
}

// After a restore of the database without the file store, the folder of
// the file is not there.
func TestPurgeOfASourceWhoseFolderIsGoneAlreadyRemovesTheRow(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	for _, path := range storedFiles(t, s) {
		if err := os.RemoveAll(filepath.Join(s.files.Dir, filepath.Dir(path))); err != nil {
			t.Fatalf("remove the folder of %s: %v", path, err)
		}
	}

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
}
