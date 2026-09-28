package review

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusReady     Status = "ready"
	StatusFinalized Status = "finalized"
)

type Verdict string

const (
	VerdictPending  Verdict = "pending"
	VerdictAccepted Verdict = "accepted"
	VerdictRejected Verdict = "rejected"
)

// Recommendation é a sugestão inicial do agente, no vocabulário de PR do GitHub.
// É só uma sugestão: o veredito de cada achado continua sendo humano.
type Recommendation string

const (
	RecommendApprove        Recommendation = "approve"
	RecommendRequestChanges Recommendation = "request_changes"
	RecommendComment        Recommendation = "comment"
)

func (r Recommendation) Valid() bool {
	switch r {
	case RecommendApprove, RecommendRequestChanges, RecommendComment:
		return true
	}
	return false
}

var (
	ErrNotFound              = errors.New("revisão não encontrada")
	ErrPendingVerdicts       = errors.New("ainda há achados sem veredito")
	ErrLineNotInDiff         = errors.New("linha não existe no diff deste arquivo")
	ErrFileNotInDiff         = errors.New("arquivo não faz parte do diff desta revisão")
	ErrNotRunning            = errors.New("a revisão não está mais em andamento")
	ErrNotReady              = errors.New("a revisão não está aguardando vereditos")
	ErrEmptyText             = errors.New("título e comentário não podem ficar vazios")
	ErrEmptySummary          = errors.New("o resumo das alterações não pode ficar vazio")
	ErrInvalidRecommendation = errors.New("sugestão inválida: use approve, request_changes ou comment")
)

type Finding struct {
	ID       string  `json:"id"`
	File     string  `json:"file"`
	Line     int     `json:"line"`
	Kind     string  `json:"kind"`
	Severity string  `json:"severity"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	Note     string  `json:"note,omitempty"`
	Verdict  Verdict `json:"verdict"`

	// Preenchidos quando o revisor ajusta o texto do agente.
	Adjusted  bool   `json:"adjusted,omitempty"`
	OrigTitle string `json:"origTitle,omitempty"`
	OrigBody  string `json:"origBody,omitempty"`
}

type Review struct {
	ID     string `json:"id"`
	Repo   string `json:"repo"`
	Base   string `json:"base"`
	Branch string `json:"branch"`
	// HeadSHA é o commit revisado; o GitHub precisa dele para ancorar os comentários.
	HeadSHA string       `json:"headSha,omitempty"`
	Status  Status       `json:"status"`
	Files   []FileChange `json:"files"`

	Findings []Finding `json:"findings"`

	// Preenchidos pelo agente no finish_review.
	ChangeSummary        string         `json:"changeSummary,omitempty"`
	Recommendation       Recommendation `json:"recommendation,omitempty"`
	RecommendationReason string         `json:"recommendationReason,omitempty"`

	// Interrupted marca uma análise que não terminou (o app fechou antes do finish_review).
	Interrupted bool `json:"interrupted,omitempty"`

	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	FinalizedAt time.Time `json:"finalizedAt,omitzero"`
}

type FileChange struct {
	Path  string `json:"path"`
	Patch string `json:"patch"`
	// Context é o que o arquivo faz no projeto (não o que mudou), escrito pelo agente.
	Context string `json:"context,omitempty"`
	// Lines é derivado de Patch; não vai para o disco e é recalculado ao carregar.
	Lines []DiffLine `json:"-"`
}

// Finding devolve uma cópia do achado com esse ID.
func (r *Review) Finding(id string) (Finding, bool) {
	if f := r.finding(id); f != nil {
		return *f, true
	}
	return Finding{}, false
}

// finding devolve um ponteiro para o achado dentro do slice, para poder alterá-lo.
// "for _, f := range" daria uma cópia; por isso o laço usa o índice.
func (r *Review) finding(id string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i]
		}
	}
	return nil
}

func (r *Review) Pending() int {
	n := 0
	for _, f := range r.Findings {
		if f.Verdict == VerdictPending {
			n++
		}
	}
	return n
}

// CountSeverity conta os achados de melhoria com a severidade dada.
func (r *Review) CountSeverity(sev string) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == sev && f.Kind != "praise" {
			n++
		}
	}
	return n
}

func (r *Review) Finalize() error {
	if r.Status != StatusReady {
		return fmt.Errorf("%w (status: %s)", ErrNotReady, r.Status)
	}
	if r.Pending() > 0 {
		return fmt.Errorf("%w: %d restantes", ErrPendingVerdicts, r.Pending())
	}
	r.Status = StatusFinalized
	r.FinalizedAt = time.Now()
	return nil
}

// Summary monta um markdown com o resumo das alterações e os achados aceitos.
func (r *Review) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Revisão de %s ← %s\n\n", r.Branch, r.Base)
	if r.ChangeSummary != "" {
		fmt.Fprintf(&b, "%s\n\n", r.ChangeSummary)
	}
	n := 0
	for _, f := range r.Findings {
		if f.Verdict != VerdictAccepted {
			continue
		}
		n++
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		note := ""
		if f.Adjusted {
			note = " · ajustado pelo revisor"
		}
		fmt.Fprintf(&b, "## %s\n\n`%s` · %s · %s%s\n\n%s\n\n", f.Title, loc, f.Kind, f.Severity, note, f.Body)
	}
	if n == 0 {
		b.WriteString("Nenhum achado aceito.\n")
	}
	return b.String()
}
