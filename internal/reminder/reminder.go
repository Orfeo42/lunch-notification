package reminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/orfeo42/lunch-notification/internal/meal"
	"github.com/orfeo42/lunch-notification/internal/todo"
)

//go:generate go tool moq -out reminder_mock.go . PlanSource TaskWriter

type PlanSource interface {
	DayPlan(ctx context.Context, day time.Time) (meal.DayPlan, error)
}

type TaskWriter interface {
	Ensure(ctx context.Context, task todo.Task) error
}

type Service struct {
	src PlanSource
	dst TaskWriter
}

func New(src PlanSource, dst TaskWriter) *Service {
	return &Service{src: src, dst: dst}
}

func (s *Service) Run(ctx context.Context, now time.Time) error {
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	day := tomorrow.Format(time.DateOnly)

	plan, err := s.src.DayPlan(ctx, tomorrow)
	if err != nil {
		return fmt.Errorf("load day plan: %w", err)
	}
	slog.InfoContext(ctx, "day plan loaded", slog.String("day", day), slog.Int("meals", len(plan.Meals)))

	if len(plan.Meals) == 0 {
		slog.InfoContext(ctx, "no meals planned", slog.String("day", day))
		return nil
	}

	var errs []error
	for _, m := range plan.Meals {
		task := todo.Task{Title: m.Kind.Label() + ": " + m.Dish, Due: tomorrow}
		if err := s.dst.Ensure(ctx, task); err != nil {
			errs = append(errs, fmt.Errorf("ensure task %q: %w", task.Title, err))
		}
	}
	return errors.Join(errs...)
}
