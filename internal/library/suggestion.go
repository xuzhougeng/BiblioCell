package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ScreeningSuggestion is one answer a model gave about one paper. It is kept
// so a person can later see how well the model agreed with their decisions;
// nothing here changes the paper's own screening status.
type ScreeningSuggestion struct {
	ID          string `json:"id"`
	PaperID     string `json:"paperId"`
	Requirement string `json:"requirement"`
	// ProfileID, ProfileName, Model and Protocol say which configured model
	// service answered, copied at the time so renaming a profile later does not
	// rewrite history.
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	Model       string `json:"model"`
	Protocol    string `json:"protocol"`
	// Verdict is included, excluded, or uncertain.
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
	// Basis says what the model read: fulltext or abstract.
	Basis string `json:"basis"`
	// Excerpt is the passage the model quoted; Page is where it sits in the
	// stored PDF, 0 when unknown.
	Excerpt string `json:"excerpt"`
	Page    int    `json:"page"`
	// Confidence is the model's own 0–1 estimate; nil when it gave none.
	Confidence *float64 `json:"confidence"`
	// RunID groups the answers of one batch run or rerun; Kind is screen (one
	// paper at a time), batch, or rerun (a calibration pass over decided papers).
	RunID        string `json:"runId"`
	Kind         string `json:"kind"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	// TaskID and TaskName say which judge task asked the question (screening
	// when empty or "screening"); TargetKind and TargetValue say where an adopted
	// yes lands, see JudgeTarget. Requirement holds the task's question.
	TaskID      string `json:"taskId"`
	TaskName    string `json:"taskName"`
	TargetKind  string `json:"targetKind"`
	TargetValue string `json:"targetValue"`
	// Skipped is set when the paper had neither full text nor abstract, so no
	// model was asked and nothing was recorded.
	Skipped   bool   `json:"skipped"`
	CreatedAt string `json:"createdAt"`
}

// SuggestionKinds a suggestion may be recorded under.
var SuggestionKinds = []string{"screen", "batch", "rerun"}

const suggestionSelect = `
	SELECT id, paper_id, requirement, profile_id, profile_name, model, protocol, verdict, reason, basis,
	       excerpt, page, confidence, run_id, kind, input_tokens, output_tokens, created_at,
	       task_id, task_name, target_kind, target_value
	FROM screening_suggestions`

func suggestionFields(s *ScreeningSuggestion) []any {
	return []any{&s.ID, &s.PaperID, &s.Requirement, &s.ProfileID, &s.ProfileName, &s.Model, &s.Protocol, &s.Verdict,
		&s.Reason, &s.Basis, &s.Excerpt, &s.Page, &s.Confidence, &s.RunID, &s.Kind, &s.InputTokens, &s.OutputTokens, &s.CreatedAt,
		&s.TaskID, &s.TaskName, &s.TargetKind, &s.TargetValue}
}

// AddScreeningSuggestion records a model's answer. The paper's own screening
// columns are never touched: a person's decision stays theirs.
func (s *Store) AddScreeningSuggestion(ctx context.Context, input ScreeningSuggestion) (ScreeningSuggestion, error) {
	input.PaperID = strings.TrimSpace(input.PaperID)
	input.Requirement = strings.TrimSpace(input.Requirement)
	if input.PaperID == "" || input.Requirement == "" {
		return ScreeningSuggestion{}, errors.New("a screening suggestion needs a paper and a requirement")
	}
	if !oneOf(input.Verdict, Verdicts...) {
		return ScreeningSuggestion{}, fmt.Errorf("invalid suggestion verdict %q", input.Verdict)
	}
	if input.Kind == "" {
		input.Kind = "screen"
	}
	if !oneOf(input.Kind, SuggestionKinds...) {
		return ScreeningSuggestion{}, fmt.Errorf("invalid suggestion kind %q", input.Kind)
	}
	if input.Page < 0 {
		input.Page = 0
	}
	if input.Confidence != nil && (*input.Confidence < 0 || *input.Confidence > 1) {
		input.Confidence = nil
	}
	input.TaskID, input.TaskName = strings.TrimSpace(input.TaskID), strings.TrimSpace(input.TaskName)
	input.TargetKind, input.TargetValue = strings.TrimSpace(input.TargetKind), strings.TrimSpace(input.TargetValue)
	if input.TargetKind == "" {
		input.TargetKind = "screening"
	}
	if !oneOf(input.TargetKind, JudgeTargetKinds...) {
		return ScreeningSuggestion{}, fmt.Errorf("invalid suggestion target %q", input.TargetKind)
	}
	if input.TargetKind == "screening" {
		input.TargetValue = ""
	} else if input.TargetValue == "" {
		return ScreeningSuggestion{}, fmt.Errorf("a %s suggestion needs a target value", input.TargetKind)
	}
	if _, err := s.GetPaper(ctx, input.PaperID); err != nil {
		return ScreeningSuggestion{}, err
	}
	id, err := newLocalID("SG")
	if err != nil {
		return ScreeningSuggestion{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO screening_suggestions(
			id, paper_id, requirement, profile_id, profile_name, model, protocol, verdict, reason, basis,
			excerpt, page, confidence, run_id, kind, input_tokens, output_tokens, created_at,
			task_id, task_name, target_kind, target_value
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.PaperID, input.Requirement, input.ProfileID, input.ProfileName, input.Model, input.Protocol,
		input.Verdict, input.Reason, input.Basis, input.Excerpt, input.Page, input.Confidence, input.RunID,
		input.Kind, input.InputTokens, input.OutputTokens, now,
		input.TaskID, input.TaskName, input.TargetKind, input.TargetValue); err != nil {
		return ScreeningSuggestion{}, fmt.Errorf("save screening suggestion: %w", err)
	}
	var saved ScreeningSuggestion
	if err := s.db.QueryRowContext(ctx, suggestionSelect+" WHERE id = ?", id).Scan(suggestionFields(&saved)...); err != nil {
		return ScreeningSuggestion{}, fmt.Errorf("read screening suggestion: %w", err)
	}
	return saved, nil
}

// ListScreeningSuggestions returns a paper's recorded suggestions, newest
// first, or every suggestion when paperID is empty.
func (s *Store) ListScreeningSuggestions(ctx context.Context, paperID string) ([]ScreeningSuggestion, error) {
	rows, err := s.db.QueryContext(ctx, suggestionSelect+` WHERE (? = '' OR paper_id = ?) ORDER BY created_at DESC, id DESC`, paperID, paperID)
	if err != nil {
		return nil, fmt.Errorf("list screening suggestions: %w", err)
	}
	defer rows.Close()
	suggestions := []ScreeningSuggestion{}
	for rows.Next() {
		var item ScreeningSuggestion
		if err := rows.Scan(suggestionFields(&item)...); err != nil {
			return nil, fmt.Errorf("read screening suggestion: %w", err)
		}
		suggestions = append(suggestions, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list screening suggestions: %w", err)
	}
	return suggestions, nil
}

// CalibrationReport pairs every recorded suggestion with the person's current
// decision on the same paper and summarises how often they agree. Filters
// narrow the suggestions by model profile, run or time; the decision is always
// the one on record now, so a person changing their mind updates the report.
func (s *Store) CalibrationReport(ctx context.Context, filter CalibrationFilter) (CalibrationReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.paper_id, p.title, g.profile_id, g.profile_name, g.model, g.run_id, g.kind, g.requirement,
		       g.verdict, g.reason, g.basis, g.excerpt, g.page, g.confidence, g.created_at,
		       c.screening_status, coalesce(c.screening_reason, ''), c.screening_method, coalesce(c.screening_reviewed_at, '')
		FROM screening_suggestions g
		JOIN papers p ON p.id = g.paper_id
		JOIN paper_curation c ON c.paper_id = g.paper_id
		WHERE g.target_kind = 'screening'
		ORDER BY g.created_at DESC, g.id DESC`)
	if err != nil {
		return CalibrationReport{}, fmt.Errorf("read calibration pairs: %w", err)
	}
	defer rows.Close()
	pairs := []CalibrationPair{}
	for rows.Next() {
		var pair CalibrationPair
		var method string
		if err := rows.Scan(&pair.SuggestionID, &pair.PaperID, &pair.Title, &pair.ProfileID, &pair.ProfileName, &pair.Model,
			&pair.RunID, &pair.Kind, &pair.Requirement, &pair.Verdict, &pair.Reason, &pair.Basis, &pair.Excerpt, &pair.Page,
			&pair.Confidence, &pair.CreatedAt, &pair.Decision, &pair.DecisionReason, &method, &pair.DecidedAt); err != nil {
			return CalibrationReport{}, fmt.Errorf("read calibration pair: %w", err)
		}
		// Only a person's decision counts as the reference; an agent proposal
		// or a pending paper is no verdict to compare against.
		if method != "human" || pair.Decision == "pending" {
			pair.Decision = ""
			pair.DecidedAt = ""
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return CalibrationReport{}, fmt.Errorf("read calibration pairs: %w", err)
	}
	return Calibrate(pairs, filter), nil
}

// GetScreeningSuggestion returns one recorded suggestion by id.
func (s *Store) GetScreeningSuggestion(ctx context.Context, id string) (ScreeningSuggestion, error) {
	var item ScreeningSuggestion
	if err := s.db.QueryRowContext(ctx, suggestionSelect+" WHERE id = ?", id).Scan(suggestionFields(&item)...); err != nil {
		return ScreeningSuggestion{}, fmt.Errorf("read screening suggestion %s: %w", id, err)
	}
	return item, nil
}

// AdoptResult counts what AdoptJudgements did.
type AdoptResult struct {
	// Adopted is the number of papers that were marked.
	Adopted int `json:"adopted"`
	// Skipped counts suggestions that changed nothing: a no or uncertain
	// answer, a screening suggestion (adopted on the screening page instead),
	// a mark already present, or a suggestion no longer on record.
	Skipped int `json:"skipped"`
}

// AdoptJudgements applies the "yes" answers among the given suggestions to
// their papers: an article type or tag task adds its value to the paper. It
// is a person's act, so a decided paper is marked too; the screening decision
// itself is never touched here. Suggestions that answered no or uncertain,
// screening suggestions, and marks already present are skipped.
func (s *Store) AdoptJudgements(ctx context.Context, ids []string) (AdoptResult, error) {
	var result AdoptResult
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		suggestion, err := s.GetScreeningSuggestion(ctx, id)
		if err != nil {
			result.Skipped++
			continue
		}
		if suggestion.Verdict != "included" || suggestion.TargetKind == "screening" || suggestion.TargetValue == "" {
			result.Skipped++
			continue
		}
		paper, err := s.GetPaper(ctx, suggestion.PaperID)
		if err != nil {
			result.Skipped++
			continue
		}
		after := paper
		switch suggestion.TargetKind {
		case "articleType":
			label := articleTypeDisplay[suggestion.TargetValue]
			if label == "" {
				label = suggestion.TargetValue
			}
			if oneOf(label, paper.ArticleTypes...) {
				result.Skipped++
				continue
			}
			after.ArticleTypes = append(append([]string{}, paper.ArticleTypes...), label)
		case "tag":
			if oneOf(suggestion.TargetValue, paper.Tags...) {
				result.Skipped++
				continue
			}
			after.Tags = append(append([]string{}, paper.Tags...), suggestion.TargetValue)
		default:
			result.Skipped++
			continue
		}
		if _, err := s.SavePaperEdits(ctx, []PaperEdit{{Before: &paper, After: after}}); err != nil {
			return result, err
		}
		result.Adopted++
	}
	return result, nil
}
