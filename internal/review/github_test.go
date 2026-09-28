package review

import (
	"errors"
	"strings"
	"testing"
)

// readyWithFindings devolve uma revisão pronta para vereditos com três achados:
// f1 na linha 2, f2 no arquivo (linha 0) e f3 na linha 2.
func readyWithFindings(t *testing.T) (*Service, string) {
	t.Helper()
	svc, _, id := newTestService(t)
	for _, f := range []Finding{
		{File: "x.go", Line: 2, Kind: "improvement", Severity: "minor", Title: "t1", Body: "b1"},
		{File: "x.go", Line: 0, Kind: "improvement", Severity: "info", Title: "t2", Body: "b2"},
		{File: "x.go", Line: 2, Kind: "improvement", Severity: "major", Title: "t3", Body: "b3"},
	} {
		if _, err := svc.AddFinding(id, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.MarkReady(id, Finish{Summary: "entrega X", Recommendation: RecommendComment}); err != nil {
		t.Fatal(err)
	}
	return svc, id
}

func TestStartRegistraHeadSHA(t *testing.T) {
	svc, _, id := newTestService(t)
	rev, _ := svc.Get(id)
	if rev.HeadSHA != "abc123" {
		t.Fatalf("HeadSHA = %q", rev.HeadSHA)
	}
}

func TestFinalizeValidaDecisao(t *testing.T) {
	svc, id := readyWithFindings(t)
	for _, fid := range []string{"f1", "f2", "f3"} {
		_ = svc.SetVerdict(id, fid, VerdictRejected)
	}
	if _, err := svc.Finalize(id, "lgtm", true); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("esperava ErrInvalidDecision, veio %v", err)
	}
	if _, err := svc.Finalize(id, RecommendApprove, true); err != nil {
		t.Fatal(err)
	}
	rev, _ := svc.Get(id)
	if rev.Decision != RecommendApprove || !rev.Publish {
		t.Fatalf("decisão %s, publish %v", rev.Decision, rev.Publish)
	}
}

func TestGitHubReviewSoComAceitos(t *testing.T) {
	svc, id := readyWithFindings(t)
	_ = svc.SetVerdict(id, "f1", VerdictAccepted)
	_ = svc.SetVerdict(id, "f2", VerdictAccepted)
	_ = svc.Adjust(id, "f3", "t3 ajustado", "b3 ajustado")
	if _, err := svc.Finalize(id, RecommendRequestChanges, true); err != nil {
		t.Fatal(err)
	}
	rev, _ := svc.Get(id)
	gh, err := rev.GitHubReview()
	if err != nil {
		t.Fatal(err)
	}

	if gh.CommitID != "abc123" || gh.Event != "REQUEST_CHANGES" {
		t.Errorf("commit %q, event %q", gh.CommitID, gh.Event)
	}
	if len(gh.Comments) != 2 {
		t.Fatalf("esperava 2 comentários de linha, veio %d", len(gh.Comments))
	}
	c := gh.Comments[1]
	if c.Path != "x.go" || c.Line != 2 || c.Side != "RIGHT" || c.Body != "**t3 ajustado**\n\nb3 ajustado" {
		t.Errorf("comentário ajustado errado: %+v", c)
	}
	// O achado de arquivo vai para o corpo, junto com o resumo.
	if !strings.HasPrefix(gh.Body, "entrega X") || !strings.Contains(gh.Body, "`x.go` · **t2**") {
		t.Errorf("corpo sem resumo ou achado de arquivo:\n%s", gh.Body)
	}
}

func TestGitHubReviewIgnoraRecusados(t *testing.T) {
	svc, id := readyWithFindings(t)
	for _, fid := range []string{"f1", "f2", "f3"} {
		_ = svc.SetVerdict(id, fid, VerdictRejected)
	}
	_, _ = svc.Finalize(id, RecommendApprove, true)
	rev, _ := svc.Get(id)
	gh, err := rev.GitHubReview()
	if err != nil {
		t.Fatal(err)
	}
	if gh.Comments == nil || len(gh.Comments) != 0 {
		t.Errorf("comments deveria ser uma lista vazia (não null), veio %#v", gh.Comments)
	}
	if strings.Contains(gh.Body, "t1") || strings.Contains(gh.Body, "t2") {
		t.Errorf("achado recusado vazou para o corpo:\n%s", gh.Body)
	}
}

func TestPublicacaoExigeLiberacao(t *testing.T) {
	svc, id := readyWithFindings(t)

	rev, _ := svc.Get(id)
	if _, err := rev.GitHubReview(); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("antes de finalizar deveria bloquear, veio %v", err)
	}

	for _, fid := range []string{"f1", "f2", "f3"} {
		_ = svc.SetVerdict(id, fid, VerdictAccepted)
	}
	_, _ = svc.Finalize(id, RecommendComment, false)
	rev, _ = svc.Get(id)
	if _, err := rev.GitHubReview(); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("sem \"publicar no PR\" deveria bloquear, veio %v", err)
	}
	if err := svc.MarkPublished(id, "https://x"); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("MarkPublished sem liberação deveria falhar, veio %v", err)
	}
}

func TestMarkPublishedEvitaDuplicar(t *testing.T) {
	svc, id := readyWithFindings(t)
	for _, fid := range []string{"f1", "f2", "f3"} {
		_ = svc.SetVerdict(id, fid, VerdictAccepted)
	}
	_, _ = svc.Finalize(id, RecommendComment, true)

	if err := svc.MarkPublished(id, "https://github.com/o/r/pull/1#pullrequestreview-9"); err != nil {
		t.Fatal(err)
	}
	rev, _ := svc.Get(id)
	if rev.PublishedAt.IsZero() || rev.PublishedURL == "" {
		t.Fatal("publicação não registrada")
	}
	if _, err := rev.GitHubReview(); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("depois de publicada deveria bloquear, veio %v", err)
	}
	if err := svc.MarkPublished(id, "outra"); !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("segunda publicação deveria falhar, veio %v", err)
	}
}
