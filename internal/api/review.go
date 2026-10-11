package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type reviewQueueInput struct {
	Limit int `query:"limit" doc:"1 to 200. The default is 50."`
}

type reviewQueueOutput struct {
	Body struct {
		Entries []service.ReviewEntry `json:"entries"`
	}
}

type verifyInput struct {
	Body struct {
		Records []service.RecordRef `json:"records" doc:"The records to approve, each with its type and ID"`
	}
}

type verifyOutput struct {
	Body struct {
		Verified int `json:"verified" doc:"The number of records that changed"`
	}
}

func registerReview(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "review-queue",
		Method:      http.MethodGet,
		Path:        "/api/review",
		Summary:     "List the records that wait for approval, oldest first",
	}, func(ctx context.Context, in *reviewQueueInput) (*reviewQueueOutput, error) {
		entries, err := svc.ReviewQueue(ctx, in.Limit)
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &reviewQueueOutput{}
		out.Body.Entries = entries
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "verify",
		Method:      http.MethodPost,
		Path:        "/api/review/verify",
		Summary:     "Approve records",
		Description: "Marks every record in the body as verified, or none when one record does not exist.",
	}, func(ctx context.Context, in *verifyInput) (*verifyOutput, error) {
		n, err := svc.Verify(ctx, in.Body.Records, service.SourceManual)
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		out := &verifyOutput{}
		out.Body.Verified = n
		return out, nil
	})
}
