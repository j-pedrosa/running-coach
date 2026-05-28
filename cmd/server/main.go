package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/j-pedrosa/running-coach/internal/api"
	"github.com/j-pedrosa/running-coach/internal/chart"
	"github.com/j-pedrosa/running-coach/internal/claude"
	"github.com/j-pedrosa/running-coach/internal/coach"
	"github.com/j-pedrosa/running-coach/internal/config"
	"github.com/j-pedrosa/running-coach/internal/scheduler"
	"github.com/j-pedrosa/running-coach/internal/store"
	"github.com/j-pedrosa/running-coach/internal/strava"
	"github.com/j-pedrosa/running-coach/internal/telegram"
	"github.com/j-pedrosa/running-coach/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Database
	st, err := store.New(cfg.DatabasePath)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(context.Background()); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Clients
	stravaClient := strava.NewClient(cfg.StravaClientID, cfg.StravaClientSecret, st, logger)
	if err := stravaClient.SeedToken(context.Background(), cfg.StravaRefreshToken); err != nil {
		logger.Error("failed to seed Strava token", "error", err)
		os.Exit(1)
	}

	claudeClient := claude.NewClient(cfg.AnthropicAPIKey, cfg.ClaudeModel, logger)
	telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramChatID, logger)
	chartClient := chart.NewClient(cfg.QuickChartURL, logger)

	// Coach orchestrator (loads plan from DB, seeds from YAML if empty)
	c := coach.New(stravaClient, claudeClient, telegramClient, chartClient, st, logger)

	// Scheduler
	sched, err := scheduler.New(c, c, cfg.Timezone, logger)
	if err != nil {
		logger.Error("failed to create scheduler", "error", err)
		os.Exit(1)
	}
	if err := sched.Start(); err != nil {
		logger.Error("failed to start scheduler", "error", err)
		os.Exit(1)
	}

	// Web static files
	webFS, err := fs.Sub(web.Content, "static")
	if err != nil {
		logger.Error("failed to load web files", "error", err)
		os.Exit(1)
	}

	// HTTP server
	router := api.NewRouter(logger, st, c, webFS)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("server starting", "port", cfg.Port, "timezone", cfg.Timezone)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	sched.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}

	logger.Info("server stopped")
}
