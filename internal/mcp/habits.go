package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type logHabitInput struct {
	Habit     string `json:"habit" jsonschema:"The name of one of the user's habits, such as gym. If no habit has this name, the error lists the habits: ask the user which one they mean."`
	LoggedOn  string `json:"logged_on,omitempty" jsonschema:"The day that the user did the habit, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
	SourceRef *int64 `json:"source_ref,omitempty" jsonschema:"The ID of the source that this log comes from, when you took the log from a capture"`
	Dictated  bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated directly that they did the habit. false when you inferred it from a journal entry or a different source; the log then waits in the review queue."`
}

func registerHabits(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "log_habit",
		Description: "Record that the user did a habit on a day. Use it when the user says that they did one of their habits in their own life, such as a run or a session of practice, also when the message has no keyword. A habit has one log for each day: a second call for the same day stores no second log. If you proposed a log before and the user now states it, call again with dictated set to true: that approves the log. This tool does not add a new habit; the user adds a habit in the admin pages. To remove a log, use delete with the type habit_log. Returns the stored log with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in logHabitInput) (*sdk.CallToolResult, service.HabitLog, error) {
		log, err := svc.LogHabit(ctx, service.LogHabitParams{
			Habit:      in.Habit,
			LoggedOn:   in.LoggedOn,
			Provenance: service.Provenance{Source: service.SourceAgent, SourceRef: in.SourceRef, Verified: in.Dictated},
		})
		if err != nil {
			return nil, service.HabitLog{}, toToolError(err)
		}
		return nil, log, nil
	})
}
