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
	entitiesInput
}

// todoOutput is the stored todo and the links that the call made.
type todoOutput struct {
	service.Todo
	entitiesOutput
}

func registerTodos(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "todo",
		Description: "Add one todo. Use it when the user names something that they must do in their own life, with or without a date, also when the message has no keyword such as \"todo\". When a journal entry names the task, give the ID of the entry as journal_ref. For a request that has more than one action, call this tool one time for each action. To change a todo or to mark it done, use update with the type todo. Returns the stored todo with its ID." + entitiesHelp + " The project of the todo is the project entity: give it as project, and as an entity only for a person, a place or a topic.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in todoInput) (*sdk.CallToolResult, todoOutput, error) {
		todo, err := svc.AddTodo(ctx, service.AddTodoParams{
			Title:      in.Title,
			DueOn:      in.DueOn,
			Project:    in.Project,
			JournalRef: in.JournalRef,
			Provenance: service.Provenance{Source: service.SourceAgent},
		})
		if err != nil {
			return nil, todoOutput{}, toToolError(err)
		}
		return nil, todoOutput{Todo: todo, entitiesOutput: linkEntities(ctx, svc, "todo", todo.ID, in.Entities)}, nil
	})
}
