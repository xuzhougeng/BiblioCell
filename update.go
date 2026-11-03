package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// The app keeps itself current the way magpie does: it asks the release feed
// shortly after launch and every six hours, and downloads a newer version
// straight away, so the top bar's Update pill appears only once all that is
// left is a restart. Quitting puts it in too: mygo runs the staged version at
// the next launch.
const (
	updateFirstCheck = 15 * time.Second
	updateEvery      = 6 * time.Hour
	updateCheckLimit = time.Minute
	// updateProgressEvery spaces the progress the window hears while downloading.
	updateProgressEvery = 250 * time.Millisecond
)

// UpdateState is what the top bar's Update pill shows.
type UpdateState struct {
	// State is "off" (a development build, or one that cannot replace
	// itself where it is installed), "idle", "downloading", "ready" (staged,
	// waiting for a restart) or "error" (the download failed).
	State   string `json:"state"`
	Current string `json:"current"`
	Latest  string `json:"latest"`
	// Notes are the release notes in Markdown, empty when the release has none.
	Notes string `json:"notes"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// updateChanged tells the window the update was found, downloaded or failed.
var updateChanged = mygo.NewEvent[UpdateState]("updateChanged")

// foundUpdate is a newer release; install downloads it, checks its signature
// and stages it beside the running app.
type foundUpdate struct {
	version, notes string
	install        func(ctx context.Context, progress func(done, total int64)) error
}

// appUpdater checks for, downloads and stages updates. The funcs are mygo's
// in the app; tests stand in for them.
type appUpdater struct {
	enabled  func() bool
	check    func(ctx context.Context) (*foundUpdate, error)
	notify   func(UpdateState)
	relaunch func()

	mu      sync.Mutex
	ctx     context.Context
	state   UpdateState
	running bool
}

func newAppUpdater() *appUpdater {
	return &appUpdater{
		enabled: mygo.Updater.Enabled,
		check: func(ctx context.Context) (*foundUpdate, error) {
			up, err := mygo.Updater.Check(ctx)
			if err != nil || up == nil {
				return nil, err
			}
			return &foundUpdate{version: up.Version, notes: up.Notes, install: up.Install}, nil
		},
		notify: func(s UpdateState) {
			if err := updateChanged.Broadcast(s); err != nil {
				log.Printf("announce update: %v", err)
			}
		},
		relaunch: mygo.App.Relaunch,
		state:    UpdateState{State: "idle", Current: mygo.App.Version()},
	}
}

// start checks after updateFirstCheck and then every updateEvery until ctx ends.
func (u *appUpdater) start(ctx context.Context) {
	u.mu.Lock()
	u.ctx = ctx
	u.mu.Unlock()
	if !u.enabled() {
		u.set(func(s *UpdateState) { s.State = "off" })
		return
	}
	go func() {
		timer := time.NewTimer(updateFirstCheck)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			u.run(ctx)
			timer.Reset(updateEvery)
		}
	}()
}

// run asks the feed once and downloads what it finds. A check that fails,
// offline say, leaves the pill as it was: the next one tries again.
func (u *appUpdater) run(ctx context.Context) {
	u.mu.Lock()
	if u.running || u.state.State == "off" {
		u.mu.Unlock()
		return
	}
	u.running = true
	staged := u.state.Latest
	if u.state.State != "ready" {
		staged = ""
	}
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
	}()

	checkCtx, cancel := context.WithTimeout(ctx, updateCheckLimit)
	found, err := u.check(checkCtx)
	cancel()
	if err != nil {
		log.Printf("check for updates: %v", err)
		return
	}
	if found == nil || found.version == staged {
		return
	}
	u.set(func(s *UpdateState) {
		*s = UpdateState{State: "downloading", Current: s.Current, Latest: found.version, Notes: found.notes}
	})
	var last time.Time
	err = found.install(ctx, func(done, total int64) {
		if done < total && time.Since(last) < updateProgressEvery {
			return
		}
		last = time.Now()
		u.set(func(s *UpdateState) { s.Done, s.Total = done, total })
	})
	if err != nil {
		log.Printf("download update %s: %v", found.version, err)
		u.set(func(s *UpdateState) { s.State, s.Error = "error", err.Error() })
		return
	}
	u.set(func(s *UpdateState) { s.State = "ready" })
}

// retry checks again now, for the pill of a download that failed.
func (u *appUpdater) retry() {
	u.mu.Lock()
	ctx := u.ctx
	u.mu.Unlock()
	if ctx != nil {
		go u.run(ctx)
	}
}

// restart relaunches into the staged update.
func (u *appUpdater) restart() error {
	if u.snapshot().State != "ready" {
		return errors.New("no update is ready to install")
	}
	u.relaunch()
	return nil
}

func (u *appUpdater) snapshot() UpdateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

func (u *appUpdater) set(change func(*UpdateState)) {
	u.mu.Lock()
	change(&u.state)
	s := u.state
	u.mu.Unlock()
	u.notify(s)
}

// UpdateStatus returns the state of the app's own update.
func (l *Library) UpdateStatus() UpdateState {
	if l.updates == nil {
		return UpdateState{State: "off"}
	}
	return l.updates.snapshot()
}

// RetryUpdate checks for an update again after a download failed.
func (l *Library) RetryUpdate() {
	if l.updates != nil {
		l.updates.retry()
	}
}

// RestartToUpdate relaunches into the downloaded update.
func (l *Library) RestartToUpdate() error {
	if l.updates == nil {
		return errors.New("updates are not available")
	}
	return l.updates.restart()
}
