package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addTodoInput struct {
	Body struct {
		Title   string `json:"title"`
		DueOn   string `json:"due_on,omitempty" doc:"The day that the todo is due, as YYYY-MM-DD"`
		Project string `json:"project,omitempty" doc:"The project of the todo, such as lifeos"`
	}
}

type todoOutput struct {
	Body service.Todo
}

type listTodosInput struct {
	Status string `query:"status" doc:"One of open, done, dropped. Empty lists every status."`
	Limit  int    `query:"limit" doc:"1 to 200. The default is 50."`
}

type listTodosOutput struct {
	Body struct {
		Todos []service.Todo `json:"todos"`
	}
}

type updateTodoInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Title   *string  `json:"title,omitempty"`
		Status  *string  `json:"status,omitempty" doc:"One of open, done, dropped"`
		DueOn   *string  `json:"due_on,omitempty" doc:"The day that the todo is due, as YYYY-MM-DD"`
		Project *string  `json:"project,omitempty"`
		Clear   []string `json:"clear,omitempty" doc:"Optional fields to empty: due_on, project"`
	}
	// RawBody is the body as it came. Body cannot tell a null from a
	// field that is absent, and the service refuses a null.
	RawBody []byte
}

func registerTodos(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "add-todo",
		Method:        http.MethodPost,
		Path:          "/api/todos",
		Summary:       "Add a todo",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addTodoInput) (*todoOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual and verified.
		todo, err := svc.AddTodo(ctx, service.AddTodoParams{
			Title:      in.Body.Title,
			DueOn:      in.Body.DueOn,
			Project:    in.Body.Project,
			Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &todoOutput{Body: todo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-todos",
		Method:      http.MethodGet,
		Path:        "/api/todos",
		Summary:     "List todos: the open todos first, then the newest first",
	}, func(ctx context.Context, in *listTodosInput) (*listTodosOutput, error) {
		todos, err := svc.ListTodos(ctx, service.ListTodosParams{Status: in.Status, Limit: in.Limit})
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &listTodosOutput{}
		out.Body.Todos = todos
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-todo",
		Method:      http.MethodGet,
		Path:        "/api/todos/{id}",
		Summary:     "Read one todo",
	}, func(ctx context.Context, in *idInput) (*todoOutput, error) {
		todo, err := svc.GetTodo(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &todoOutput{Body: todo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-todo",
		Method:      http.MethodPatch,
		Path:        "/api/todos/{id}",
		Summary:     "Change a todo, or mark it done",
		Description: "Changes the fields in the body. A field that is not in the body keeps its value.",
	}, func(ctx context.Context, in *updateTodoInput) (*todoOutput, error) {
		todo, err := patch[service.Todo](ctx, svc, "todo", in.ID, in.RawBody, in.Body.Clear)
		if err != nil {
			return nil, err
		}
		return &todoOutput{Body: todo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-todo",
		Method:        http.MethodDelete,
		Path:          "/api/todos/{id}",
		Summary:       "Delete a todo",
		Description:   "Marks the todo as deleted. The row stays in the database.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteTodo(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
