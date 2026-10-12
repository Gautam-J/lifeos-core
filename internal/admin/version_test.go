package admin_test

import (
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/version"
)

// The footer names the release, so that a bug report can name it too.
func TestTheFooterShowsTheRelease(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v9.8.7"
	h, _ := newTestAdmin(t)

	page := get(h, "/admin/items").Body.String()

	if !strings.Contains(page, "Release v9.8.7.") {
		t.Error("the footer does not show the release v9.8.7")
	}
}
