package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// todayIST is today's date for Gautam, as the service computes it.
func todayIST() string {
	return time.Now().In(service.IST).Format(time.DateOnly)
}

// addEntry stores one journal entry through the service and returns it.
func addEntry(t *testing.T, pool *pgxpool.Pool, body, entryDate string) service.JournalEntry {
	t.Helper()
	entry, err := service.New(pool).AddJournalEntry(t.Context(), service.AddJournalEntryParams{
		Body:       body,
		EntryDate:  entryDate,
		Provenance: service.Provenance{Source: service.SourceAgent, Verified: true},
	})
	if err != nil {
		t.Fatalf("add %q: %v", body, err)
	}
	return entry
}

func countEvents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestJournalPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/journal")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/journal"`, `<textarea name="body"`, `name="entry_date"`, "No entries yet.", `href="/admin/journal"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

// A browser sends each line break of a textarea as CR LF. The stored
// entry must have the line breaks that Gautam typed, not two characters
// for each. All other characters stay as they are.
func TestAddJournalEntryStoresTheTextOfTheForm(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/journal", url.Values{"body": {"  planned the schema\r\n\r\nno gym :(  \r\n"}, "entry_date": {""}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "entry-added" {
		t.Errorf("HX-Trigger = %q, want entry-added so that the form resets", got)
	}
	page := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(page), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", page)
	}
	for _, want := range []string{"planned the schema", todayIST(), "manual", "yes"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}

	var body, source string
	var verified bool
	if err := pool.QueryRow(t.Context(), "SELECT body, source, verified FROM journal_entries").Scan(&body, &source, &verified); err != nil {
		t.Fatalf("read the row (want exactly one): %v", err)
	}
	if body != "  planned the schema\n\nno gym :(  \n" {
		t.Errorf("stored body = %q", body)
	}
	if source != "manual" || !verified {
		t.Errorf("source, verified = %q, %v, want manual, true", source, verified)
	}
}

func TestAddJournalEntryShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"no text", url.Values{"body": {" \r\n"}}, "body: must not be empty"},
		{"date in the future", url.Values{"body": {"a day"}, "entry_date": {"2999-01-01"}}, "entry_date: must not be in the future"},
	}
	h, pool := newTestAdmin(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/journal", tt.form)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200: htmx does not show a 4xx response", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Errorf("the response does not show %q:\n%s", tt.wantText, rec.Body)
			}
			if got := rec.Header().Get("HX-Trigger"); got != "" {
				t.Errorf("HX-Trigger = %q, want none so that the form keeps its input", got)
			}
		})
	}
	if n := countEvents(t, pool); n != 0 {
		t.Errorf("events = %d after refused input, want 0", n)
	}
}

// The entry is text. The page must show it as text, and the edit form must
// give back each character, or a click on Save changes the entry.
func TestJournalPagesShowTheEntryAsText(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "  a <b>bold</b> day\n\n</textarea><script>alert(1)</script> \n", "2026-01-05")

	list := get(h, "/admin/journal").Body.String()
	edit := get(h, fmt.Sprintf("/admin/journal/%d/edit", entry.ID))

	if edit.Code != http.StatusOK {
		t.Fatalf("edit page: status = %d, want 200", edit.Code)
	}
	const escaped = "  a &lt;b&gt;bold&lt;/b&gt; day\n\n&lt;/textarea&gt;&lt;script&gt;alert(1)&lt;/script&gt; \n"
	if !strings.Contains(list, `<div class="prose">`+escaped+`</div>`) {
		t.Errorf("the list does not show the entry as text with its spaces and line breaks:\n%s", list)
	}
	for _, want := range []string{
		fmt.Sprintf(`action="/admin/journal/%d"`, entry.ID),
		// The line break after the start tag is the one that a browser drops.
		`rows="12">` + "\n" + escaped + `</textarea>`,
		`value="2026-01-05"`,
		fmt.Sprintf(`href="/admin/journal/%d/edit"`, entry.ID),
	} {
		if !strings.Contains(list+edit.Body.String(), want) {
			t.Errorf("the pages do not contain %q", want)
		}
	}
	if strings.Contains(list+edit.Body.String(), "<script>alert(") {
		t.Error("a page contains the entry as raw HTML")
	}
}

func TestSaveJournalEntryChangesTheEntryAndGoesBackToTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "a day", "2026-01-05")

	rec := postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"a good day\r\nwith two lines"}, "entry_date": {"2026-01-04"}})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/journal" {
		t.Fatalf("status = %d, Location = %q; want 303 to /admin/journal\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetJournalEntry(t.Context(), entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "a good day\nwith two lines" || got.EntryDate != "2026-01-04" {
		t.Errorf("entry = %+v", got)
	}
	if got.Source != service.SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v; want no change", got.Source, got.Verified)
	}
}

// The browser sends CR LF for the line breaks of the stored text. A click
// on Save with no change must not count as a change.
func TestSaveJournalEntryWithNoChangeWritesNoEvent(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "line one\nline two\n", "2026-01-05")

	rec := postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"line one\r\nline two\r\n"}, "entry_date": {"2026-01-05"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", rec.Code, rec.Body)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestSaveJournalEntryShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "a day", "2026-01-05")

	rec := postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"typed text"}, "entry_date": {"2999-01-01"}})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with the form", rec.Code)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "entry_date: must not be in the future") {
		t.Errorf("the page does not show the reason:\n%s", page)
	}
	if !strings.Contains(page, ">\ntyped text</textarea>") || !strings.Contains(page, `value="2999-01-01"`) {
		t.Error("the form lost the typed input")
	}
	if got, _ := service.New(pool).GetJournalEntry(t.Context(), entry.ID); got.Body != "a day" {
		t.Errorf("refused input changed the entry: %+v", got)
	}
}

func TestJournalPagesReturn404ForAnEntryThatIsNotThere(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/admin/journal/9999/edit", "/admin/journal/abc/edit"} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
	for _, path := range []string{"/admin/journal/9999", "/admin/journal/abc"} {
		if rec := postForm(h, path, url.Values{"body": {"x"}, "entry_date": {"2026-01-05"}}); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestDeleteJournalEntryRemovesTheEntryFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	addEntry(t, pool, "kept entry", "2026-01-05")
	gone := addEntry(t, pool, "gone entry", "2026-01-06")
	path := fmt.Sprintf("/admin/journal/%d/delete", gone.ID)

	rec := postForm(h, path, nil)

	page := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(page), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, page)
	}
	if strings.Contains(page, "gone entry") || !strings.Contains(page, "kept entry") {
		t.Errorf("the list must show the kept entry and not the deleted one:\n%s", page)
	}
	if _, err := service.New(pool).GetJournalEntry(t.Context(), gone.ID); err != service.ErrNotFound {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}

	// A second click, for example from a page that was open in two tabs.
	again := postForm(h, path, nil)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), fmt.Sprintf("Entry %d is not in the list", gone.ID)) {
		t.Errorf("second delete: status = %d, want 200 with a message\n%s", again.Code, again.Body)
	}
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (two creates, one delete)", n)
	}
}

func TestDatabaseFailureOnTheJournalPages(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	if rec := get(h, "/admin/journal"); rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
		t.Errorf("page: status = %d, body = %q; want 500 and only \"internal error\"", rec.Code, rec.Body)
	}
	clicks := map[string]url.Values{
		"/admin/journal":          {"body": {"a day"}},
		"/admin/journal/1/delete": nil,
	}
	for path, form := range clicks {
		rec := postForm(h, path, form)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s: status = %d, want 200: htmx does not show a 5xx response", path, rec.Code)
		}
		page := strings.TrimSpace(rec.Body.String())
		if !strings.HasPrefix(page, `<div id="list"><p class="error" role="alert">`) || !strings.Contains(page, "internal error") {
			t.Errorf("POST %s: the response is not the list with the error:\n%s", path, page)
		}
	}
}

// The review queue has records of more than one type. The Edit link of a
// journal entry must go to the journal, not to the item with the same ID.
func TestReviewPageLinksAJournalEntryToItsEditPage(t *testing.T) {
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, "Dune", false)
	entry, err := service.New(pool).AddJournalEntry(t.Context(), service.AddJournalEntryParams{
		Body: "an entry that is not verified", Provenance: service.Provenance{Source: service.SourceAgent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != item.ID {
		t.Fatalf("the test needs one ID for the two records; got %d and %d", entry.ID, item.ID)
	}

	body := get(h, "/admin/review").Body.String()

	for _, want := range []string{
		fmt.Sprintf(`href="/admin/items/%d/edit"`, item.ID),
		fmt.Sprintf(`href="/admin/journal/%d/edit"`, entry.ID),
		`name="type" value="journal"`,
		"journal " + todayIST() + ": an entry that is not verified",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}

	rec := postForm(h, "/admin/review/verify", url.Values{"type": {"journal"}, "id": {fmt.Sprint(entry.ID)}})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "an entry that is not verified") || !strings.Contains(rec.Body.String(), "Dune") {
		t.Errorf("after the approval the queue must show the item and not the entry:\n%s", rec.Body)
	}
}
