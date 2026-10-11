package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type searchInput struct {
	Query string   `json:"query" jsonschema:"One or more words. A record matches when it has every word. A part of a word matches, and upper and lower case are the same. Use few words: each word that you add gives fewer hits."`
	Types []string `json:"types,omitempty" jsonschema:"The record types to look in: item, journal, todo, source. Leave it out to look in every type."`
	Limit int      `json:"limit,omitempty" jsonschema:"1 to 200. The default is 50."`
}

type searchOutput struct {
	Hits []service.SearchHit `json:"hits"`
}

func registerSearch(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "search",
		Description: "Find records by their words: list items, journal entries, todos and sources. Returns the newest hits first. A hit has the type, the ID and one line that says what the record holds; to read the full record, use get. A hit with verified set to false waits for the user's approval: tell the user when you use such a record. Use this tool to find the ID of a record before you call update or delete. For a count, a sum, a list by date or a question about habits, use query.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, searchOutput, error) {
		hits, err := svc.Search(ctx, service.SearchParams{Query: in.Query, Types: in.Types, Limit: in.Limit})
		if err != nil {
			return nil, searchOutput{}, toToolError(err)
		}
		return nil, searchOutput{Hits: hits}, nil
	})
}
