package coach

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/j-pedrosa/running-coach/internal/chart"
	"github.com/j-pedrosa/running-coach/internal/claude"
	"github.com/j-pedrosa/running-coach/internal/models"
	"github.com/j-pedrosa/running-coach/internal/retry"
	"github.com/j-pedrosa/running-coach/internal/store"
	"github.com/j-pedrosa/running-coach/internal/strava"
	"github.com/j-pedrosa/running-coach/internal/telegram"
)

//go:embed prompts/athlete.md
var defaultAthleteMD string

var ErrAlreadyRunning = errors.New("coach pipeline is already running")
var ErrNoNewActivity = errors.New("no new activity to report")

type Status struct {
	Running   bool      `json:"running"`
	Step      string    `json:"step,omitempty"`
	LastRun   time.Time `json:"last_run,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	Result    string    `json:"result,omitempty"`
}

type Coach struct {
	strava     *strava.Client
	claude     *claude.Client
	telegram   *telegram.Client
	chart      *chart.Client
	store      *store.Store
	athlete    string
	planConfig *PlanConfig
	logger     *slog.Logger

	mu     sync.Mutex
	status Status
}

func New(
	stravaClient *strava.Client,
	claudeClient *claude.Client,
	telegramClient *telegram.Client,
	chartClient *chart.Client,
	st *store.Store,
	logger *slog.Logger,
) *Coach {
	ctx := context.Background()

	// Load plan from DB, or seed from YAML if empty
	plan, err := LoadPlanFromDB(ctx, st, logger)
	if err != nil {
		logger.Error("failed to load plan from DB", "error", err)
	}
	if plan == nil {
		plan, err = SeedFromYAML(ctx, st, logger)
		if err != nil {
			logger.Error("failed to seed plan from YAML", "error", err)
		}
	}

	// Load athlete from DB, or seed from file
	athlete, _ := st.GetAthleteProfile(ctx)
	if athlete == "" {
		athlete = loadPrompt("/app/config/athlete.md", defaultAthleteMD)
		if athlete != defaultAthleteMD {
			st.SaveAthleteProfile(ctx, athlete)
			logger.Info("seeded athlete profile from file to database")
		}
	}

	return &Coach{
		strava:     stravaClient,
		claude:     claudeClient,
		telegram:   telegramClient,
		chart:      chartClient,
		store:      st,
		planConfig: plan,
		athlete:    athlete,
		logger:     logger,
	}
}

// ReloadPlan refreshes the plan config from the database.
func (c *Coach) ReloadPlan(ctx context.Context) error {
	plan, err := LoadPlanFromDB(ctx, c.store, c.logger)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.planConfig = plan
	c.mu.Unlock()
	return nil
}

// UpdateAthlete saves a new athlete profile to DB and reloads it.
func (c *Coach) UpdateAthlete(ctx context.Context, content string) error {
	if err := c.store.SaveAthleteProfile(ctx, content); err != nil {
		return err
	}
	c.mu.Lock()
	c.athlete = content
	c.mu.Unlock()
	c.store.LogEvent(ctx, "profile", "Athlete profile updated", "")
	return nil
}

func (c *Coach) GetPlanConfig() *PlanConfig { return c.planConfig }

func (c *Coach) setStep(step string) {
	c.mu.Lock()
	c.status.Step = step
	c.mu.Unlock()
}

// SendNudge sends a motivational message the evening before a run day.
func (c *Coach) SendNudge(ctx context.Context) error {
	tomorrow := time.Now().Add(24 * time.Hour)
	day := tomorrow.Weekday()

	var session string
	if c.planConfig != nil {
		week := c.planConfig.CurrentWeek()
		session = c.planConfig.RunDesc(week, day)
	}

	msg := fmt.Sprintf("🏃 <b>Amanhã é dia de corrida!</b>\n\n")
	if session != "" {
		msg += fmt.Sprintf("Sessão planeada: <b>%s</b>\n\n", session)
	}
	msg += "Prepara a roupa, carrega o relógio, e descansa bem esta noite. 💪"

	if err := c.telegram.SendMessage(ctx, msg); err != nil {
		return fmt.Errorf("sending nudge: %w", err)
	}
	c.store.LogEvent(ctx, "nudge", "Motivational nudge sent", fmt.Sprintf("Tomorrow: %s", dayName(day)))
	return nil
}

// Chat sends a user message to Claude with full athlete/plan/history context.
func (c *Coach) Chat(ctx context.Context, userMsg string) (string, error) {
	// Build history summary from recent activities
	activities, _ := c.store.ListActivities(ctx, 10)
	var historyLines string
	for _, a := range activities {
		historyLines += fmt.Sprintf("- %s (%s): %.2fkm, %s, pace %s/km, HR %.0f/%.0f, %s\n",
			a.Date.Format("2 Jan"), dayName(a.Date.Weekday()),
			a.Distance/1000, formatDuration(a.MovingTime),
			a.AvgPace, a.AvgHR, a.MaxHR, a.PlanSession)
	}

	planMD := "No plan configured."
	if c.planConfig != nil {
		planMD = c.planConfig.ToMarkdown()
	}

	system := fmt.Sprintf(`You are a personal running coach chatbot. You have full access to the athlete's profile, training plan, and recent run history. Answer questions about their training, progress, health, plan adjustments, and running in general. Be direct, knowledgeable, and supportive. Respond in European Portuguese (pt-PT).

## Athlete Profile
%s

## Current Training Plan
%s

## Recent Run History (last 10 sessions)
%s

## IMPORTANT: Proposing Changes
You can propose changes to the training plan or athlete profile. When you do, include a JSON block at the END of your response using this exact format:

To update a week in the plan:
<!--PROPOSAL:{"type":"update_week","week":5,"saturday":"new description","monday":"new description","wednesday":"new description"}-->

To create a completely new plan (archives the current one):
<!--PROPOSAL:{"type":"new_plan","name":"Plan Name","start_date":"2026-06-28","total_weeks":8,"goal":"Run 10km","goal_km":10,"weeks":{"1":{"saturday":"...","monday":"...","wednesday":"..."}}}-->

To update the athlete profile:
<!--PROPOSAL:{"type":"update_athlete","content":"full updated markdown content"}-->

Only include PROPOSAL blocks when the user explicitly asks you to change something. Always explain what you're changing in your text response before the proposal block.`, c.athlete, planMD, historyLines)

	result, err := c.claude.SendMessage(ctx, system, userMsg)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	return result.Text, nil
}

// ApplyProposal applies a change proposed by Claude in the chat.
func (c *Coach) ApplyProposal(ctx context.Context, proposalJSON json.RawMessage) error {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(proposalJSON, &base); err != nil {
		return fmt.Errorf("parsing proposal type: %w", err)
	}

	switch base.Type {
	case "update_week":
		var p struct {
			Week      int    `json:"week"`
			Saturday  string `json:"saturday"`
			Monday    string `json:"monday"`
			Wednesday string `json:"wednesday"`
		}
		if err := json.Unmarshal(proposalJSON, &p); err != nil {
			return err
		}
		if c.planConfig == nil || c.planConfig.ID == 0 {
			return fmt.Errorf("no active plan")
		}
		if c.planConfig.Weeks == nil {
			c.planConfig.Weeks = make(map[int]PlanWeekConfig)
		}
		c.planConfig.Weeks[p.Week] = PlanWeekConfig{
			Saturday: p.Saturday, Monday: p.Monday, Wednesday: p.Wednesday,
		}
		_, err := SavePlanToDB(ctx, c.store, c.planConfig)
		if err != nil {
			return err
		}
		c.store.LogEvent(ctx, "plan", fmt.Sprintf("Week %d updated via chat", p.Week), "")

	case "new_plan":
		var p struct {
			Name       string                 `json:"name"`
			StartDate  string                 `json:"start_date"`
			TotalWeeks int                    `json:"total_weeks"`
			Goal       string                 `json:"goal"`
			GoalKm     float64                `json:"goal_km"`
			Weeks      map[int]PlanWeekConfig `json:"weeks"`
		}
		if err := json.Unmarshal(proposalJSON, &p); err != nil {
			return err
		}
		t, _ := time.Parse("2006-01-02", p.StartDate)
		newPlan := &PlanConfig{
			Name: p.Name, StartDate: p.StartDate, TotalWeeks: p.TotalWeeks,
			Goal: p.Goal, GoalKm: p.GoalKm, Weeks: p.Weeks, startTime: t,
		}
		_, err := SavePlanToDB(ctx, c.store, newPlan)
		if err != nil {
			return err
		}
		c.store.LogEvent(ctx, "plan", "New plan created via chat: "+p.Name, "")

	case "update_athlete":
		var p struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(proposalJSON, &p); err != nil {
			return err
		}
		if err := c.UpdateAthlete(ctx, p.Content); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown proposal type: %s", base.Type)
	}

	// Reload plan after any change
	return c.ReloadPlan(ctx)
}

func (c *Coach) GetStatus() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Coach) GetAthlete() string { return c.athlete }
func (c *Coach) GetPlan() string    { return c.planConfig.ToMarkdown() }

// Backfill fetches recent activities from Strava and saves them to the DB (no Claude/Telegram).
func (c *Coach) Backfill(ctx context.Context, count int) (int, error) {
	c.logger.Info("backfilling activities from Strava", "count", count)

	// Only fetch activities from 2026 onwards
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	activities, err := c.strava.GetActivities(ctx, count, after)
	if err != nil {
		return 0, fmt.Errorf("fetching activities: %w", err)
	}

	saved := 0
	for _, summary := range activities {
		if summary.Type != "Run" && summary.Type != "Walk" {
			continue
		}

		existing, _ := c.store.GetActivity(ctx, summary.ID)

		var activity *models.Activity
		needsFetch := existing == nil || len(existing.Laps) == 0
		if existing != nil && existing.PlanSession != "" && !needsFetch {
			continue // fully populated, skip
		}

		if needsFetch {
			activity, _, err = c.strava.FetchFullActivity(ctx, summary.ID)
			if err != nil {
				c.logger.Warn("failed to fetch activity detail, skipping", "strava_id", summary.ID, "error", err)
				continue
			}
		} else {
			activity = existing
		}

		MatchActivityToPlan(activity, c.planConfig)

		if err := c.store.SaveActivity(ctx, activity); err != nil {
			c.logger.Warn("failed to save activity", "strava_id", summary.ID, "error", err)
			continue
		}

		c.logger.Info("backfilled activity", "name", activity.Name, "date", activity.Date.Format("2006-01-02"), "plan", activity.PlanSession)
		saved++
	}

	c.logger.Info("backfill complete", "fetched", len(activities), "saved", saved)
	return saved, nil
}

func (c *Coach) Run(ctx context.Context, force bool) error {
	c.mu.Lock()
	if c.status.Running {
		c.mu.Unlock()
		return ErrAlreadyRunning
	}
	c.status.Running = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.status.Running = false
		c.status.LastRun = time.Now()
		c.mu.Unlock()
	}()

	err := c.run(ctx, force)
	c.mu.Lock()
	if err != nil {
		c.status.LastError = err.Error()
		c.status.Result = "error"
		c.status.Step = ""
		c.store.LogEvent(ctx, "error", "Pipeline failed", err.Error())
	} else {
		c.status.LastError = ""
		c.status.Result = "success"
		c.status.Step = ""
		c.store.LogEvent(ctx, "success", "Coaching report generated", "")
	}
	c.mu.Unlock()
	return err
}

func (c *Coach) run(ctx context.Context, force bool) error {
	// 1. Fetch latest activity from Strava
	c.setStep("strava")
	c.logger.Info("fetching latest activity from Strava")
	var latest *strava.ActivitySummary
	err := retry.Do(ctx, c.logger, "strava-latest", 2, func() error {
		var e error
		latest, e = c.strava.GetLatestActivity(ctx)
		return e
	})
	if err != nil {
		return fmt.Errorf("fetching latest activity: %w", err)
	}
	if latest == nil {
		return ErrNoNewActivity
	}

	// 2. Check deduplication
	if !force {
		lastReported, _ := c.store.GetConfig(ctx, "last_reported_activity_id")
		if lastReported == strconv.FormatInt(latest.ID, 10) {
			c.logger.Info("activity already reported, skipping", "strava_id", latest.ID)
			return ErrNoNewActivity
		}
	}

	// 3. Fetch full activity detail + streams
	c.setStep("strava-detail")
	c.logger.Info("fetching activity detail", "strava_id", latest.ID, "name", latest.Name)
	var activity *models.Activity
	var rawJSON string
	err = retry.Do(ctx, c.logger, "strava-detail", 2, func() error {
		var e error
		activity, rawJSON, e = c.strava.FetchFullActivity(ctx, latest.ID)
		return e
	})
	if err != nil {
		return fmt.Errorf("fetching full activity: %w", err)
	}
	activity.RawJSON = rawJSON

	// 4. Match to plan and save activity to DB
	MatchActivityToPlan(activity, c.planConfig)
	if err := c.store.SaveActivity(ctx, activity); err != nil {
		return fmt.Errorf("saving activity: %w", err)
	}

	// 5. Build Claude prompt
	systemPrompt := c.buildSystemPrompt()
	userMessage := c.buildUserMessage(activity)

	// 6. Send to Claude
	c.setStep("claude")
	c.logger.Info("generating coaching report via Claude")
	var result *claude.Result
	err = retry.Do(ctx, c.logger, "claude", 2, func() error {
		var e error
		result, e = c.claude.SendMessage(ctx, systemPrompt, userMessage)
		return e
	})
	if err != nil {
		return fmt.Errorf("generating report: %w", err)
	}

	// 7. Generate chart
	c.setStep("chart")
	var chartURL, chartConfig string
	if len(activity.Splits) > 0 {
		c.logger.Info("generating splits chart")
		chartURL, chartConfig, err = c.chart.GenerateSplitsChart(ctx, activity.Splits)
		if err != nil {
			c.logger.Warn("chart generation failed, continuing without chart", "error", err)
		}
	}

	// 8. Save report to DB
	report := &models.Report{
		ActivityID:   activity.ID,
		ReportText:   result.Text,
		ChartURL:     chartURL,
		ChartConfig:  chartConfig,
		Model:        result.Model,
		PromptTokens: result.InputTokens,
		OutputTokens: result.OutputTokens,
	}
	if err := c.store.SaveReport(ctx, report); err != nil {
		return fmt.Errorf("saving report: %w", err)
	}

	// 9. Send to Telegram
	c.setStep("telegram")
	c.logger.Info("sending report to Telegram")
	if chartURL != "" {
		caption := fmt.Sprintf("📊 Splits — %s (%s)", activity.Name, activity.Date.Format("02 Jan"))
		if err := c.telegram.SendPhoto(ctx, chartURL, caption); err != nil {
			c.logger.Warn("failed to send chart to Telegram", "error", err)
		}
	}
	if err := c.telegram.SendMessage(ctx, result.Text); err != nil {
		return fmt.Errorf("sending report to Telegram: %w", err)
	}

	// 10. Update dedup marker
	c.store.SetConfig(ctx, "last_reported_activity_id", strconv.FormatInt(latest.ID, 10))

	c.logger.Info("coaching pipeline complete",
		"activity", activity.Name,
		"distance", fmt.Sprintf("%.2fkm", activity.Distance/1000),
		"tokens_in", result.InputTokens,
		"tokens_out", result.OutputTokens)

	return nil
}

func (c *Coach) buildSystemPrompt() string {
	return fmt.Sprintf(`You are an experienced, empathetic personal running coach. You analyze each training session and provide detailed feedback in European Portuguese (pt-PT).

Your tone: direct, honest, motivating but not over the top. Celebrate real wins, be clear about what to improve.

## Athlete Profile
%s

## Current Training Plan
%s

## How to Interpret Strava Data for Interval Sessions
- When the plan says "Run Xmin / Walk Xmin × N reps", the athlete runs and walks WITHIN THE SAME Strava activity.
- Walk intervals appear as LAPS with slower pace (~10:30+/km) and lower HR — NOT as stops.
- The laps data from the watch is the most accurate source for intervals. Use it to determine if the athlete did intervals (alternating run/walk laps) or continuous running.
- Total time ALWAYS includes 5 min warm-up + 5 min cool-down + walk intervals between reps.
- Example: "Run 2min/Walk 2min ×8" = 5 warm-up + (2+2)×8 + 5 cool-down = ~42 min total. This is NORMAL and CORRECT.
- DO NOT assume the athlete ran continuously just because total time is high. Analyze the laps first.

## Response Format
Respond in European Portuguese (pt-PT) with these sections (use markdown):

📊 **RESUMO DA SESSÃO**
2-3 sentences with key data (actual running time, distance, pace, HR). State which session was planned and whether the athlete followed it.

🧠 **O QUE MAIS ME IMPRESSIONOU**
Detailed analysis: pace per lap, HR zones, cardiac pattern, adaptation signs

⚠️ **FLAG HONESTA**
Compare what was done vs what was in the plan. If the athlete did more or less than prescribed, say it clearly and explain the consequences.

🎯 **CONCLUSÃO**
Overall assessment of the session against the planned session for this day

📈 **PROGRESSO**
Comparison with previous sessions, trends

🏃 **PRÓXIMAS SESSÕES**
What to do in upcoming sessions (specific, based on the plan)

💥 **RESUMO DIRETO**
1-2 raw, honest sentences about the current state`, c.athlete, c.planConfig.ToMarkdown())
}

func (c *Coach) buildUserMessage(a *models.Activity) string {
	splitsJSON, _ := json.MarshalIndent(a.Splits, "", "  ")

	// Build the planned session context
	planContext := ""
	if a.PlanWeek > 0 && a.PlanSession != "" {
		planContext = fmt.Sprintf("\n**Planned session:** Week %d — %s", a.PlanWeek, a.PlanSession)
		planContext += "\n⚠️ COMPARE what was done vs what was planned. If the athlete did more or less than the plan, analyze why and the consequences."
	}

	// Build laps section if available
	lapsSection := ""
	if len(a.Laps) > 0 {
		lapsJSON, _ := json.MarshalIndent(a.Laps, "", "  ")
		lapsSection = fmt.Sprintf(`

**Laps (watch data — more accurate than per-km splits for intervals):**
%s
⚠️ Laps show the actual intervals recorded by the watch. Use this data to determine if the athlete did intervals (alternating run/walk) or continuous running. Laps with slow pace (~10:30+/km) and low HR are walking. Laps with fast pace (~8:00-9:30/km) and high HR are running.`, string(lapsJSON))
	}

	return fmt.Sprintf(`New Strava activity to analyze:

**Activity:** %s
**Date:** %s (%s)
**Type:** %s
**Distance:** %.2f km
**Total time:** %s
**Moving time:** %s
**Average pace:** %s/km
**Average HR:** %.0f bpm
**Max HR:** %.0f bpm%s

**Splits per km:**
%s%s

Remember: the first ~5 min (first lap) is warm-up and the last ~5 min is cool-down. Analyze the ACTUAL running time, not the total.`,
		a.Name,
		a.Date.Format("2 Jan 2006 15:04"),
		dayName(a.Date.Weekday()),
		a.Type,
		a.Distance/1000,
		formatDuration(a.ElapsedTime),
		formatDuration(a.MovingTime),
		a.AvgPace,
		a.AvgHR,
		a.MaxHR,
		planContext,
		string(splitsJSON),
		lapsSection)
}

func formatDuration(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func loadPrompt(mountedPath, embedded string) string {
	if data, err := os.ReadFile(mountedPath); err == nil {
		return string(data)
	}
	return embedded
}
