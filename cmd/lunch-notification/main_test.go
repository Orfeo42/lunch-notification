package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"

	"github.com/orfeo42/lunch-notification/internal/config"
	"github.com/orfeo42/lunch-notification/internal/gsheets"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestRun(t *testing.T) {
	t.Run("unknown command returns ErrUnknownCommand", func(t *testing.T) {
		err := run(context.Background(), []string{"bogus"})
		require.ErrorIs(t, err, ErrUnknownCommand)
	})

	t.Run("run-once with missing credentials file returns not-exist error", func(t *testing.T) {
		t.Setenv("GOOGLE_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "missing.json"))
		err := run(context.Background(), []string{"run-once"})
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.NotErrorIs(t, err, ErrUnknownCommand)
	})
}

func TestNewPlanSource(t *testing.T) {
	t.Run("unknown source returns ErrUnknownSource", func(t *testing.T) {
		_, err := newPlanSource(context.Background(), config.Config{Source: "nope"}, http.DefaultClient)
		require.ErrorIs(t, err, ErrUnknownSource)
	})

	t.Run("gsheets with empty sheet id returns ErrMissingSheetID", func(t *testing.T) {
		_, err := newPlanSource(context.Background(), config.Config{Source: "gsheets"}, http.DefaultClient)
		require.ErrorIs(t, err, gsheets.ErrMissingSheetID)
	})
}

func TestNewTaskWriter(t *testing.T) {
	t.Run("unknown tasks backend returns ErrUnknownTasks", func(t *testing.T) {
		_, err := newTaskWriter(context.Background(), config.Config{Tasks: "nope"}, http.DefaultClient)
		require.ErrorIs(t, err, ErrUnknownTasks)
	})
}

func TestLogRunError(t *testing.T) {
	t.Run("google 5xx logs warning", func(t *testing.T) {
		buf := captureLogs(t)
		logRunError(context.Background(), fmt.Errorf("wrap: %w", &googleapi.Error{Code: 503}))
		assert.Contains(t, buf.String(), `"level":"WARN"`)
		assert.Contains(t, buf.String(), "run failed upstream")
	})

	t.Run("google 403 logs error", func(t *testing.T) {
		buf := captureLogs(t)
		logRunError(context.Background(), fmt.Errorf("wrap: %w", &googleapi.Error{Code: 403}))
		assert.Contains(t, buf.String(), `"level":"ERROR"`)
		assert.Contains(t, buf.String(), "run failed")
	})

	t.Run("url error logs warning", func(t *testing.T) {
		buf := captureLogs(t)
		logRunError(context.Background(), &url.Error{Op: "Get", URL: "http://x", Err: errors.New("boom")})
		assert.Contains(t, buf.String(), `"level":"WARN"`)
	})

	t.Run("deadline exceeded logs warning", func(t *testing.T) {
		buf := captureLogs(t)
		logRunError(context.Background(), fmt.Errorf("wrap: %w", context.DeadlineExceeded))
		assert.Contains(t, buf.String(), `"level":"WARN"`)
	})

	t.Run("other error logs error", func(t *testing.T) {
		buf := captureLogs(t)
		logRunError(context.Background(), errors.New("boom"))
		assert.Contains(t, buf.String(), `"level":"ERROR"`)
	})
}
