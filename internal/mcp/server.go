// Package mcp is the MCP adapter. Each tool reads its input, calls one
// service function and returns the result. Tools hold no logic; they carry
// the descriptions that the model reads.
package mcp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/version"
)

// NewServer returns the MCP server named lifeos with every tool registered.
// Its instructions hold the text of me.md as it is when NewServer is called,
// so Handler makes a new server for each request.
func NewServer(svc *service.Service) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "lifeos", Version: version.Version}, &sdk.ServerOptions{Instructions: sessionInstructions(context.Background(), svc)})
	registerItems(server, svc)
	registerRecords(server, svc)
	registerJournal(server, svc)
	registerTodos(server, svc)
	registerHabits(server, svc)
	registerCapture(server, svc)
	registerSearch(server, svc)
	registerQuery(server, svc)
	registerWiki(server, svc)
	registerContext(server, svc)
	return server
}

// Handler serves the lifeos MCP server over streamable HTTP. It keeps no
// session: each tool call is one request, and each request gets a new
// server, so that the instructions hold the current text of me.md. A
// session would hold a stream open, and that stream would block the
// shutdown of the HTTP server.
//
// Each request needs a token that access accepts. The MCP library refuses
// a request to a loopback address that names a different host, as a guard
// against a web page that reaches a local server. That guard is off: the
// tunnel sends each request with the public hostname, and a web page that
// reaches the listener has no token of Cloudflare Access. serve.go refuses
// a request that a page of a different site sends through Access.
func Handler(svc *service.Service, access *AccessCheck) http.Handler {
	return access.Require(sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return NewServer(svc) },
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
