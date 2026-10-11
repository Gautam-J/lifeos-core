package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type logHabitInput struct {
	Body struct {
		Habit    string `json:"habit" doc:"The name of an active habit, such as gym"`
		LoggedOn string `json:"logged_on,omitempty" doc:"The day, as YYYY-MM-DD. The default is today in India Standard Time."`
	}
}

type habitLogOutput struct {
	Body service.HabitLog
}

type habitsTodayOutput struct {
	Body service.HabitChecklist
}

func registerHabits(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "log-habit",
		Method:      http.MethodPost,
		Path:        "/api/habits/log",
		Summary:     "Log a habit for a day",
		Description: "A habit has one log for each day. A second request for the same day changes nothing and returns the stored log.",
	}, func(ctx context.Context, in *logHabitInput) (*habitLogOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual and verified.
		log, err := svc.LogHabit(ctx, service.LogHabitParams{
			Habit:      in.Body.Habit,
			LoggedOn:   in.Body.LoggedOn,
			Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &habitLogOutput{Body: log}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "habits-today",
		Method:      http.MethodGet,
		Path:        "/api/habits/today",
		Summary:     "List the active habits with their state for today",
		Description: "A habit is done when it has a verified log for today in India Standard Time.",
	}, func(ctx context.Context, _ *struct{}) (*habitsTodayOutput, error) {
		list, err := svc.HabitsToday(ctx)
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		return &habitsTodayOutput{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-habit-log",
		Method:      http.MethodGet,
		Path:        "/api/habits/logs/{id}",
		Summary:     "Read one habit log",
	}, func(ctx context.Context, in *idInput) (*habitLogOutput, error) {
		log, err := svc.GetHabitLog(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &habitLogOutput{Body: log}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-habit-log",
		Method:        http.MethodDelete,
		Path:          "/api/habits/logs/{id}",
		Summary:       "Delete a habit log",
		Description:   "Marks the log as deleted. The row stays in the database. The habit can get a new log for the same day.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteHabitLog(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
