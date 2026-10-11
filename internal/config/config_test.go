package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/orfeo42/lunch-notification/internal/schedule"
)

func TestLoad(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, schedule.Clock{Hour: 20, Minute: 0}, cfg.RunAt)
		assert.Equal(t, "Europe/Rome", cfg.Timezone.String())
		assert.Equal(t, "gsheets", cfg.Source)
		assert.Equal(t, "gtasks", cfg.Tasks)
		assert.Equal(t, "Menù!A1:H4", cfg.SheetRange)
		assert.Equal(t, "@default", cfg.TasklistID)
		assert.Equal(t, "/data/credentials.json", cfg.GoogleCredentialsFile)
		assert.Equal(t, "/data/token.json", cfg.GoogleTokenFile)
	})

	t.Run("overrides", func(t *testing.T) {
		t.Setenv("RUN_AT", "07:30")
		t.Setenv("TIMEZONE", "UTC")
		t.Setenv("SHEET_ID", "x")
		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, schedule.Clock{Hour: 7, Minute: 30}, cfg.RunAt)
		assert.Equal(t, "UTC", cfg.Timezone.String())
		assert.Equal(t, "x", cfg.SheetID)
	})

	t.Run("invalid RUN_AT", func(t *testing.T) {
		t.Setenv("RUN_AT", "25:00")
		_, err := Load()
		require.Error(t, err)
	})

	t.Run("invalid TIMEZONE", func(t *testing.T) {
		t.Setenv("TIMEZONE", "Mars/Base")
		_, err := Load()
		require.Error(t, err)
	})
}
