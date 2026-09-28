package debt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// fakeGit tem dois arquivos: a.go com 10 linhas e b.go com 3. Só b.go
// mudou na branch.
type fakeGit struct{}

func (fakeGit) Head(_ context.Context, _, ref string) (string, error) {
	return "sha-" + ref + "-0123456789", nil
}

func (fakeGit) ChangedFiles(context.Context, string, string, string) ([]string, error) {
	return []string{"b.go"}, nil
}

func (fakeGit) ReadFile(_ context.Context, _, _, path string) (string, error) {
	switch path {
	case "a.go":
		var b strings.Builder
		for i := 1; i <= 10; i++ {
			fmt.Fprintf(&b, "linha %d\r\n", i)
		}
		return b.String(), nil
	case "b.go":
		return "x\ny\nz\n", nil
	}
	return "", errors.New("não existe")
}

var labels = []Label{{Name: "tech-debt"}, {Name: "backend"}}

func newScan(t *testing.T, branch string) (*Service, string) {
	t.Helper()
	svc := NewService(fakeGit{}, nil)
	sc, err := svc.Start(context.Background(), StartInput{
		Repo: "repo", Base: "main", Branch: branch,
		Labels: labels, Projects: []Project{{Number: 3, Title: "Roadmap"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, sc.ID
}

func text(title string) Text {
	return Text{Title: title, Description: "desc", Reason: "motivo", EffortHours: 4, Labels: []string{"tech-debt"}}
}

func TestStartEscopo(t *testing.T) {
	svc, id := newScan(t, "")
	sc, _ := svc.Get(id)
	if sc.Commit != "sha-main-0123456789" || sc.Files != nil {
		t.Errorf("projeto inteiro deveria usar o commit da base e não restringir arquivos: %+v", sc)
	}

	svc, id = newScan(t, "feat")
	sc, _ = svc.Get(id)
	if sc.Commit != "sha-feat-0123456789" || len(sc.Files) != 1 {
		t.Errorf("modo diff deveria usar o commit da branch e os arquivos do diff: %+v", sc)
	}
}

func TestAddDebtValida(t *testing.T) {
	ctx := context.Background()
	svc, id := newScan(t, "feat")

	cases := []struct {
		name string
		d    Debt
		want error
	}{
		{"fora do diff", Debt{Text: text("t"), File: "a.go"}, ErrFileNotInScope},
		{"linha além do arquivo", Debt{Text: text("t"), File: "b.go", StartLine: 2, EndLine: 9}, ErrLineOutOfRange},
		{"fim antes do início", Debt{Text: text("t"), File: "b.go", StartLine: 3, EndLine: 2}, ErrLineOutOfRange},
		{"sem esforço", Debt{Text: Text{Title: "t", Description: "d", Reason: "r"}, File: "b.go"}, ErrInvalidEffort},
		{"sem motivo", Debt{Text: Text{Title: "t", Description: "d", EffortHours: 1}, File: "b.go"}, ErrMissingText},
		{"label desconhecida", Debt{Text: Text{Title: "t", Description: "d", Reason: "r", EffortHours: 1, Labels: []string{"bug"}}, File: "b.go"}, ErrUnknownLabel},
	}
	for _, c := range cases {
		if _, err := svc.AddDebt(ctx, id, c.d); !errors.Is(err, c.want) {
			t.Errorf("%s: esperava %v, veio %v", c.name, c.want, err)
		}
	}
	sc, _ := svc.Get(id)
	if len(sc.Debts) != 0 {
		t.Fatalf("nenhuma dívida inválida deveria entrar, entraram %d", len(sc.Debts))
	}
}

func TestAddDebtGuardaTrecho(t *testing.T) {
	svc, id := newScan(t, "")
	did, err := svc.AddDebt(context.Background(), id, Debt{Text: text("t"), File: "a.go", StartLine: 5, EndLine: 6})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := svc.Get(id)
	d, _ := sc.Debt(did)
	if len(d.Snippet) != 6 || d.Snippet[0].N != 3 || d.Snippet[5].N != 8 {
		t.Fatalf("trecho deveria ir da linha 3 à 8, veio %+v", d.Snippet)
	}
	if d.Snippet[0].Text != "linha 3" {
		t.Errorf("o \\r do CRLF deveria sair do trecho: %q", d.Snippet[0].Text)
	}

	// Só StartLine: a dívida é de uma linha.
	did, _ = svc.AddDebt(context.Background(), id, Debt{Text: text("t2"), File: "a.go", StartLine: 10})
	sc, _ = svc.Get(id)
	if d, _ := sc.Debt(did); d.EndLine != 10 || d.Location() != "a.go:10" {
		t.Errorf("EndLine deveria ser 10, veio %d (%s)", d.EndLine, d.Location())
	}
}

// ready devolve um mapeamento com três dívidas (d1, d2, d3), pronto para vereditos.
func ready(t *testing.T) (*Service, string) {
	t.Helper()
	svc, id := newScan(t, "")
	for i, line := range []int{1, 0, 4} {
		d := Debt{Text: text(fmt.Sprintf("t%d", i+1)), File: "a.go", StartLine: line}
		if _, err := svc.AddDebt(context.Background(), id, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.MarkReady(id, "três dívidas"); err != nil {
		t.Fatal(err)
	}
	return svc, id
}

func TestAjusteEDesfazer(t *testing.T) {
	svc, id := ready(t)
	adj := Text{Title: "novo", Description: "nova desc", Reason: "novo motivo", EffortHours: 1.5, Labels: []string{"backend", "backend"}}
	if err := svc.Adjust(id, "d1", adj); err != nil {
		t.Fatal(err)
	}
	sc, _ := svc.Get(id)
	d, _ := sc.Debt("d1")
	if !d.Adjusted || d.Verdict != review.VerdictAccepted || d.EffortHours != 1.5 || len(d.Labels) != 1 {
		t.Fatalf("ajuste não aplicado: %+v", d)
	}
	if d.Orig == nil || d.Orig.Title != "t1" || d.Orig.EffortHours != 4 {
		t.Fatalf("original do agente perdido: %+v", d.Orig)
	}

	if err := svc.Adjust(id, "d1", Text{Title: "x", Description: "y", Reason: "z", EffortHours: 0}); !errors.Is(err, ErrInvalidEffort) {
		t.Fatalf("esperava ErrInvalidEffort, veio %v", err)
	}

	if err := svc.SetVerdict(id, "d1", review.VerdictPending); err != nil {
		t.Fatal(err)
	}
	sc, _ = svc.Get(id)
	d, _ = sc.Debt("d1")
	if d.Adjusted || d.Title != "t1" || d.EffortHours != 4 {
		t.Errorf("desfazer deveria voltar ao texto do agente: %+v", d)
	}
}

func TestFinalizeEIssues(t *testing.T) {
	svc, id := ready(t)
	_ = svc.SetVerdict(id, "d1", review.VerdictAccepted)
	if _, err := svc.Finalize(id, "", true); !errors.Is(err, review.ErrPendingVerdicts) {
		t.Fatalf("esperava ErrPendingVerdicts, veio %v", err)
	}
	_ = svc.Adjust(id, "d2", Text{Title: "arquivo", Description: "d", Reason: "r", EffortHours: 2})
	_ = svc.SetVerdict(id, "d3", review.VerdictRejected)

	if _, err := svc.Finalize(id, "Inexistente", true); !errors.Is(err, ErrUnknownProject) {
		t.Fatalf("esperava ErrUnknownProject, veio %v", err)
	}
	sc, err := svc.Finalize(id, "Roadmap", true)
	if err != nil {
		t.Fatal(err)
	}
	if sc.AcceptedHours() != 6 || sc.TotalHours() != 12 {
		t.Errorf("horas aceitas %v, total %v", sc.AcceptedHours(), sc.TotalHours())
	}

	issues, err := sc.Issues("https://github.com/o/r/")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].DebtID != "d1" || issues[1].DebtID != "d2" {
		t.Fatalf("esperava as issues de d1 e d2, veio %+v", issues)
	}
	body := issues[0].Body
	for _, want := range []string{
		"[`a.go:1`](https://github.com/o/r/blob/sha-main-0123456789/a.go#L1-L1)",
		"**Esforço estimado:** 4 h",
		"decrivo:sha-mai/debt-1/d1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("corpo sem %q:\n%s", want, body)
		}
	}
	if !strings.Contains(issues[1].Body, "https://github.com/o/r/blob/sha-main-0123456789/a.go)") {
		t.Errorf("dívida de arquivo deveria linkar o arquivo sem linhas:\n%s", issues[1].Body)
	}
	if issues[1].Labels == nil {
		t.Error("labels deveria ser lista vazia, não null")
	}
}

func TestMarkPublishedPorDivida(t *testing.T) {
	svc, id := ready(t)
	_ = svc.SetVerdict(id, "d1", review.VerdictAccepted)
	_ = svc.SetVerdict(id, "d2", review.VerdictAccepted)
	_ = svc.SetVerdict(id, "d3", review.VerdictRejected)

	if _, err := svc.Finalize(id, "", false); err != nil {
		t.Fatal(err)
	}
	sc, _ := svc.Get(id)
	if _, err := sc.Issues(""); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("sem \"criar issues\" deveria bloquear, veio %v", err)
	}

	svc, id = ready(t)
	_ = svc.SetVerdict(id, "d1", review.VerdictAccepted)
	_ = svc.SetVerdict(id, "d2", review.VerdictAccepted)
	_ = svc.SetVerdict(id, "d3", review.VerdictRejected)
	_, _ = svc.Finalize(id, "", true)

	if err := svc.MarkPublished(id, "d3", "https://x/3"); !errors.Is(err, ErrNotAccepted) {
		t.Fatalf("dívida recusada não vira issue, veio %v", err)
	}
	if err := svc.MarkPublished(id, "d1", "https://x/1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkPublished(id, "d1", "https://x/9"); !errors.Is(err, ErrAlreadyIssued) {
		t.Fatalf("segunda issue para a mesma dívida deveria falhar, veio %v", err)
	}
	sc, _ = svc.Get(id)
	issues, _ := sc.Issues("")
	if len(issues) != 1 || issues[0].DebtID != "d2" {
		t.Fatalf("só a d2 deveria faltar, veio %+v", issues)
	}
}

// memStore guarda em memória, para testar a retomada do histórico.
type memStore struct{ saved map[string]Scan }

func (m *memStore) Save(s Scan) error { m.saved[s.ID] = s; return nil }
func (m *memStore) LoadAll() ([]Scan, error) {
	var out []Scan
	for _, s := range m.saved {
		out = append(out, s)
	}
	return out, nil
}

func TestLoadRetomaInterrompido(t *testing.T) {
	db := &memStore{saved: map[string]Scan{}}
	svc := NewService(fakeGit{}, db)
	sc, _ := svc.Start(context.Background(), StartInput{Repo: "r", Base: "main"})

	svc2 := NewService(fakeGit{}, db)
	if err := svc2.Load(); err != nil {
		t.Fatal(err)
	}
	got, _ := svc2.Get(sc.ID)
	if got.Status != review.StatusReady || !got.Interrupted {
		t.Errorf("deveria virar ready+interrupted, veio %s/%v", got.Status, got.Interrupted)
	}
	next, _ := svc2.Start(context.Background(), StartInput{Repo: "r", Base: "main"})
	if next.ID == sc.ID {
		t.Fatalf("ID %s repetido", next.ID)
	}
}
