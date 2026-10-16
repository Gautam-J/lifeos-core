package api_test

import (
	"net/http"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/version"
)

// The OpenAPI document names the release of the binary, not fixed text.
func TestTheOpenAPIDocumentNamesTheRelease(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v9.8.7"
	h, _ := newTestAPI(t)

	status, doc, raw := do(t, h, http.MethodGet, "/api/openapi.json", "")

	if status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", status, raw)
	}
	info, _ := doc["info"].(map[string]any)
	if got := info["version"]; got != "v9.8.7" {
		t.Errorf("info.version is %v, want v9.8.7", got)
	}
}
