package gitrepo

import (
	"context"

	"github.com/jhonataT/ai-decrivo-golang/internal/debt"
	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// Source adapta o Repo à interface review.DiffSource.
type Source struct{}

// Garante em tempo de compilação que Source satisfaz review.DiffSource.
var _ review.DiffSource = Source{}

func (Source) Head(ctx context.Context, repoPath, branch string) (string, error) {
	repo, err := Open(repoPath)
	if err != nil {
		return "", err
	}
	return repo.RevParse(ctx, branch)
}

func (Source) Changes(ctx context.Context, repoPath, base, branch string) ([]review.FileChange, error) {
	repo, err := Open(repoPath)

	if err != nil {
		return nil, err
	}

	diffs, err := repo.Changes(ctx, base, branch)
	if err != nil {
		return nil, err
	}

	files := make([]review.FileChange, len(diffs))
	for i, d := range diffs {
		files[i] = review.FileChange{Path: d.Path, Patch: d.Patch, Lines: review.ParsePatch(d.Patch)}
	}
	return files, nil
}

// Garante em tempo de compilação que Source também serve ao mapeamento de dívidas.
var _ debt.Source = Source{}

func (Source) ChangedFiles(ctx context.Context, repoPath, base, branch string) ([]string, error) {
	repo, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return repo.ChangedFiles(ctx, base, branch)
}

func (Source) ReadFile(ctx context.Context, repoPath, commit, path string) (string, error) {
	repo, err := Open(repoPath)
	if err != nil {
		return "", err
	}
	return repo.ShowFile(ctx, commit, path)
}
