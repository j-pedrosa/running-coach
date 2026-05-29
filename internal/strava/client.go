package strava

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/j-pedrosa/running-coach/internal/models"
)

const baseURL = "https://www.strava.com"

type TokenStore interface {
	GetConfig(ctx context.Context, key string) (string, error)
	SetConfig(ctx context.Context, key, value string) error
}

type Client struct {
	http         *http.Client
	clientID     string
	clientSecret string
	store        TokenStore
	logger       *slog.Logger
}

func NewClient(clientID, clientSecret string, store TokenStore, logger *slog.Logger) *Client {
	return &Client{
		http:         &http.Client{Timeout: 30 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		store:        store,
		logger:       logger,
	}
}

func (c *Client) SeedToken(ctx context.Context, refreshToken string) error {
	existing, err := c.store.GetConfig(ctx, "strava_refresh_token")
	if err != nil {
		return fmt.Errorf("checking existing token: %w", err)
	}
	if existing != "" {
		return nil
	}
	c.logger.Info("seeding initial Strava refresh token")
	return c.store.SetConfig(ctx, "strava_refresh_token", refreshToken)
}

func (c *Client) ensureToken(ctx context.Context) (string, error) {
	accessToken, _ := c.store.GetConfig(ctx, "strava_access_token")
	expiresAtStr, _ := c.store.GetConfig(ctx, "strava_expires_at")

	if accessToken != "" && expiresAtStr != "" {
		expiresAt, _ := strconv.ParseInt(expiresAtStr, 10, 64)
		if time.Now().Unix() < expiresAt-300 {
			return accessToken, nil
		}
	}

	refreshToken, err := c.store.GetConfig(ctx, "strava_refresh_token")
	if err != nil || refreshToken == "" {
		return "", fmt.Errorf("no refresh token available")
	}

	c.logger.Info("refreshing Strava access token")

	resp, err := c.http.PostForm(baseURL+"/oauth/token", url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, body)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	c.store.SetConfig(ctx, "strava_access_token", tok.AccessToken)
	c.store.SetConfig(ctx, "strava_refresh_token", tok.RefreshToken)
	c.store.SetConfig(ctx, "strava_expires_at", strconv.FormatInt(tok.ExpiresAt, 10))

	return tok.AccessToken, nil
}

func (c *Client) GetActivities(ctx context.Context, perPage int, after int64) ([]ActivitySummary, error) {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://www.strava.com/api/v3/athlete/activities?per_page=%d", perPage)
	if after > 0 {
		url += fmt.Sprintf("&after=%d", after)
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching activities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("activities API error (%d): %s", resp.StatusCode, body)
	}

	var activities []ActivitySummary
	if err := json.NewDecoder(resp.Body).Decode(&activities); err != nil {
		return nil, fmt.Errorf("decoding activities: %w", err)
	}

	return activities, nil
}

func (c *Client) GetLatestActivity(ctx context.Context) (*ActivitySummary, error) {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://www.strava.com/api/v3/athlete/activities?per_page=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching activities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("activities API error (%d): %s", resp.StatusCode, body)
	}

	var activities []ActivitySummary
	if err := json.NewDecoder(resp.Body).Decode(&activities); err != nil {
		return nil, fmt.Errorf("decoding activities: %w", err)
	}
	if len(activities) == 0 {
		return nil, nil
	}

	return &activities[0], nil
}

func (c *Client) GetActivityDetail(ctx context.Context, id int64) (*ActivityDetail, error) {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://www.strava.com/api/v3/activities/%d", id)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching activity detail: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("activity detail API error (%d): %s", resp.StatusCode, body)
	}

	var detail ActivityDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("decoding activity detail: %w", err)
	}

	return &detail, nil
}

func (c *Client) GetActivityStreams(ctx context.Context, id int64) (*StreamSet, error) {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://www.strava.com/api/v3/activities/%d/streams?keys=heartrate,time,distance&key_type=distance", id)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching streams: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("streams API error (%d): %s", resp.StatusCode, body)
	}

	var streams StreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return nil, fmt.Errorf("decoding streams: %w", err)
	}

	set := &StreamSet{}
	for _, s := range streams {
		stream := &Stream{Data: s.Data}
		switch s.Type {
		case "heartrate":
			set.Heartrate = stream
		case "time":
			set.Time = stream
		case "distance":
			set.Distance = stream
		}
	}

	return set, nil
}

func (c *Client) BuildSplits(detail *ActivityDetail, streams *StreamSet) []models.Split {
	splits := make([]models.Split, len(detail.SplitsMetric))

	for i, s := range detail.SplitsMetric {
		pace := speedToPace(s.AverageSpeed)
		splits[i] = models.Split{
			Kilometer:  s.Split,
			Distance:   s.Distance,
			MovingTime: s.MovingTime,
			Pace:       pace,
			AvgSpeed:   s.AverageSpeed,
			AvgHR:      s.AverageHeartrate,
		}
	}

	// If splits don't have HR but streams do, merge HR from streams
	if streams != nil && streams.Distance != nil && streams.Heartrate != nil {
		hrPerKm := computeHRPerKm(streams)
		for i := range splits {
			if splits[i].AvgHR == 0 && i < len(hrPerKm) {
				splits[i].AvgHR = hrPerKm[i]
			}
		}
	}

	return splits
}

func computeHRPerKm(streams *StreamSet) []float64 {
	if len(streams.Distance.Data) != len(streams.Heartrate.Data) {
		return nil
	}

	var result []float64
	var hrSum float64
	var count int
	km := 1

	for i, dist := range streams.Distance.Data {
		hrSum += streams.Heartrate.Data[i]
		count++

		if dist >= float64(km)*1000 || i == len(streams.Distance.Data)-1 {
			if count > 0 {
				result = append(result, hrSum/float64(count))
			}
			hrSum = 0
			count = 0
			km++
		}
	}

	return result
}

// ComputeHRZones calculates time-based HR zones from the stream data.
func ComputeHRZones(streams *StreamSet) []models.HRZone {
	if streams == nil || streams.Heartrate == nil || streams.Time == nil {
		return nil
	}
	if len(streams.Heartrate.Data) != len(streams.Time.Data) {
		return nil
	}

	zones := []models.HRZone{
		{Name: "Z1", Min: 0, Max: 114},
		{Name: "Z2", Min: 115, Max: 135},
		{Name: "Z3", Min: 136, Max: 150},
		{Name: "Z4", Min: 151, Max: 165},
		{Name: "Z5", Min: 166, Max: 999},
	}

	hr := streams.Heartrate.Data
	t := streams.Time.Data

	for i := 1; i < len(hr); i++ {
		dt := int(t[i] - t[i-1])
		bpm := int(hr[i])
		for z := range zones {
			if bpm >= zones[z].Min && bpm <= zones[z].Max {
				zones[z].Seconds += dt
				break
			}
		}
	}

	total := 0
	for _, z := range zones {
		total += z.Seconds
	}
	if total > 0 {
		for i := range zones {
			zones[i].Percent = (zones[i].Seconds * 100) / total
		}
	}

	return zones
}

func speedToPace(speed float64) string {
	if speed <= 0 {
		return "0:00"
	}
	paceSeconds := 1000.0 / speed
	mins := int(paceSeconds) / 60
	secs := int(paceSeconds) % 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}

func (c *Client) FetchFullActivity(ctx context.Context, id int64) (*models.Activity, string, error) {
	detail, err := c.GetActivityDetail(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("getting detail: %w", err)
	}

	streams, err := c.GetActivityStreams(ctx, id)
	if err != nil {
		c.logger.Warn("could not fetch streams, using splits only", "error", err)
		streams = nil
	}

	splits := c.BuildSplits(detail, streams)

	rawJSON, _ := json.Marshal(detail)

	// Convert Strava laps to model laps
	laps := make([]models.Lap, len(detail.Laps))
	for i, l := range detail.Laps {
		laps[i] = models.Lap{
			Index:      l.LapIndex + 1,
			Distance:   l.Distance,
			MovingTime: l.MovingTime,
			Pace:       speedToPace(l.AverageSpeed),
			AvgHR:      l.AverageHeartrate,
			MaxHR:      l.MaxHeartrate,
		}
	}

	hrZones := ComputeHRZones(streams)

	activity := &models.Activity{
		StravaID:    detail.ID,
		Name:        detail.Name,
		Date:        detail.StartDate,
		Type:        detail.Type,
		Distance:    detail.Distance,
		MovingTime:  detail.MovingTime,
		ElapsedTime: detail.ElapsedTime,
		AvgPace:     speedToPace(detail.AverageSpeed),
		AvgHR:       detail.AverageHeartrate,
		MaxHR:       detail.MaxHeartrate,
		Splits:      splits,
		Laps:        laps,
		HRZones:     hrZones,
		RawJSON:     string(rawJSON),
	}

	return activity, string(rawJSON), nil
}
