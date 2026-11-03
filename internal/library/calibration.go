package library

import (
	"sort"
	"strings"
)

// Verdicts a model may answer with, in the order the confusion matrix lists them.
var Verdicts = []string{"included", "excluded", "uncertain"}

// CalibrationFilter narrows which suggestions a report covers. Empty fields
// match everything; From and To are inclusive ISO dates or timestamps compared
// against the suggestion's creation time.
type CalibrationFilter struct {
	ProfileID string `json:"profileId"`
	Model     string `json:"model"`
	RunID     string `json:"runId"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// CalibrationPair is one suggestion beside the person's decision on the same
// paper. Decision is empty while the paper is still pending or was never
// decided by a person.
type CalibrationPair struct {
	SuggestionID   string   `json:"suggestionId"`
	PaperID        string   `json:"paperId"`
	Title          string   `json:"title"`
	ProfileID      string   `json:"profileId"`
	ProfileName    string   `json:"profileName"`
	Model          string   `json:"model"`
	RunID          string   `json:"runId"`
	Kind           string   `json:"kind"`
	Requirement    string   `json:"requirement"`
	Verdict        string   `json:"verdict"`
	Reason         string   `json:"reason"`
	Basis          string   `json:"basis"`
	Excerpt        string   `json:"excerpt"`
	Page           int      `json:"page"`
	Confidence     *float64 `json:"confidence"`
	CreatedAt      string   `json:"createdAt"`
	Decision       string   `json:"decision"`
	DecisionReason string   `json:"decisionReason"`
	DecidedAt      string   `json:"decidedAt"`
}

// Agreement counts how often one model's suggestions matched the decisions.
type Agreement struct {
	ProfileID   string  `json:"profileId"`
	ProfileName string  `json:"profileName"`
	Model       string  `json:"model"`
	Compared    int     `json:"compared"`
	Agreed      int     `json:"agreed"`
	Rate        float64 `json:"rate"`
}

// RunSummary describes one batch run or rerun for the filter list.
type RunSummary struct {
	RunID       string  `json:"runId"`
	Kind        string  `json:"kind"`
	ProfileID   string  `json:"profileId"`
	ProfileName string  `json:"profileName"`
	Model       string  `json:"model"`
	Requirement string  `json:"requirement"`
	StartedAt   string  `json:"startedAt"`
	Total       int     `json:"total"`
	Compared    int     `json:"compared"`
	Agreed      int     `json:"agreed"`
	Rate        float64 `json:"rate"`
}

// MatrixCell is one count of the confusion matrix: suggestions that said
// Verdict on papers the person decided Decision.
type MatrixCell struct {
	Verdict  string `json:"verdict"`
	Decision string `json:"decision"`
	Count    int    `json:"count"`
}

// CalibrationReport is what the comparison view shows.
type CalibrationReport struct {
	// Total is the number of suggestions after filtering and deduplication;
	// Compared is how many of them have a person's decision to compare with;
	// Pending is the rest.
	Total    int     `json:"total"`
	Compared int     `json:"compared"`
	Agreed   int     `json:"agreed"`
	Pending  int     `json:"pending"`
	Rate     float64 `json:"rate"`
	// Verdicts is the axis of Matrix, in order.
	Verdicts []string     `json:"verdicts"`
	Matrix   []MatrixCell `json:"matrix"`
	ByModel  []Agreement  `json:"byModel"`
	// Mismatches lists compared pairs where the suggestion and the decision
	// differ, newest first.
	Mismatches []CalibrationPair `json:"mismatches"`
	// Models and Runs list what the unfiltered suggestions offer as filters.
	Models []Agreement  `json:"models"`
	Runs   []RunSummary `json:"runs"`
}

func modelKey(profileID, model string) string { return profileID + "\x00" + model }

func rate(agreed, compared int) float64 {
	if compared == 0 {
		return 0
	}
	return float64(agreed) / float64(compared)
}

// matches applies the filter to one pair. Dates compare as strings: ISO
// timestamps sort chronologically, and a bare date "2026-01-31" as To is
// padded to the end of that day.
func (f CalibrationFilter) matches(pair CalibrationPair) bool {
	if f.ProfileID != "" && pair.ProfileID != f.ProfileID {
		return false
	}
	if f.Model != "" && pair.Model != f.Model {
		return false
	}
	if f.RunID != "" && pair.RunID != f.RunID {
		return false
	}
	if from := strings.TrimSpace(f.From); from != "" && pair.CreatedAt < from {
		return false
	}
	if to := strings.TrimSpace(f.To); to != "" {
		if len(to) == len("2006-01-02") {
			to += "T23:59:59.999Z"
		}
		if pair.CreatedAt > to {
			return false
		}
	}
	return true
}

// Calibrate summarises suggestion/decision pairs. Pairs are expected newest
// first. Within one run every paper is asked once, so a run is compared as it
// is; outside a run filter only the newest suggestion per paper and model
// counts, so a rerun with the same model replaces its earlier answer instead
// of counting twice. The Models and Runs lists come from all pairs, so the
// filter menus still offer what the filter currently hides.
func Calibrate(pairs []CalibrationPair, filter CalibrationFilter) CalibrationReport {
	sorted := make([]CalibrationPair, len(pairs))
	copy(sorted, pairs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt != sorted[j].CreatedAt {
			return sorted[i].CreatedAt > sorted[j].CreatedAt
		}
		return sorted[i].SuggestionID > sorted[j].SuggestionID
	})

	report := CalibrationReport{Verdicts: Verdicts, Matrix: []MatrixCell{}, ByModel: []Agreement{}, Mismatches: []CalibrationPair{}, Models: []Agreement{}, Runs: []RunSummary{}}
	report.Models = agreementsByModel(sorted, true)
	report.Runs = runsOf(sorted)

	seen := map[string]bool{}
	selected := []CalibrationPair{}
	for _, pair := range sorted {
		if !filter.matches(pair) {
			continue
		}
		if filter.RunID == "" {
			key := pair.PaperID + "\x00" + modelKey(pair.ProfileID, pair.Model)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		selected = append(selected, pair)
	}

	cells := map[string]int{}
	for _, pair := range selected {
		report.Total++
		if pair.Decision == "" {
			report.Pending++
			continue
		}
		report.Compared++
		cells[pair.Verdict+"\x00"+pair.Decision]++
		if pair.Verdict == pair.Decision {
			report.Agreed++
		} else {
			report.Mismatches = append(report.Mismatches, pair)
		}
	}
	report.Rate = rate(report.Agreed, report.Compared)
	for _, verdict := range Verdicts {
		for _, decision := range Verdicts {
			report.Matrix = append(report.Matrix, MatrixCell{Verdict: verdict, Decision: decision, Count: cells[verdict+"\x00"+decision]})
		}
	}
	report.ByModel = agreementsByModel(selected, false)
	return report
}

// agreementsByModel counts agreement per model profile. With dedupe set, only
// the newest suggestion per paper and model is counted, matching Calibrate's
// unfiltered view; pairs must be newest first.
func agreementsByModel(pairs []CalibrationPair, dedupe bool) []Agreement {
	byKey := map[string]*Agreement{}
	order := []string{}
	seen := map[string]bool{}
	for _, pair := range pairs {
		key := modelKey(pair.ProfileID, pair.Model)
		if dedupe {
			paperKey := pair.PaperID + "\x00" + key
			if seen[paperKey] {
				continue
			}
			seen[paperKey] = true
		}
		entry, ok := byKey[key]
		if !ok {
			entry = &Agreement{ProfileID: pair.ProfileID, ProfileName: pair.ProfileName, Model: pair.Model}
			byKey[key] = entry
			order = append(order, key)
		}
		if pair.Decision == "" {
			continue
		}
		entry.Compared++
		if pair.Verdict == pair.Decision {
			entry.Agreed++
		}
	}
	result := make([]Agreement, 0, len(order))
	for _, key := range order {
		entry := byKey[key]
		entry.Rate = rate(entry.Agreed, entry.Compared)
		result = append(result, *entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].ProfileName != result[j].ProfileName {
			return result[i].ProfileName < result[j].ProfileName
		}
		return result[i].Model < result[j].Model
	})
	return result
}

// runsOf lists the batch runs and reruns among the pairs, newest first, each
// with its own agreement so runs with different models or prompts compare at
// a glance. Pairs must be newest first.
func runsOf(pairs []CalibrationPair) []RunSummary {
	byRun := map[string]*RunSummary{}
	order := []string{}
	for _, pair := range pairs {
		if pair.RunID == "" {
			continue
		}
		entry, ok := byRun[pair.RunID]
		if !ok {
			entry = &RunSummary{RunID: pair.RunID, Kind: pair.Kind, ProfileID: pair.ProfileID, ProfileName: pair.ProfileName, Model: pair.Model, Requirement: pair.Requirement}
			byRun[pair.RunID] = entry
			order = append(order, pair.RunID)
		}
		// Pairs are newest first, so the last one seen started the run.
		entry.StartedAt = pair.CreatedAt
		entry.Total++
		if pair.Decision == "" {
			continue
		}
		entry.Compared++
		if pair.Verdict == pair.Decision {
			entry.Agreed++
		}
	}
	result := make([]RunSummary, 0, len(order))
	for _, id := range order {
		entry := byRun[id]
		entry.Rate = rate(entry.Agreed, entry.Compared)
		result = append(result, *entry)
	}
	return result
}
