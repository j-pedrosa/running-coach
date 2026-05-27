# Running Coach

Personal running coach app — Go backend + vanilla JS frontend, containerized with Docker.

## Stack
- **Backend**: Go 1.26, `net/http` (stdlib router), `modernc.org/sqlite`, `robfig/cron/v3`
- **Frontend**: Vanilla JS + Chart.js (no build tools)
- **External APIs**: Strava (activities), Anthropic/Claude (coaching reports), Telegram Bot (notifications), QuickChart.io (chart PNGs)
- **Database**: SQLite (pure Go, no CGO)

## Project Layout
- `cmd/server/main.go` — entry point, wires all components
- `internal/config/` — env var loading
- `internal/store/` — SQLite data layer + migrations
- `internal/models/` — Activity, Report, Split structs
- `internal/strava/` — Strava OAuth2 + API client
- `internal/claude/` — Anthropic Messages API client
- `internal/telegram/` — Telegram Bot API client
- `internal/chart/` — QuickChart.io client
- `internal/coach/` — orchestrator pipeline (fetch → analyze → notify)
- `internal/scheduler/` — cron wrapper (daily 9pm Lisbon)
- `internal/api/` — HTTP handlers + router + middleware
- `web/` — frontend static files (embedded into binary)

## Build & Run
```bash
# Local
go build -o running-coach ./cmd/server && ./running-coach

# Docker
docker compose up --build
```

## Conventions
- Use `log/slog` for all logging
- Pass `context.Context` through all API calls
- Wrap errors with `fmt.Errorf("doing X: %w", err)`
- No CGO — use `modernc.org/sqlite` for SQLite
- Credentials in `.env` file, never committed
