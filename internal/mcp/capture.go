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

func registerCapture(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "capture",
		Description: "Keep a text, a link or a file exactly as it is, as a source. Give one of text, url and path. Use it for material that the user wants to keep, such as an article, a note or a document. Do not use it for a journal entry (use journal) or for one fact (use add_item, todo or log_habit). Never send a password, a card number or an ID number: LifeOS refuses them. The same content a second time returns the source that exists. When you take a record from a source, give the ID of the source as source_ref to add_item, todo or log_habit. Returns the stored source with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in captureInput) (*sdk.CallToolResult, service.Capture, error) {
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Text:      in.Text,
			URL:       in.URL,
			InboxPath: in.Path,
			Title:     in.Title,
			Source:    service.SourceAgent,
		})
		if err != nil {
			return nil, service.Capture{}, toToolError(err)
		}
		return nil, capture, nil
	})
}
