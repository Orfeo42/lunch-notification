package gsheets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/orfeo42/lunch-notification/internal/meal"
)

func layout() [][]string {
	return [][]string{
		{"Pasto", "sabato", "domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì"},
		{"Data"},
		{"Pranzo", "", "patate arrosto + sovracosce", "polpettone", "polpettone", "Pollo", "Tomino", "frittata"},
		{"Cena", "fuori", "pasta ricotta zucchine", "minestrone", "minestrone", "pesce/brodino", "pesce/brodino", "zelli"},
	}
}

func TestNew(t *testing.T) {
	t.Run("empty sheet id returns ErrMissingSheetID", func(t *testing.T) {
		src, err := New(&sheets.Service{}, "", "Menù!A1:H4")
		require.ErrorIs(t, err, ErrMissingSheetID)
		assert.Nil(t, src)
	})

	t.Run("valid args returns source", func(t *testing.T) {
		src, err := New(&sheets.Service{}, "abc", "Menù!A1:H4")
		require.NoError(t, err)
		assert.NotNil(t, src)
	})
}

func TestParsePlan(t *testing.T) {
	monday := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

	t.Run("monday returns lunch and dinner", func(t *testing.T) {
		plan, err := parsePlan(layout(), monday)
		require.NoError(t, err)
		assert.Equal(t, monday, plan.Day)
		assert.Equal(t, []meal.Meal{
			{Kind: meal.Lunch, Dish: "polpettone"},
			{Kind: meal.Dinner, Dish: "minestrone"},
		}, plan.Meals)
	})

	t.Run("saturday keeps empty lunch and literal fuori", func(t *testing.T) {
		plan, err := parsePlan(layout(), saturday)
		require.NoError(t, err)
		assert.Equal(t, []meal.Meal{
			{Kind: meal.Lunch, Dish: ""},
			{Kind: meal.Dinner, Dish: "fuori"},
		}, plan.Meals)
	})

	t.Run("short row yields empty dish", func(t *testing.T) {
		rows := [][]string{
			{"Pasto", "sabato", "domenica", "lunedì"},
			{"Pranzo", "x"},
		}
		plan, err := parsePlan(rows, monday)
		require.NoError(t, err)
		assert.Equal(t, []meal.Meal{{Kind: meal.Lunch, Dish: ""}}, plan.Meals)
	})

	t.Run("header without weekday returns ErrWeekdayNotFound", func(t *testing.T) {
		rows := [][]string{
			{"Pasto", "sabato", "domenica"},
			{"Pranzo", "a", "b"},
		}
		_, err := parsePlan(rows, monday)
		require.ErrorIs(t, err, ErrWeekdayNotFound)
		assert.Contains(t, err.Error(), "lunedì")
	})

	t.Run("no header row returns ErrWeekdayNotFound", func(t *testing.T) {
		_, err := parsePlan([][]string{{"Pranzo", "a"}}, monday)
		require.ErrorIs(t, err, ErrWeekdayNotFound)
	})

	t.Run("Data row is skipped", func(t *testing.T) {
		plan, err := parsePlan(layout(), monday)
		require.NoError(t, err)
		assert.Len(t, plan.Meals, 2)
	})

	t.Run("header and labels tolerate case and whitespace", func(t *testing.T) {
		rows := [][]string{
			{"Pasto", " Sabato ", " LUNEDÌ "},
			{" pranzo ", "a", "  zuppa  "},
			{"CENA ", "b", " riso "},
		}
		plan, err := parsePlan(rows, monday)
		require.NoError(t, err)
		assert.Equal(t, []meal.Meal{
			{Kind: meal.Lunch, Dish: "zuppa"},
			{Kind: meal.Dinner, Dish: "riso"},
		}, plan.Meals)
	})
}

func newTestSource(t *testing.T, handler http.HandlerFunc) *Source {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	svc, err := sheets.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	src, err := New(svc, "sheet1", "Menù!A1:H4")
	require.NoError(t, err)
	return src
}

func TestDayPlan(t *testing.T) {
	monday := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	t.Run("fetches range and parses plan", func(t *testing.T) {
		var gotPath string
		src := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.EscapedPath()
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"range":"Menù!A1:H4","majorDimension":"ROWS","values":[
				["Pasto","sabato","domenica","lunedì","martedì","mercoledì","giovedì","venerdì"],
				["Data"],
				["Pranzo","","patate arrosto + sovracosce","polpettone","polpettone","Pollo","Tomino","frittata"],
				["Cena","fuori","pasta ricotta zucchine","minestrone","minestrone","pesce/brodino","pesce/brodino","zelli"]]}`))
			assert.NoError(t, err)
		})

		plan, err := src.DayPlan(context.Background(), monday)
		require.NoError(t, err)
		assert.Equal(t, "/v4/spreadsheets/sheet1/values/Men%C3%B9%21A1%3AH4", gotPath)
		assert.Equal(t, []meal.Meal{
			{Kind: meal.Lunch, Dish: "polpettone"},
			{Kind: meal.Dinner, Dish: "minestrone"},
		}, plan.Meals)
	})

	t.Run("http 500 returns error", func(t *testing.T) {
		src := newTestSource(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		})

		_, err := src.DayPlan(context.Background(), monday)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get sheet values")
	})

	t.Run("missing weekday returns ErrWeekdayNotFound", func(t *testing.T) {
		src := newTestSource(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"values":[["Pasto","sabato"],["Pranzo","x"]]}`))
			assert.NoError(t, err)
		})

		_, err := src.DayPlan(context.Background(), monday)
		require.ErrorIs(t, err, ErrWeekdayNotFound)
	})
}
