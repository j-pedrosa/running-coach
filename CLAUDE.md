# Running Coach

Personal running coach — Go backend + vanilla JS frontend, Docker Desktop.

## Stack
- **Backend**: Go 1.25+, stdlib `net/http` (Go 1.22 method routing), `modernc.org/sqlite` (no CGO), `robfig/cron/v3`, `gopkg.in/yaml.v3`
- **Frontend**: Vanilla JS + Chart.js 4 (no build tools, embedded via `embed.FS`)
- **APIs**: Strava (OAuth2 + activities/laps/streams), Anthropic Claude (Messages API), Telegram Bot (HTML mode), QuickChart.io
- **Database**: SQLite (pure Go)

## Project Layout
- `cmd/server/main.go` — entry point, wires all components, graceful shutdown
- `internal/config/` — env var loading + validation
- `internal/store/` — SQLite CRUD + schema migrations
- `internal/models/` — Activity, Report, Split, Lap, HRZone structs
- `internal/strava/` — OAuth2 token rotation, activity detail, laps, HR streams, per-km split merge
- `internal/claude/` — Anthropic Messages API client
- `internal/telegram/` — send text (HTML mode, auto-split) + photos
- `internal/chart/` — QuickChart.io short URL generation
- `internal/coach/` — orchestrator pipeline (fetch → plan match → analyze → chart → notify → dedup)
- `internal/coach/plan.go` — plan config loading from YAML, plan matching, plan status for dashboard
- `internal/scheduler/` — `robfig/cron` at 9pm Europe/Lisbon
- `internal/api/` — HTTP handlers + router + middleware (logging, CORS, recovery)
- `web/static/` — frontend (index.html, app.js, style.css)
- `config/` — runtime config (gitignored): `athlete.md`, `plan-config.yaml`

## Key Design Decisions
- **Single plan source**: `config/plan-config.yaml` drives both the dashboard plan tracker and Claude's coaching context (converted to markdown at runtime via `PlanConfig.ToMarkdown()`)
- **Strava token rotation**: tokens stored in SQLite `config` table, auto-refreshed before expiry
- **HR zones**: computed from Strava HR stream data (per-second), not per-km split averages
- **Laps over splits**: watch laps are the primary data source for interval session analysis
- **Deduplication**: `last_reported_activity_id` in SQLite config prevents duplicate reports

## Build & Run
```bash
go build -o running-coach ./cmd/server   # local
docker compose up --build                # docker
```

## Conventions
- `log/slog` for all logging (never `log` or `fmt.Println`)
- `context.Context` through all API calls
- Error wrapping: `fmt.Errorf("doing X: %w", err)`
- No CGO — `modernc.org/sqlite` only
- Credentials in `.env`, personal data in `config/` — never committed
- Frontend: no npm, no build step — vanilla JS with Chart.js from CDN
