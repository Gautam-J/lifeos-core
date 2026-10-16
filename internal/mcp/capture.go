package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type captureInput struct {
	Text  string `json:"text,omitempty" jsonschema:"A text to keep, word for word. Do not correct, shorten or summarize it."`
	URL   string `json:"url,omitempty" jsonschema:"A link that starts with http:// or https://. LifeOS stores the link; it does not get the page."`
	Path  string `json:"path,omitempty" jsonschema:"The path of a file in the inbox folder on the Pi. LifeOS moves the file into its file store."`
	Title string `json:"title,omitempty" jsonschema:"A short label for the capture. A file with no title gets its file name."`
}

// captureOutput is the stored source and the next step of the agent.
type captureOutput struct {
	service.Capture
	Next string `json:"next" jsonschema:"What you do after this call."`
}

type markProcessedInput struct {
	ID int64 `json:"id" jsonschema:"The ID of the source."`
}

func registerCapture(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "capture",
		Description: "Keep a text, a link or a file exactly as it is, as a source. Give one of text, url and path. Use it for material that the user wants to keep, such as an article, a note or a document. Do not use it for a journal entry (use journal) or for one fact (use add_item, todo or log_habit). Never send a password, a card number or an ID number: LifeOS refuses them. The same content a second time returns the source that exists. Returns the stored source with its ID, and the next step: how to work the source into the wiki.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in captureInput) (*sdk.CallToolResult, captureOutput, error) {
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Text:      in.Text,
			URL:       in.URL,
			InboxPath: in.Path,
			Title:     in.Title,
			Source:    service.SourceAgent,
		})
		if err != nil {
			return nil, captureOutput{}, toToolError(err)
		}
		return nil, captureOutput{Capture: capture, Next: captureNext(capture.ID, capture.ProcessedAt != nil)}, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "mark_processed",
		Description: "Record that the work on one source is complete: its facts are in records and its ideas are in the wiki, or the user said that nothing is to be taken from it. Call it as the last step of the work on a source. It changes no content of the source. To list the sources that wait, use query: SELECT id, kind, title FROM sources WHERE processed_at IS NULL AND deleted_at IS NULL ORDER BY id. Returns the source.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in markProcessedInput) (*sdk.CallToolResult, service.Capture, error) {
		capture, err := svc.MarkProcessed(ctx, in.ID, service.SourceAgent)
		if err != nil {
			return nil, service.Capture{}, toToolError(err)
		}
		return nil, capture, nil
	})
}
