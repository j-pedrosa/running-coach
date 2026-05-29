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

type Nudger interface {
	SendNudge(ctx context.Context) error
}

type Scheduler struct {
	cron   *cron.Cron
	runner Runner
	nudger Nudger
	logger *slog.Logger
}

func New(runner Runner, nudger Nudger, tz string, logger *slog.Logger) (*Scheduler, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}

	c := cron.New(cron.WithLocation(loc))
	return &Scheduler{cron: c, runner: runner, nudger: nudger, logger: logger}, nil
}

func (s *Scheduler) Start() error {
	// Daily coaching report at 9pm
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

	// Motivational nudge at 8pm the evening before run days (Fri→Sat, Sun→Mon)
	if s.nudger != nil {
		_, err = s.cron.AddFunc("0 20 * * 5,0", func() { // Friday and Sunday at 8pm
			s.logger.Info("sending motivational nudge")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.nudger.SendNudge(ctx); err != nil {
				s.logger.Error("nudge failed", "error", err)
			}
		})
		if err != nil {
			return err
		}
	}

	s.cron.Start()
	s.logger.Info("scheduler started", "schedule", "coaching 21:00 daily, nudge 20:00 Fri+Sun")
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
