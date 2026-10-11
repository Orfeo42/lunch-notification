package gsheets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"google.golang.org/api/sheets/v4"

	"github.com/orfeo42/lunch-notification/internal/meal"
	"github.com/orfeo42/lunch-notification/internal/reminder"
)

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const (
	ErrMissingSheetID  sentinelError = "missing sheet id"
	ErrWeekdayNotFound sentinelError = "weekday column not found"
)

var weekdayNames = [7]string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"}

type Source struct {
	svc     *sheets.Service
	sheetID string
	rng     string
}

var _ reminder.PlanSource = (*Source)(nil)

func New(svc *sheets.Service, sheetID, rng string) (*Source, error) {
	if sheetID == "" {
		return nil, ErrMissingSheetID
	}
	return &Source{svc: svc, sheetID: sheetID, rng: rng}, nil
}

func (s *Source) DayPlan(ctx context.Context, day time.Time) (meal.DayPlan, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.sheetID, s.rng).Context(ctx).Do()
	if err != nil {
		return meal.DayPlan{}, fmt.Errorf("get sheet values: %w", err)
	}

	rows := make([][]string, 0, len(resp.Values))
	for _, raw := range resp.Values {
		row := make([]string, 0, len(raw))
		for _, cell := range raw {
			row = append(row, fmt.Sprint(cell))
		}
		rows = append(rows, row)
	}

	plan, err := parsePlan(rows, day)
	if err != nil {
		return meal.DayPlan{}, err
	}

	slog.InfoContext(ctx, "menu loaded",
		slog.String("weekday", weekdayNames[day.Weekday()]),
		slog.Int("meals", len(plan.Meals)),
	)
	return plan, nil
}

func parsePlan(rows [][]string, day time.Time) (meal.DayPlan, error) {
	name := weekdayNames[day.Weekday()]

	headerIdx := -1
	col := -1
	for i, row := range rows {
		isHeader := false
		for j, cell := range row {
			normalized := strings.ToLower(strings.TrimSpace(cell))
			if slices.Contains(weekdayNames[:], normalized) {
				isHeader = true
			}
			if normalized == name {
				col = j
			}
		}
		if isHeader {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 || col < 0 {
		return meal.DayPlan{}, fmt.Errorf("%w: %s", ErrWeekdayNotFound, name)
	}

	var meals []meal.Meal
	for _, row := range rows[headerIdx+1:] {
		if len(row) == 0 {
			continue
		}
		kind, err := meal.ParseKind(row[0])
		if errors.Is(err, meal.ErrUnknownKind) {
			continue
		}
		if err != nil {
			return meal.DayPlan{}, fmt.Errorf("parse meal kind: %w", err)
		}
		dish := ""
		if col < len(row) {
			dish = strings.TrimSpace(row[col])
		}
		meals = append(meals, meal.Meal{Kind: kind, Dish: dish})
	}

	return meal.DayPlan{Day: day, Meals: meals}, nil
}
