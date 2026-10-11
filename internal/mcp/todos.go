package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type todoInput struct {
	Title    string `json:"title" jsonschema:"One action, in a few words"`
	DueOn    string `json:"due_on,omitempty" jsonschema:"The day that the todo is due, as YYYY-MM-DD in India Standard Time. Leave it out when the user gave no date."`
	Project  string `json:"project,omitempty" jsonschema:"The project of the todo, such as lifeos. Leave it out when it is not clear."`
	Dictated bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated this todo directly. false when you propose it from a journal entry, a document or a different source; the todo then waits in the review queue."`
}

func registerTodos(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "todo",
		Description: "Add one todo. For a request that has more than one action, call this tool one time for each action. To change a todo or to mark it done, use update with the type todo. Returns the stored todo with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in todoInput) (*sdk.CallToolResult, service.Todo, error) {
		todo, err := svc.AddTodo(ctx, service.AddTodoParams{
			Title:      in.Title,
			DueOn:      in.DueOn,
			Project:    in.Project,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
		if err != nil {
			return nil, service.Todo{}, toToolError(err)
		}
		return nil, todo, nil
	})
}
