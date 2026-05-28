package retry

import (
	"context"
	"log/slog"
	"time"
)

// Do retries fn up to maxAttempts times with exponential backoff.
func Do(ctx context.Context, logger *slog.Logger, name string, maxAttempts int, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt == maxAttempts {
			break
		}
		wait := time.Duration(attempt) * 2 * time.Second
		logger.Warn("retrying after error", "operation", name, "attempt", attempt, "error", err, "wait", wait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return err
}
