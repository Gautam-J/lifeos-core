package api

import (
	"bytes"
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type ingestInput struct {
	Body struct {
		Text  string `json:"text,omitempty" doc:"A text. It is stored exactly as it is written."`
		URL   string `json:"url,omitempty" doc:"A link that starts with http:// or https://. LifeOS stores the link; it does not get the page."`
		Title string `json:"title,omitempty" doc:"A label for the capture"`
	}
}

type ingestFileInput struct {
	Name  string `query:"name" doc:"The name of the file, such as policy.pdf"`
	Title string `query:"title" doc:"A label for the capture. The default is the name of the file."`
	// RawBody is the file. The service finds the type of the file in its
	// bytes, so the Content-Type header can have any value.
	RawBody []byte `contentType:"application/octet-stream"`
}

type sourceOutput struct {
	Body service.Capture
}

func registerSources(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "ingest",
		Method:      http.MethodPost,
		Path:        "/api/ingest",
		Summary:     "Capture a text or a link",
		Description: "Stores the text or the link as a source. Give one of text and url. The same content a second time returns the source that exists.",
	}, func(ctx context.Context, in *ingestInput) (*sourceOutput, error) {
		// A request to the REST API comes from Gautam's own devices.
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Text:   in.Body.Text,
			URL:    in.Body.URL,
			Title:  in.Body.Title,
			Source: service.SourceManual,
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &sourceOutput{Body: capture}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ingest-file",
		Method:      http.MethodPost,
		Path:        "/api/ingest/file",
		Summary:     "Capture a file",
		Description: "Stores the body of the request as a file in the file store. The same file a second time returns the source that exists.",
		// Huma answers 413 for a body of this size or more, so the limit
		// is one byte above the largest file.
		MaxBodyBytes: svc.MaxFileBytes() + 1,
	}, func(ctx context.Context, in *ingestFileInput) (*sourceOutput, error) {
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Upload:   bytes.NewReader(in.RawBody),
			FileName: in.Name,
			Title:    in.Title,
			Source:   service.SourceManual,
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &sourceOutput{Body: capture}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-source",
		Method:      http.MethodGet,
		Path:        "/api/sources/{id}",
		Summary:     "Read one source",
	}, func(ctx context.Context, in *idInput) (*sourceOutput, error) {
		capture, err := svc.GetCapture(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &sourceOutput{Body: capture}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-source",
		Method:        http.MethodDelete,
		Path:          "/api/sources/{id}",
		Summary:       "Delete a source",
		Description:   "Marks the source as deleted. The row and the file stay.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteCapture(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
