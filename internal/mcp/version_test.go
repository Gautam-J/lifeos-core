package mcp_test

import (
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/version"
)

// The answer to initialize names the release of the binary, not fixed text.
func TestServerInfoNamesTheRelease(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v9.8.7"
	session, _ := connect(t)

	if got := session.InitializeResult().ServerInfo.Version; got != "v9.8.7" {
		t.Errorf("the server info has the version %q, want v9.8.7", got)
	}
}
