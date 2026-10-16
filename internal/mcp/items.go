package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addItemInput struct {
	Kind       string         `json:"kind" jsonschema:"One of movie, book, place, game. If the kind is not clear, ask the user. Never guess."`
	Title      string         `json:"title" jsonschema:"Title or name of the item"`
	Rating     *float64       `json:"rating,omitempty" jsonschema:"The user's rating, 0 to 5 in steps of 0.5"`
	OccurredOn string         `json:"occurred_on,omitempty" jsonschema:"Day watched, read, visited or played, as YYYY-MM-DD in India Standard Time"`
	Attrs      map[string]any `json:"attrs,omitempty" jsonschema:"More fields for this kind, such as author, director or year"`
	JournalRef int64          `json:"journal_ref,omitempty" jsonschema:"The ID of the journal entry that reports this, when you take it from an entry"`
}

func registerItems(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "add_item",
		Description: "Add a movie, book, place or game to the user's lists. Use it when the user watched, read, visited or played something in their own life, or wants to, also when the message has no keyword. First use search with the title, so that the same item is not added two times. Send a rating only when the user states it, or says yes to a rating that you propose. Returns the stored item with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in addItemInput) (*sdk.CallToolResult, service.Item, error) {
		item, err := svc.AddItem(ctx, service.AddItemParams{
			Kind:       in.Kind,
			Title:      in.Title,
			Rating:     in.Rating,
			OccurredOn: in.OccurredOn,
			Attrs:      in.Attrs,
			JournalRef: in.JournalRef,
			Provenance: service.Provenance{Source: service.SourceAgent},
		})
		if err != nil {
			return nil, service.Item{}, toToolError(err)
		}
		return nil, item, nil
	})
}
