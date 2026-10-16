package main

import (
	"bytes"
	"testing"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
)

// The Claude app does not give the server instructions to the model, so
// Gautam pastes them into the project. The text comes from the binary of
// the release, so the copy does not drift.
func TestInstructionsCommandPrintsTheServerInstructions(t *testing.T) {
	var out bytes.Buffer
	writeInstructions(&out)

	if got, want := out.String(), lifeosmcp.Instructions()+"\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestInstructionsIsACommand(t *testing.T) {
	for _, c := range commands {
		if c.name == "instructions" {
			return
		}
	}
	t.Error("lifeos has no command instructions")
}
