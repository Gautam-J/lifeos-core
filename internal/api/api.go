// Package api is the REST adapter. Handlers read the request, call one
// service function and write the result. They hold no logic.
package api

import (
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// Register adds every /api route to router.
func Register(router chi.Router, svc *service.Service) {
	config := huma.DefaultConfig("LifeOS API", "0.1.0")
	config.DocsPath = "/api/docs"
	config.OpenAPIPath = "/api/openapi"
	config.SchemasPath = "/api/schemas"
	api := humachi.New(router, config)

	registerItems(api, svc)
}

// toHTTPError turns a service error into a Huma error. where is the part of
// the request that held the input: "body" or "query". It logs an unknown
// error and hides its text, because the text can name internals.
func toHTTPError(where string, err error) error {
	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		return huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
			Location: where + "." + invalid.Field,
			Message:  invalid.Message,
		})
	}
	if errors.Is(err, service.ErrNotFound) {
		return huma.Error404NotFound("not found")
	}
	slog.Error("api: request failed", "error", err)
	return huma.Error500InternalServerError("internal error")
}
