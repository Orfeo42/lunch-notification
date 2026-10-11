package gtasks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/api/tasks/v1"

	"github.com/orfeo42/lunch-notification/internal/reminder"
	"github.com/orfeo42/lunch-notification/internal/todo"
)

var _ reminder.TaskWriter = (*Writer)(nil)

type Writer struct {
	svc    *tasks.Service
	listID string
}

func New(svc *tasks.Service, listID string) *Writer {
	return &Writer{svc: svc, listID: listID}
}

func (w *Writer) Ensure(ctx context.Context, task todo.Task) error {
	due := time.Date(task.Due.Year(), task.Due.Month(), task.Due.Day(), 0, 0, 0, 0, time.UTC)
	date := due.Format(time.DateOnly)

	found := false
	err := w.svc.Tasks.List(w.listID).
		DueMin(due.Format(time.RFC3339)).
		DueMax(due.AddDate(0, 0, 1).Format(time.RFC3339)).
		ShowCompleted(true).
		ShowHidden(true).
		Pages(ctx, func(page *tasks.Tasks) error {
			for _, t := range page.Items {
				if t.Title == task.Title && strings.HasPrefix(t.Due, date) {
					found = true
				}
			}
			return nil
		})
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}

	if found {
		slog.InfoContext(ctx, "task already present", slog.String("title", task.Title), slog.String("due", date))
		return nil
	}

	created, err := w.svc.Tasks.Insert(w.listID, &tasks.Task{
		Title: task.Title,
		Notes: task.Notes,
		Due:   due.Format(time.RFC3339),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	slog.InfoContext(ctx, "task created", slog.String("title", task.Title), slog.String("due", date), slog.String("task_id", created.Id))
	return nil
}
