package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/j-pedrosa/running-coach/internal/coach"
	"github.com/j-pedrosa/running-coach/internal/store"
)

type handlers struct {
	store  *store.Store
	coach  *coach.Coach
	logger *slog.Logger
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) handleTrigger(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := h.coach.Run(ctx, force); err != nil {
			h.logger.Error("manual trigger failed", "error", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "triggered"})
}

func (h *handlers) handleBackfill(w http.ResponseWriter, r *http.Request) {
	count := 30
	if v := r.URL.Query().Get("count"); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c > 0 && c <= 200 {
			count = c
		}
	}

	saved, err := h.coach.Backfill(r.Context(), count)
	if err != nil {
		h.logger.Error("backfill failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"saved": saved, "requested": count})
}

func (h *handlers) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.coach.GetStatus())
}

func (h *handlers) handleListActivities(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	activities, err := h.store.ListActivities(r.Context(), limit)
	if err != nil {
		h.logger.Error("listing activities", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list activities"})
		return
	}
	writeJSON(w, http.StatusOK, activities)
}

func (h *handlers) handleLatestActivity(w http.ResponseWriter, r *http.Request) {
	activity, err := h.store.GetLatestActivity(r.Context())
	if err != nil {
		h.logger.Error("getting latest activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get activity"})
		return
	}
	if activity == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no activities found"})
		return
	}
	writeJSON(w, http.StatusOK, activity)
}

func (h *handlers) handleLatestReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.store.GetLatestReport(r.Context())
	if err != nil {
		h.logger.Error("getting latest report", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get report"})
		return
	}
	if report == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no reports found"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handlers) handlePlan(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(h.coach.GetPlan()))
}

func (h *handlers) handlePlanStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	activities, err := h.store.ListActivities(ctx, 100)
	if err != nil {
		h.logger.Error("listing activities for plan status", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list activities"})
		return
	}

	cfg := h.coach.GetPlanConfig()
	strengthDone := make(map[int]bool)
	if cfg != nil && cfg.ID > 0 {
		strengthDone, _ = h.store.GetStrengthDoneMap(ctx, cfg.ID)
	}

	status := coach.BuildPlanStatus(cfg, activities, strengthDone)
	writeJSON(w, http.StatusOK, status)
}

func (h *handlers) handleToggleStrength(w http.ResponseWriter, r *http.Request) {
	weekStr := r.URL.Query().Get("week")
	week, err := strconv.Atoi(weekStr)
	if err != nil || week < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid week"})
		return
	}

	cfg := h.coach.GetPlanConfig()
	if cfg == nil || cfg.ID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active plan"})
		return
	}

	current, _ := h.store.GetStrengthDone(r.Context(), cfg.ID, week)
	newVal := !current

	if err := h.store.SetStrengthDone(r.Context(), cfg.ID, week, newVal); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"week": week, "done": newVal})
}

func (h *handlers) handleReportByActivity(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("activityID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid activity ID"})
		return
	}

	report, err := h.store.GetReportByActivity(r.Context(), id)
	if err != nil {
		h.logger.Error("getting report by activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get report"})
		return
	}
	if report == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no report for this activity"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handlers) handleEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListEvents(r.Context(), 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list events"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *handlers) handleHealthDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	expiresAt, _ := h.store.GetConfig(ctx, "strava_expires_at")
	lastReported, _ := h.store.GetConfig(ctx, "last_reported_activity_id")

	var stravaExpiry string
	if expiresAt != "" {
		if ts, err := strconv.ParseInt(expiresAt, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			stravaExpiry = t.Format(time.RFC3339)
		}
	}

	status := h.coach.GetStatus()
	planCfg := h.coach.GetPlanConfig()
	currentWeek := 0
	totalWeeks := 0
	if planCfg != nil {
		currentWeek = planCfg.CurrentWeek()
		totalWeeks = planCfg.TotalWeeks
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"strava_token_expires": stravaExpiry,
		"last_reported_id":     lastReported,
		"last_run":             status.LastRun,
		"last_result":          status.Result,
		"last_error":           status.LastError,
		"plan_week":            currentWeek,
		"plan_total_weeks":     totalWeeks,
		"running":              status.Running,
		"step":                 status.Step,
	})
}

func (h *handlers) handleAthlete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(h.coach.GetAthlete()))
}

func (h *handlers) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	reply, err := h.coach.Chat(r.Context(), req.Message)
	if err != nil {
		h.logger.Error("chat failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get response"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"reply": reply})
}

func (h *handlers) handleApplyProposal(w http.ResponseWriter, r *http.Request) {
	var proposal json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&proposal); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proposal"})
		return
	}

	if err := h.coach.ApplyProposal(r.Context(), proposal); err != nil {
		h.logger.Error("apply proposal failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "applied"})
}

func (h *handlers) handleListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.store.ListPlans(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list plans"})
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (h *handlers) handleArchivePlan(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid plan ID"})
		return
	}
	if err := h.store.ArchivePlan(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.coach.ReloadPlan(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

func (h *handlers) handleGetAthleteProfile(w http.ResponseWriter, r *http.Request) {
	content, err := h.store.GetAthleteProfile(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

func (h *handlers) handleUpdateAthleteProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
		return
	}
	if err := h.coach.UpdateAthlete(r.Context(), req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
