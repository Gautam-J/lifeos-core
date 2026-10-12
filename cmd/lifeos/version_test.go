package main

import (
	"bytes"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/version"
)

// A bug report names the release, so the binary must tell which one it is.
func TestVersionCommandPrintsTheRelease(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v9.8.7"

	var out bytes.Buffer
	writeVersion(&out)

	if got, want := out.String(), "lifeos v9.8.7\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestVersionIsACommand(t *testing.T) {
	for _, c := range commands {
		if c.name == "version" {
			return
		}
	}
	t.Error("lifeos has no command version")
}
