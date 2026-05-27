package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/j-pedrosa/running-coach/internal/models"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Config key-value

func (s *Store) GetConfig(ctx context.Context, key string) (string, error) {
	var val string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting config %q: %w", key, err)
	}
	return val, nil
}

func (s *Store) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO config (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value)
	if err != nil {
		return fmt.Errorf("setting config %q: %w", key, err)
	}
	return nil
}

// Activities

func (s *Store) SaveActivity(ctx context.Context, a *models.Activity) error {
	splitsJSON, err := json.Marshal(a.Splits)
	if err != nil {
		return fmt.Errorf("marshaling splits: %w", err)
	}
	lapsJSON, _ := json.Marshal(a.Laps)
	hrZonesJSON, _ := json.Marshal(a.HRZones)

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO activities (strava_id, name, date, type, distance, moving_time, elapsed_time, avg_pace, avg_hr, max_hr, splits_json, laps_json, hr_zones_json, raw_json, plan_week, plan_session)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(strava_id) DO UPDATE SET
		   name=excluded.name, distance=excluded.distance, moving_time=excluded.moving_time,
		   elapsed_time=excluded.elapsed_time, avg_pace=excluded.avg_pace, avg_hr=excluded.avg_hr,
		   max_hr=excluded.max_hr, splits_json=excluded.splits_json, laps_json=excluded.laps_json,
		   hr_zones_json=excluded.hr_zones_json, raw_json=excluded.raw_json,
		   plan_week=excluded.plan_week, plan_session=excluded.plan_session`,
		a.StravaID, a.Name, a.Date.UTC(), a.Type, a.Distance, a.MovingTime, a.ElapsedTime,
		a.AvgPace, a.AvgHR, a.MaxHR, string(splitsJSON), string(lapsJSON), string(hrZonesJSON), a.RawJSON, a.PlanWeek, a.PlanSession)
	if err != nil {
		return fmt.Errorf("saving activity: %w", err)
	}

	id, _ := result.LastInsertId()
	if id > 0 {
		a.ID = id
	}
	return nil
}

func (s *Store) GetActivity(ctx context.Context, stravaID int64) (*models.Activity, error) {
	return s.scanActivity(s.db.QueryRowContext(ctx,
		`SELECT id, strava_id, name, date, type, distance, moving_time, elapsed_time, avg_pace, avg_hr, max_hr, splits_json, laps_json, hr_zones_json, raw_json, plan_week, plan_session, created_at
		 FROM activities WHERE strava_id = ?`, stravaID))
}

func (s *Store) GetLatestActivity(ctx context.Context) (*models.Activity, error) {
	return s.scanActivity(s.db.QueryRowContext(ctx,
		`SELECT id, strava_id, name, date, type, distance, moving_time, elapsed_time, avg_pace, avg_hr, max_hr, splits_json, laps_json, hr_zones_json, raw_json, plan_week, plan_session, created_at
		 FROM activities ORDER BY date DESC LIMIT 1`))
}

func (s *Store) ListActivities(ctx context.Context, limit int) ([]models.Activity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, strava_id, name, date, type, distance, moving_time, elapsed_time, avg_pace, avg_hr, max_hr, splits_json, laps_json, hr_zones_json, raw_json, plan_week, plan_session, created_at
		 FROM activities ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing activities: %w", err)
	}
	defer rows.Close()

	var activities []models.Activity
	for rows.Next() {
		a, err := s.scanActivityRow(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, *a)
	}
	return activities, rows.Err()
}

func (s *Store) scanActivity(row *sql.Row) (*models.Activity, error) {
	a := &models.Activity{}
	var splitsJSON, lapsJSON, hrZonesJSON, rawJSON, planSession sql.NullString
	var dateStr string
	err := row.Scan(&a.ID, &a.StravaID, &a.Name, &dateStr, &a.Type, &a.Distance,
		&a.MovingTime, &a.ElapsedTime, &a.AvgPace, &a.AvgHR, &a.MaxHR,
		&splitsJSON, &lapsJSON, &hrZonesJSON, &rawJSON,
		&a.PlanWeek, &planSession, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning activity: %w", err)
	}
	a.Date = parseDate(dateStr)
	if splitsJSON.Valid {
		json.Unmarshal([]byte(splitsJSON.String), &a.Splits)
	}
	if lapsJSON.Valid {
		json.Unmarshal([]byte(lapsJSON.String), &a.Laps)
	}
	if hrZonesJSON.Valid {
		json.Unmarshal([]byte(hrZonesJSON.String), &a.HRZones)
	}
	if rawJSON.Valid {
		a.RawJSON = rawJSON.String
	}
	if planSession.Valid {
		a.PlanSession = planSession.String
	}
	return a, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func (s *Store) scanActivityRow(row scannable) (*models.Activity, error) {
	a := &models.Activity{}
	var splitsJSON, lapsJSON, hrZonesJSON, rawJSON, planSession sql.NullString
	var dateStr string
	err := row.Scan(&a.ID, &a.StravaID, &a.Name, &dateStr, &a.Type, &a.Distance,
		&a.MovingTime, &a.ElapsedTime, &a.AvgPace, &a.AvgHR, &a.MaxHR,
		&splitsJSON, &lapsJSON, &hrZonesJSON, &rawJSON,
		&a.PlanWeek, &planSession, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning activity row: %w", err)
	}
	a.Date = parseDate(dateStr)
	if splitsJSON.Valid {
		json.Unmarshal([]byte(splitsJSON.String), &a.Splits)
	}
	if lapsJSON.Valid {
		json.Unmarshal([]byte(lapsJSON.String), &a.Laps)
	}
	if hrZonesJSON.Valid {
		json.Unmarshal([]byte(hrZonesJSON.String), &a.HRZones)
	}
	if rawJSON.Valid {
		a.RawJSON = rawJSON.String
	}
	if planSession.Valid {
		a.PlanSession = planSession.String
	}
	return a, nil
}

func parseDate(dateStr string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05-07:00", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Reports

func (s *Store) SaveReport(ctx context.Context, r *models.Report) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO reports (activity_id, report_text, chart_url, chart_config, model, prompt_tokens, output_tokens)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ActivityID, r.ReportText, r.ChartURL, r.ChartConfig, r.Model, r.PromptTokens, r.OutputTokens)
	if err != nil {
		return fmt.Errorf("saving report: %w", err)
	}
	id, _ := result.LastInsertId()
	r.ID = id
	return nil
}

func (s *Store) GetLatestReport(ctx context.Context) (*models.Report, error) {
	r := &models.Report{}
	var chartURL, chartConfig, model sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, activity_id, report_text, chart_url, chart_config, model, prompt_tokens, output_tokens, created_at
		 FROM reports ORDER BY created_at DESC LIMIT 1`).
		Scan(&r.ID, &r.ActivityID, &r.ReportText, &chartURL, &chartConfig, &model,
			&r.PromptTokens, &r.OutputTokens, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest report: %w", err)
	}
	if chartURL.Valid {
		r.ChartURL = chartURL.String
	}
	if chartConfig.Valid {
		r.ChartConfig = chartConfig.String
	}
	if model.Valid {
		r.Model = model.String
	}
	return r, nil
}

func (s *Store) GetReportByActivity(ctx context.Context, activityID int64) (*models.Report, error) {
	r := &models.Report{}
	var chartURL, chartConfig, model sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, activity_id, report_text, chart_url, chart_config, model, prompt_tokens, output_tokens, created_at
		 FROM reports WHERE activity_id = ?`, activityID).
		Scan(&r.ID, &r.ActivityID, &r.ReportText, &chartURL, &chartConfig, &model,
			&r.PromptTokens, &r.OutputTokens, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting report by activity: %w", err)
	}
	if chartURL.Valid {
		r.ChartURL = chartURL.String
	}
	if chartConfig.Valid {
		r.ChartConfig = chartConfig.String
	}
	if model.Valid {
		r.Model = model.String
	}
	return r, nil
}
