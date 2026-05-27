package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxMessageLength = 4096

type Client struct {
	http     *http.Client
	botToken string
	chatID   string
	logger   *slog.Logger
}

func NewClient(botToken, chatID string, logger *slog.Logger) *Client {
	return &Client{
		http:     &http.Client{Timeout: 30 * time.Second},
		botToken: botToken,
		chatID:   chatID,
		logger:   logger,
	}
}

func (c *Client) SendMessage(ctx context.Context, text string) error {
	html := markdownToHTML(text)
	chunks := splitMessage(html, maxMessageLength)
	for i, chunk := range chunks {
		if err := c.sendText(ctx, chunk); err != nil {
			return fmt.Errorf("sending message chunk %d/%d: %w", i+1, len(chunks), err)
		}
	}
	return nil
}

func (c *Client) sendText(ctx context.Context, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)

	data := url.Values{
		"chat_id":    {c.chatID},
		"text":       {text},
		"parse_mode": {"HTML"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Telegram API error (%d): %s", resp.StatusCode, body)
	}

	return nil
}

func (c *Client) SendPhoto(ctx context.Context, photoURL, caption string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", c.botToken)

	data := url.Values{
		"chat_id": {c.chatID},
		"photo":   {photoURL},
		"caption": {caption},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sending photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Log but check if it's a URL issue
		var result struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		json.Unmarshal(body, &result)
		return fmt.Errorf("Telegram photo error (%d): %s", resp.StatusCode, result.Description)
	}

	return nil
}

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	// Split on double newlines (paragraph boundaries), which works with
	// both markdown headers (## ) and emoji section headers (📊, 🧠, etc.)
	paragraphs := strings.Split(text, "\n\n")

	var chunks []string
	var current strings.Builder

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If adding this paragraph would exceed the limit, flush current chunk
		if current.Len()+len(para)+2 > maxLen && current.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}

		// If a single paragraph is longer than maxLen, split it by lines
		if len(para) > maxLen {
			if current.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(current.String()))
				current.Reset()
			}
			lines := strings.Split(para, "\n")
			for _, line := range lines {
				if current.Len()+len(line)+1 > maxLen && current.Len() > 0 {
					chunks = append(chunks, strings.TrimSpace(current.String()))
					current.Reset()
				}
				if current.Len() > 0 {
					current.WriteString("\n")
				}
				current.WriteString(line)
			}
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}

	return chunks
}

var (
	reH1     = regexp.MustCompile(`(?m)^# (.+)$`)
	reH2     = regexp.MustCompile(`(?m)^## (.+)$`)
	reH3     = regexp.MustCompile(`(?m)^### (.+)$`)
	reBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
)

// markdownToHTML converts Claude's markdown report to Telegram-safe HTML.
// Telegram supports: <b>, <i>, <u>, <s>, <a>, <code>, <pre>
func markdownToHTML(text string) string {
	// Headers → bold
	text = reH3.ReplaceAllString(text, "<b>$1</b>")
	text = reH2.ReplaceAllString(text, "\n<b>$1</b>")
	text = reH1.ReplaceAllString(text, "\n<b>$1</b>")
	// Bold
	text = reBold.ReplaceAllString(text, "<b>$1</b>")
	// Italic (simple cases only — avoid breaking bold)
	text = reItalic.ReplaceAllString(text, "<i>$1</i>")
	// Horizontal rules → empty line
	text = strings.ReplaceAll(text, "---", "")
	// Bullet points — keep as-is (Telegram renders them fine as plain text)
	return strings.TrimSpace(text)
}
