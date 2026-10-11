package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type journalInput struct {
	Body      string `json:"body" jsonschema:"The entry, word for word as the user wrote it. Do not correct, shorten, summarize or translate it."`
	EntryDate string `json:"entry_date,omitempty" jsonschema:"The day that the entry is about, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
}

func registerJournal(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "journal",
		Description: "Store a journal entry. Use it only for text that the user gives you for their journal. An entry holds the user's own words: never write an entry yourself, and never put your analysis in it. Returns the stored entry with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in journalInput) (*sdk.CallToolResult, service.JournalEntry, error) {
		// The words are the user's own, so the row is verified.
		entry, err := svc.AddJournalEntry(ctx, service.AddJournalEntryParams{
			Body:       in.Body,
			EntryDate:  in.EntryDate,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: true},
		})
		if err != nil {
			return nil, service.JournalEntry{}, toToolError(err)
		}
		return nil, entry, nil
	})
}
