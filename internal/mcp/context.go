package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type contextInput struct{}

func registerContext(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "context",
		Description: "Call this first in each conversation with the user about their own life. It returns me.md, a short page about the user: who they are, what they work on now, their projects, the people who come up most, and how they want you to work. Use it to match a name in a message to a project or a person, and to avoid a question that the page answers. When the page is missing, the result says how to make it. Do not change the page without a yes of the user in this conversation.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ contextInput) (*sdk.CallToolResult, service.SessionContext, error) {
		got, err := svc.Context(ctx)
		if err != nil {
			return nil, service.SessionContext{}, toToolError(err)
		}
		return nil, got, nil
	})
}
