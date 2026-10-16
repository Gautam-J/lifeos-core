package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/api"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// maxUploadBytes is the file limit of these tests.
const maxUploadBytes = 4096

var pdf = []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")

// newFileAPI returns the API of a service that has a file store. filesDir
// is the folder of the file store.
func newFileAPI(t *testing.T) (h http.Handler, filesDir string) {
	t.Helper()
	filesDir = t.TempDir()
	svc := service.New(testdb.New(t)).WithFiles(service.Files{Dir: filesDir, Inbox: t.TempDir(), MaxBytes: maxUploadBytes})
	router := chi.NewRouter()
	api.Register(router, svc)
	return router, filesDir
}

// upload sends the bytes of one file as the body of a request.
func upload(t *testing.T, h http.Handler, path, contentType string, content []byte) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewReader(content))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestIngestATextThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/ingest", `{"text":"  Renewal on 12 March.\n","title":"Insurance"}`)
	if status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200\n%s", status, raw)
	}
	if added["kind"] != "text" || added["content"] != "  Renewal on 12 March.\n" || added["title"] != "Insurance" {
		t.Errorf("source = %s, want the text exactly as it was sent", raw)
	}
	if added["source"] != "manual" {
		t.Errorf("source = %v, want manual", added["source"])
	}

	path := fmt.Sprintf("/api/sources/%d", int64(added["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["content"] != added["content"] || got["sha256"] != added["sha256"] {
		t.Errorf("GET: status = %d, source = %s", status, raw)
	}

	// The same text a second time is the same source.
	status, again, raw := do(t, h, "POST", "/api/ingest", `{"text":"  Renewal on 12 March.\n"}`)
	if status != http.StatusOK || again["id"] != added["id"] {
		t.Errorf("second POST: status = %d, source = %s, want the first source", status, raw)
	}
}

func TestIngestALink(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/ingest", `{"url":"https://example.com/article"}`)
	if status != http.StatusOK || added["kind"] != "link" || added["url"] != "https://example.com/article" {
		t.Errorf("status = %d, source = %s", status, raw)
	}
	if _, hasContent := added["content"]; hasContent {
		t.Errorf("a link has content: %s", raw)
	}
}

func TestIngestRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"nothing to capture", `{}`, 422, "body.text"},
		{"a text and a link", `{"text":"a note","url":"https://example.com"}`, 422, "body.text"},
		{"a text of spaces", `{"text":"   "}`, 422, "body.text"},
		{"a card number", `{"text":"card 4111 1111 1111 1111"}`, 422, "a card number"},
		{"a link with no scheme", `{"url":"example.com"}`, 422, "body.url"},
		{"a title that is too long", `{"text":"a note","title":"` + strings.Repeat("a", 301) + `"}`, 422, "body.title"},
		{"a field that a caller must not set", `{"text":"a note","verified":false}`, 422, "verified"},
		{"a path on the Pi", `{"path":"/etc/passwd"}`, 422, "path"},
		{"text of the wrong type", `{"text":5}`, 422, "body.text"},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		status, _, raw := do(t, h, "POST", "/api/ingest", tt.body)
		if status != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d\n%s", tt.name, status, tt.wantStatus, raw)
		}
		if !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: body does not contain %q\n%s", tt.name, tt.wantInBody, raw)
		}
		if strings.Contains(raw, "4111 1111") {
			t.Errorf("%s: the answer repeats the card number\n%s", tt.name, raw)
		}
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM sources").Scan(&n); err != nil || n != 0 {
		t.Errorf("sources = %d, %v, want 0", n, err)
	}
}

func TestIngestAFileThenReadIt(t *testing.T) {
	h, filesDir := newFileAPI(t)

	status, raw := upload(t, h, "/api/ingest/file?name=policy.pdf", "application/pdf", pdf)
	if status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200\n%s", status, raw)
	}
	for _, want := range []string{`"kind":"file"`, `"title":"policy.pdf"`, `"media_type":"application/pdf"`, fmt.Sprintf(`"size_bytes":%d`, len(pdf)), `"source":"manual"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the answer does not contain %s\n%s", want, raw)
		}
	}
	_, got, _ := do(t, h, "GET", "/api/sources/1", "")
	stored, err := os.ReadFile(filepath.Join(filesDir, got["file_path"].(string)))
	if err != nil || !bytes.Equal(stored, pdf) {
		t.Errorf("the stored file: error = %v, same bytes = %v", err, bytes.Equal(stored, pdf))
	}

	// A title and a different content type. The service finds the type in
	// the bytes; it does not use the header.
	status, raw = upload(t, h, "/api/ingest/file?name=scan.bin&title=Receipt", "text/plain", []byte("\x89PNG\r\n\x1a\n...."))
	if status != http.StatusOK || !strings.Contains(raw, `"title":"Receipt"`) || !strings.Contains(raw, `"media_type":"image/png"`) {
		t.Errorf("status = %d\n%s", status, raw)
	}
}

func TestIngestAFileRejectsBadRequests(t *testing.T) {
	h, filesDir := newFileAPI(t)

	tests := []struct {
		name       string
		content    []byte
		wantStatus int
		wantInBody string
	}{
		{"an empty file", nil, 400, "request body is required"},
		{"a file over the limit", bytes.Repeat([]byte{1}, maxUploadBytes+1), 413, ""},
		{"a text file with a card number", []byte("card 4111 1111 1111 1111"), 422, "a card number"},
	}
	for _, tt := range tests {
		status, raw := upload(t, h, "/api/ingest/file?name=a.bin", "application/octet-stream", tt.content)
		if status != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d\n%s", tt.name, status, tt.wantStatus, raw)
		}
		if !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: body does not contain %q\n%s", tt.name, tt.wantInBody, raw)
		}
	}
	// The largest file that the limit permits.
	if status, raw := upload(t, h, "/api/ingest/file", "application/octet-stream", bytes.Repeat([]byte{1}, maxUploadBytes)); status != http.StatusOK {
		t.Errorf("a file of the maximum size: status = %d\n%s", status, raw)
	}
	entries, err := os.ReadDir(filesDir)
	if err != nil || len(entries) != 1 {
		t.Errorf("entries in the file store = %d, %v, want only the folder of the one stored file", len(entries), err)
	}
}

func TestReadAndDeleteASource(t *testing.T) {
	h, pool := newTestAPI(t)
	_, added, _ := do(t, h, "POST", "/api/ingest", `{"text":"a note"}`)
	path := fmt.Sprintf("/api/sources/%d", int64(added["id"].(float64)))

	if status, _, raw := do(t, h, "DELETE", path, ""); status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q, want 204 and no body", status, raw)
	}
	for _, method := range []string{"GET", "DELETE"} {
		if status, _, _ := do(t, h, method, path, ""); status != http.StatusNotFound {
			t.Errorf("%s after the delete: status = %d, want 404", method, status)
		}
	}
	if status, _, _ := do(t, h, "GET", "/api/sources/9999", ""); status != http.StatusNotFound {
		t.Errorf("GET of an unknown source: status = %d, want 404", status)
	}
	if status, _, _ := do(t, h, "PATCH", path, `{"content":"x"}`); status != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: status = %d, want 405: a source has no field to change", status)
	}
	// The delete is manual and has its event.
	var source string
	err := pool.QueryRow(t.Context(), "SELECT source FROM events WHERE action = 'delete' AND record_type = 'sources'").Scan(&source)
	if err != nil || source != "manual" {
		t.Errorf("delete event: source = %q, %v, want manual", source, err)
	}
}
