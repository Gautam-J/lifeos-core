package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// captureText stores one text as the REST API does.
func captureText(t *testing.T, s *Service, text string) Capture {
	t.Helper()
	capture, err := s.Capture(context.Background(), CaptureParams{Text: text, Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture(%q): %v", text, err)
	}
	return capture
}

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// A capture is verbatim. The service must not trim or change the text.
func TestCaptureStoresATextExactlyAsItCame(t *testing.T) {
	s, pool := newTestService(t)
	const text = "  Renewal on 12 March.\n\nCall the agent first.  \n"

	got, err := s.Capture(context.Background(), CaptureParams{Text: text, Title: "  Insurance note ", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if got.ID == 0 {
		t.Error("Capture returned ID 0")
	}
	if got.Kind != "text" || got.Content != text || got.Title != "Insurance note" {
		t.Errorf("kind, content, title = %q, %q, %q", got.Kind, got.Content, got.Title)
	}
	if got.SHA256 != hashOf(text) {
		t.Errorf("sha256 = %q, want the hash of the text as it came", got.SHA256)
	}
	if got.URL != "" || got.FilePath != "" || got.ProcessedAt != nil {
		t.Errorf("url, file_path, processed_at = %q, %q, %v, want them empty", got.URL, got.FilePath, got.ProcessedAt)
	}
	// A capture is what came in, not a fact that an agent inferred.
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want agent, true", got.Source, got.Verified)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	stored, err := s.GetCapture(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("GetCapture: %v", err)
	}
	if stored.Content != text {
		t.Errorf("stored content = %q, want %q", stored.Content, text)
	}
}

func TestCaptureStoresALink(t *testing.T) {
	s, _ := newTestService(t)
	const link = "https://example.com/a?b=c#d"

	got, err := s.Capture(context.Background(), CaptureParams{URL: "  " + link + "\n", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if got.Kind != "link" || got.URL != link || got.Content != "" {
		t.Errorf("kind, url, content = %q, %q, %q", got.Kind, got.URL, got.Content)
	}
	if got.SHA256 != hashOf(link) {
		t.Errorf("sha256 = %q, want the hash of the URL", got.SHA256)
	}
	if got.Source != SourceManual || !got.Verified {
		t.Errorf("source, verified = %q, %v, want manual, true", got.Source, got.Verified)
	}
}

func TestCaptureAppendsACreateEvent(t *testing.T) {
	s, pool := newTestService(t)
	capture := captureText(t, s, "a note")

	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.RecordType != "sources" || e.RecordID != capture.ID || e.Source != "manual" {
		t.Errorf("event = %+v", e)
	}
	if e.Old != nil || e.New["content"] != "a note" || e.New["kind"] != "text" {
		t.Errorf("old, new = %v, %v", e.Old, e.New)
	}
}

// The same thing captured two times is one source (E1 in the design doc).
func TestCaptureOfTheSameContentReturnsTheStoredSource(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	first := captureText(t, s, "a note")

	again, err := s.Capture(ctx, CaptureParams{Text: "a note", Title: "a new title", Source: SourceAgent})
	if err != nil {
		t.Fatalf("second Capture: %v", err)
	}
	if again.ID != first.ID || again.Title != "" || again.Source != SourceManual {
		t.Errorf("second capture = %+v, want the first source as it was", again)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want only the first create event", n)
	}

	// A text that differs in one space is a different text.
	if other := captureText(t, s, "a note "); other.ID == first.ID {
		t.Error("a different text got the ID of the first source")
	}
	// A link and a text with the same characters are two sources.
	link, err := s.Capture(ctx, CaptureParams{URL: "https://example.com", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture of the link: %v", err)
	}
	if text := captureText(t, s, "https://example.com"); text.ID == link.ID {
		t.Error("a text got the ID of a link")
	}
	if linkAgain, _ := s.Capture(ctx, CaptureParams{URL: "https://example.com", Source: SourceManual}); linkAgain.ID != link.ID {
		t.Errorf("second capture of the link has the ID %d, want %d", linkAgain.ID, link.ID)
	}
}

func TestCaptureRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		p     CaptureParams
		field string
	}{
		{"nothing to capture", CaptureParams{Source: SourceManual}, "text"},
		{"a text and a link", CaptureParams{Text: "a note", URL: "https://example.com", Source: SourceManual}, "text"},
		{"a text of only spaces", CaptureParams{Text: " \n\t ", Source: SourceManual}, "text"},
		{"a text with a NUL", CaptureParams{Text: "a\x00b", Source: SourceManual}, "text"},
		{"a text that is not UTF-8", CaptureParams{Text: "a\xffb", Source: SourceManual}, "text"},
		{"a text that is too long", CaptureParams{Text: strings.Repeat("a", maxCaptureTextBytes+1), Source: SourceManual}, "text"},
		{"a text with a card number", CaptureParams{Text: "card " + testCard, Source: SourceManual}, "text"},
		{"a link with no scheme", CaptureParams{URL: "example.com/a", Source: SourceManual}, "url"},
		{"a link to a file on the Pi", CaptureParams{URL: "file:///etc/passwd", Source: SourceManual}, "url"},
		{"a link with no host", CaptureParams{URL: "https:///a", Source: SourceManual}, "url"},
		{"a link with a space", CaptureParams{URL: "https://example.com/a b", Source: SourceManual}, "url"},
		{"a link that is too long", CaptureParams{URL: "https://example.com/" + strings.Repeat("a", maxURLLength), Source: SourceManual}, "url"},
		{"a link with a password", CaptureParams{URL: "https://gautam:hunter2@example.com/a", Source: SourceManual}, "url"},
		{"a link with a user name", CaptureParams{URL: "https://gautam@example.com/a", Source: SourceManual}, "url"},
		{"a link with a token", CaptureParams{URL: "https://example.com/?key=" + awsKey, Source: SourceManual}, "url"},
		{"a title that is too long", CaptureParams{Text: "a note", Title: strings.Repeat("a", maxTitleLength+1), Source: SourceManual}, "title"},
		{"a title with a card number", CaptureParams{Text: "a note", Title: testCard, Source: SourceManual}, "title"},
		{"an unknown source", CaptureParams{Text: "a note", Source: "robot"}, "source"},
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
}

func TestCaptureAcceptsATextOfTheMaximumLength(t *testing.T) {
	s, _ := newTestService(t)
	text := strings.Repeat("a", maxCaptureTextBytes)
	if got := captureText(t, s, text); len(got.Content) != maxCaptureTextBytes {
		t.Errorf("stored %d bytes, want %d", len(got.Content), maxCaptureTextBytes)
	}
}

func TestCaptureStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "ALTER TABLE events RENAME TO events_gone"); err != nil {
		t.Fatalf("rename events: %v", err)
	}

	if _, err := s.Capture(ctx, CaptureParams{Text: "a note", Source: SourceManual}); err == nil {
		t.Fatal("Capture gave no error when the event could not be stored")
	}
	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources = %d, want 0: the source and its event are one transaction", n)
	}
}

func TestGetCapture(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	if _, err := s.GetCapture(ctx, 12345); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCapture of an unknown ID: error = %v, want ErrNotFound", err)
	}
}

func TestListCapturesReturnsTheNewestFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	empty, err := s.ListCaptures(ctx, 0)
	if err != nil {
		t.Fatalf("ListCaptures: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListCaptures with no source = %v, want an empty list, not nil", empty)
	}

	captureText(t, s, "first")
	captureText(t, s, "second")
	captureText(t, s, "third")

	all, err := s.ListCaptures(ctx, 0)
	if err != nil {
		t.Fatalf("ListCaptures: %v", err)
	}
	var got []string
	for _, c := range all {
		got = append(got, c.Content)
	}
	if !equal(got, []string{"third", "second", "first"}) {
		t.Errorf("order = %v, want third, second, first", got)
	}
	two, err := s.ListCaptures(ctx, 2)
	if err != nil || len(two) != 2 {
		t.Errorf("ListCaptures(2) = %d sources, error %v", len(two), err)
	}
	if _, err := s.ListCaptures(ctx, 201); err == nil {
		t.Error("ListCaptures(201) gave no error")
	}
}

// Two requests with the same content at the same moment: the unique index
// lets one insert through, and the other request gets the stored source.
func TestCaptureOfTheSameContentAtTheSameMomentStoresOneSource(t *testing.T) {
	s, pool := newTestService(t)
	const callers = 8

	ids := make(chan int64, callers)
	for range callers {
		go func() {
			capture, err := s.Capture(context.Background(), CaptureParams{Text: "a note", Source: SourceManual})
			if err != nil {
				t.Errorf("Capture: %v", err)
			}
			ids <- capture.ID
		}()
	}
	first := <-ids
	for range callers - 1 {
		if id := <-ids; id != first {
			t.Errorf("a caller got the ID %d, and a different caller got %d", id, first)
		}
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}
