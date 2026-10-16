package admin_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

func TestSourcesPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/sources")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/sources"`, `<textarea name="text"`, `name="url"`, `name="title"`, "No sources yet.", `href="/admin/sources"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

// A browser sends each line break of a textarea as CR LF. The stored text
// must have the line breaks that Gautam typed.
func TestAddSourceStoresTheTextOfTheForm(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/sources", url.Values{"text": {"  Renewal on 12 March.\r\n\r\nCall first.  \r\n"}, "url": {""}, "title": {"Insurance"}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "source-added" {
		t.Errorf("HX-Trigger = %q, want source-added so that the form resets", got)
	}
	page := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(page), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", page)
	}
	for _, want := range []string{"Insurance", "Renewal on 12 March.", "text", "manual"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}

	var content, source string
	if err := pool.QueryRow(t.Context(), "SELECT content, source FROM sources").Scan(&content, &source); err != nil {
		t.Fatalf("read the row (want exactly one): %v", err)
	}
	if content != "  Renewal on 12 March.\n\nCall first.  \n" {
		t.Errorf("stored content = %q", content)
	}
	if source != "manual" {
		t.Errorf("source = %q, want manual", source)
	}
}

func TestAddSourceStoresALink(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/sources", url.Values{"text": {""}, "url": {"https://example.com/article"}})

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `href="https://example.com/article"`) {
		t.Errorf("status = %d, want 200 and the link in the list\n%s", rec.Code, rec.Body)
	}
	var kind string
	if err := pool.QueryRow(t.Context(), "SELECT kind FROM sources").Scan(&kind); err != nil || kind != "link" {
		t.Errorf("kind = %q, %v, want link", kind, err)
	}
}

func TestAddSourceShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"nothing", url.Values{"text": {""}, "url": {""}}, "text: give one of these"},
		{"a text and a link", url.Values{"text": {"a note"}, "url": {"https://example.com"}}, "text: give one of these"},
		{"a card number", url.Values{"text": {"card 4111 1111 1111 1111"}}, "text: must not hold a card number"},
		{"a link with no scheme", url.Values{"url": {"example.com"}}, "url: must start with http:// or https://"},
	}
	h, pool := newTestAdmin(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/sources", tt.form)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 so that htmx swaps the list", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Errorf("the response does not contain %q:\n%s", tt.wantText, rec.Body)
			}
			if rec.Header().Get("HX-Trigger") != "" {
				t.Error("HX-Trigger is set, so the form loses what Gautam typed")
			}
			if strings.Contains(rec.Body.String(), "4111 1111") {
				t.Error("the page repeats the card number")
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM sources").Scan(&n); err != nil || n != 0 {
		t.Errorf("sources = %d, %v, want 0", n, err)
	}
}

// The list shows the start of a long text, and a file by its name.
func TestSourcesListShowsEachKind(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool).WithFiles(service.Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: 4096})
	long := "Start of the article. " + strings.Repeat("word ", 100) + "THE END"
	for _, p := range []service.CaptureParams{
		{Text: long, Source: service.SourceAgent},
		{Upload: bytes.NewReader([]byte("%PDF-1.7\n")), FileName: "policy.pdf", Source: service.SourceManual},
	} {
		if _, err := svc.Capture(t.Context(), p); err != nil {
			t.Fatalf("Capture: %v", err)
		}
	}

	page := get(h, "/admin/sources").Body.String()

	for _, want := range []string{"Start of the article.", "…", "agent", "policy.pdf", "application/pdf", "9 bytes"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}
	if strings.Contains(page, "THE END") {
		t.Error("the list shows the end of a long text")
	}
}

func TestDeleteSourceRemovesItFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	capture, err := service.New(pool).Capture(t.Context(), service.CaptureParams{Text: "a note to delete", Source: service.SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	path := fmt.Sprintf("/admin/sources/%d/delete", capture.ID)

	rec := postForm(h, path, nil)

	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "a note to delete") || !strings.Contains(rec.Body.String(), "No sources yet.") {
		t.Errorf("status = %d, want 200 and an empty list\n%s", rec.Code, rec.Body)
	}
	if _, err := service.New(pool).GetCapture(t.Context(), capture.ID); err != service.ErrNotFound {
		t.Errorf("GetCapture after the delete: error = %v, want ErrNotFound", err)
	}

	// A second click, or a click in a stale tab.
	rec = postForm(h, path, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "is not in the list") {
		t.Errorf("second delete: status = %d\n%s", rec.Code, rec.Body)
	}
	if rec := postForm(h, "/admin/sources/abc/delete", nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete with a bad ID: status = %d, want 404", rec.Code)
	}
}

// #9: the list cuts a long text, so a source has a page that shows all of
// it. A source is verbatim (rule 12), so the page has no edit form.
func TestSourcePageShowsTheWholeText(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)
	long := "  Start of the article. <b>bold</b>\n\n" + strings.Repeat("word ", 100) + "THE END  \n"
	capture, err := svc.Capture(t.Context(), service.CaptureParams{Text: long, Title: "Article", Source: service.SourceAgent})
	if err != nil {
		t.Fatal(err)
	}

	list := get(h, "/admin/sources").Body.String()
	rec := get(h, fmt.Sprintf("/admin/sources/%d", capture.ID))

	if !strings.Contains(list, fmt.Sprintf(`href="/admin/sources/%d"`, capture.ID)) {
		t.Error("the list has no link to the page of the source")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	page := rec.Body.String()
	escaped := "  Start of the article. &lt;b&gt;bold&lt;/b&gt;\n\n" + strings.Repeat("word ", 100) + "THE END  \n"
	for _, want := range []string{`<div class="prose">` + escaped + `</div>`, "Article", "agent", capture.SHA256, `href="/admin/sources"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
	if strings.Contains(page, "<form") || strings.Contains(page, "<textarea") {
		t.Error("the page of a source has a form: a source is verbatim and has no edit")
	}
}

func TestSourcePageShowsALinkAndAFile(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool).WithFiles(service.Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: 4096})
	link, err := svc.Capture(t.Context(), service.CaptureParams{URL: "https://example.com/a?b=1", Source: service.SourceManual})
	if err != nil {
		t.Fatal(err)
	}
	file, err := svc.Capture(t.Context(), service.CaptureParams{Upload: bytes.NewReader([]byte("%PDF-1.7\n")), FileName: "policy.pdf", Source: service.SourceManual})
	if err != nil {
		t.Fatal(err)
	}

	linkPage := get(h, fmt.Sprintf("/admin/sources/%d", link.ID)).Body.String()
	filePage := get(h, fmt.Sprintf("/admin/sources/%d", file.ID)).Body.String()

	if !strings.Contains(linkPage, `href="https://example.com/a?b=1"`) {
		t.Error("the page of a link has no link")
	}
	for _, want := range []string{"policy.pdf", "application/pdf", "9 bytes"} {
		if !strings.Contains(filePage, want) {
			t.Errorf("the page of a file does not contain %q", want)
		}
	}
}

func TestSourcePageReturns404ForASourceThatIsNotThere(t *testing.T) {
	h, pool := newTestAdmin(t)
	deleted, err := service.New(pool).Capture(t.Context(), service.CaptureParams{Text: "gone", Source: service.SourceManual})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.New(pool).DeleteCapture(t.Context(), deleted.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/admin/sources/9999", "/admin/sources/abc", fmt.Sprintf("/admin/sources/%d", deleted.ID)} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
}
