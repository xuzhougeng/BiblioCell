package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

// ExtractPaperMetadata reads the stored full text with the chat model. It makes
// no database changes; the desktop window lets the person select suggestions.
func (l *Library) ExtractPaperMetadata(paperID string) (library.MetadataProposal, error) {
	if l.store == nil || l.settings == nil {
		return library.MetadataProposal{}, errors.New("library database is not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	paper, err := l.store.GetPaper(ctx, paperID)
	if err != nil {
		return library.MetadataProposal{}, err
	}
	file, pages, err := l.store.PaperFulltextPages(ctx, paperID)
	if err != nil {
		return library.MetadataProposal{}, err
	}
	fulltext := strings.TrimSpace(strings.Join(pages, "\n\n"))
	if file.ID == "" || fulltext == "" {
		return library.MetadataProposal{}, errors.New("请先导入可读取文本的全文；本功能不会使用摘要代替全文")
	}
	profile, err := l.settings.Get().ModelFor(settings.TaskChat)
	if err != nil {
		return library.MetadataProposal{}, err
	}
	provider, err := agent.NewProvider(profile.Config)
	if err != nil {
		return library.MetadataProposal{}, err
	}
	proposal, err := agent.ExtractMetadata(ctx, provider, paper.Title, fulltext)
	if err != nil {
		return library.MetadataProposal{}, err
	}
	proposal.PaperID, proposal.FileID, proposal.FileTitle = paper.ID, file.ID, file.Title
	// Each claim is traced to its PDF page, and the evidence names the model that read it.
	proposal.Claims = library.LocateClaimPages(proposal.Claims, pages)
	proposal.Extractor = "fulltext-metadata:" + profile.Name + "/" + profile.Config.Model
	return proposal, nil
}

// ApplyPaperMetadata saves only the suggestions selected by the person, with
// conflict detection and evidence in the same transaction.
func (l *Library) ApplyPaperMetadata(before library.Paper, proposal library.MetadataProposal) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return l.store.ApplyPaperMetadata(ctx, before, proposal)
}
