package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"sds/api/internal/runs"
	"sds/api/internal/store"
)

type Scheduler struct {
	store      *store.Store
	dispatcher *runs.Dispatcher
	interval   time.Duration
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewScheduler(s *store.Store, d *runs.Dispatcher, tickInterval time.Duration) *Scheduler {
	if tickInterval <= 0 {
		tickInterval = 5 * time.Second
	}
	return &Scheduler{
		store:      s,
		dispatcher: d,
		interval:   tickInterval,
		stopCh:     make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.loop()
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC()
	due, err := s.store.GetDueSchedules(ctx, now)
	if err != nil {
		slog.Error("Scheduler failed to query due schedules", "err", err)
		return
	}

	for _, sch := range due {
		// Skip if a run of the same type is already active
		active, err := s.store.HasActiveRunOfType(ctx, sch.Type)
		if err != nil {
			slog.Error("Scheduler failed to check active run", "type", sch.Type, "err", err)
			continue
		}
		if active {
			slog.Debug("Skipping scheduled run, already active", "type", sch.Type)
			continue
		}

		run, err := s.dispatcher.CreateAndDispatchRun(ctx, sch.Type, "scheduled", sch.Params)
		if err != nil {
			slog.Error("Scheduler failed to trigger run", "type", sch.Type, "err", err)
			continue
		}

		intervalSec := sch.IntervalSeconds
		if intervalSec <= 0 {
			intervalSec = 86400
		}
		nextRun := now.Add(time.Duration(intervalSec) * time.Second)
		if err := s.store.UpdateScheduleNextRun(ctx, sch.ID, run.RunID, nextRun); err != nil {
			slog.Error("Scheduler failed to update schedule next run", "id", sch.ID, "err", err)
		} else {
			slog.Info("Triggered scheduled run", "type", sch.Type, "run_id", run.RunID, "next_run_at", nextRun)
		}
	}
}
