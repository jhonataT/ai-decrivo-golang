package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type Repo struct {
	Path string
}

func Open(path string) (*Repo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("Report the repository path")
	}

	abs, err := filepath.Abs(path)

	if err != nil {
		return nil, fmt.Errorf("Invalid path: %w", err)
	}

	r := &Repo{Path: abs}

	if _, err := r.run(context.Background(), "rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("%s is not a git repository", abs)
	}

	return r, nil
}

func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Path

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, errBuf.String())
	}

	return out.String(), nil
}

func (r *Repo) Branches(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads")

	if err != nil {
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

type FileDiff struct {
	Path   string
	Status string // A, M, D, R...
	Patch  string
}

// RevParse resolve uma referência (branch, tag) para o SHA do commit.
func (r *Repo) RevParse(ctx context.Context, ref string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *Repo) MergeBase(ctx context.Context, base, branch string) (string, error) {
	out, err := r.run(ctx, "merge-base", base, branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *Repo) Changes(ctx context.Context, base, branch string) ([]FileDiff, error) {
	mb, err := r.MergeBase(ctx, base, branch)
	if err != nil {
		return nil, err
	}

	out, err := r.run(ctx, "diff", "--name-status", mb+".."+branch)
	if err != nil {
		return nil, err
	}

	var files []FileDiff
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status, path := parts[0], parts[1]

		if status == "D" || skipFile(path) {
			continue
		}

		patch, err := r.run(ctx, "diff", "--unified=5", mb+".."+branch, "--", path)
		if err != nil {
			return nil, err
		}
		if patch == "" || len(patch) > 60_000 {
			continue
		}

		files = append(files, FileDiff{Path: path, Status: status, Patch: patch})
	}
	return files, nil
}

func skipFile(path string) bool {
	skipNames := []string{"package-lock.json", "go.sum", "yarn.lock", "pnpm-lock.yaml"}
	base := filepath.Base(path)
	if slices.Contains(skipNames, base) {
		return true
	}
	skipExt := []string{".png", ".jpg", ".svg", ".pdf", ".ico", ".woff", ".woff2"}
	return slices.Contains(skipExt, filepath.Ext(path))
}
