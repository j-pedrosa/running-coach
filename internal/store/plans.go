package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PlanRow represents a plan record from the database.
type PlanRow struct {
	ID         int64
	Name       string
	StartDate  string
	TotalWeeks int
	Goal       string
	GoalKm     float64
	Schedule   string
	Notes      string
	Status     string
	CreatedAt  time.Time
	ArchivedAt *time.Time
}

// PlanWeekRow represents a week's session descriptions.
type PlanWeekRow struct {
	WeekNumber    int
	SaturdayDesc  string
	MondayDesc    string
	WednesdayDesc string
}

// GetActivePlan returns the currently active plan, or nil if none.
func (s *Store) GetActivePlan(ctx context.Context) (*PlanRow, error) {
	p := &PlanRow{}
	var archivedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, start_date, total_weeks, goal, goal_km, schedule, notes, status, created_at, archived_at
		 FROM plans WHERE status = 'active' ORDER BY created_at DESC LIMIT 1`).
		Scan(&p.ID, &p.Name, &p.StartDate, &p.TotalWeeks, &p.Goal, &p.GoalKm,
			&p.Schedule, &p.Notes, &p.Status, &p.CreatedAt, &archivedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting active plan: %w", err)
	}
	if archivedAt.Valid {
		p.ArchivedAt = &archivedAt.Time
	}
	return p, nil
}

// ListPlans returns all plans ordered by creation date (newest first).
func (s *Store) ListPlans(ctx context.Context) ([]PlanRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, start_date, total_weeks, goal, goal_km, schedule, notes, status, created_at, archived_at
		 FROM plans ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing plans: %w", err)
	}
	defer rows.Close()

	var plans []PlanRow
	for rows.Next() {
		p := PlanRow{}
		var archivedAt sql.NullTime
		if err := rows.Scan(&p.ID, &p.Name, &p.StartDate, &p.TotalWeeks, &p.Goal, &p.GoalKm,
			&p.Schedule, &p.Notes, &p.Status, &p.CreatedAt, &archivedAt); err != nil {
			return nil, err
		}
		if archivedAt.Valid {
			p.ArchivedAt = &archivedAt.Time
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// SavePlan inserts a new plan or updates an existing one. Returns the plan ID.
func (s *Store) SavePlan(ctx context.Context, p *PlanRow) (int64, error) {
	if p.ID > 0 {
		_, err := s.db.ExecContext(ctx,
			`UPDATE plans SET name=?, start_date=?, total_weeks=?, goal=?, goal_km=?, schedule=?, notes=?, status=?
			 WHERE id=?`,
			p.Name, p.StartDate, p.TotalWeeks, p.Goal, p.GoalKm, p.Schedule, p.Notes, p.Status, p.ID)
		if err != nil {
			return 0, fmt.Errorf("updating plan: %w", err)
		}
		return p.ID, nil
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO plans (name, start_date, total_weeks, goal, goal_km, schedule, notes, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.StartDate, p.TotalWeeks, p.Goal, p.GoalKm, p.Schedule, p.Notes, p.Status)
	if err != nil {
		return 0, fmt.Errorf("inserting plan: %w", err)
	}
	return result.LastInsertId()
}

// ArchivePlan sets a plan's status to 'archived'.
func (s *Store) ArchivePlan(ctx context.Context, planID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE plans SET status='archived', archived_at=CURRENT_TIMESTAMP WHERE id=?`, planID)
	if err != nil {
		return fmt.Errorf("archiving plan: %w", err)
	}
	return nil
}

// GetPlanWeeks returns all weeks for a plan.
func (s *Store) GetPlanWeeks(ctx context.Context, planID int64) ([]PlanWeekRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT week_number, saturday_desc, monday_desc, wednesday_desc
		 FROM plan_weeks WHERE plan_id=? ORDER BY week_number`, planID)
	if err != nil {
		return nil, fmt.Errorf("getting plan weeks: %w", err)
	}
	defer rows.Close()

	var weeks []PlanWeekRow
	for rows.Next() {
		var w PlanWeekRow
		if err := rows.Scan(&w.WeekNumber, &w.SaturdayDesc, &w.MondayDesc, &w.WednesdayDesc); err != nil {
			return nil, err
		}
		weeks = append(weeks, w)
	}
	return weeks, rows.Err()
}

// SavePlanWeeks replaces all weeks for a plan.
func (s *Store) SavePlanWeeks(ctx context.Context, planID int64, weeks []PlanWeekRow) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plan_weeks WHERE plan_id=?`, planID)
	if err != nil {
		return fmt.Errorf("clearing plan weeks: %w", err)
	}

	for _, w := range weeks {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO plan_weeks (plan_id, week_number, saturday_desc, monday_desc, wednesday_desc)
			 VALUES (?, ?, ?, ?, ?)`,
			planID, w.WeekNumber, w.SaturdayDesc, w.MondayDesc, w.WednesdayDesc)
		if err != nil {
			return fmt.Errorf("inserting week %d: %w", w.WeekNumber, err)
		}
	}
	return nil
}

// GetStrengthDone returns whether a strength session is marked done.
func (s *Store) GetStrengthDone(ctx context.Context, planID int64, week int) (bool, error) {
	var done bool
	err := s.db.QueryRowContext(ctx,
		`SELECT done FROM strength_sessions WHERE plan_id=? AND week_number=?`, planID, week).Scan(&done)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return done, err
}

// SetStrengthDone sets the strength session done flag.
func (s *Store) SetStrengthDone(ctx context.Context, planID int64, week int, done bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO strength_sessions (plan_id, week_number, done, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(plan_id, week_number) DO UPDATE SET done=excluded.done, updated_at=CURRENT_TIMESTAMP`,
		planID, week, done)
	return err
}

// GetStrengthDoneMap returns all strength done flags for a plan.
func (s *Store) GetStrengthDoneMap(ctx context.Context, planID int64) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT week_number, done FROM strength_sessions WHERE plan_id=?`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[int]bool)
	for rows.Next() {
		var week int
		var done bool
		if err := rows.Scan(&week, &done); err != nil {
			return nil, err
		}
		m[week] = done
	}
	return m, rows.Err()
}

// GetAthleteProfile returns the latest athlete profile markdown.
func (s *Store) GetAthleteProfile(ctx context.Context) (string, error) {
	var content string
	err := s.db.QueryRowContext(ctx,
		`SELECT content FROM athlete_profiles ORDER BY updated_at DESC LIMIT 1`).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting athlete profile: %w", err)
	}
	return content, nil
}

// SaveAthleteProfile inserts or updates the athlete profile.
func (s *Store) SaveAthleteProfile(ctx context.Context, content string) error {
	// Check if one exists
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM athlete_profiles LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO athlete_profiles (content) VALUES (?)`, content)
	} else {
		_, err = s.db.ExecContext(ctx,
			`UPDATE athlete_profiles SET content=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, content, id)
	}
	if err != nil {
		return fmt.Errorf("saving athlete profile: %w", err)
	}
	return nil
}
