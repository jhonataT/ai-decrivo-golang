// Package debt é o mapeamento de dívidas técnicas: o agente aponta as dívidas
// de um repositório (ou só das que uma branch introduz), quem revisa aceita,
// ajusta ou recusa cada uma, e as aceitas viram issues no GitHub.
package debt

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

var (
	ErrInvalidEffort  = errors.New("o esforço estimado precisa ser um número de horas maior que zero (ex.: 4 ou 1,5)")
	ErrMissingText    = errors.New("título, descrição e motivo não podem ficar vazios")
	ErrUnknownLabel   = errors.New("label não existe no repositório")
	ErrUnknownProject = errors.New("projeto não existe na lista do mapeamento")
	ErrFileNotInScope = errors.New("arquivo fora do escopo do mapeamento (não faz parte do diff da branch)")
	ErrFileNotFound   = errors.New("arquivo não existe no commit mapeado")
	ErrLineOutOfRange = errors.New("intervalo de linhas fora do arquivo")
	ErrEmptySummary   = errors.New("o resumo do mapeamento não pode ficar vazio")
	ErrAlreadyIssued  = errors.New("a dívida já tem issue")
	ErrNotAccepted    = errors.New("só dívidas aceitas viram issue")
	ErrEmptyIssueURL  = errors.New("informe a URL da issue criada")
	ErrNotPublishable = errors.New("mapeamento não liberado para criar issues")
)

// Label e Project são o que o agente encontrou no repositório (gh label list,
// gh project list) ao iniciar; quem revisa escolhe entre eles.
type Label struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Project struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// Text reúne o que quem revisa pode ajustar numa dívida.
type Text struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Reason      string   `json:"reason"`
	EffortHours float64  `json:"effortHours"`
	Labels      []string `json:"labels,omitempty"`
}

// Line é uma linha do trecho de código mostrado junto da dívida.
type Line struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

type Debt struct {
	ID string `json:"id"`
	Text
	File string `json:"file"`
	// StartLine e EndLine são 0 quando a dívida é do arquivo como um todo.
	StartLine int            `json:"startLine,omitempty"`
	EndLine   int            `json:"endLine,omitempty"`
	Snippet   []Line         `json:"snippet,omitempty"`
	Verdict   review.Verdict `json:"verdict"`

	// Preenchidos quando quem revisa ajusta o texto do agente.
	Adjusted bool  `json:"adjusted,omitempty"`
	Orig     *Text `json:"orig,omitempty"`

	IssueURL string `json:"issueUrl,omitempty"`
}

// Location é "arquivo:10-20", "arquivo:10" ou só o arquivo.
func (d Debt) Location() string {
	switch {
	case d.StartLine == 0:
		return d.File
	case d.EndLine == d.StartLine:
		return fmt.Sprintf("%s:%d", d.File, d.StartLine)
	}
	return fmt.Sprintf("%s:%d-%d", d.File, d.StartLine, d.EndLine)
}

type Scan struct {
	ID   string `json:"id"`
	Repo string `json:"repo"`
	Base string `json:"base"`
	// Branch vazia: o projeto inteiro, no estado da Base. Preenchida: só as
	// dívidas que a branch introduz, restritas aos arquivos do diff (Files).
	Branch string   `json:"branch,omitempty"`
	Commit string   `json:"commit"`
	Files  []string `json:"files,omitempty"`

	Status   review.Status `json:"status"`
	Labels   []Label       `json:"labels,omitempty"`
	Projects []Project     `json:"projects,omitempty"`
	Debts    []Debt        `json:"debts"`
	Summary  string        `json:"summary,omitempty"`

	// Escolhidos por quem revisa ao finalizar.
	Project string `json:"project,omitempty"`
	Publish bool   `json:"publish,omitempty"`

	// Interrupted marca um mapeamento que não terminou (o app fechou antes do finish_debt_scan).
	Interrupted bool `json:"interrupted,omitempty"`

	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	FinalizedAt time.Time `json:"finalizedAt,omitzero"`
}

// Scope descreve o que foi mapeado, para a tela e para o agente.
func (s *Scan) Scope() string {
	if s.Branch == "" {
		return "projeto inteiro em " + s.Base
	}
	return "diff de " + s.Branch + " contra " + s.Base
}

func (s *Scan) Debt(id string) (Debt, bool) {
	if d := s.debt(id); d != nil {
		return *d, true
	}
	return Debt{}, false
}

// debt devolve um ponteiro para a dívida dentro do slice, para poder alterá-la.
func (s *Scan) debt(id string) *Debt {
	for i := range s.Debts {
		if s.Debts[i].ID == id {
			return &s.Debts[i]
		}
	}
	return nil
}

func (s *Scan) Pending() int {
	n := 0
	for _, d := range s.Debts {
		if d.Verdict == review.VerdictPending {
			n++
		}
	}
	return n
}

// AcceptedHours soma o esforço das dívidas aceitas.
func (s *Scan) AcceptedHours() float64 {
	var h float64
	for _, d := range s.Debts {
		if d.Verdict == review.VerdictAccepted {
			h += d.EffortHours
		}
	}
	return h
}

// TotalHours soma o esforço de todas as dívidas como o agente estimou,
// ignorando os ajustes de quem revisa.
func (s *Scan) TotalHours() float64 {
	var h float64
	for _, d := range s.Debts {
		if d.Orig != nil {
			h += d.Orig.EffortHours
		} else {
			h += d.EffortHours
		}
	}
	return h
}

func (s *Scan) HasLabel(name string) bool {
	return slices.ContainsFunc(s.Labels, func(l Label) bool { return l.Name == name })
}

func (s *Scan) inScope(file string) bool {
	return s.Branch == "" || slices.Contains(s.Files, file)
}

// validate confere o texto de uma dívida e devolve as labels sem repetição.
func (s *Scan) validate(t Text) (Text, error) {
	if t.Title == "" || t.Description == "" || t.Reason == "" {
		return t, ErrMissingText
	}
	if t.EffortHours <= 0 {
		return t, fmt.Errorf("%w (recebido: %v)", ErrInvalidEffort, t.EffortHours)
	}
	var labels []string
	for _, l := range t.Labels {
		if !s.HasLabel(l) {
			return t, fmt.Errorf("%w: %q", ErrUnknownLabel, l)
		}
		if !slices.Contains(labels, l) {
			labels = append(labels, l)
		}
	}
	t.Labels = labels
	return t, nil
}

func (s *Scan) Finalize(project string, publish bool) error {
	if s.Status != review.StatusReady {
		return fmt.Errorf("%w (status: %s)", review.ErrNotReady, s.Status)
	}
	if s.Pending() > 0 {
		return fmt.Errorf("%w: %d restantes", review.ErrPendingVerdicts, s.Pending())
	}
	if project != "" && !slices.ContainsFunc(s.Projects, func(p Project) bool { return p.Title == project }) {
		return fmt.Errorf("%w: %q", ErrUnknownProject, project)
	}
	s.Project = project
	s.Publish = publish
	s.Status = review.StatusFinalized
	s.FinalizedAt = time.Now()
	return nil
}

// Hours formata horas sem casas decimais desnecessárias: 4 → "4 h", 1.5 → "1.5 h".
func Hours(h float64) string {
	return strconv.FormatFloat(h, 'f', -1, 64) + " h"
}
