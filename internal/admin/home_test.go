package admin_test

import (
	"net/http"
	"testing"
)

// The address that Gautam types has no path. It must open the admin pages.
func TestTheBareAddressOpensTheItemsPage(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/", "/admin", "/admin/"} {
		rec := get(h, path)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("GET %s: status = %d, want 303", path, rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/admin/items" {
			t.Errorf("GET %s: Location = %q, want /admin/items", path, got)
		}
	}
}
