# lunch-notification

Long-running container that, once a day at RUN_AT in TIMEZONE, reads tomorrow's menu from a Google Sheet and ensures one Google Task per meal ("Pranzo: <dish>", "Cena: <dish>") due tomorrow.

It is idempotent (re-runs never duplicate tasks) and catches up on startup if today's RUN_AT already passed. Subcommands: `serve` (default), `run-once` (single immediate run) and `auth` (one-time OAuth consent).

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
3. Google Auth Platform -> Overview -> Get started: app name `lunch-notification`, your email as support and contact email, audience **External**.
4. Branding -> App domain (required to publish), served by GitHub Pages from `docs/` on `main`. Leave the app logo empty: uploading one triggers verification.
   - Application home page: `https://orfeo42.github.io/-lunch-notification/`
   - Privacy policy: `https://orfeo42.github.io/-lunch-notification/privacy.html`
   - Terms of service: `https://orfeo42.github.io/-lunch-notification/terms.html`
   - Authorized domain: `orfeo42.github.io`
5. Audience -> **Publish app** so the status is **In production**. Warning: in "Testing" the refresh token expires after 7 days and the job stops silently. Skip the verification prompt; an unverified app works for your own account.
6. Clients -> Create client -> Desktop app -> download the JSON into the repo root as `credentials.json` (rename the downloaded `client_secret_*.json`).

## One-time auth (on a laptop with a browser)

Requires [just](https://github.com/casey/just) and Go.

1. Run `just auth` from the repo root. Paths default to `credentials.json` and `token.json`; override with `just auth <credentials> <token>`.
2. Open the printed URL and sign in with the Google account that owns the sheet and the tasks.
3. On "Google hasn't verified this app" click **Advanced -> Go to lunch-notification (unsafe)**, then allow both the Sheets and Tasks permissions.
4. The browser tab shows "Authorization complete" and `token.json` is written. The recipe sets both files to mode `600`.
5. Copy `credentials.json` and `token.json` to the Pi in `deploy/secrets/`. Never commit them.

If the job logs `invalid_grant`, the token was revoked or expired: run `just auth` again and copy the new `token.json` to the Pi.

## Deploy on the Pi

The container runs as the image's unprivileged `nonroot` user (uid 65532) and joins your user's group through `group_add`, so the secrets only need to be group-readable.

1. Copy `deploy/compose.yaml` and `deploy/.env.example` to `~/apps/lunch-notification/` on the Pi, and `credentials.json` + `token.json` to `~/apps/lunch-notification/secrets/`.
2. `cd ~/apps/lunch-notification && cp .env.example .env`, then set `GID` in `.env` to the output of `id -g`.
3. `chmod 750 secrets && chmod 640 secrets/*.json`
4. `docker compose up -d`
5. Logs: `docker compose logs -f`. Expect `starting` and `next run scheduled`, preceded by `catch-up run` when started after `RUN_AT`.

## Manual run

Run once immediately, outside the schedule. It ensures tomorrow's tasks with the same config and secrets, then exits; existing tasks are not duplicated.

- From your machine: `just pi-run-once` (defaults: host `raspberry`, dir `~/apps/lunch-notification`; override with `just pi-run-once <host> <dir>`).
- On the Pi: `docker compose run --rm lunch-notification run-once`

## Development

- `go test -short ./...`
