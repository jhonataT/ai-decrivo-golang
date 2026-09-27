package review

import (
	"context"
	"errors"
	"testing"
)

type fakeDiffs struct{}

func (fakeDiffs) Changes(context.Context, string, string, string) ([]FileChange, error) {
	patch := "@@ -1,1 +1,2 @@\n a\n+b\n"
	return []FileChange{{Path: "x.go", Patch: patch, Lines: ParsePatch(patch)}}, nil
}

// memStore guarda em memória e pode ser programado para falhar.
type memStore struct {
	saved map[string]Review
	fail  bool
}

func (m *memStore) Save(r Review) error {
	if m.fail {
		return errors.New("disco cheio")
	}
	m.saved[r.ID] = r
	return nil
}

func (m *memStore) LoadAll() ([]Review, error) {
	var out []Review
	for _, r := range m.saved {
		out = append(out, r)
	}
	return out, nil
}

func newTestService(t *testing.T) (*Service, *memStore, string) {
	t.Helper()
	db := &memStore{saved: map[string]Review{}}
	svc := NewService(fakeDiffs{}, db)
	rev, err := svc.Start(context.Background(), "repo", "main", "feat")
	if err != nil {
		t.Fatal(err)
	}
	return svc, db, rev.ID
}

func TestFalhaAoSalvarNaoAlteraMemoria(t *testing.T) {
	svc, db, id := newTestService(t)

	db.fail = true
	if _, err := svc.AddFinding(id, Finding{File: "x.go", Line: 2, Title: "t"}); err == nil {
		t.Fatal("esperava erro de salvamento")
	}
	rev, _ := svc.Get(id)
	if len(rev.Findings) != 0 {
		t.Fatalf("a memória mudou mesmo com falha ao salvar: %d achados", len(rev.Findings))
	}

	db.fail = false
	if _, err := svc.AddFinding(id, Finding{File: "x.go", Line: 2, Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if got := len(db.saved[id].Findings); got != 1 {
		t.Fatalf("disco deveria ter 1 achado, tem %d", got)
	}
}

func TestMarkReadyValidaSugestao(t *testing.T) {
	svc, _, id := newTestService(t)

	err := svc.MarkReady(id, Finish{Summary: "entrega X", Recommendation: "lgtm"})
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("esperava ErrInvalidRecommendation, veio %v", err)
	}
	if err := svc.MarkReady(id, Finish{Recommendation: RecommendApprove}); !errors.Is(err, ErrEmptySummary) {
		t.Fatalf("esperava ErrEmptySummary, veio %v", err)
	}
	if err := svc.MarkReady(id, Finish{Summary: "entrega X", Recommendation: RecommendApprove, Reason: "ok"}); err != nil {
		t.Fatal(err)
	}
	rev, _ := svc.Get(id)
	if rev.Status != StatusReady || rev.Recommendation != RecommendApprove {
		t.Fatalf("status %s, sugestão %s", rev.Status, rev.Recommendation)
	}
}

func TestLoadRetomaHistorico(t *testing.T) {
	svc, db, id := newTestService(t) // fica em "running", como se o app caísse
	_, _ = svc.AddFinding(id, Finding{File: "x.go", Line: 2, Title: "t"})

	// Um novo Service lendo o mesmo "disco", como ao reabrir o app.
	svc2 := NewService(fakeDiffs{}, db)
	if err := svc2.Load(); err != nil {
		t.Fatal(err)
	}
	rev, err := svc2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Status != StatusReady || !rev.Interrupted {
		t.Errorf("análise interrompida deveria virar ready+interrupted; veio %s/%v", rev.Status, rev.Interrupted)
	}
	if len(rev.Files[0].Lines) == 0 {
		t.Error("Lines deveria ser recalculado a partir do Patch")
	}
	// O próximo ID continua a sequência, sem sobrescrever o histórico.
	next, _ := svc2.Start(context.Background(), "repo", "main", "outra")
	if next.ID == id {
		t.Fatalf("ID %s repetido", next.ID)
	}
	if cur, _ := svc2.Current(); cur.ID != next.ID {
		t.Errorf("Current deveria ser a nova revisão %s, veio %s", next.ID, cur.ID)
	}
}
