package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// #40: nothing waits for approval, so the review routes are gone.
func TestTheReviewRoutesAreGone(t *testing.T) {
	h, _ := newTestAPI(t)

	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/review", ""},
		{http.MethodPost, "/api/review/verify", `{"records":[{"type":"todo","id":1}]}`},
	} {
		if status := serve(h, httptestRequest(r.method, r.path, r.body)); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status %d, want 404 or 405", r.method, r.path, status)
		}
	}
}

func httptestRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func serve(h http.Handler, req *http.Request) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}
