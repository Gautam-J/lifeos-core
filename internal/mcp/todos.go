package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type todoInput struct {
	Title      string `json:"title" jsonschema:"One action, in a few words"`
	DueOn      string `json:"due_on,omitempty" jsonschema:"The day that the todo is due, as YYYY-MM-DD in India Standard Time. Leave it out when the user gave no date."`
	Project    string `json:"project,omitempty" jsonschema:"The project of the todo, such as lifeos. Leave it out when it is not clear."`
	JournalRef int64  `json:"journal_ref,omitempty" jsonschema:"The ID of the journal entry that reports this, when you take it from an entry"`
}

func registerTodos(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "todo",
		Description: "Add one todo. Use it when the user names something that they must do in their own life, with or without a date, also when the message has no keyword such as \"todo\". When a journal entry names the task, give the ID of the entry as journal_ref. For a request that has more than one action, call this tool one time for each action. To change a todo or to mark it done, use update with the type todo. Returns the stored todo with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in todoInput) (*sdk.CallToolResult, service.Todo, error) {
		todo, err := svc.AddTodo(ctx, service.AddTodoParams{
			Title:      in.Title,
			DueOn:      in.DueOn,
			Project:    in.Project,
			JournalRef: in.JournalRef,
			Provenance: service.Provenance{Source: service.SourceAgent},
		})
		if err != nil {
			return nil, service.Todo{}, toToolError(err)
		}
		return nil, todo, nil
	})
}
