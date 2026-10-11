package reminder

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/orfeo42/lunch-notification/internal/meal"
	"github.com/orfeo42/lunch-notification/internal/todo"
)

type testError string

func (e testError) Error() string { return string(e) }

const (
	errSource testError = "source down"
	errWriter testError = "writer down"
)

func newSource(meals ...meal.Meal) *PlanSourceMock {
	return &PlanSourceMock{
		DayPlanFunc: func(ctx context.Context, day time.Time) (meal.DayPlan, error) {
			return meal.DayPlan{Day: day, Meals: meals}, nil
		},
	}
}

func okWriter() *TaskWriterMock {
	return &TaskWriterMock{
		EnsureFunc: func(ctx context.Context, task todo.Task) error { return nil },
	}
}

func TestRun(t *testing.T) {
	t.Run("tomorrow is next-day midnight across DST boundary", func(t *testing.T) {
		rome, err := time.LoadLocation("Europe/Rome")
		require.NoError(t, err)
		now := time.Date(2026, 10, 24, 21, 0, 0, 0, rome)
		want := time.Date(2026, 10, 25, 0, 0, 0, 0, rome)
		src := newSource(meal.Meal{Kind: meal.Lunch, Dish: "polpettone"})
		dst := okWriter()

		require.NoError(t, New(src, dst).Run(context.Background(), now))

		require.Len(t, src.DayPlanCalls(), 1)
		assert.True(t, want.Equal(src.DayPlanCalls()[0].Day))
		assert.Equal(t, rome, src.DayPlanCalls()[0].Day.Location())
		require.Len(t, dst.EnsureCalls(), 1)
		assert.True(t, want.Equal(dst.EnsureCalls()[0].Task.Due))
	})

	t.Run("two meals produce two tasks", func(t *testing.T) {
		src := newSource(
			meal.Meal{Kind: meal.Lunch, Dish: "polpettone"},
			meal.Meal{Kind: meal.Dinner, Dish: "minestrone"},
		)
		dst := okWriter()

		require.NoError(t, New(src, dst).Run(context.Background(), time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)))

		require.Len(t, dst.EnsureCalls(), 2)
		assert.Equal(t, "Pranzo: polpettone", dst.EnsureCalls()[0].Task.Title)
		assert.Equal(t, "Cena: minestrone", dst.EnsureCalls()[1].Task.Title)
	})

	t.Run("source error is wrapped and no task is written", func(t *testing.T) {
		src := &PlanSourceMock{
			DayPlanFunc: func(ctx context.Context, day time.Time) (meal.DayPlan, error) {
				return meal.DayPlan{}, errSource
			},
		}
		dst := okWriter()

		err := New(src, dst).Run(context.Background(), time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC))

		require.ErrorIs(t, err, errSource)
		assert.Contains(t, err.Error(), "load day plan")
		assert.Empty(t, dst.EnsureCalls())
	})

	t.Run("ensure failure does not stop remaining meals", func(t *testing.T) {
		src := newSource(
			meal.Meal{Kind: meal.Lunch, Dish: "polpettone"},
			meal.Meal{Kind: meal.Dinner, Dish: "minestrone"},
		)
		dst := &TaskWriterMock{
			EnsureFunc: func(ctx context.Context, task todo.Task) error {
				if task.Title == "Pranzo: polpettone" {
					return errWriter
				}
				return nil
			},
		}

		err := New(src, dst).Run(context.Background(), time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC))

		require.ErrorIs(t, err, errWriter)
		assert.Contains(t, err.Error(), "Pranzo: polpettone")
		require.Len(t, dst.EnsureCalls(), 2)
		assert.Equal(t, "Cena: minestrone", dst.EnsureCalls()[1].Task.Title)
	})

	t.Run("no meals returns nil without tasks", func(t *testing.T) {
		src := newSource()
		dst := okWriter()

		require.NoError(t, New(src, dst).Run(context.Background(), time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)))

		assert.Empty(t, dst.EnsureCalls())
	})

	t.Run("dish is kept literally", func(t *testing.T) {
		src := newSource(meal.Meal{Kind: meal.Dinner, Dish: "fuori"})
		dst := okWriter()

		require.NoError(t, New(src, dst).Run(context.Background(), time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)))

		require.Len(t, dst.EnsureCalls(), 1)
		assert.Equal(t, "Cena: fuori", dst.EnsureCalls()[0].Task.Title)
	})
}
