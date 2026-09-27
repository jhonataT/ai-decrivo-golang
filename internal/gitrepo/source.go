package gitrepo

import (
	"context"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// Source adapta o Repo à interface review.DiffSource.
type Source struct{}

// Garante em tempo de compilação que Source satisfaz review.DiffSource.
var _ review.DiffSource = Source{}

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
