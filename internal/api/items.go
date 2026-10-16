package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addItemInput struct {
	Body struct {
		Kind       string         `json:"kind" doc:"One of movie, book, place, game"`
		Title      string         `json:"title"`
		Rating     *float64       `json:"rating,omitempty" doc:"0 to 5 in steps of 0.5"`
		OccurredOn string         `json:"occurred_on,omitempty" doc:"Day watched, read, visited or played, as YYYY-MM-DD"`
		Attrs      map[string]any `json:"attrs,omitempty" doc:"More fields for this kind, such as author or year"`
	}
}

type itemOutput struct {
	Body service.Item
}

type getItemInput struct {
	ID int64 `path:"id"`
}

type listItemsInput struct {
	Kind  string `query:"kind" doc:"One of movie, book, place, game. Empty lists every kind."`
	Limit int    `query:"limit" doc:"1 to 200. The default is 50."`
}

type listItemsOutput struct {
	Body struct {
		Items []service.Item `json:"items"`
	}
}

func registerItems(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "add-item",
		Method:        http.MethodPost,
		Path:          "/api/items",
		Summary:       "Add a list item",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addItemInput) (*itemOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual.
		item, err := svc.AddItem(ctx, service.AddItemParams{
			Kind:       in.Body.Kind,
			Title:      in.Body.Title,
			Rating:     in.Body.Rating,
			OccurredOn: in.Body.OccurredOn,
			Attrs:      in.Body.Attrs,
			Provenance: service.Provenance{Source: service.SourceManual},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &itemOutput{Body: item}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-item",
		Method:      http.MethodGet,
		Path:        "/api/items/{id}",
		Summary:     "Read one list item",
	}, func(ctx context.Context, in *getItemInput) (*itemOutput, error) {
		item, err := svc.GetItem(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &itemOutput{Body: item}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-items",
		Method:      http.MethodGet,
		Path:        "/api/items",
		Summary:     "List items, newest first",
	}, func(ctx context.Context, in *listItemsInput) (*listItemsOutput, error) {
		items, err := svc.ListItems(ctx, service.ListItemsParams{Kind: in.Kind, Limit: in.Limit})
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &listItemsOutput{}
		out.Body.Items = items
		return out, nil
	})
}
