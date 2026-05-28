package coach

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/j-pedrosa/running-coach/internal/models"
	"github.com/j-pedrosa/running-coach/internal/store"
	"gopkg.in/yaml.v3"
)

// PlanConfig is the in-memory representation of a training plan.
type PlanConfig struct {
	ID         int64                  `yaml:"-" json:"id"`
	Name       string                 `yaml:"name" json:"name"`
	StartDate  string                 `yaml:"start_date" json:"start_date"`
	TotalWeeks int                    `yaml:"total_weeks" json:"total_weeks"`
	Schedule   string                 `yaml:"schedule" json:"schedule"`
	Notes      string                 `yaml:"notes" json:"notes"`
	Goal       string                 `yaml:"goal" json:"goal"`
	GoalKm     float64                `yaml:"goal_km" json:"goal_km"`
	Weeks      map[int]PlanWeekConfig `yaml:"weeks" json:"weeks"`
	startTime  time.Time
}

type PlanWeekConfig struct {
	Saturday  string `yaml:"saturday" json:"saturday"`
	Monday    string `yaml:"monday" json:"monday"`
	Wednesday string `yaml:"wednesday" json:"wednesday"`
}

// LoadPlanFromDB loads the active plan from the database.
func LoadPlanFromDB(ctx context.Context, st *store.Store, logger *slog.Logger) (*PlanConfig, error) {
	plan, err := st.GetActivePlan(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting active plan: %w", err)
	}
	if plan == nil {
		return nil, nil // no plan in DB
	}

	weeks, err := st.GetPlanWeeks(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("getting plan weeks: %w", err)
	}

	weekMap := make(map[int]PlanWeekConfig, len(weeks))
	for _, w := range weeks {
		weekMap[w.WeekNumber] = PlanWeekConfig{
			Saturday:  w.SaturdayDesc,
			Monday:    w.MondayDesc,
			Wednesday: w.WednesdayDesc,
		}
	}

	t, _ := time.Parse("2006-01-02", plan.StartDate)

	cfg := &PlanConfig{
		ID:         plan.ID,
		Name:       plan.Name,
		StartDate:  plan.StartDate,
		TotalWeeks: plan.TotalWeeks,
		Goal:       plan.Goal,
		GoalKm:     plan.GoalKm,
		Schedule:   plan.Schedule,
		Notes:      plan.Notes,
		Weeks:      weekMap,
		startTime:  t,
	}

	logger.Info("loaded plan from database", "id", plan.ID, "name", plan.Name)
	return cfg, nil
}

// SavePlanToDB persists a PlanConfig to the database. Archives the current active plan if different.
func SavePlanToDB(ctx context.Context, st *store.Store, cfg *PlanConfig) (int64, error) {
	// Archive current active plan if we're creating a new one
	if cfg.ID == 0 {
		existing, _ := st.GetActivePlan(ctx)
		if existing != nil {
			st.ArchivePlan(ctx, existing.ID)
		}
	}

	row := &store.PlanRow{
		ID:         cfg.ID,
		Name:       cfg.Name,
		StartDate:  cfg.StartDate,
		TotalWeeks: cfg.TotalWeeks,
		Goal:       cfg.Goal,
		GoalKm:     cfg.GoalKm,
		Schedule:   cfg.Schedule,
		Notes:      cfg.Notes,
		Status:     "active",
	}

	planID, err := st.SavePlan(ctx, row)
	if err != nil {
		return 0, err
	}

	weeks := make([]store.PlanWeekRow, 0, len(cfg.Weeks))
	for num, w := range cfg.Weeks {
		weeks = append(weeks, store.PlanWeekRow{
			WeekNumber:    num,
			SaturdayDesc:  w.Saturday,
			MondayDesc:    w.Monday,
			WednesdayDesc: w.Wednesday,
		})
	}

	if err := st.SavePlanWeeks(ctx, planID, weeks); err != nil {
		return 0, err
	}

	cfg.ID = planID
	return planID, nil
}

// SeedFromYAML loads plan from YAML file and saves to DB. Returns nil if no file found.
func SeedFromYAML(ctx context.Context, st *store.Store, logger *slog.Logger) (*PlanConfig, error) {
	cfg := LoadPlanConfig(logger)
	if cfg == nil {
		return nil, nil
	}

	planID, err := SavePlanToDB(ctx, st, cfg)
	if err != nil {
		return nil, fmt.Errorf("seeding plan from YAML: %w", err)
	}
	cfg.ID = planID

	// Migrate old strength_done_wX config keys
	for w := 1; w <= cfg.TotalWeeks; w++ {
		val, _ := st.GetConfig(ctx, fmt.Sprintf("strength_done_w%d", w))
		if val == "true" {
			st.SetStrengthDone(ctx, planID, w, true)
		}
	}

	logger.Info("seeded plan from YAML to database", "id", planID, "name", cfg.Name)
	return cfg, nil
}

// ToMarkdown generates a training plan description for Claude's context.
func (p *PlanConfig) ToMarkdown() string {
	if p == nil {
		return "No training plan configured."
	}

	s := fmt.Sprintf("# Training Plan\n\n")
	s += fmt.Sprintf("- **Started:** %s\n", p.StartDate)
	s += fmt.Sprintf("- **Total weeks:** %d\n", p.TotalWeeks)
	s += fmt.Sprintf("- **Current week:** %d\n", p.CurrentWeek())
	if p.Schedule != "" {
		s += fmt.Sprintf("\n## Schedule\n%s\n", p.Schedule)
	}

	s += "\n## Weekly Sessions\n"
	for w := 1; w <= p.TotalWeeks; w++ {
		wk, ok := p.Weeks[w]
		if !ok {
			continue
		}
		status := ""
		cw := p.CurrentWeek()
		if w < cw {
			status = " (DONE)"
		} else if w == cw {
			status = " (CURRENT)"
		}
		s += fmt.Sprintf("\n### Week %d%s\n", w, status)
		s += fmt.Sprintf("- **Saturday (Run):** %s\n", wk.Saturday)
		s += fmt.Sprintf("- **Monday (Run):** %s\n", wk.Monday)
		s += fmt.Sprintf("- **Wednesday (Strength):** %s\n", wk.Wednesday)
	}

	if p.Notes != "" {
		s += fmt.Sprintf("\n## Coaching Notes\n%s\n", p.Notes)
	}

	return s
}

func LoadPlanConfig(logger *slog.Logger) *PlanConfig {
	paths := []string{"/app/config/plan-config.yaml", "config/plan-config.yaml"}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg PlanConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			logger.Error("failed to parse plan config", "path", path, "error", err)
			continue
		}
		t, err := time.Parse("2006-01-02", cfg.StartDate)
		if err != nil {
			logger.Error("failed to parse plan start_date", "date", cfg.StartDate, "error", err)
			continue
		}
		cfg.startTime = t
		logger.Info("loaded plan config", "path", path, "start", cfg.StartDate, "weeks", cfg.TotalWeeks)
		return &cfg
	}

	logger.Warn("no plan-config.yaml found, plan features disabled")
	return nil
}

func (p *PlanConfig) RunDesc(week int, day time.Weekday) string {
	if p == nil {
		return ""
	}
	w, ok := p.Weeks[week]
	if !ok {
		return ""
	}
	switch day {
	case time.Saturday:
		return w.Saturday
	case time.Monday:
		return w.Monday
	}
	return ""
}

func (p *PlanConfig) StrengthDesc(week int) string {
	if p == nil {
		return ""
	}
	w, ok := p.Weeks[week]
	if !ok {
		return ""
	}
	return w.Wednesday
}

// MatchActivityToPlan sets PlanWeek and PlanSession on an activity.
func MatchActivityToPlan(a *models.Activity, plan *PlanConfig) {
	if plan == nil {
		return
	}

	days := daysSince(a.Date, plan.startTime)
	if days < 0 {
		return
	}

	week := (days / 7) + 1
	a.PlanWeek = week

	if week > plan.TotalWeeks {
		a.PlanSession = "Fora do plano"
		return
	}

	day := a.Date.Weekday()
	desc := plan.RunDesc(week, day)
	if desc != "" {
		a.PlanSession = fmt.Sprintf("S%d %s — %s", week, dayName(day), desc)
		return
	}

	a.PlanSession = fmt.Sprintf("S%d %s — Extra", week, dayName(day))
}

// ── Plan Status for frontend ──────────────────────────

type PlanStatus struct {
	Name        string       `json:"name"`
	Goal        string       `json:"goal"`
	GoalKm      float64      `json:"goal_km"`
	CurrentWeek int          `json:"current_week"`
	TotalWeeks  int          `json:"total_weeks"`
	Progress    int          `json:"progress"`
	Weeks       []WeekStatus `json:"weeks"`
}

type WeekStatus struct {
	Week     int             `json:"week"`
	Status   string          `json:"status"`
	Sessions []SessionStatus `json:"sessions"`
}

type SessionStatus struct {
	Day         string `json:"day"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Status      string `json:"status"`
	ActivityID  int64  `json:"activity_id,omitempty"`
}

func BuildPlanStatus(plan *PlanConfig, activities []models.Activity, strengthDone map[int]bool) PlanStatus {
	if plan == nil {
		return PlanStatus{}
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	start := plan.startTime.UTC().Truncate(24 * time.Hour)

	currentWeek := (daysSince(now, start) / 7) + 1
	if currentWeek < 1 {
		currentWeek = 1
	}
	if currentWeek > plan.TotalWeeks {
		currentWeek = plan.TotalWeeks
	}

	// Index activities by week + weekday
	actIndex := make(map[string]*models.Activity)
	for i := range activities {
		a := &activities[i]
		if a.PlanWeek > 0 && a.PlanWeek <= plan.TotalWeeks {
			actIndex[fmt.Sprintf("%d-%d", a.PlanWeek, a.Date.Weekday())] = a
		}
	}

	// Sat→Fri week: Saturday=day0
	weekdayOffset := func(day time.Weekday) int {
		return (int(day) - int(time.Saturday) + 7) % 7
	}
	sessionDate := func(week int, day time.Weekday) time.Time {
		return start.AddDate(0, 0, (week-1)*7+weekdayOffset(day))
	}
	isPast := func(week int, day time.Weekday) bool {
		return sessionDate(week, day).Before(now)
	}

	weeks := make([]WeekStatus, plan.TotalWeeks)
	for w := 1; w <= plan.TotalWeeks; w++ {
		ws := WeekStatus{Week: w}

		if w < currentWeek {
			ws.Status = "done"
		} else if w == currentWeek {
			ws.Status = "current"
		} else {
			ws.Status = "upcoming"
		}

		// Saturday run
		satSession := SessionStatus{Day: "Sáb", Type: "run", Description: plan.RunDesc(w, time.Saturday)}
		if a, ok := actIndex[fmt.Sprintf("%d-%d", w, time.Saturday)]; ok {
			satSession.Status = "done"
			satSession.ActivityID = a.ID
		} else if isPast(w, time.Saturday) {
			satSession.Status = "missed"
		} else {
			satSession.Status = "upcoming"
		}

		// Monday run
		monSession := SessionStatus{Day: "Seg", Type: "run", Description: plan.RunDesc(w, time.Monday)}
		if a, ok := actIndex[fmt.Sprintf("%d-%d", w, time.Monday)]; ok {
			monSession.Status = "done"
			monSession.ActivityID = a.ID
		} else if isPast(w, time.Monday) {
			monSession.Status = "missed"
		} else {
			monSession.Status = "upcoming"
		}

		// Wednesday strength — checked manually via UI
		wedSession := SessionStatus{Day: "Qua", Type: "strength", Description: plan.StrengthDesc(w)}
		if strengthDone[w] {
			wedSession.Status = "done"
		} else if isPast(w, time.Wednesday) {
			wedSession.Status = "missed"
		} else {
			wedSession.Status = "upcoming"
		}

		ws.Sessions = []SessionStatus{satSession, monSession, wedSession}

		// Auto-detect: if all sessions are done, mark the week as done
		allDone := true
		for _, s := range ws.Sessions {
			if s.Status != "done" {
				allDone = false
				break
			}
		}
		if allDone {
			ws.Status = "done"
		}

		weeks[w-1] = ws
	}

	// Calculate progress as percentage of fully completed weeks
	doneWeeks := 0
	for _, w := range weeks {
		if w.Status == "done" {
			doneWeeks++
		}
	}
	progress := (doneWeeks * 100) / plan.TotalWeeks

	return PlanStatus{
		Name:        plan.Name,
		Goal:        plan.Goal,
		GoalKm:      plan.GoalKm,
		CurrentWeek: currentWeek,
		TotalWeeks:  plan.TotalWeeks,
		Progress:    progress,
		Weeks:       weeks,
	}
}

func daysSince(date, start time.Time) int {
	d := date.UTC().Truncate(24 * time.Hour)
	s := start.UTC().Truncate(24 * time.Hour)
	return int(d.Sub(s).Hours() / 24)
}

func dayName(d time.Weekday) string {
	names := map[time.Weekday]string{
		time.Monday: "Seg", time.Tuesday: "Ter", time.Wednesday: "Qua",
		time.Thursday: "Qui", time.Friday: "Sex", time.Saturday: "Sáb", time.Sunday: "Dom",
	}
	return names[d]
}

// WeekForDate returns the plan week number for a given strava ID (for API use).
func (p *PlanConfig) WeekForDate(date time.Time) int {
	if p == nil {
		return 0
	}
	days := daysSince(date, p.startTime)
	if days < 0 {
		return 0
	}
	w := (days / 7) + 1
	if w > p.TotalWeeks {
		return 0
	}
	return w
}

// CurrentWeek returns the current plan week based on today's date (for prompt context).
func (p *PlanConfig) CurrentWeek() int {
	if p == nil {
		return 0
	}
	return p.WeekForDate(time.Now())
}

// CurrentWeekStr returns "X of Y" for prompt injection.
func (p *PlanConfig) CurrentWeekStr() string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(p.CurrentWeek()) + " of " + strconv.Itoa(p.TotalWeeks)
}
