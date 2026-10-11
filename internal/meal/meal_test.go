package meal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKind(t *testing.T) {
	t.Run("pranzo is lunch", func(t *testing.T) {
		got, err := ParseKind("Pranzo")
		require.NoError(t, err)
		assert.Equal(t, Lunch, got)
	})

	t.Run("cena is dinner ignoring case and spaces", func(t *testing.T) {
		got, err := ParseKind("  CENA ")
		require.NoError(t, err)
		assert.Equal(t, Dinner, got)
	})

	t.Run("unknown label returns error", func(t *testing.T) {
		_, err := ParseKind("Data")
		require.ErrorIs(t, err, ErrUnknownKind)
	})
}

func TestKindLabel(t *testing.T) {
	t.Run("lunch label", func(t *testing.T) {
		assert.Equal(t, "Pranzo", Lunch.Label())
	})

	t.Run("dinner label", func(t *testing.T) {
		assert.Equal(t, "Cena", Dinner.Label())
	})

	t.Run("unknown kind falls back to raw value", func(t *testing.T) {
		assert.Equal(t, "snack", Kind("snack").Label())
	})
}
