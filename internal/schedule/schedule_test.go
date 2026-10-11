package schedule

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rome(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Rome")
	require.NoError(t, err)
	return loc
}

func TestClockUnmarshalText(t *testing.T) {
	t.Run("valid evening time", func(t *testing.T) {
		var c Clock
		require.NoError(t, c.UnmarshalText([]byte("20:00")))
		assert.Equal(t, Clock{Hour: 20, Minute: 0}, c)
	})

	t.Run("valid zero padded time", func(t *testing.T) {
		var c Clock
		require.NoError(t, c.UnmarshalText([]byte("07:05")))
		assert.Equal(t, Clock{Hour: 7, Minute: 5}, c)
	})

	t.Run("hour out of range", func(t *testing.T) {
		var c Clock
		err := c.UnmarshalText([]byte("25:00"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClock))
	})

	t.Run("not a time", func(t *testing.T) {
		var c Clock
		err := c.UnmarshalText([]byte("abc"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClock))
	})

	t.Run("empty", func(t *testing.T) {
		var c Clock
		err := c.UnmarshalText([]byte(""))
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidClock))
	})
}

func TestClockOn(t *testing.T) {
	loc := rome(t)

	t.Run("keeps day and location", func(t *testing.T) {
		day := time.Date(2026, 3, 4, 15, 30, 10, 5, loc)
		got := Clock{Hour: 20, Minute: 15}.On(day)
		assert.Equal(t, time.Date(2026, 3, 4, 20, 15, 0, 0, loc), got)
	})
}

func TestNext(t *testing.T) {
	loc := rome(t)
	at := Clock{Hour: 20}

	t.Run("before time returns today", func(t *testing.T) {
		now := time.Date(2026, 10, 11, 10, 0, 0, 0, loc)
		assert.Equal(t, time.Date(2026, 10, 11, 20, 0, 0, 0, loc), Next(now, at))
	})

	t.Run("after time returns tomorrow", func(t *testing.T) {
		now := time.Date(2026, 10, 11, 21, 0, 0, 0, loc)
		assert.Equal(t, time.Date(2026, 10, 12, 20, 0, 0, 0, loc), Next(now, at))
	})

	t.Run("exactly at time returns tomorrow", func(t *testing.T) {
		now := time.Date(2026, 10, 11, 20, 0, 0, 0, loc)
		assert.Equal(t, time.Date(2026, 10, 12, 20, 0, 0, 0, loc), Next(now, at))
	})

	t.Run("across DST end keeps wall clock", func(t *testing.T) {
		now := time.Date(2026, 10, 24, 21, 0, 0, 0, loc)
		got := Next(now, at)
		assert.Equal(t, time.Date(2026, 10, 25, 20, 0, 0, 0, loc), got)
		assert.Equal(t, 24*time.Hour, got.Sub(now))
	})
}

func TestMissed(t *testing.T) {
	loc := rome(t)
	at := Clock{Hour: 20}

	t.Run("before time is not missed", func(t *testing.T) {
		assert.False(t, Missed(time.Date(2026, 10, 11, 19, 59, 0, 0, loc), at))
	})

	t.Run("after time is missed", func(t *testing.T) {
		assert.True(t, Missed(time.Date(2026, 10, 11, 20, 1, 0, 0, loc), at))
	})

	t.Run("exactly at time is missed", func(t *testing.T) {
		assert.True(t, Missed(time.Date(2026, 10, 11, 20, 0, 0, 0, loc), at))
	})
}

func TestLoop(t *testing.T) {
	at := Clock{Hour: 20}

	t.Run("started after time runs catch-up then next day", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			calls := make(chan time.Time, 10)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			time.Sleep(21 * time.Hour)
			go func() {
				Loop(ctx, at, time.UTC, func(_ context.Context, now time.Time) { calls <- now })
				close(done)
			}()
			synctest.Wait()
			require.Len(t, calls, 1)

			time.Sleep(23 * time.Hour)
			synctest.Wait()
			require.Len(t, calls, 2)
			<-calls
			second := <-calls
			assert.Equal(t, time.Date(2000, 1, 2, 20, 0, 0, 0, time.UTC), second)

			cancel()
			<-done
		})
	})

	t.Run("started before time waits until run time", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			calls := make(chan time.Time, 10)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				Loop(ctx, at, time.UTC, func(_ context.Context, now time.Time) { calls <- now })
				close(done)
			}()
			synctest.Wait()
			require.Empty(t, calls)

			time.Sleep(20 * time.Hour)
			synctest.Wait()
			require.Len(t, calls, 1)

			cancel()
			<-done
		})
	})

	t.Run("cancel makes loop return", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				Loop(ctx, at, time.UTC, func(context.Context, time.Time) {})
				close(done)
			}()
			synctest.Wait()
			cancel()
			synctest.Wait()
			select {
			case <-done:
			default:
				t.Fatal("loop did not return after cancel")
			}
		})
	})

	t.Run("already cancelled context skips catch-up", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			time.Sleep(21 * time.Hour)
			called := false
			Loop(ctx, at, time.UTC, func(context.Context, time.Time) { called = true })
			assert.False(t, called)
		})
	})
}
