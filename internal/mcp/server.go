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
	server := sdk.NewServer(&sdk.Implementation{Name: "lifeos", Version: "0.1.0"}, &sdk.ServerOptions{Instructions: instructions})
	registerItems(server, svc)
	registerRecords(server, svc)
	registerJournal(server, svc)
	registerTodos(server, svc)
	registerHabits(server, svc)
	registerCapture(server, svc)
	registerSearch(server, svc)
	registerQuery(server, svc)
	registerWiki(server, svc)
	return server
}

// Handler serves the lifeos MCP server over streamable HTTP. It keeps no
// session: each tool call is one request. A session would hold a stream
// open, and that stream would block the shutdown of the HTTP server.
//
// Each request needs a token that access accepts. The MCP library refuses
// a request to a loopback address that names a different host, as a guard
// against a web page that reaches a local server. That guard is off: the
// tunnel sends each request with the public hostname, and no web page can
// make a token of Cloudflare Access.
func Handler(svc *service.Service, access *AccessCheck) http.Handler {
	server := NewServer(svc)
	return access.Require(sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true},
	))
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
