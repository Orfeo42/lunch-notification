package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/tasks/v1"

	"github.com/orfeo42/lunch-notification/internal/config"
	"github.com/orfeo42/lunch-notification/internal/googleauth"
	"github.com/orfeo42/lunch-notification/internal/gsheets"
	"github.com/orfeo42/lunch-notification/internal/gtasks"
	"github.com/orfeo42/lunch-notification/internal/reminder"
	"github.com/orfeo42/lunch-notification/internal/schedule"
)

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const (
	ErrUnknownSource  sentinelError = "unknown source"
	ErrUnknownTasks   sentinelError = "unknown tasks backend"
	ErrUnknownCommand sentinelError = "unknown command"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := run(ctx, os.Args[1:])
	if err == nil {
		return
	}
	slog.ErrorContext(ctx, "fatal", slog.Any("error", err))
	stop()
	if errors.Is(err, ErrUnknownCommand) {
		os.Exit(2)
	}
	os.Exit(1)
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "serve" {
		return serve(ctx)
	}
	if args[0] == "auth" {
		return auth(ctx)
	}
	if args[0] == "run-once" {
		return runOnce(ctx)
	}
	return fmt.Errorf("%w: %s", ErrUnknownCommand, args[0])
}

func auth(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	prompt := func(authURL string) {
		fmt.Println("Open this URL in your browser:")
		fmt.Println(authURL)
	}
	if err := googleauth.Authorize(ctx, cfg.GoogleCredentialsFile, cfg.GoogleTokenFile, prompt); err != nil {
		return fmt.Errorf("authorize: %w", err)
	}
	return nil
}

func runOnce(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	svc, err := newService(ctx, cfg)
	if err != nil {
		return err
	}
	now := time.Now().In(&cfg.Timezone)
	slog.InfoContext(ctx, "manual run", slog.Time("now", now))
	if err := svc.Run(ctx, now); err != nil {
		return fmt.Errorf("manual run: %w", err)
	}
	return nil
}

func serve(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	svc, err := newService(ctx, cfg)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "starting",
		slog.String("source", cfg.Source),
		slog.String("tasks", cfg.Tasks),
		slog.String("run_at", fmt.Sprintf("%02d:%02d", cfg.RunAt.Hour, cfg.RunAt.Minute)),
		slog.String("timezone", cfg.Timezone.String()),
	)

	schedule.Loop(ctx, cfg.RunAt, &cfg.Timezone, func(ctx context.Context, now time.Time) {
		if err := svc.Run(ctx, now); err != nil {
			logRunError(ctx, err)
		}
	})
	return nil
}

func newService(ctx context.Context, cfg config.Config) (*reminder.Service, error) {
	httpClient, err := googleauth.Client(ctx, cfg.GoogleCredentialsFile, cfg.GoogleTokenFile)
	if err != nil {
		return nil, fmt.Errorf("google client: %w", err)
	}
	src, err := newPlanSource(ctx, cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("plan source: %w", err)
	}
	dst, err := newTaskWriter(ctx, cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("task writer: %w", err)
	}
	return reminder.New(src, dst), nil
}

func newPlanSource(ctx context.Context, cfg config.Config, httpClient *http.Client) (reminder.PlanSource, error) {
	switch cfg.Source {
	case "gsheets":
		svc, err := sheets.NewService(ctx, option.WithHTTPClient(httpClient))
		if err != nil {
			return nil, fmt.Errorf("sheets service: %w", err)
		}
		src, err := gsheets.New(svc, cfg.SheetID, cfg.SheetRange)
		if err != nil {
			return nil, fmt.Errorf("gsheets source: %w", err)
		}
		return src, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownSource, cfg.Source)
	}
}

func newTaskWriter(ctx context.Context, cfg config.Config, httpClient *http.Client) (reminder.TaskWriter, error) {
	switch cfg.Tasks {
	case "gtasks":
		svc, err := tasks.NewService(ctx, option.WithHTTPClient(httpClient))
		if err != nil {
			return nil, fmt.Errorf("tasks service: %w", err)
		}
		return gtasks.New(svc, cfg.TasklistID), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownTasks, cfg.Tasks)
	}
}

func logRunError(ctx context.Context, err error) {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code >= http.StatusInternalServerError {
			slog.WarnContext(ctx, "run failed upstream", slog.Any("error", err))
			return
		}
		slog.ErrorContext(ctx, "run failed", slog.Any("error", err))
		return
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) || errors.Is(err, context.DeadlineExceeded) {
		slog.WarnContext(ctx, "run failed upstream", slog.Any("error", err))
		return
	}
	slog.ErrorContext(ctx, "run failed", slog.Any("error", err))
}
