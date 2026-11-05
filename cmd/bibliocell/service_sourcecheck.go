package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

// sourceCheckReason starts the reason of every proposal the second check
// leaves, so the review queue says where it came from.
const sourceCheckReason = "AI 二次核查："

// sourceChecks counts papers whose AI second check has not finished; the
// sources page shows it while the check runs.
type sourceChecks struct {
	mu      sync.Mutex
	pending int
}

func (c *sourceChecks) add(n int) {
	c.mu.Lock()
	c.pending += n
	c.mu.Unlock()
}

func (c *sourceChecks) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending
}

// SourceCheckPending says how many papers a source update added are still
// waiting for their AI second check.
func (l *Library) SourceCheckPending() (int, error) {
	return l.checks.count(), nil
}

// checkHarvested starts the AI second check of the papers a source run added,
// when the person turned it on. It returns at once: the checks run in the
// background and the sources page hears when they finish.
func (l *Library) checkHarvested(sourceID string, paperIDs []string) {
	if l.settings == nil || l.store == nil || len(paperIDs) == 0 {
		return
	}
	config := l.settings.Get()
	check := config.SourceCheck
	if !check.Enabled || check.Prompt == "" {
		return
	}
	if _, err := config.ModelFor(settings.TaskScreen); err != nil {
		log.Printf("AI second check of %s skipped: %v", sourceID, err)
		return
	}
	l.checks.add(len(paperIDs))
	l.announceSourceChecks()
	go l.runSourceCheck(sourceID, check.Prompt, paperIDs, max(config.BatchScreenConcurrency, 1))
}

// runSourceCheck asks the screening model about each paper and records its
// verdict as the paper's proposed screening, for the person to adopt or
// dismiss. The decision itself stays theirs.
func (l *Library) runSourceCheck(sourceID, prompt string, paperIDs []string, workers int) {
	runID := fmt.Sprintf("SC-%s-%d", sourceID, time.Now().UnixNano())
	jobs := make(chan string)
	var wg sync.WaitGroup
	for range min(workers, len(paperIDs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if err := l.checkPaper(id, prompt, runID); err != nil {
					log.Printf("AI second check of %s: %v", id, err)
				}
				l.checks.add(-1)
			}
		}()
	}
	for _, id := range paperIDs {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	l.announceSourceChecks()
}

// checkPaper screens one paper and leaves the answer as a proposal.
func (l *Library) checkPaper(id, prompt, runID string) error {
	suggestion, err := l.judge(id, library.ScreeningTask, prompt, "", runID, "batch")
	if err != nil {
		return err
	}
	if suggestion.Skipped {
		return nil // Neither abstract nor full text: the person reads it unaided.
	}
	verdict := suggestion.Verdict
	reason := sourceCheckReason + suggestion.Reason
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err = l.store.UpdatePaper(ctx, id, library.PaperPatch{ProposedScreening: &verdict, ProposedReason: &reason})
	if errors.Is(err, library.ErrPaperNotFound) {
		return nil // Deleted while the model read it.
	}
	return err
}

// announceSourceChecks has the sources page reread its state, including the
// pending count and the proposals a finished check left on papers.
func (l *Library) announceSourceChecks() {
	automation, err := l.store.SourceAutomation(context.Background())
	if err != nil {
		log.Printf("read source automation: %v", err)
		return
	}
	l.announceSources(automation)
}
