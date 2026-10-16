package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type logHabitInput struct {
	Habit      string `json:"habit" jsonschema:"The name of one of the user's habits, such as gym. If no habit has this name, the error lists the habits: ask the user which one they mean."`
	LoggedOn   string `json:"logged_on,omitempty" jsonschema:"The day that the user did the habit, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
	JournalRef int64  `json:"journal_ref,omitempty" jsonschema:"The ID of the journal entry that reports this, when you take it from an entry"`
	entitiesInput
}

// habitLogOutput is the stored log and the links that the call made.
type habitLogOutput struct {
	service.HabitLog
	entitiesOutput
}

func registerHabits(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "log_habit",
		Description: "Record that the user did a habit on a day. Use it when the user says that they did one of their habits in their own life, such as a run or a session of practice, also when the message has no keyword. When a journal entry reports the habit, give the ID of the entry as journal_ref. A habit has one log for each day: a second call for the same day stores no second log. This tool does not add a new habit; the user adds a habit in the admin pages. To remove a log, use delete with the type habit_log. Returns the stored log with its ID." + entitiesHelp,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in logHabitInput) (*sdk.CallToolResult, habitLogOutput, error) {
		log, err := svc.LogHabit(ctx, service.LogHabitParams{
			Habit:      in.Habit,
			LoggedOn:   in.LoggedOn,
			JournalRef: in.JournalRef,
			Provenance: service.Provenance{Source: service.SourceAgent},
		})
		if err != nil {
			return nil, habitLogOutput{}, toToolError(err)
		}
		return nil, habitLogOutput{HabitLog: log, entitiesOutput: linkEntities(ctx, svc, "habit_log", log.ID, in.Entities)}, nil
	})
}
