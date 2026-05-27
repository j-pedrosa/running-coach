package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port               int
	StravaClientID     string
	StravaClientSecret string
	StravaRefreshToken string
	TelegramBotToken   string
	TelegramChatID     string
	AnthropicAPIKey    string
	ClaudeModel        string
	QuickChartURL      string
	DatabasePath       string
	Timezone           string
}

func Load() (*Config, error) {
	port := 8080
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
		port = p
	}

	cfg := &Config{
		Port:               port,
		StravaClientID:     os.Getenv("STRAVA_CLIENT_ID"),
		StravaClientSecret: os.Getenv("STRAVA_CLIENT_SECRET"),
		StravaRefreshToken: os.Getenv("STRAVA_REFRESH_TOKEN"),
		TelegramBotToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:     os.Getenv("TELEGRAM_CHAT_ID"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		ClaudeModel:        os.Getenv("CLAUDE_MODEL"),
		QuickChartURL:      os.Getenv("QUICKCHART_URL"),
		DatabasePath:       os.Getenv("DATABASE_PATH"),
		Timezone:           os.Getenv("TZ"),
	}

	if cfg.ClaudeModel == "" {
		cfg.ClaudeModel = "claude-sonnet-4-5-20250929"
	}
	if cfg.QuickChartURL == "" {
		cfg.QuickChartURL = "https://quickchart.io"
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "./data/running-coach.db"
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "Europe/Lisbon"
	}

	var missing []string
	for _, pair := range []struct{ name, val string }{
		{"STRAVA_CLIENT_ID", cfg.StravaClientID},
		{"STRAVA_CLIENT_SECRET", cfg.StravaClientSecret},
		{"STRAVA_REFRESH_TOKEN", cfg.StravaRefreshToken},
		{"TELEGRAM_BOT_TOKEN", cfg.TelegramBotToken},
		{"TELEGRAM_CHAT_ID", cfg.TelegramChatID},
		{"ANTHROPIC_API_KEY", cfg.AnthropicAPIKey},
	} {
		if pair.val == "" {
			missing = append(missing, pair.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %v", missing)
	}

	return cfg, nil
}
