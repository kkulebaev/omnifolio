package scheduler

import (
	"context"
	"log/slog"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron *cron.Cron
	log  *slog.Logger
}

type Job struct {
	Name string
	Spec string
	Run  func(context.Context) error
}

func New(log *slog.Logger) *Scheduler {
	return &Scheduler{
		cron: cron.New(),
		log:  log,
	}
}

func (s *Scheduler) Register(ctx context.Context, jobs ...Job) error {
	for _, j := range jobs {
		j := j
		_, err := s.cron.AddFunc(j.Spec, func() {
			s.log.Info("cron: starting", "job", j.Name)
			if err := j.Run(ctx); err != nil {
				s.log.Error("cron: failed", "job", j.Name, "err", err)
				return
			}
			s.log.Info("cron: ok", "job", j.Name)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop stops the scheduler and waits for in-flight jobs.
func (s *Scheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
}
