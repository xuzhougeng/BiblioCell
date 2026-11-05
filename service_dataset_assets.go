package main

import (
	"bibliocell/internal/library"
	"context"
	"errors"
	"github.com/egoist/mygo"
)

func (l *Library) ListDatasetManifests() ([]library.DatasetManifest, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListDatasetManifests(context.Background())
}
func (l *Library) SaveDatasetPlan(id string, revision int, plan library.DatasetPlan) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.SaveDatasetPlan(context.Background(), id, revision, plan)
}
func (l *Library) SaveDatasetAsset(asset library.DatasetAsset) (library.DatasetAsset, error) {
	if l.store == nil {
		return library.DatasetAsset{}, errors.New("library database is not ready")
	}
	return l.store.SaveDatasetAsset(context.Background(), asset)
}
func (l *Library) VerifyDatasetAsset(ctx context.Context, datasetID, id string) (library.DatasetAsset, error) {
	if l.store == nil {
		return library.DatasetAsset{}, errors.New("library database is not ready")
	}
	return l.store.VerifyDatasetAsset(ctx, datasetID, id)
}
func (l *Library) RemoveDatasetAsset(datasetID, id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.RemoveDatasetAsset(context.Background(), datasetID, id)
}
func (l *Library) PrepareDatasetDirectory(id string) (string, error) {
	if l.store == nil {
		return "", errors.New("library database is not ready")
	}
	return l.store.PrepareDatasetDirectory(context.Background(), id)
}
func (l *Library) ExportDatasetManifest(id string) (string, error) {
	if l.store == nil {
		return "", errors.New("library database is not ready")
	}
	return l.store.ExportDatasetManifest(context.Background(), id)
}
func (l *Library) OpenDatasetDirectory(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	path, err := l.store.DatasetDirectoryPath(context.Background(), id)
	if err != nil {
		return err
	}
	return mygo.Shell.OpenPath(path)
}
func (l *Library) DiscoverGEOFiles(ctx context.Context, id, accession string) (library.DatasetDiscovery, error) {
	if l.store == nil {
		return library.DatasetDiscovery{}, errors.New("library database is not ready")
	}
	return l.store.DiscoverGEOFiles(ctx, id, accession)
}
