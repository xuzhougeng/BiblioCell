package library

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

const (
	sourceCheckEvery = time.Minute
	// sourceRetryDelay brings a failed source back soon after, say, waking offline
	// instead of a full interval later.
	sourceRetryDelay = time.Hour
)

// SourceFailure says why a source's latest run failed.
type SourceFailure struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// SourceAutomation is the saved schedule plus what each enabled source's latest
// run did, whether a person or the schedule started it.
type SourceAutomation struct {
	Enabled       bool            `json:"enabled"`
	IntervalHours int             `json:"intervalHours"`
	Running       bool            `json:"running"`
	LastRunAt     string          `json:"lastRunAt"`
	NextRunAt     string          `json:"nextRunAt"`
	Added         int             `json:"added"`
	Skipped       int             `json:"skipped"`
	Failures      []SourceFailure `json:"failures"`
}

func (s *Store) automationConfig(ctx context.Context) (enabled bool, hours int, err error) {
	err = s.db.QueryRowContext(ctx, "SELECT enabled, interval_hours FROM source_automation WHERE id = 1").Scan(&enabled, &hours)
	if err != nil {
		return false, 0, fmt.Errorf("read source automation: %w", err)
	}
	return enabled, hours, nil
}

// enabledSources lists the sources that automation may contact.
func (s *Store) enabledSources(ctx context.Context) ([]SourceStatus, error) {
	sources := []SourceStatus{}
	for _, definition := range sourceDefinitions {
		if !definition.supported {
			continue
		}
		status, err := s.SourceStatus(ctx, definition.id)
		if err != nil {
			return nil, err
		}
		if status.Enabled {
			sources = append(sources, status)
		}
	}
	return sources, nil
}

// nextSourceRun is when a source falls due; zero means it never ran. Working
// from the latest run, a manual sync counts too, and an update missed while the
// app was closed is simply overdue at the next start.
func nextSourceRun(source SourceStatus, hours int) time.Time {
	if source.LastRun == nil {
		return time.Time{}
	}
	started, err := time.Parse(time.RFC3339Nano, source.LastRun.StartedAt)
	if err != nil {
		return time.Time{}
	}
	interval := time.Duration(hours) * time.Hour
	if source.LastRun.Status == "failed" {
		interval = min(interval, sourceRetryDelay)
	}
	return started.Add(interval)
}

// SourceAutomation reads the schedule and summarizes the latest runs.
func (s *Store) SourceAutomation(ctx context.Context) (SourceAutomation, error) {
	if s == nil || s.db == nil {
		return SourceAutomation{}, errors.New("library database is not ready")
	}
	enabled, hours, err := s.automationConfig(ctx)
	if err != nil {
		return SourceAutomation{}, err
	}
	sources, err := s.enabledSources(ctx)
	if err != nil {
		return SourceAutomation{}, err
	}
	automation := SourceAutomation{Enabled: enabled, IntervalHours: hours, Failures: []SourceFailure{}}
	now := time.Now().UTC()
	var next time.Time
	for i, source := range sources {
		if due := nextSourceRun(source, hours); i == 0 || due.Before(next) {
			next = due
		}
		run := source.LastRun
		if run == nil {
			continue
		}
		automation.Running = automation.Running || run.Status == "running"
		automation.LastRunAt = max(automation.LastRunAt, run.FinishedAt)
		automation.Added += run.PapersAdded
		if source.Kind == "repository" {
			automation.Added += run.RecordsNew
		}
		automation.Skipped += run.Duplicates
		if run.Status == "failed" {
			automation.Failures = append(automation.Failures, SourceFailure{Source: source.Name, Message: run.ErrorMessage})
		}
	}
	if enabled && len(sources) > 0 {
		if next.Before(now) {
			next = now
		}
		automation.NextRunAt = next.Format(time.RFC3339Nano)
	}
	return automation, nil
}

// SaveSourceAutomation persists the schedule without starting a run. The next
// check, within a minute, picks up whatever is due under it.
func (s *Store) SaveSourceAutomation(ctx context.Context, enabled bool, intervalHours int) (SourceAutomation, error) {
	if s == nil || s.db == nil {
		return SourceAutomation{}, errors.New("library database is not ready")
	}
	if intervalHours < 1 || intervalHours > 720 {
		return SourceAutomation{}, errors.New("update interval must be between 1 and 720 hours")
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE source_automation SET enabled = ?, interval_hours = ? WHERE id = 1",
		boolInt(enabled), intervalHours); err != nil {
		return SourceAutomation{}, fmt.Errorf("save source automation: %w", err)
	}
	return s.SourceAutomation(ctx)
}

// StartSourceUpdates runs due sources in the background until stop is called.
// onChange fires when a run starts and when it ends, on the background goroutine.
func (s *Store) StartSourceUpdates(onChange func()) (stop func()) {
	return s.startSourceUpdates(newRemoteHarvester(), sourceCheckEvery, onChange)
}

func (s *Store) startSourceUpdates(harvester sourceHarvester, every time.Duration, onChange func()) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			if err := s.runDueSources(ctx, harvester, onChange); err != nil && ctx.Err() == nil {
				log.Printf("source updates: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// harvesterFunc adapts a function to sourceHarvester.
type harvesterFunc func(context.Context, SourceConfig) ([]harvestRecord, error)

func (f harvesterFunc) Harvest(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	return f(ctx, config)
}

// runDueSources updates each source whose time has come, one after another. A
// failed source keeps its reason in harvest_runs and the others still run.
func (s *Store) runDueSources(ctx context.Context, harvester sourceHarvester, onChange func()) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE harvest_runs SET status = 'failed', finished_at = ?, error_message = '上次更新未完成，应用可能被强制退出'
		WHERE status = 'running' AND started_at <= ?`,
		now.Format(time.RFC3339Nano), now.Add(-sourceRunLimit).Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("close interrupted runs: %w", err)
	}
	sources, err := s.enabledSources(ctx)
	if err != nil {
		return err
	}
	announced := harvesterFunc(func(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
		onChange()
		return harvester.Harvest(ctx, config)
	})
	for _, source := range sources {
		// Read each time: switching automation off stops a cycle already under way.
		enabled, hours, err := s.automationConfig(ctx)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		if time.Now().Before(nextSourceRun(source, hours)) {
			continue
		}
		_, err = s.syncSource(ctx, source.ID, announced)
		if errors.Is(err, ErrSyncBusy) {
			return nil // Whoever holds the run is working; the next check carries on.
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		onChange()
	}
	return nil
}
