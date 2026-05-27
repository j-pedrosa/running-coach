package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

type Runner interface {
	Run(ctx context.Context, force bool) error
}

type Scheduler struct {
	cron   *cron.Cron
	runner Runner
	logger *slog.Logger
}

func New(runner Runner, tz string, logger *slog.Logger) (*Scheduler, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}

	c := cron.New(cron.WithLocation(loc))
	return &Scheduler{cron: c, runner: runner, logger: logger}, nil
}

func (s *Scheduler) Start() error {
	_, err := s.cron.AddFunc("0 21 * * *", func() {
		s.logger.Info("scheduled coaching run triggered")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.runner.Run(ctx, false); err != nil {
			s.logger.Error("scheduled run failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	s.cron.Start()
	s.logger.Info("scheduler started", "schedule", "daily at 21:00")
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
