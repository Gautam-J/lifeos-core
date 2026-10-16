package admin_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// #33: the Write button puts the cursor in the text box of the journal. A
// script does it; the link to /admin#write still works with no script.
func TestPagesLoadTheScriptOfTheWriteButton(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/admin", "/admin/todos"} {
		t.Run(path, func(t *testing.T) {
			page := get(h, path).Body.String()

			m := regexp.MustCompile(`<script src="(/admin/static/write\.js\?v=[0-9a-f]{12})" defer>`).FindStringSubmatch(page)
			if m == nil {
				t.Fatal("the page does not load write.js with a version and with defer")
			}
			if !strings.Contains(page, `<a class="btn btn-primary" href="/admin#write" data-write>Write</a>`) {
				t.Error("the Write link is not marked with data-write, or it lost its address")
			}
			rec := get(h, m[1])
			js := rec.Body.String()
			if rec.Code != http.StatusOK || !strings.Contains(js, "#write-form textarea") || !strings.Contains(js, "hashchange") {
				t.Errorf("write.js: status = %d, or it does not focus the text box on a click and on a hash", rec.Code)
			}
		})
	}
}
