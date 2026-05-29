# Running Coach

A fully automated personal running coach that fetches your Strava data, generates AI-powered coaching reports via Claude, sends them to Telegram, and serves a live dashboard — all running locally on Docker Desktop.

## What It Does

1. **Fetches** your latest run from Strava (activity data, per-km splits, watch laps, HR streams)
2. **Matches** the run to your training plan (which session was planned for that day)
3. **Analyzes** via Claude API — generates a detailed coaching report comparing actual vs planned
4. **Sends** the report + a splits chart to Telegram
5. **Displays** everything on a dark-themed web dashboard with charts, HR zones, and plan tracking

Runs automatically at 9pm daily, or manually via the dashboard "Run Now" button.

## Dashboard

- **Last Run tab** — session card, splits chart, laps chart, donut HR zones, full coaching report, run history with click-to-expand reports
- **Plan tab** — week-by-week progress with session completion tracking (runs auto-detected from Strava, strength sessions via checkbox)

## Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go (stdlib `net/http`, no framework) |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no CGO) |
| Frontend | Vanilla JS + Chart.js (no build tools, embedded in binary) |
| Scheduler | `robfig/cron/v3` (daily 9pm, configurable timezone) |
| AI | Anthropic Claude API (Messages API) |
| Notifications | Telegram Bot API |
| Charts | Chart.js (browser) + QuickChart.io (Telegram PNGs) |
| Container | Docker multi-stage build |

## Quick Start

See [INSTALL.md](INSTALL.md) for detailed setup instructions.

```bash
# 1. Clone
git clone git@github.com:j-pedrosa/running-coach.git
cd running-coach

# 2. Configure
cp .env.example .env                              # Add your API keys
cp config/athlete.md.example config/athlete.md    # Your profile
cp config/plan-config.yaml.example config/plan-config.yaml  # Your plan

# 3. Run
docker compose up --build

# 4. Open
open http://localhost:8080
```

## Project Structure

```
running-coach/
├── cmd/server/main.go           # Entry point — wires all components
├── internal/
│   ├── config/                  # Environment variable loading
│   ├── strava/                  # Strava OAuth2 + activity/laps/streams API
│   ├── claude/                  # Anthropic Messages API client
│   ├── telegram/                # Telegram Bot API (HTML mode)
│   ├── chart/                   # QuickChart.io PNG generation
│   ├── coach/                   # Orchestrator pipeline + plan matching
│   ├── scheduler/               # Cron wrapper (daily 9pm)
│   ├── api/                     # HTTP handlers + router + middleware
│   ├── models/                  # Activity, Report, Split, Lap, HRZone
│   └── store/                   # SQLite data layer + migrations
├── web/static/                  # Frontend (embedded into binary)
├── config/                      # Athlete profile + plan config (gitignored)
├── Dockerfile                   # Multi-stage build
├── docker-compose.yml           # Docker Desktop config
└── .env.example                 # Required environment variables
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check |
| POST | `/api/trigger` | Manual coaching run (`?force=true`) |
| GET | `/api/status` | Pipeline status |
| GET | `/api/activities` | List activities (`?limit=10`) |
| GET | `/api/activities/latest` | Latest activity with splits + laps |
| GET | `/api/reports/latest` | Latest coaching report |
| GET | `/api/reports/{activityID}` | Report for a specific activity |
| GET | `/api/plan/status` | Training plan status with completion data |
| POST | `/api/plan/toggle-strength` | Toggle strength session done (`?week=3`) |
| POST | `/api/backfill` | Backfill historical activities (`?count=30`) |
| GET | `/api/plan` | Training plan as markdown |
| GET | `/api/athlete` | Athlete profile as markdown |

## License

Personal project. Not licensed for redistribution.
