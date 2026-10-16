package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type contextInput struct{}

func registerContext(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "context",
		Description: "Call this first in each conversation with the user about their own life. It returns, in this order: me.md, a short page about the user (who they are, what they work on now, their projects, the people who come up most, how they want you to work); the journal entries of today and yesterday in IST, with ID and time; the open todos due today or before (overdue first), the open todos due tomorrow and the todos done today and yesterday, each with ID and project; and a line of counts. Each row has a ref such as journal:57 or todo:12: cite it, and use the ID to update a row without a search first. Use it to match a name in a message to a project or a person, to close a todo that a message finishes, and to avoid a question that it answers. A section with no rows is left out. A journal body that ends with … is cut: call get for the whole text. When the result says that me.md is missing, it says how to make it. Do not change me.md without a yes of the user in this conversation.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ contextInput) (*sdk.CallToolResult, service.SessionContext, error) {
		got, err := svc.Context(ctx, time.Time{})
		if err != nil {
			return nil, service.SessionContext{}, toToolError(err)
		}
		return nil, got, nil
	})
}
