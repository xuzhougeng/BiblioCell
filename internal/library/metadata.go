package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MetadataClaim is a proposal, never a screening or verification decision.
type MetadataClaim struct {
	Field      string         `json:"field"`
	Values     []string       `json:"values"`
	Species    []SpeciesEntry `json:"species"`
	Repository string         `json:"repository"`
	Accession  string         `json:"accession"`
	Role       string         `json:"role"`
	Excerpt    string         `json:"excerpt"`
}

type MetadataProposal struct {
	PaperID   string          `json:"paperId"`
	FileID    string          `json:"fileId"`
	FileTitle string          `json:"fileTitle"`
	Truncated bool            `json:"truncated"`
	Omitted   int             `json:"omitted"`
	Claims    []MetadataClaim `json:"claims"`
}

// CheckedMetadataClaims drops malformed or ungrounded claims. Whitespace can
// differ after PDF extraction, but a quote must occur in the supplied text.
func CheckedMetadataClaims(claims []MetadataClaim, fulltext string) []MetadataClaim {
	text := strings.Join(strings.Fields(fulltext), " ")
	checked := []MetadataClaim{}
	seen := map[string]bool{}
	for _, claim := range claims {
		claim.Excerpt = strings.TrimSpace(claim.Excerpt)
		quote := strings.Join(strings.Fields(claim.Excerpt), " ")
		if len([]rune(quote)) < 10 || len([]rune(quote)) > 2000 || !strings.Contains(text, quote) {
			continue
		}
		claim.Values = cleanStrings(claim.Values)
		claim.Species = cleanSpecies(claim.Species)
		key := claim.Field
		switch claim.Field {
		case "species":
			if len(claim.Species) == 0 || len(claim.Species) > 50 {
				continue
			}
		case "organs", "modalities", "platforms", "articleTypes":
			if len(claim.Values) == 0 || len(claim.Values) > 50 {
				continue
			}
		case "dataset":
			claim.Repository = strings.ToLower(strings.TrimSpace(claim.Repository))
			claim.Accession = strings.TrimSpace(claim.Accession)
			if claim.Repository == "" || len([]rune(claim.Repository)) > 100 || claim.Accession == "" || len(claim.Accession) > 200 || !strings.Contains(strings.ToLower(quote), strings.ToLower(claim.Accession)) {
				continue
			}
			if !oneOf(claim.Role, "generated", "reused", "reprocessed", "benchmarked", "unknown") {
				claim.Role = "unknown"
			}
			key += ":" + claim.Repository + ":" + strings.ToLower(claim.Accession)
		default:
			continue
		}
		if !seen[key] {
			checked = append(checked, claim)
			seen[key] = true
		}
	}
	return checked
}

// ApplyPaperMetadata atomically saves the person's selected suggestions and their
// evidence. Concurrent edits conflict, and existing dataset verification survives.
func (s *Store) ApplyPaperMetadata(ctx context.Context, before Paper, proposal MetadataProposal) error {
	if before.ID == "" || before.ID != proposal.PaperID || len(proposal.Claims) == 0 || len(proposal.Claims) > 55 {
		return errors.New("请选择要应用的全文建议")
	}
	file, fulltext, err := s.PaperFulltextSource(ctx, before.ID)
	if err != nil {
		return err
	}
	if file.ID == "" || file.ID != proposal.FileID {
		return errors.New("全文附件已变化，请重新提取")
	}
	claims := CheckedMetadataClaims(proposal.Claims, fulltext)
	if len(claims) != len(proposal.Claims) {
		return errors.New("建议缺少可核对的全文依据，请重新提取")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := readPaper(ctx, tx, before.ID)
	if err != nil {
		return err
	}
	after := before
	for _, claim := range claims {
		switch claim.Field {
		case "species":
			after.Species = claim.Species
		case "organs":
			after.Organs = claim.Values
		case "modalities":
			after.Modalities = claim.Values
		case "platforms":
			after.Platforms = claim.Values
		case "articleTypes":
			after.ArticleTypes = claim.Values
		}
	}
	normalizePaper(&before)
	normalizePaper(&after)
	normalizePaper(&current)
	merged, changed, conflict, err := mergeFields(before, after, current, serverPaperFields)
	if err != nil {
		return err
	}
	if conflict {
		return errors.New("这些字段已被其他操作修改，请重新提取并核对后再保存")
	}
	if changed {
		if err := savePaper(ctx, tx, &merged); err != nil {
			return err
		}
	}
	for _, claim := range claims {
		value, _ := json.Marshal(claim.Values)
		if claim.Field == "species" {
			value, _ = json.Marshal(claim.Species)
		}
		if claim.Field == "dataset" {
			input := DatasetInput{Repositories: []RepositoryInput{{Repository: claim.Repository, Accession: claim.Accession}}}
			id, err := existingDatasetID(ctx, tx, input)
			if err != nil {
				return err
			}
			if id == "" {
				id, err = newLocalID("DS")
				if err != nil {
					return err
				}
				d := Dataset{ID: id, Title: claim.Repository + " " + claim.Accession, Status: "identified", AwaitingReview: true,
					Repositories: []DatasetRepository{{Repository: claim.Repository, Accession: claim.Accession, EvidenceText: claim.Excerpt}}}
				if err := saveDataset(ctx, tx, &d); err != nil {
					return err
				}
			}
			if err := linkDataset(ctx, tx, id, PaperLinkInput{PaperID: before.ID, Role: claim.Role, EvidenceText: claim.Excerpt}); err != nil {
				return err
			}
			value = []byte(claim.Repository + " " + claim.Accession)
		}
		if _, err := addEvidence(ctx, tx, EvidenceInput{PaperID: before.ID, Field: claim.Field, Value: string(value), FileID: file.ID, Excerpt: claim.Excerpt, Extractor: "fulltext-metadata"}); err != nil {
			return fmt.Errorf("保存全文依据：%w", err)
		}
	}
	return tx.Commit()
}
