# Installation Guide

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Mac/Windows/Linux)
- A [Strava](https://www.strava.com) account with API access
- An [Anthropic](https://console.anthropic.com) API key
- A [Telegram](https://telegram.org) bot (created via [@BotFather](https://t.me/botfather))

## Step 1 — Strava API Setup

1. Go to [Strava API Settings](https://www.strava.com/settings/api)
2. Create an application (any name, "localhost" as website)
3. Note your **Client ID** and **Client Secret**
4. Generate a refresh token with `activity:read_all` scope. You can use [this tool](https://developers.strava.com/docs/getting-started/#d-oauth) or:

```bash
# 1. Open in browser to authorize:
# https://www.strava.com/oauth/authorize?client_id=YOUR_ID&response_type=code&redirect_uri=http://localhost&scope=activity:read_all

# 2. Copy the "code" parameter from the redirect URL, then:
curl -X POST https://www.strava.com/oauth/token \
  -d client_id=YOUR_ID \
  -d client_secret=YOUR_SECRET \
  -d code=YOUR_CODE \
  -d grant_type=authorization_code
```

The response contains your `refresh_token`.

## Step 2 — Telegram Bot Setup

1. Message [@BotFather](https://t.me/botfather) on Telegram
2. Send `/newbot` and follow the prompts
3. Copy the **bot token**
4. Send a message to your bot, then get your chat ID:

```bash
curl https://api.telegram.org/botYOUR_TOKEN/getUpdates | python3 -m json.tool
# Look for "chat": {"id": YOUR_CHAT_ID}
```

## Step 3 — Anthropic API Key

1. Go to [Anthropic Console](https://console.anthropic.com/settings/keys)
2. Create an API key

## Step 4 — Configure the App

```bash
# Clone the repo
git clone git@github.com:j-pedrosa/running-coach.git
cd running-coach

# Create .env from example
cp .env.example .env
```

Edit `.env` with your credentials:

```env
STRAVA_CLIENT_ID=your_client_id
STRAVA_CLIENT_SECRET=your_client_secret
STRAVA_REFRESH_TOKEN=your_refresh_token
ANTHROPIC_API_KEY=sk-ant-...
TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_CHAT_ID=your_chat_id
```

## Step 5 — Configure Your Profile

```bash
# Athlete profile (sent to Claude as coaching context)
cp config/athlete.md.example config/athlete.md
# Edit with your personal data, goals, injuries, coaching principles

# Training plan (drives the dashboard + Claude reports)
cp config/plan-config.yaml.example config/plan-config.yaml
# Edit with your plan: start date, weekly sessions, strength descriptions
```

The `plan-config.yaml` is the single source of truth for both the dashboard plan tracker and Claude's coaching context. Weeks run Saturday to Friday.

## Step 6 — Run

```bash
docker compose up --build
```

Open http://localhost:8080

## Step 7 — Initial Data Load

Click **"Run Now"** on the dashboard, or backfill your recent history:

```bash
# Fetch last 30 activities from Strava (only 2026+ runs are kept)
curl -X POST http://localhost:8080/api/backfill?count=30
```

## Updating

```bash
git pull
docker compose up --build
```

The SQLite database persists in `./data/` via Docker volume. Config files persist in `./config/`.

## Troubleshooting

**Strava token errors:** The app auto-refreshes the Strava access token. If you get persistent auth errors, delete the old tokens from the DB:
```bash
docker compose stop
sqlite3 data/running-coach.db "DELETE FROM config WHERE key LIKE 'strava_%';"
docker compose start
```

**Claude model not found:** Check that `CLAUDE_MODEL` in `.env` matches a valid model ID. Default is `claude-sonnet-4-5-20250929`.

**Telegram message too long:** Reports are auto-split at paragraph boundaries. If you still hit issues, the report text is always available on the dashboard.
