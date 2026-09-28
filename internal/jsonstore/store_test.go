package jsonstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	st, err := Reviews(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := review.Review{
		ID: "rev-7", Repo: "G:/app", Base: "main", Branch: "feat",
		Status:         review.StatusReady,
		Files:          []review.FileChange{{Path: "a.go", Patch: "@@ -1 +1 @@\n-a\n+b\n", Context: "faz A"}},
		Findings:       []review.Finding{{ID: "f1", File: "a.go", Line: 1, Title: "ação", Verdict: review.VerdictPending}},
		ChangeSummary:  "entrega B",
		Recommendation: review.RecommendComment,
		CreatedAt:      time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC),
	}
	if err := st.Save(in); err != nil {
		t.Fatal(err)
	}
	// Salvar de novo sobrescreve, sem deixar temporários para trás.
	in.ChangeSummary = "entrega C"
	if err := st.Save(in); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(st.Dir())
	if len(entries) != 1 || entries[0].Name() != "rev-7.json" {
		t.Fatalf("esperava só rev-7.json na pasta, veio %v", entries)
	}

	out, err := st.LoadAll()
	if err != nil || len(out) != 1 {
		t.Fatalf("LoadAll = %d revisões, err %v", len(out), err)
	}
	got := out[0]
	if got.ChangeSummary != "entrega C" || got.Files[0].Context != "faz A" || got.Findings[0].Title != "ação" {
		t.Errorf("dados não bateram: %+v", got)
	}
	if got.Files[0].Lines != nil {
		t.Error("Lines não deveria ir para o disco")
	}
}

func TestLoadAllIgnoraArquivoCorrompido(t *testing.T) {
	st, _ := Reviews(t.TempDir())
	_ = st.Save(review.Review{ID: "rev-1"})
	_ = os.WriteFile(filepath.Join(st.Dir(), "rev-2.json"), []byte("{quebrado"), 0o644)

	out, err := st.LoadAll()
	if len(out) != 1 || out[0].ID != "rev-1" {
		t.Fatalf("a revisão boa deveria carregar; veio %v", out)
	}
	if err == nil || !strings.Contains(err.Error(), "rev-2.json") {
		t.Fatalf("esperava erro citando rev-2.json, veio %v", err)
	}
}

func TestSaveRecusaIDPerigoso(t *testing.T) {
	st, _ := Reviews(t.TempDir())
	if err := st.Save(review.Review{ID: `..\..\x`}); err == nil {
		t.Fatal("ID com caminho deveria ser recusado")
	}
}
