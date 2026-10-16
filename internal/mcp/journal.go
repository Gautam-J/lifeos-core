package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type journalInput struct {
	Body      string `json:"body" jsonschema:"The entry, word for word as the user wrote it. Do not correct, shorten, summarize or translate it."`
	EntryDate string `json:"entry_date,omitempty" jsonschema:"The day that the entry is about, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
	entitiesInput
}

// journalOutput is the stored entry and the next step of the agent.
type journalOutput struct {
	service.JournalEntry
	entitiesOutput
	Next string `json:"next" jsonschema:"What you do after this call."`
}

func registerJournal(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "journal",
		Description: "Store a journal entry: what the user did, saw, felt or thought. Use it when a message tells about the user's own life, also when the message has no keyword such as \"journal\". Send the words unchanged. An entry holds the user's own words: never write an entry yourself, and never put your analysis in it. Returns the stored entry with its ID, and the next step: how to connect the entry to the wiki." + entitiesHelp,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in journalInput) (*sdk.CallToolResult, journalOutput, error) {
		entry, err := svc.AddJournalEntry(ctx, service.AddJournalEntryParams{
			Body:       in.Body,
			EntryDate:  in.EntryDate,
			Provenance: service.Provenance{Source: service.SourceAgent},
		})
		if err != nil {
			return nil, journalOutput{}, toToolError(err)
		}
		return nil, journalOutput{JournalEntry: entry, entitiesOutput: linkEntities(ctx, svc, "journal", entry.ID, in.Entities), Next: journalNext(entry)}, nil
	})
}
