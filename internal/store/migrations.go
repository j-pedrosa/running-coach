package store

import (
	"context"
	"fmt"
)

var migrations = []string{
	// v1: initial schema
	`CREATE TABLE IF NOT EXISTS config (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS activities (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		strava_id    INTEGER UNIQUE NOT NULL,
		name         TEXT,
		date         DATETIME NOT NULL,
		type         TEXT,
		distance     REAL,
		moving_time  INTEGER,
		elapsed_time INTEGER,
		avg_pace     TEXT,
		avg_hr       REAL,
		max_hr       REAL,
		splits_json  TEXT,
		raw_json     TEXT,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS reports (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		activity_id    INTEGER NOT NULL REFERENCES activities(id),
		report_text    TEXT NOT NULL,
		chart_url      TEXT,
		chart_config   TEXT,
		model          TEXT,
		prompt_tokens  INTEGER,
		output_tokens  INTEGER,
		created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// v2: plan matching columns
	`ALTER TABLE activities ADD COLUMN plan_week INTEGER DEFAULT 0;
	 ALTER TABLE activities ADD COLUMN plan_session TEXT DEFAULT '';`,

	// v3: laps and hr zones data
	`ALTER TABLE activities ADD COLUMN laps_json TEXT DEFAULT '';
	 ALTER TABLE activities ADD COLUMN hr_zones_json TEXT DEFAULT '';`,

	// v4: pipeline event log
	`CREATE TABLE IF NOT EXISTS events (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		type       TEXT NOT NULL,
		message    TEXT NOT NULL,
		detail     TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	// v5: plans, plan weeks, strength sessions, athlete profiles in DB
	`CREATE TABLE IF NOT EXISTS plans (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT NOT NULL,
		start_date   TEXT NOT NULL,
		total_weeks  INTEGER NOT NULL,
		goal         TEXT DEFAULT '',
		goal_km      REAL DEFAULT 0,
		schedule     TEXT DEFAULT '',
		notes        TEXT DEFAULT '',
		status       TEXT DEFAULT 'active',
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
		archived_at  DATETIME
	);

	CREATE TABLE IF NOT EXISTS plan_weeks (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		plan_id         INTEGER NOT NULL REFERENCES plans(id),
		week_number     INTEGER NOT NULL,
		saturday_desc   TEXT DEFAULT '',
		monday_desc     TEXT DEFAULT '',
		wednesday_desc  TEXT DEFAULT '',
		UNIQUE(plan_id, week_number)
	);

	CREATE TABLE IF NOT EXISTS strength_sessions (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		plan_id      INTEGER NOT NULL REFERENCES plans(id),
		week_number  INTEGER NOT NULL,
		done         BOOLEAN DEFAULT FALSE,
		updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(plan_id, week_number)
	);

	CREATE TABLE IF NOT EXISTS athlete_profiles (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		content      TEXT NOT NULL,
		updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
}

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	var current int
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&current); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		if _, err := s.db.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("applying migration %d: %w", i+1, err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			return fmt.Errorf("recording migration %d: %w", i+1, err)
		}
	}

	return nil
}
