# AI Agents

This project uses AI agents in two ways:

## 1. Claude API (Runtime)

The coaching report pipeline calls the Anthropic Messages API at runtime:

- **Model:** `claude-sonnet-4-5-20250929` (configurable via `CLAUDE_MODEL` env var)
- **System prompt:** built from `config/athlete.md` (athlete profile) + `config/plan-config.yaml` (training plan, auto-converted to markdown). Includes coaching persona, analysis rules, and interval interpretation guidelines.
- **User message:** activity data (distance, time, pace, HR, per-km splits, watch laps) + matched plan session for the day.
- **Output:** structured coaching report in European Portuguese with sections for session summary, analysis, plan compliance, progress, and next sessions.
- **Token usage:** ~2500-4500 input tokens, ~1500-3500 output tokens per report.

### Prompt Architecture

```
System: coaching persona + athlete profile + training plan + interval rules + response format
User:   activity JSON (splits + laps) + planned session context
```

The system prompt instructs Claude to:
- Respond in European Portuguese (pt-PT)
- Use laps data (not just per-km splits) to identify run/walk intervals
- Compare actual performance against the specific planned session
- Account for warm-up and cool-down in time calculations

### Cost Estimate

At Sonnet 4.5 pricing (~$3/M input, ~$15/M output), each daily report costs approximately $0.03-0.06. Running daily for a month: ~$1-2.

## 2. Claude Code (Development)

This project was built with [Claude Code](https://claude.ai/claude-code) as a pair programming tool. The `CLAUDE.md` file provides project context for Claude Code sessions.

## Adding New AI Features

The Claude client (`internal/claude/client.go`) is a thin wrapper around the Anthropic Messages API. To add new AI-powered features:

1. Build the system prompt and user message
2. Call `client.SendMessage(ctx, system, user)`
3. Parse the response text

The client handles authentication, error handling, and token usage logging.
