package mcp

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addItemInput struct {
	Kind       string         `json:"kind" jsonschema:"One of movie, book, place, game. If the kind is not clear, ask the user. Never guess."`
	Title      string         `json:"title" jsonschema:"Title or name of the item"`
	Rating     *float64       `json:"rating,omitempty" jsonschema:"The user's rating, 0 to 5 in steps of 0.5"`
	OccurredOn string         `json:"occurred_on,omitempty" jsonschema:"Day watched, read, visited or played, as YYYY-MM-DD in India Standard Time"`
	Attrs      map[string]any `json:"attrs,omitempty" jsonschema:"More fields for this kind, such as author, director or year"`
	Dictated   bool           `json:"dictated,omitempty" jsonschema:"true only when the user stated this item directly. false when you inferred it from a source; the row then waits in the review queue."`
}

type getInput struct {
	Type string `json:"type" jsonschema:"Record type. The only type is: item"`
	ID   int64  `json:"id" jsonschema:"Record ID"`
}

func registerItems(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "add_item",
		Description: "Add a movie, book, place or game to the user's lists. Returns the stored item with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in addItemInput) (*sdk.CallToolResult, service.Item, error) {
		item, err := svc.AddItem(ctx, service.AddItemParams{
			Kind:       in.Kind,
			Title:      in.Title,
			Rating:     in.Rating,
			OccurredOn: in.OccurredOn,
			Attrs:      in.Attrs,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
		if err != nil {
			return nil, service.Item{}, toToolError(err)
		}
		return nil, item, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get",
		Description: "Read one record by type and ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in getInput) (*sdk.CallToolResult, service.Item, error) {
		if in.Type != "item" {
			return nil, service.Item{}, errors.New("invalid input: type: must be item")
		}
		item, err := svc.GetItem(ctx, in.ID)
		if err != nil {
			return nil, service.Item{}, toToolError(err)
		}
		return nil, item, nil
	})
}
