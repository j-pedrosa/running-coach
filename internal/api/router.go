package api

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/j-pedrosa/running-coach/internal/coach"
	"github.com/j-pedrosa/running-coach/internal/store"
)

func NewRouter(logger *slog.Logger, st *store.Store, c *coach.Coach, webFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{store: st, coach: c, logger: logger}

	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("POST /api/trigger", h.handleTrigger)
	mux.HandleFunc("GET /api/status", h.handleStatus)
	mux.HandleFunc("GET /api/activities", h.handleListActivities)
	mux.HandleFunc("GET /api/activities/latest", h.handleLatestActivity)
	mux.HandleFunc("GET /api/reports/latest", h.handleLatestReport)
	mux.HandleFunc("POST /api/backfill", h.handleBackfill)
	mux.HandleFunc("GET /api/plan", h.handlePlan)
	mux.HandleFunc("GET /api/plan/status", h.handlePlanStatus)
	mux.HandleFunc("POST /api/plan/toggle-strength", h.handleToggleStrength)
	mux.HandleFunc("GET /api/reports/{activityID}", h.handleReportByActivity)
	mux.HandleFunc("GET /api/athlete", h.handleAthlete)

	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		mux.Handle("GET /", fileServer)
	}

	var handler http.Handler = mux
	handler = withCORS(handler)
	handler = withLogging(logger, handler)
	handler = withRecover(logger, handler)

	return handler
}
