package mcp

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type queryInput struct {
	SQL string `json:"sql" jsonschema:"One SELECT statement for PostgreSQL, with no parameter. Give the columns that you need and a LIMIT."`
}

// queryDescription names each table. A test checks it against the tables
// of the database, so a new table needs a new name here.
const queryDescription = "Run one read-only SQL statement on the user's data. Use it for a question that needs a count, a sum, a range of dates or a join, such as \"how many books did I read this year\" or \"on which days of last week did I go to the gym\". To find a record by a word, use search. " +
	"The tables are: catalog_items (the lists: kind, title, rating, occurred_on, attrs), journal_entries (body, entry_date), todos (title, status, due_on, project, done_at), habits (name, active), habit_logs (habit_id, logged_on), sources (kind, title, content, url), events (the log of every write) and goose_db_version (the migrations). For the columns of a table, read information_schema.columns. " +
	"Each record table has id, source, verified, created_at, updated_at and deleted_at. Add deleted_at IS NULL to leave out the deleted rows. Add verified to count only the rows that the user approved. A date is a day in India Standard Time; a timestamp is in UTC. " +
	"Returns the names of the columns and at most 200 rows, with each value as text. When truncated is true, the statement has more rows: count or sum in SQL, or add a LIMIT. The connection can only read: a statement that changes data fails. To change a record, use update or delete."

type queryOutput = service.QueryResult

func registerQuery(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "query",
		Description: queryDescription,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in queryInput) (*sdk.CallToolResult, queryOutput, error) {
		result, err := svc.Query(ctx, in.SQL)
		// The agent wrote the statement, so the agent corrects it. The
		// text of toToolError tells the agent to ask the user.
		var invalid *service.ValidationError
		if errors.As(err, &invalid) {
			return nil, queryOutput{}, errors.New(invalid.Error() + ". Correct the statement, then call query again.")
		}
		if err != nil {
			return nil, queryOutput{}, toToolError(err)
		}
		return nil, result, nil
	})
}
