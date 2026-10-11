package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type searchInput struct {
	Query string   `query:"query" doc:"One or more words. A record matches when it has every word. A part of a word matches, and upper and lower case are the same."`
	Types []string `query:"types" doc:"The record types to look in, with a comma between them: item, journal, todo, source. Empty looks in every type."`
	Limit int      `query:"limit" doc:"1 to 200. The default is 50."`
}

type searchOutput struct {
	Body struct {
		Hits []service.SearchHit `json:"hits"`
	}
}

func registerSearch(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodGet,
		Path:        "/api/search",
		Summary:     "Find records by their words, the newest first",
		Description: "Looks in list items, journal entries, todos and sources. A hit has the type, the ID and one line that says what the record holds. Read the full record with the GET route of its type.",
	}, func(ctx context.Context, in *searchInput) (*searchOutput, error) {
		hits, err := svc.Search(ctx, service.SearchParams{Query: in.Query, Types: in.Types, Limit: in.Limit})
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &searchOutput{}
		out.Body.Hits = hits
		return out, nil
	})
}
