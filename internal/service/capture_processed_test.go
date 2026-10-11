package service

import (
	"context"
	"errors"
	"testing"
)

func TestMarkProcessedSetsTheTimeAndNothingElse(t *testing.T) {
	s, pool := newTestService(t)
	source := captureText(t, s, "An article about agents.")
	if source.ProcessedAt != nil {
		t.Fatalf("a new source has processed_at = %v, want none", source.ProcessedAt)
	}

	marked, err := s.MarkProcessed(context.Background(), source.ID, SourceAgent)
	if err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}
	if marked.ProcessedAt == nil {
		t.Fatal("processed_at is not set")
	}
	if marked.Content != source.Content || marked.SHA256 != source.SHA256 || !marked.Verified || marked.Source != source.Source {
		t.Errorf("the source changed: %+v", marked)
	}
	stored, err := s.GetCapture(context.Background(), source.ID)
	if err != nil || stored.ProcessedAt == nil || !stored.ProcessedAt.Equal(*marked.ProcessedAt) {
		t.Errorf("stored processed_at = %v, error %v, want %v", stored.ProcessedAt, err, marked.ProcessedAt)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("%d events, want 2", len(events))
	}
	e := events[1]
	if e.Action != "process" || e.RecordType != "sources" || e.RecordID != source.ID || e.Source != "agent" {
		t.Errorf("event = %+v, want a process event of the source by agent", e)
	}
	if e.Old["processed_at"] != nil || e.New["processed_at"] == nil {
		t.Errorf("event values: old processed_at = %v, new = %v, want none and a time", e.Old["processed_at"], e.New["processed_at"])
	}
}

func TestMarkProcessedASecondTimeIsNotAWrite(t *testing.T) {
	s, pool := newTestService(t)
	source := captureText(t, s, "An article about agents.")
	first, err := s.MarkProcessed(context.Background(), source.ID, SourceAgent)
	if err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	second, err := s.MarkProcessed(context.Background(), source.ID, SourceManual)
	if err != nil {
		t.Fatalf("MarkProcessed a second time: %v", err)
	}
	if !second.ProcessedAt.Equal(*first.ProcessedAt) || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Errorf("the second call changed the source: %+v", second)
	}
	if n := len(allEvents(t, pool)); n != 2 {
		t.Errorf("%d events, want 2", n)
	}
}

func TestMarkProcessedRefuses(t *testing.T) {
	s, pool := newTestService(t)
	source := captureText(t, s, "An article about agents.")
	gone := captureText(t, s, "A note to delete.")
	if err := s.DeleteCapture(context.Background(), gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteCapture: %v", err)
	}
	before := len(allEvents(t, pool))

	for name, id := range map[string]int64{"an ID of no source": 999, "a deleted source": gone.ID} {
		if _, err := s.MarkProcessed(context.Background(), id, SourceAgent); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
	_, err := s.MarkProcessed(context.Background(), source.ID, "robot")
	wantInvalid(t, err, "source")

	if n := len(allEvents(t, pool)); n != before {
		t.Errorf("%d events, want %d", n, before)
	}
}
