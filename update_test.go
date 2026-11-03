package main

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeUpdater is an appUpdater whose feed answers with found (or err) and
// which records what the window heard.
type fakeUpdater struct {
	*appUpdater
	mu        sync.Mutex
	heard     []UpdateState
	checks    int
	relaunchs int
}

func newFakeUpdater(found *foundUpdate, err error) *fakeUpdater {
	f := &fakeUpdater{}
	f.appUpdater = &appUpdater{
		enabled: func() bool { return true },
		check: func(context.Context) (*foundUpdate, error) {
			f.mu.Lock()
			f.checks++
			f.mu.Unlock()
			return found, err
		},
		notify: func(s UpdateState) {
			f.mu.Lock()
			f.heard = append(f.heard, s)
			f.mu.Unlock()
		},
		relaunch: func() { f.relaunchs++ },
		state:    UpdateState{State: "idle", Current: "0.2.0"},
	}
	return f
}

func TestUpdateDownloadsAndWaitsForRestart(t *testing.T) {
	f := newFakeUpdater(&foundUpdate{version: "0.3.0", notes: "- faster", install: func(_ context.Context, progress func(int64, int64)) error {
		progress(5, 10)
		progress(10, 10)
		return nil
	}}, nil)
	f.run(context.Background())

	got := f.snapshot()
	if got.State != "ready" || got.Latest != "0.3.0" || got.Notes != "- faster" || got.Current != "0.2.0" || got.Done != 10 || got.Total != 10 {
		t.Fatalf("state = %+v, want ready 0.3.0 fully downloaded", got)
	}
	if first := f.heard[0]; first.State != "downloading" || first.Latest != "0.3.0" {
		t.Fatalf("first notice = %+v, want downloading 0.3.0", first)
	}
	if err := f.restart(); err != nil || f.relaunchs != 1 {
		t.Fatalf("restart: err %v, relaunches %d", err, f.relaunchs)
	}

	// The same version staged again is not downloaded twice.
	f.run(context.Background())
	if n := len(f.heard); n != 4 {
		t.Fatalf("a second check of the staged version announced %d states, want none new", n-4)
	}
}

func TestUpdateUpToDateOrOfflineKeepsThePillAway(t *testing.T) {
	for name, f := range map[string]*fakeUpdater{
		"up to date": newFakeUpdater(nil, nil),
		"offline":    newFakeUpdater(nil, errors.New("dial tcp: no route to host")),
	} {
		f.run(context.Background())
		if got := f.snapshot(); got.State != "idle" || len(f.heard) != 0 {
			t.Errorf("%s: state %+v, %d notices; want idle and silent", name, got, len(f.heard))
		}
		if err := f.restart(); err == nil || f.relaunchs != 0 {
			t.Errorf("%s: restart without an update relaunched", name)
		}
	}
}

func TestUpdateDownloadFailureSaysWhy(t *testing.T) {
	f := newFakeUpdater(&foundUpdate{version: "0.3.0", install: func(context.Context, func(int64, int64)) error {
		return errors.New("signature does not match")
	}}, nil)
	f.run(context.Background())
	if got := f.snapshot(); got.State != "error" || got.Error != "signature does not match" || got.Latest != "0.3.0" {
		t.Fatalf("state = %+v, want the error of 0.3.0", got)
	}
	if err := f.restart(); err == nil {
		t.Fatal("restart into a failed download should refuse")
	}
}

func TestUpdateOffInBuildsThatCannotUpdate(t *testing.T) {
	f := newFakeUpdater(&foundUpdate{version: "0.3.0"}, nil)
	f.enabled = func() bool { return false }
	f.start(context.Background())
	f.run(context.Background())
	if got := f.snapshot(); got.State != "off" || f.checks != 0 {
		t.Fatalf("state = %+v after %d checks, want off and never asked", got, f.checks)
	}
	if got := (&Library{}).UpdateStatus(); got.State != "off" {
		t.Fatalf("library without an updater reports %q", got.State)
	}
}
