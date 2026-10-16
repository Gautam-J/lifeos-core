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

func addStoredEntity(t *testing.T, pool *pgxpool.Pool, kind, name string, aliases ...string) service.Entity {
	t.Helper()
	e, err := service.New(pool).AddEntity(t.Context(), service.AddEntityParams{
		Kind: kind, Name: name, Aliases: aliases, Provenance: service.Provenance{Source: service.SourceAgent},
	})
	if err != nil {
		t.Fatalf("add %q: %v", name, err)
	}
	return e
}

func entityPath(id int64) string { return fmt.Sprintf("/admin/entities/%d", id) }

func TestEntitiesPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/entities")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/entities"`, `name="kind"`, `name="name"`, `name="aliases"`, `name="wiki_path"`, "No entities yet.", `href="/admin/entities"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestAddEntityStoresTheRowAsManualAndShowsIt(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/entities", url.Values{"kind": {"person"}, "name": {"Ravi <b>Kumar</b>"}, "aliases": {"R, Ravi ,"}, "wiki_path": {""}})

	if rec.Code != http.StatusOK || rec.Header().Get("HX-Trigger") != "entity-added" {
		t.Fatalf("status = %d, HX-Trigger = %q\n%s", rec.Code, rec.Header().Get("HX-Trigger"), rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<b>") || !strings.Contains(body, "Ravi &lt;b&gt;Kumar&lt;/b&gt;") {
		t.Errorf("the name is not escaped:\n%s", body)
	}
	var source string
	var aliases []string
	if err := pool.QueryRow(t.Context(), "SELECT source, aliases FROM entities").Scan(&source, &aliases); err != nil {
		t.Fatal(err)
	}
	if source != "manual" || strings.Join(aliases, "|") != "R|Ravi" {
		t.Errorf("source = %q, aliases = %v, want manual and R, Ravi", source, aliases)
	}
}

func TestAddEntityShowsTheReasonOfARefusal(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredEntity(t, pool, "person", "Ravi")

	rec := postForm(h, "/admin/entities", url.Values{"kind": {"person"}, "name": {"ravi"}})

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "already") {
		t.Errorf("status = %d, want 200 with the reason\n%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("HX-Trigger") != "" {
		t.Error("the form would reset after a refusal")
	}
}

func TestEntityPageShowsTheEntityItsRecordsAndTheForms(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)
	ravi := addStoredEntity(t, pool, "person", "Ravi Kumar", "R")
	other := addStoredEntity(t, pool, "person", "Sam")
	addStoredEntity(t, pool, "place", "Goa")
	entry, err := svc.AddJournalEntry(t.Context(), service.AddJournalEntryParams{Body: "met Ravi", Provenance: service.Provenance{Source: service.SourceManual}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Tag(t.Context(), service.TagParams{Type: "journal", ID: entry.ID, Add: []service.EntityRef{{ID: ravi.ID}}, Actor: service.SourceManual}); err != nil {
		t.Fatal(err)
	}

	rec := get(h, entityPath(ravi.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Ravi Kumar", `value="R"`, "met Ravi", fmt.Sprintf(`href="/admin/journal/%d/edit"`, entry.ID),
		`action="` + entityPath(ravi.ID) + `"`, `action="` + entityPath(ravi.ID) + `/merge"`, `action="` + entityPath(ravi.ID) + `/delete"`,
		fmt.Sprintf(`<option value="%d">Sam</option>`, other.ID), fmt.Sprintf(`action="%s/unlink"`, entityPath(ravi.ID)),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q\n%s", want, body)
		}
	}
	if strings.Contains(body, ">Goa<") {
		t.Error("the merge list holds an entity of a different kind")
	}
	for _, path := range []string{"/admin/entities/9999", "/admin/entities/x"} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestSaveEntityChangesTheFieldsAsManual(t *testing.T) {
	h, pool := newTestAdmin(t)
	ravi := addStoredEntity(t, pool, "person", "Ravi")

	rec := postForm(h, entityPath(ravi.ID), url.Values{"name": {"Ravi Kumar"}, "aliases": {"R, RK"}, "wiki_path": {"people/ravi-kumar.md"}, "archived": {"true"}})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != entityPath(ravi.ID) {
		t.Fatalf("status = %d, Location = %q, want 303 to the page\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetEntity(t.Context(), ravi.ID)
	if err != nil || got.Name != "Ravi Kumar" || strings.Join(got.Aliases, "|") != "R|RK" || got.WikiPath != "people/ravi-kumar.md" || got.ArchivedAt == nil {
		t.Errorf("entity = %+v, %v", got, err)
	}

	// An empty page and no check box clear the page and bring it back.
	rec = postForm(h, entityPath(ravi.ID), url.Values{"name": {"Ravi Kumar"}, "aliases": {"R, RK"}, "wiki_path": {""}})
	got, _ = service.New(pool).GetEntity(t.Context(), ravi.ID)
	if rec.Code != http.StatusSeeOther || got.WikiPath != "" || got.ArchivedAt != nil {
		t.Errorf("status = %d, entity = %+v, want no page and not archived", rec.Code, got)
	}

	bad := postForm(h, entityPath(ravi.ID), url.Values{"name": {" "}})
	if bad.Code != http.StatusOK || !strings.Contains(bad.Body.String(), `role="alert"`) {
		t.Errorf("a refused save: status = %d\n%s", bad.Code, bad.Body)
	}
}

func TestMergeEntityMovesTheOtherIntoThisOne(t *testing.T) {
	h, pool := newTestAdmin(t)
	keep := addStoredEntity(t, pool, "person", "Ravi Kumar")
	drop := addStoredEntity(t, pool, "person", "R")

	rec := postForm(h, entityPath(keep.ID)+"/merge", url.Values{"drop": {fmt.Sprint(drop.ID)}})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != entityPath(keep.ID) {
		t.Fatalf("status = %d, Location = %q\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetEntity(t.Context(), keep.ID)
	if err != nil || strings.Join(got.Aliases, "|") != "R" {
		t.Errorf("entity = %+v, %v, want the alias R", got, err)
	}
	if rec := get(h, entityPath(drop.ID)); rec.Code != http.StatusNotFound {
		t.Errorf("the merged entity: status = %d, want 404", rec.Code)
	}
	bad := postForm(h, entityPath(keep.ID)+"/merge", url.Values{"drop": {fmt.Sprint(keep.ID)}})
	if bad.Code != http.StatusOK || !strings.Contains(bad.Body.String(), `role="alert"`) {
		t.Errorf("a merge with itself: status = %d\n%s", bad.Code, bad.Body)
	}
}

func TestUnlinkRemovesOneLink(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)
	ravi := addStoredEntity(t, pool, "person", "Ravi")
	todo := addStoredTodo(t, pool, "call", true)
	if _, err := svc.Tag(t.Context(), service.TagParams{Type: "todo", ID: todo.ID, Add: []service.EntityRef{{ID: ravi.ID}}, Actor: service.SourceManual}); err != nil {
		t.Fatal(err)
	}

	rec := postForm(h, entityPath(ravi.ID)+"/unlink", url.Values{"type": {"todo"}, "id": {fmt.Sprint(todo.ID)}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", rec.Code, rec.Body)
	}
	var n int
	pool.QueryRow(t.Context(), "SELECT count(*) FROM record_entities").Scan(&n)
	if n != 0 {
		t.Errorf("links = %d, want 0", n)
	}
}

func TestDeleteEntityIsSoftAndTheDeletedPageListsIt(t *testing.T) {
	h, pool := newTestAdmin(t)
	ravi := addStoredEntity(t, pool, "person", "Ravi")

	rec := postForm(h, entityPath(ravi.ID)+"/delete", nil)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/entities" {
		t.Fatalf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	if body := get(h, "/admin/deleted").Body.String(); !strings.Contains(body, "person: Ravi") {
		t.Errorf("the deleted page does not list the entity:\n%s", body)
	}
	if body := get(h, "/admin/entities").Body.String(); strings.Contains(body, "Ravi") {
		t.Error("the list still shows a deleted entity")
	}
}

func TestTodoFormsOfferTheNamesOfTheProjects(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "call", true) // project lifeos

	for _, path := range []string{"/admin/todos", fmt.Sprintf("/admin/todos/%d/edit", todo.ID)} {
		body := get(h, path).Body.String()
		if !strings.Contains(body, `<datalist id="projects">`) || !strings.Contains(body, `<option value="lifeos">`) || !strings.Contains(body, `list="projects"`) {
			t.Errorf("%s: the project field has no list of project names\n%s", path, body)
		}
	}
}
