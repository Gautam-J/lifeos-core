package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addJournalInput struct {
	Body struct {
		Body      string `json:"body" doc:"The entry. It is stored exactly as it is written."`
		EntryDate string `json:"entry_date,omitempty" doc:"The day that the entry is about, as YYYY-MM-DD. The default is today in India Standard Time."`
	}
}

type journalOutput struct {
	Body service.JournalEntry
}

type updateJournalInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Body      *string `json:"body,omitempty" doc:"The entry. It is stored exactly as it is written."`
		EntryDate *string `json:"entry_date,omitempty" doc:"The day that the entry is about, as YYYY-MM-DD"`
	}
	// RawBody is the body as it came. Body cannot tell a null from a
	// field that is absent, and the service refuses a null.
	RawBody []byte
}

func registerJournal(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "add-journal-entry",
		Method:        http.MethodPost,
		Path:          "/api/journal",
		Summary:       "Write a journal entry",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addJournalInput) (*journalOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual.
		entry, err := svc.AddJournalEntry(ctx, service.AddJournalEntryParams{
			Body:       in.Body.Body,
			EntryDate:  in.Body.EntryDate,
			Provenance: service.Provenance{Source: service.SourceManual},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &journalOutput{Body: entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-journal-entry",
		Method:      http.MethodGet,
		Path:        "/api/journal/{id}",
		Summary:     "Read one journal entry",
	}, func(ctx context.Context, in *idInput) (*journalOutput, error) {
		entry, err := svc.GetJournalEntry(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &journalOutput{Body: entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-journal-entry",
		Method:      http.MethodPatch,
		Path:        "/api/journal/{id}",
		Summary:     "Correct a journal entry",
		Description: "Changes the fields in the body. A field that is not in the body keeps its value.",
	}, func(ctx context.Context, in *updateJournalInput) (*journalOutput, error) {
		entry, err := patch[service.JournalEntry](ctx, svc, "journal", in.ID, in.RawBody, nil)
		if err != nil {
			return nil, err
		}
		return &journalOutput{Body: entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-journal-entry",
		Method:        http.MethodDelete,
		Path:          "/api/journal/{id}",
		Summary:       "Delete a journal entry",
		Description:   "Marks the entry as deleted. The row stays in the database.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteJournalEntry(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
