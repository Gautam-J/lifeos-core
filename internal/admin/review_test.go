package admin_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

func TestReviewPageListsTheRecordsThatWait(t *testing.T) {
	h, pool := newTestAdmin(t)
	waits := addStored(t, pool, `<i>Inferred</i>`, false)
	addStored(t, pool, "Approved before", true)

	rec := get(h, "/admin/review")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"movie: &lt;i&gt;Inferred&lt;/i&gt;, rating 4.5, on 2024-03-01",
		`hx-post="/admin/review/verify"`,
		`name="type" value="item"`,
		fmt.Sprintf(`name="id" value="%d"`, waits.ID),
		fmt.Sprintf(`href="/admin/items/%d/edit"`, waits.ID),
		"agent",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, "<i>Inferred") {
		t.Error("the page contains the title as raw HTML")
	}
	if strings.Contains(body, "Approved before") {
		t.Error("the page lists a record that is verified")
	}
}

func TestReviewPageSaysWhenNothingWaits(t *testing.T) {
	h, _ := newTestAdmin(t)

	body := get(h, "/admin/review").Body.String()

	if !strings.Contains(body, "No record waits for review.") {
		t.Errorf("the page does not say that the queue is empty:\n%s", body)
	}
}

func TestVerifyRecordApprovesTheRecordAndShowsTheQueue(t *testing.T) {
	h, pool := newTestAdmin(t)
	first := addStored(t, pool, "First", false)
	addStored(t, pool, "Second", false)

	rec := postForm(h, "/admin/review/verify", url.Values{"type": {"item"}, "id": {fmt.Sprint(first.ID)}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	if strings.Contains(body, "First") || !strings.Contains(body, "Second") {
		t.Errorf("the queue must show Second and not First:\n%s", body)
	}
	got, err := service.New(pool).GetItem(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Verified {
		t.Error("the item is not verified")
	}
	var source string
	if err := pool.QueryRow(t.Context(), "SELECT source FROM events WHERE action = 'verify'").Scan(&source); err != nil || source != "manual" {
		t.Errorf("verify event source = %q (err %v), want manual", source, err)
	}
}

func TestVerifyRecordShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)
	waits := addStored(t, pool, "Waits", false)

	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"record that does not exist", url.Values{"type": {"item"}, "id": {"9999"}}, "records: item 9999 does not exist"},
		{"unknown type", url.Values{"type": {"song"}, "id": {fmt.Sprint(waits.ID)}}, "is not a record type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/review/verify", tt.form)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200: htmx does not show a 4xx response", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantText) || !strings.Contains(body, "Waits") {
				t.Errorf("the response must show %q and the queue:\n%s", tt.wantText, body)
			}
		})
	}
	if rec := postForm(h, "/admin/review/verify", url.Values{"type": {"item"}, "id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("ID that is not a number: status = %d, want 400", rec.Code)
	}
	got, err := service.New(pool).GetItem(t.Context(), waits.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verified {
		t.Error("a refused request verified the item")
	}
}

// htmx does not show the body of a 5xx response. A click on Delete or
// Approve must put the failure in the list.
func TestDatabaseFailureOnAClickShowsInTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	clicks := map[string]struct {
		path string
		form url.Values
		want string
	}{
		"delete":  {"/admin/items/1/delete", nil, "The item was not deleted: internal error. Try again."},
		"approve": {"/admin/review/verify", url.Values{"type": {"item"}, "id": {"1"}}, "The record was not approved: internal error. Try again."},
	}
	for name, click := range clicks {
		rec := postForm(h, click.path, click.form)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", name, rec.Code)
		}
		want := `<div id="list"><p class="error" role="alert">` + click.want + `</p></div>`
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Errorf("%s: body = %q, want only the list with the error", name, got)
		}
	}
}

func TestDatabaseFailureOnAPageGivesAPlain500(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"edit page":   get(h, "/admin/items/1/edit"),
		"review page": get(h, "/admin/review"),
		"save":        postForm(h, "/admin/items/1", url.Values{"kind": {"movie"}, "title": {"Dune"}}),
	} {
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", name, rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "internal error" {
			t.Errorf("%s: body = %q, want only \"internal error\"", name, got)
		}
	}
}
