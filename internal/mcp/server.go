// Package mcp is the MCP adapter. Each tool reads its input, calls one
// service function and returns the result. Tools hold no logic; they carry
// the descriptions that the model reads.
package mcp

import (
	"errors"
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// NewServer returns the MCP server named lifeos with every tool registered.
func NewServer(svc *service.Service) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "lifeos", Version: "0.1.0"}, nil)
	registerItems(server, svc)
	registerRecords(server, svc)
	registerJournal(server, svc)
	registerTodos(server, svc)
	registerHabits(server, svc)
	registerCapture(server, svc)
	registerSearch(server, svc)
	registerQuery(server, svc)
	return server
}

// Handler serves the lifeos MCP server over streamable HTTP. It keeps no
// session: each tool call is one request. A session would hold a stream
// open, and that stream would block the shutdown of the HTTP server.
func Handler(svc *service.Service) http.Handler {
	server := NewServer(svc)
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true},
	)
}

// toToolError turns a service error into the error text that the model
// reads. It logs an unknown error and hides its text.
func toToolError(err error) error {
	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		return errors.New("invalid input: " + invalid.Error() + ". Ask the user; do not guess a value.")
	}
	if errors.Is(err, service.ErrNotFound) {
		return errors.New("not found")
	}
	slog.Error("mcp: tool failed", "error", err)
	return errors.New("internal error")
}
