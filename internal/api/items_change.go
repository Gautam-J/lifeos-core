package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type updateItemInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Kind       *string        `json:"kind,omitempty" doc:"One of movie, book, place, game"`
		Title      *string        `json:"title,omitempty"`
		Rating     *float64       `json:"rating,omitempty" doc:"0 to 5 in steps of 0.5"`
		OccurredOn *string        `json:"occurred_on,omitempty" doc:"Day watched, read, visited or played, as YYYY-MM-DD"`
		Attrs      map[string]any `json:"attrs,omitempty" doc:"Replaces every attr of the item"`
		Clear      []string       `json:"clear,omitempty" doc:"Optional fields to empty: rating, occurred_on"`
	}
	// RawBody is the body as it came. Body cannot tell a null from a
	// field that is absent, and the service refuses a null.
	RawBody []byte
}

func registerItemChanges(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "update-item",
		Method:      http.MethodPatch,
		Path:        "/api/items/{id}",
		Summary:     "Correct a list item",
		Description: "Changes the fields in the body. A field that is not in the body keeps its value.",
	}, func(ctx context.Context, in *updateItemInput) (*itemOutput, error) {
		// Huma checked the names and the types of the body already.
		var fields map[string]any
		if err := json.Unmarshal(in.RawBody, &fields); err != nil {
			return nil, huma.Error400BadRequest("the body is not a JSON object")
		}
		delete(fields, "clear")
		// A request to the REST API comes from Gautam's own devices, so
		// the change is his and the verified flag stays as it is.
		record, err := svc.Update(ctx, "item", in.ID, service.UpdateParams{
			Fields: fields,
			Clear:  in.Body.Clear,
			Actor:  service.SourceManual,
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		// Update returns an Item for the type item.
		return &itemOutput{Body: record.(service.Item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-item",
		Method:        http.MethodDelete,
		Path:          "/api/items/{id}",
		Summary:       "Delete a list item",
		Description:   "Marks the item as deleted. The row stays in the database.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *getItemInput) (*struct{}, error) {
		if err := svc.DeleteItem(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
