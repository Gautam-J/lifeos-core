package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// addDeleted stores one item and deletes it, as the items page does.
func addDeleted(t *testing.T, pool *pgxpool.Pool, title string) service.Item {
	t.Helper()
	item := addStored(t, pool, title, true)
	if err := service.New(pool).DeleteItem(t.Context(), item.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	return item
}

func countItems(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatalf("count the items: %v", err)
	}
	return n
}

func TestDeletedPageListsTheDeletedRecords(t *testing.T) {
	h, pool := newTestAdmin(t)
	gone := addDeleted(t, pool, `<i>Gone</i>`)
	addStored(t, pool, "Stays", true)

	rec := get(h, "/admin/deleted")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"movie: &lt;i&gt;Gone&lt;/i&gt;, rating 4.5, on 2024-03-01",
		`hx-post="/admin/deleted/purge"`,
		`name="type" value="item"`,
		fmt.Sprintf(`name="id" value="%d"`, gone.ID),
		fmt.Sprintf(`hx-confirm="Delete item %d forever? You cannot get it back."`, gone.ID),
		`<a href="/admin/deleted">Deleted</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, "<i>Gone") {
		t.Error("the page contains the title as raw HTML")
	}
	if strings.Contains(body, "Stays") {
		t.Error("the page lists a record that is not deleted")
	}
}

func TestDeletedPageSaysWhenNothingIsDeleted(t *testing.T) {
	h, _ := newTestAdmin(t)

	body := get(h, "/admin/deleted").Body.String()

	if !strings.Contains(body, "No record is deleted.") {
		t.Errorf("the page does not say that the list is empty:\n%s", body)
	}
}

func TestPurgeRecordRemovesTheRecordAndShowsTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	first := addDeleted(t, pool, "First")
	addDeleted(t, pool, "Second")

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"item"}, "id": {fmt.Sprint(first.ID)}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	if strings.Contains(body, "First") || !strings.Contains(body, "Second") {
		t.Errorf("the list must show Second and not First:\n%s", body)
	}
	if n := countItems(t, pool); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
}

func TestPurgeRecordShowsAMessageWhenTheRecordIsNotDeleted(t *testing.T) {
	h, pool := newTestAdmin(t)
	live := addStored(t, pool, "Stays", true)

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"item"}, "id": {fmt.Sprint(live.ID)}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	want := fmt.Sprintf("item %d is not in the list. It was possibly removed before.", live.ID)
	if body := rec.Body.String(); !strings.Contains(body, want) || !strings.Contains(body, `role="alert"`) {
		t.Errorf("the response does not show the message %q:\n%s", want, body)
	}
	if n := countItems(t, pool); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
}

func TestPurgeRecordShowsWhyTheServiceRefused(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)
	entry, err := svc.AddJournalEntry(t.Context(), service.AddJournalEntryParams{Body: "Renewed the policy.", Provenance: service.Provenance{Source: service.SourceManual}})
	if err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}
	_, err = svc.AddTodo(t.Context(), service.AddTodoParams{Title: "Renew the policy", JournalRef: entry.ID, Provenance: service.Provenance{Source: service.SourceAgent}})
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	if err := svc.DeleteJournalEntry(t.Context(), entry.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
		t.Fatalf("DeleteJournalEntry: %v", err)
	}

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"journal"}, "id": {fmt.Sprint(entry.ID)}})

	body := rec.Body.String()
	if !strings.Contains(body, "a record in todos names this journal entry") {
		t.Errorf("the response does not say why the entry stays:\n%s", body)
	}
	if !strings.Contains(body, "Renewed the policy.") {
		t.Errorf("the list does not show the entry any more:\n%s", body)
	}

	rec = postForm(h, "/admin/deleted/purge", url.Values{"type": {"habit"}, "id": {"1"}})
	if body := rec.Body.String(); !strings.Contains(body, "type: must be one of") {
		t.Errorf("a wrong type does not show the reason:\n%s", body)
	}
}

func TestPurgeRecordRefusesAnIDThatIsNotANumber(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"item"}, "id": {"one"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
