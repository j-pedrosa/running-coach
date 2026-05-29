package models

import "time"

type Split struct {
	Kilometer  int     `json:"kilometer"`
	Distance   float64 `json:"distance"`
	MovingTime int     `json:"moving_time"`
	Pace       string  `json:"pace"`
	AvgHR      float64 `json:"avg_hr"`
	AvgSpeed   float64 `json:"avg_speed"`
}

type Lap struct {
	Index      int     `json:"index"`
	Distance   float64 `json:"distance"`
	MovingTime int     `json:"moving_time"`
	Pace       string  `json:"pace"`
	AvgHR      float64 `json:"avg_hr"`
	MaxHR      float64 `json:"max_hr"`
}

type HRZone struct {
	Name    string `json:"name"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
	Seconds int    `json:"seconds"`
	Percent int    `json:"percent"`
}

type Activity struct {
	ID          int64     `json:"id"`
	StravaID    int64     `json:"strava_id"`
	Name        string    `json:"name"`
	Date        time.Time `json:"date"`
	Type        string    `json:"type"`
	Distance    float64   `json:"distance"`
	MovingTime  int       `json:"moving_time"`
	ElapsedTime int       `json:"elapsed_time"`
	AvgPace     string    `json:"avg_pace"`
	AvgHR       float64   `json:"avg_hr"`
	MaxHR       float64   `json:"max_hr"`
	Splits      []Split   `json:"splits"`
	Laps        []Lap     `json:"laps"`
	HRZones     []HRZone  `json:"hr_zones"`
	PlanWeek    int       `json:"plan_week,omitempty"`
	PlanSession string    `json:"plan_session,omitempty"`
	RawJSON     string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}
