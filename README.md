# lunch-notification

Long-running container that, once a day at RUN_AT in TIMEZONE, reads tomorrow's menu from a Google Sheet and ensures one Google Task per meal ("Pranzo: <dish>", "Cena: <dish>") due tomorrow.

It is idempotent (re-runs never duplicate tasks) and catches up on startup if today's RUN_AT already passed. Subcommands: `serve` (default) and `auth` (one-time OAuth consent).

## Architecture

- Ports live in `internal/reminder`: `PlanSource` (reads the menu) and `TaskWriter` (creates the tasks).
- Adapters: `internal/gsheets` (PlanSource) and `internal/gtasks` (TaskWriter), selected in `cmd/lunch-notification` by the `SOURCE` and `TASKS` env vars.
- To add a new adapter: implement `PlanSource` or `TaskWriter` in a new `internal/<name>` package.
- Register it in the selection switch in `cmd/lunch-notification`.
- Select it with the matching env value (`SOURCE=<name>` or `TASKS=<name>`).

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| RUN_AT | 20:00 | Daily run time, HH:MM |
| TIMEZONE | Europe/Rome | IANA timezone for RUN_AT |
| SOURCE | gsheets | Plan source adapter |
| TASKS | gtasks | Task writer adapter |
| SHEET_ID | none | Required for gsheets |
| SHEET_RANGE | Menù!A1:H4 | Range holding the menu |
| TASKLIST_ID | @default | Google Tasks list |
| GOOGLE_CREDENTIALS_FILE | /data/credentials.json | OAuth client JSON |
| GOOGLE_TOKEN_FILE | /data/token.json | OAuth token written by `auth` |

## Google Cloud setup

Free, no billing needed.

1. Create a Google Cloud project.
2. Enable the Google Sheets API and Google Tasks API, or run `gcloud services enable sheets.googleapis.com tasks.googleapis.com --project <id>`.
3. Configure the OAuth consent screen (Google Auth Platform): user type External, add yourself as test user, then **Publish app -> In production**. Warning: in "Testing" the refresh token expires after 7 days and the job stops silently. Unverified is fine for your own account (click Advanced -> continue).
4. Credentials -> Create credentials -> OAuth client ID -> Desktop app -> download the JSON as `credentials.json`.

## One-time auth (on a laptop with a browser)

`GOOGLE_CREDENTIALS_FILE=./credentials.json GOOGLE_TOKEN_FILE=./token.json go run ./cmd/lunch-notification auth`

Copy both files to the Pi in `deploy/secrets/` and run `chmod 600` on them. Never commit them.

## Deploy on the Pi

- `cd deploy && cp .env.example .env && docker compose up -d`
- Logs: `docker compose logs -f`

## Development

- `go test -short ./...`
