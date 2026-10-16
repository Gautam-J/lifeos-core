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

// addStored stores one item through the service and returns it. manual
// false gives a row that an agent wrote.
func addStored(t *testing.T, pool *pgxpool.Pool, title string, manual bool) service.Item {
	t.Helper()
	rating := 4.5
	source := service.SourceManual
	if !manual {
		source = service.SourceAgent
	}
	item, err := service.New(pool).AddItem(t.Context(), service.AddItemParams{
		Kind:       "movie",
		Title:      title,
		Rating:     &rating,
		OccurredOn: "2024-03-01",
		Attrs:      map[string]any{"year": 2024},
		Provenance: service.Provenance{Source: source},
	})
	if err != nil {
		t.Fatalf("add %q: %v", title, err)
	}
	return item
}

func TestItemsPageLinksToEditAndDelete(t *testing.T) {
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, "Dune", true)

	body := get(h, "/admin/items").Body.String()

	for _, want := range []string{
		fmt.Sprintf(`href="/admin/items/%d/edit"`, item.ID),
		fmt.Sprintf(`hx-post="/admin/items/%d/delete"`, item.ID),
		`hx-confirm="Delete Dune?"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
}

// The title goes into an HTML attribute of the edit form. A quote or a
// tag in the title must not end the attribute.
func TestEditPageShowsTheStoredValuesAsText(t *testing.T) {
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, `Dune "2" <b>`, true)

	rec := get(h, fmt.Sprintf("/admin/items/%d/edit", item.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		fmt.Sprintf(`action="/admin/items/%d"`, item.ID),
		`value="Dune &#34;2&#34; &lt;b&gt;"`,
		`<option value="movie" selected>`,
		`value="4.5"`,
		`value="2024-03-01"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, `<b>`) {
		t.Error("the page contains the title as raw HTML")
	}
}

func TestEditPageReturns404ForAnItemThatIsNotThere(t *testing.T) {
	h, pool := newTestAdmin(t)
	deleted := addStored(t, pool, "Deleted", true)
	if err := service.New(pool).DeleteItem(t.Context(), deleted.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/admin/items/9999/edit", "/admin/items/abc/edit", fmt.Sprintf("/admin/items/%d/edit", deleted.ID)} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
	for _, path := range []string{"/admin/items/9999", "/admin/items/abc"} {
		rec := postForm(h, path, url.Values{"kind": {"movie"}, "title": {"New"}})
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestSaveItemChangesTheItemAndGoesBackToTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, "Dune", false)

	// An empty rating and an empty date remove the stored values.
	rec := postForm(h, fmt.Sprintf("/admin/items/%d", item.ID), url.Values{
		"kind": {"book"}, "title": {"Dune (novel)"}, "rating": {""}, "occurred_on": {""},
	})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/items" {
		t.Fatalf("status = %d, Location = %q; want 303 to /admin/items\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetItem(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "book" || got.Title != "Dune (novel)" || got.Rating != nil || got.OccurredOn != "" {
		t.Errorf("item = %+v", got)
	}
	if got.Attrs["year"] != float64(2024) {
		t.Errorf("attrs = %v, want no change: the form has no attrs field", got.Attrs)
	}
	// An edit is not an approval: the row stays in the review queue.
	if got.Source != service.SourceAgent {
		t.Errorf("source = %q; want no change", got.Source)
	}
	var source string
	if err := pool.QueryRow(t.Context(), "SELECT source FROM events WHERE action = 'update'").Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "manual" {
		t.Errorf("update event source = %q, want manual", source)
	}
}

func TestSaveItemShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"rating above 5", url.Values{"kind": {"movie"}, "title": {"Typed title"}, "rating": {"7"}}, "rating: must be from 0 to 5"},
		{"rating that is not a number", url.Values{"kind": {"movie"}, "title": {"Typed title"}, "rating": {"four"}}, "rating: must be a number"},
		{"date in the future", url.Values{"kind": {"movie"}, "title": {"Typed title"}, "occurred_on": {"2099-01-01"}}, "occurred_on: must not be in the future"},
		{"no kind", url.Values{"title": {"Typed title"}}, "kind: must be one of"},
	}
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, "Dune", true)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, fmt.Sprintf("/admin/items/%d", item.ID), tt.form)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 with the form", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantText) {
				t.Errorf("the page does not show %q", tt.wantText)
			}
			// The form keeps what Gautam typed.
			if !strings.Contains(body, `value="Typed title"`) {
				t.Error("the form lost the typed title")
			}
		})
	}
	got, err := service.New(pool).GetItem(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Dune" || *got.Rating != 4.5 {
		t.Errorf("refused input changed the item: %+v", got)
	}
}

func TestDeleteItemRemovesTheItemFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	kept := addStored(t, pool, "Kept", true)
	gone := addStored(t, pool, "Gone", true)
	path := fmt.Sprintf("/admin/items/%d/delete", gone.ID)

	rec := postForm(h, path, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	if strings.Contains(body, "Gone") || !strings.Contains(body, "Kept") {
		t.Errorf("the list must show Kept and not Gone:\n%s", body)
	}
	var deleted bool
	var source string
	if err := pool.QueryRow(t.Context(), "SELECT deleted_at IS NOT NULL FROM catalog_items WHERE id = $1", gone.ID).Scan(&deleted); err != nil || !deleted {
		t.Errorf("the row is not marked as deleted (err %v)", err)
	}
	if err := pool.QueryRow(t.Context(), "SELECT source FROM events WHERE action = 'delete'").Scan(&source); err != nil || source != "manual" {
		t.Errorf("delete event source = %q (err %v), want manual", source, err)
	}

	// A second click, for example from a page that was open in two tabs.
	again := postForm(h, path, nil)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), fmt.Sprintf("Item %d is not in the list", gone.ID)) {
		t.Errorf("second delete: status = %d, want 200 with a message\n%s", again.Code, again.Body)
	}
	if !strings.Contains(again.Body.String(), "Kept") {
		t.Errorf("the list lost the item %d", kept.ID)
	}
}

// A click on Save with no change is not a write.
func TestSaveItemWithNoChangeWritesNoEvent(t *testing.T) {
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, "Dune", true)

	rec := postForm(h, fmt.Sprintf("/admin/items/%d", item.ID), url.Values{
		"kind": {"movie"}, "title": {"Dune"}, "rating": {"4.5"}, "occurred_on": {"2024-03-01"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", rec.Code, rec.Body)
	}
	var events int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("events = %d, want 1 (the create)", events)
	}
}
