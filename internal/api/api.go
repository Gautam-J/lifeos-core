// Package api is the REST adapter. Handlers read the request, call one
// service function and write the result. They hold no logic.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/version"
)

// Register adds every /api route to router.
func Register(router chi.Router, svc *service.Service) {
	config := huma.DefaultConfig("LifeOS API", version.Version)
	config.DocsPath = "/api/docs"
	config.OpenAPIPath = "/api/openapi"
	config.SchemasPath = "/api/schemas"
	api := humachi.New(router, config)

	registerItems(api, svc)
	registerItemChanges(api, svc)
	registerJournal(api, svc)
	registerTodos(api, svc)
	registerHabits(api, svc)
	registerSources(api, svc)
	registerSearch(api, svc)
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

// patch sends the body of a PATCH request to service.Update and returns
// the record after the change. raw is the body as it came: only a map of
// fields can tell a null from a field that is absent, and the service
// refuses a null. T is the struct of the record type.
func patch[T any](ctx context.Context, svc *service.Service, recordType string, id int64, raw []byte, clear []string) (T, error) {
	var zero T
	// Huma checked the names and the types of the body already.
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return zero, huma.Error400BadRequest("the body is not a JSON object")
	}
	delete(fields, "clear")
	// A request to the REST API comes from Gautam's own devices, so the
	// change is his.
	record, err := svc.Update(ctx, recordType, id, service.UpdateParams{
		Fields: fields,
		Clear:  clear,
		Actor:  service.SourceManual,
	})
	if err != nil {
		return zero, toHTTPError("body", err)
	}
	return record.(T), nil
}

// idInput is the input of a route that has only the record ID in its path.
type idInput struct {
	ID int64 `path:"id"`
}
