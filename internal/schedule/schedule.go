package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const ErrInvalidClock sentinelError = "invalid clock, expected HH:MM"

type Clock struct {
	Hour   int
	Minute int
}

func (c *Clock) UnmarshalText(text []byte) error {
	parsed, err := time.Parse("15:04", string(text))
	if err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidClock, string(text))
	}
	c.Hour = parsed.Hour()
	c.Minute = parsed.Minute()
	return nil
}

func (c Clock) On(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), c.Hour, c.Minute, 0, 0, day.Location())
}

func Next(now time.Time, at Clock) time.Time {
	today := at.On(now)
	if now.Before(today) {
		return today
	}
	return at.On(time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location()))
}

func Missed(now time.Time, at Clock) bool {
	return !now.Before(at.On(now))
}

func Loop(ctx context.Context, at Clock, loc *time.Location, run func(ctx context.Context, now time.Time)) {
	now := time.Now().In(loc)
	if ctx.Err() == nil && Missed(now, at) {
		slog.InfoContext(ctx, "catch-up run", slog.Time("now", now))
		run(ctx, now)
	}
	for {
		next := Next(time.Now().In(loc), at)
		slog.InfoContext(ctx, "next run scheduled", slog.Time("at", next))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			slog.InfoContext(ctx, "scheduler stopped")
			return
		case <-timer.C:
			run(ctx, time.Now().In(loc))
		}
	}
}
