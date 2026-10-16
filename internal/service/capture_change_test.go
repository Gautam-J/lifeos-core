package service

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestDeleteCaptureKeepsTheRowAndTheFile(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()
	added, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), FileName: "policy.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// An agent deletes only after Gautam said yes.
	err = s.DeleteCapture(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent without a yes: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteCapture(ctx, added.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Fatalf("DeleteCapture: %v", err)
	}
	if _, err := s.GetCapture(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCapture after the delete: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1: a delete keeps the row", n)
	}
	// Only the admin pages remove a record for good, so the file stays.
	if !exists(filepath.Join(s.files.Dir, added.FilePath)) {
		t.Error("the stored file is gone after a soft delete")
	}
	events := allEvents(t, pool)
	if e := events[len(events)-1]; e.Action != "delete" || e.RecordType != "sources" || e.RecordID != added.ID || e.Source != "agent" || e.Old["title"] != "policy.pdf" || e.New != nil {
		t.Errorf("event = %+v, want a delete of the source with the old row", e)
	}
	if err := s.DeleteCapture(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
	if all, _ := s.ListCaptures(ctx, 0); len(all) != 0 {
		t.Errorf("ListCaptures = %+v, want no source", all)
	}
}

// A deleted source does not block a new capture of the same content.
func TestCaptureAfterADeleteStoresANewSource(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()

	text := captureText(t, s, "a note")
	file, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	for _, id := range []int64{text.ID, file.ID} {
		if err := s.DeleteCapture(ctx, id, DeleteParams{Actor: SourceManual}); err != nil {
			t.Fatalf("DeleteCapture(%d): %v", id, err)
		}
	}

	textAgain := captureText(t, s, "a note")
	fileAgain, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), Source: SourceManual})
	if err != nil {
		t.Fatalf("second Capture of the file: %v", err)
	}
	if textAgain.ID == text.ID || fileAgain.ID == file.ID {
		t.Errorf("new IDs = %d, %d, want IDs that differ from %d, %d", textAgain.ID, fileAgain.ID, text.ID, file.ID)
	}
	if fileAgain.FilePath != file.FilePath {
		t.Errorf("file_path = %q, want %q: the same bytes are one stored file", fileAgain.FilePath, file.FilePath)
	}
	if n := countRows(t, pool, "sources"); n != 4 {
		t.Errorf("sources has %d rows, want 4 (two deleted, two live)", n)
	}
	if files := storedFiles(t, s); len(files) != 1 {
		t.Errorf("files in the file store = %v, want 1", files)
	}
}

func TestRecordFunctionsWorkOnASource(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := captureText(t, s, "a note")

	got, err := s.Get(ctx, "source", added.ID)
	if capture, ok := got.(Capture); err != nil || !ok || capture.ID != added.ID || capture.Content != "a note" {
		t.Fatalf("Get = %#v, %v, want the source", got, err)
	}

	// A capture is verbatim, so it has no field to correct.
	_, err = s.Update(ctx, "source", added.ID, UpdateParams{Fields: map[string]any{"content": "a new note"}, Actor: SourceManual})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "type" {
		t.Errorf("Update: error = %v, want a ValidationError on type", err)
	}

	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want only the create event", n)
	}

	if err := s.Delete(ctx, "source", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "source", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}
