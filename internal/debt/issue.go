package debt

import (
	"fmt"
	"strings"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// Issue é o que o agente passa para o gh issue create. O Decrivo só monta o
// conteúdo; quem cria a issue é o agente.
type Issue struct {
	DebtID string   `json:"debtId"`
	Key    string   `json:"key"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// Key identifica a dívida no corpo da issue, para o agente achar uma issue já
// criada (por exemplo, se caiu antes do mark_debt_published) e não duplicar.
func (s *Scan) Key(d Debt) string {
	return fmt.Sprintf("decrivo:%s/%s/%s", short(s.Commit), s.ID, d.ID)
}

// CanPublish diz se o agente já pode criar as issues.
func (s *Scan) CanPublish() error {
	switch {
	case s.Status != review.StatusFinalized:
		return fmt.Errorf("%w: o mapeamento ainda não foi finalizado (status: %s)", ErrNotPublishable, s.Status)
	case !s.Publish:
		return fmt.Errorf("%w: quem revisou não marcou \"criar issues\"", ErrNotPublishable)
	case s.Commit == "":
		return fmt.Errorf("%w: mapeamento sem o commit registrado", ErrNotPublishable)
	}
	return nil
}

// Issues devolve as issues das dívidas aceitas que ainda não foram criadas.
// repoURL (https://github.com/dono/repo) serve para o link fixo do trecho.
func (s *Scan) Issues(repoURL string) ([]Issue, error) {
	if err := s.CanPublish(); err != nil {
		return nil, err
	}
	repoURL = strings.TrimSuffix(strings.TrimSpace(repoURL), "/")
	out := []Issue{}
	for _, d := range s.Debts {
		if d.Verdict != review.VerdictAccepted || d.IssueURL != "" {
			continue
		}
		labels := d.Labels
		if labels == nil {
			labels = []string{}
		}
		out = append(out, Issue{DebtID: d.ID, Key: s.Key(d), Title: d.Title, Body: s.issueBody(d, repoURL), Labels: labels})
	}
	return out, nil
}

func (s *Scan) issueBody(d Debt, repoURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", d.Description)
	fmt.Fprintf(&b, "**Por que é dívida técnica**\n\n%s\n\n", d.Reason)

	where := "`" + d.Location() + "`"
	if repoURL != "" {
		link := fmt.Sprintf("%s/blob/%s/%s", repoURL, s.Commit, d.File)
		if d.StartLine > 0 {
			link += fmt.Sprintf("#L%d-L%d", d.StartLine, d.EndLine)
		}
		where = fmt.Sprintf("[%s](%s)", where, link)
	}
	fmt.Fprintf(&b, "**Onde:** %s\n", where)
	fmt.Fprintf(&b, "**Esforço estimado:** %s\n\n", Hours(d.EffortHours))
	fmt.Fprintf(&b, "---\n<sub>Mapeada com o Decrivo (%s) · %s</sub>\n", s.Scope(), s.Key(d))
	return b.String()
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
