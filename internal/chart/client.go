package chart

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/j-pedrosa/running-coach/internal/models"
)

type Client struct {
	http    *http.Client
	baseURL string
	logger  *slog.Logger
}

func NewClient(baseURL string, logger *slog.Logger) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: baseURL,
		logger:  logger,
	}
}

func (c *Client) GenerateSplitsChart(ctx context.Context, splits []models.Split) (chartURL, chartConfig string, err error) {
	config := buildChartConfig(splits)
	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", "", fmt.Errorf("marshaling chart config: %w", err)
	}

	chartConfig = string(configJSON)

	reqBody := map[string]any{
		"chart":           config,
		"width":           800,
		"height":          400,
		"backgroundColor": "#1a1a2e",
		"format":          "png",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshaling request: %w", err)
	}

	// Use /chart/create to get a short URL back (not the raw image)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chart/create", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("calling QuickChart: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("QuickChart error (%d): %s", resp.StatusCode, respBody)
	}

	var result struct {
		Success bool   `json:"success"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("decoding response: %w", err)
	}

	if !result.Success || result.URL == "" {
		return "", "", fmt.Errorf("QuickChart returned no URL: %s", respBody)
	}

	return result.URL, chartConfig, nil
}

func BuildChartConfigJSON(splits []models.Split) string {
	config := buildChartConfig(splits)
	data, _ := json.Marshal(config)
	return string(data)
}

func buildChartConfig(splits []models.Split) map[string]any {
	labels := make([]string, len(splits))
	paceData := make([]float64, len(splits))
	hrData := make([]float64, len(splits))
	bgColors := make([]string, len(splits))

	for i, s := range splits {
		labels[i] = fmt.Sprintf("Km %d", s.Kilometer)

		// Convert pace string to seconds for chart
		if s.AvgSpeed > 0 {
			paceData[i] = 1000.0 / s.AvgSpeed // seconds per km
		}
		hrData[i] = s.AvgHR

		// Color coding: green for warm-up/cool-down, blue for running
		if s.AvgHR > 0 && s.AvgHR < 110 {
			bgColors[i] = "rgba(76, 175, 80, 0.7)" // green — warm-up/cool-down
		} else {
			bgColors[i] = "rgba(66, 165, 245, 0.7)" // blue — running
		}
	}

	return map[string]any{
		"type": "bar",
		"data": map[string]any{
			"labels": labels,
			"datasets": []map[string]any{
				{
					"label":           "Ritmo (s/km)",
					"data":            paceData,
					"backgroundColor": bgColors,
					"borderColor":     bgColors,
					"borderWidth":     1,
					"yAxisID":         "y",
					"order":           2,
				},
				{
					"label":       "FC (bpm)",
					"data":        hrData,
					"type":        "line",
					"borderColor": "rgba(255, 152, 0, 1)",
					"backgroundColor": "rgba(255, 152, 0, 0.1)",
					"borderWidth": 2,
					"pointRadius": 4,
					"fill":        false,
					"yAxisID":     "y1",
					"order":       1,
				},
			},
		},
		"options": map[string]any{
			"responsive": true,
			"plugins": map[string]any{
				"legend": map[string]any{
					"labels": map[string]any{
						"color": "#e0e0e0",
					},
				},
				"title": map[string]any{
					"display": true,
					"text":    "Splits — Ritmo & Frequência Cardíaca",
					"color":   "#e0e0e0",
				},
			},
			"scales": map[string]any{
				"x": map[string]any{
					"ticks": map[string]any{"color": "#aaa"},
					"grid":  map[string]any{"color": "rgba(255,255,255,0.1)"},
				},
				"y": map[string]any{
					"type":     "linear",
					"position": "left",
					"title": map[string]any{
						"display": true,
						"text":    "Ritmo (s/km)",
						"color":   "#aaa",
					},
					"ticks": map[string]any{"color": "#aaa"},
					"grid":  map[string]any{"color": "rgba(255,255,255,0.1)"},
				},
				"y1": map[string]any{
					"type":     "linear",
					"position": "right",
					"title": map[string]any{
						"display": true,
						"text":    "FC (bpm)",
						"color":   "#aaa",
					},
					"ticks": map[string]any{"color": "#aaa"},
					"grid":  map[string]any{"display": false},
				},
			},
		},
	}
}
