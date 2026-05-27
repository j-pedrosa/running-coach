# Contributing

This is a personal project, but contributions and ideas are welcome.

## Development Setup

### Prerequisites

- Go 1.22+ (1.25+ recommended)
- Docker Desktop
- SQLite3 CLI (for debugging)

### Local Development (without Docker)

```bash
# Install deps
go mod download

# Create .env and config files (see INSTALL.md)

# Run locally
go build -o running-coach ./cmd/server && ./running-coach
```

The server starts on http://localhost:8080. Frontend changes are embedded at build time — rebuild after editing `web/static/` files.

### Docker Development

```bash
docker compose up --build
```

Rebuild takes ~8 seconds thanks to Docker layer caching.

### Database Access

The SQLite database is at `./data/running-coach.db`:

```bash
# Stop container first to avoid locks
docker compose stop
sqlite3 data/running-coach.db

# Useful queries
SELECT date, name, plan_session FROM activities ORDER BY date DESC;
SELECT id, activity_id, created_at FROM reports ORDER BY created_at DESC;
SELECT * FROM config;
```

## Code Style

- **Logging:** `log/slog` everywhere, never `log` or `fmt.Println`
- **Errors:** wrap with `fmt.Errorf("doing X: %w", err)` for context
- **Context:** pass `context.Context` through all API calls
- **No CGO:** use `modernc.org/sqlite`, not `mattn/go-sqlite3`
- **HTTP:** stdlib `net/http` with Go 1.22 method routing — no external router
- **Frontend:** vanilla JS, no build tools, no frameworks. Chart.js from CDN.

## Project Layout

| Directory | Responsibility |
|-----------|---------------|
| `cmd/server/` | Entry point, wiring |
| `internal/config/` | Env var loading + validation |
| `internal/strava/` | Strava API client (OAuth2, activities, laps, streams) |
| `internal/claude/` | Anthropic Messages API client |
| `internal/telegram/` | Telegram Bot API (HTML mode, message splitting) |
| `internal/chart/` | QuickChart.io PNG generation |
| `internal/coach/` | Orchestrator pipeline + plan matching + prompt building |
| `internal/scheduler/` | Cron wrapper |
| `internal/api/` | HTTP handlers, router, middleware |
| `internal/models/` | Data types (Activity, Report, Split, Lap, HRZone) |
| `internal/store/` | SQLite CRUD + migrations |
| `web/` | Embedded frontend (vanilla JS + Chart.js) |
| `config/` | Runtime config (gitignored, only .example committed) |

## Adding a New Feature

1. Create a branch from `main`
2. Make your changes
3. Verify: `go build ./... && go vet ./...`
4. Test with Docker: `docker compose up --build`
5. Open a PR

## Configuration Files

Personal data lives in `config/` (gitignored):

| File | Purpose |
|------|---------|
| `config/athlete.md` | Athlete profile sent to Claude as coaching context |
| `config/plan-config.yaml` | Training plan — drives dashboard + Claude reports |

Only `.example` templates are committed. Never commit real config files.
