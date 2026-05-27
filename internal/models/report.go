package models

import "time"

type Report struct {
	ID            int64     `json:"id"`
	ActivityID    int64     `json:"activity_id"`
	ReportText    string    `json:"report_text"`
	ChartURL      string    `json:"chart_url"`
	ChartConfig   string    `json:"chart_config"`
	Model         string    `json:"model"`
	PromptTokens  int       `json:"prompt_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	CreatedAt     time.Time `json:"created_at"`
}
