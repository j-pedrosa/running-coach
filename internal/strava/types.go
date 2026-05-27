package strava

import "time"

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

type ActivitySummary struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	StartDate        time.Time `json:"start_date"`
	Distance         float64   `json:"distance"`
	MovingTime       int       `json:"moving_time"`
	ElapsedTime      int       `json:"elapsed_time"`
	AverageSpeed     float64   `json:"average_speed"`
	AverageHeartrate float64   `json:"average_heartrate"`
	MaxHeartrate     float64   `json:"max_heartrate"`
	HasHeartrate     bool      `json:"has_heartrate"`
}

type ActivityDetail struct {
	ActivitySummary
	SplitsMetric []StravaSplit `json:"splits_metric"`
	Laps         []Lap         `json:"laps"`
}

type StravaSplit struct {
	Distance         float64 `json:"distance"`
	ElapsedTime      int     `json:"elapsed_time"`
	MovingTime       int     `json:"moving_time"`
	AverageSpeed     float64 `json:"average_speed"`
	AverageHeartrate float64 `json:"average_heartrate"`
	Split            int     `json:"split"`
}

type Lap struct {
	Name             string  `json:"name"`
	LapIndex         int     `json:"lap_index"`
	Distance         float64 `json:"distance"`
	MovingTime       int     `json:"moving_time"`
	ElapsedTime      int     `json:"elapsed_time"`
	AverageSpeed     float64 `json:"average_speed"`
	AverageHeartrate float64 `json:"average_heartrate"`
	MaxHeartrate     float64 `json:"max_heartrate"`
}

type StreamSet struct {
	Heartrate *Stream `json:"heartrate"`
	Time      *Stream `json:"time"`
	Distance  *Stream `json:"distance"`
}

type Stream struct {
	Data []float64 `json:"data"`
}

type StreamResponse []struct {
	Type string    `json:"type"`
	Data []float64 `json:"data"`
}
