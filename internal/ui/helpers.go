package ui

import (
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// num devolve string vazia para 0, para não poluir a coluna de números.
func num(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func itoa(n int) string { return strconv.Itoa(n) }

// byLine agrupa os achados de um arquivo pela linha do arquivo novo.
func byLine(r *review.Review, file string) map[int][]review.Finding {
	m := map[int][]review.Finding{}
	for _, f := range r.Findings {
		if f.File == file && f.Line > 0 {
			m[f.Line] = append(m[f.Line], f)
		}
	}
	return m
}

// fileLevel são os achados sem linha específica (Line == 0).
func fileLevel(r *review.Review, file string) []review.Finding {
	var out []review.Finding
	for _, f := range r.Findings {
		if f.File == file && f.Line == 0 {
			out = append(out, f)
		}
	}
	return out
}

// fileCounts devolve quantos achados do arquivo estão pendentes e o total.
func fileCounts(r *review.Review, file string) (pending, total int) {
	for _, f := range r.Findings {
		if f.File != file {
			continue
		}
		total++
		if f.Verdict == review.VerdictPending {
			pending++
		}
	}
	return pending, total
}

type tally struct{ Accepted, Adjusted, Rejected, Judged int }

func countVerdicts(r *review.Review) tally {
	var t tally
	for _, f := range r.Findings {
		switch {
		case f.Verdict == review.VerdictRejected:
			t.Rejected++
		case f.Verdict == review.VerdictAccepted && f.Adjusted:
			t.Adjusted++
		case f.Verdict == review.VerdictAccepted:
			t.Accepted++
		}
	}
	t.Judged = t.Accepted + t.Adjusted + t.Rejected
	return t
}

// splitPath separa "internal/ui/pages.templ" em "internal/ui/" e "pages.templ".
func splitPath(p string) (dir, name string) {
	dir, name = path.Split(p)
	return dir, name
}

func fileAnchor(i int) string { return "file-" + strconv.Itoa(i) }

func findingURL(r *review.Review, f review.Finding) string {
	return "/ui/reviews/" + r.ID + "/findings/" + f.ID
}

// locked: vereditos só são aceitos com a revisão pronta (nem rodando, nem finalizada).
func locked(r *review.Review) bool { return r.Status != review.StatusReady }

func lockHint(r *review.Review) string {
	if r.Status == review.StatusFinalized {
		return "revisão finalizada"
	}
	return "disponível quando o agente terminar"
}

func recLabel(rec review.Recommendation) string {
	switch rec {
	case review.RecommendApprove:
		return "Aprovar"
	case review.RecommendRequestChanges:
		return "Solicitar alterações"
	case review.RecommendComment:
		return "Apenas comentar"
	}
	return string(rec)
}

var decisions = []review.Recommendation{review.RecommendApprove, review.RecommendComment, review.RecommendRequestChanges}

// defaultDecision parte da sugestão do agente; quem revisa pode trocar.
func defaultDecision(r *review.Review) review.Recommendation {
	if r.Recommendation.Valid() {
		return r.Recommendation
	}
	return review.RecommendComment
}

func recClass(rec review.Recommendation) string {
	if rec == "" {
		return "rec-none"
	}
	return "rec-" + string(rec)
}

func statusLabel(r *review.Review) string {
	switch {
	case r.Status == review.StatusFinalized:
		return "finalizada"
	case r.Status == review.StatusRunning:
		return "em análise"
	case r.Interrupted:
		return "interrompida"
	}
	return "aguardando vereditos"
}

// formatTime mostra a data no fuso local; vazio para data zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("02/01/2006 15:04")
}

// repoName mostra só o nome da pasta do repositório (funciona com / e \).
func repoName(p string) string { return filepath.Base(filepath.FromSlash(p)) }

func verdictClass(f review.Finding) string {
	if f.Adjusted {
		return "v-adjusted"
	}
	return "v-" + string(f.Verdict)
}

func verdictLabel(f review.Finding) string {
	switch {
	case f.Adjusted:
		return "ajustado e aprovado"
	case f.Verdict == review.VerdictAccepted:
		return "aprovado"
	case f.Verdict == review.VerdictRejected:
		return "retirado"
	}
	return "pendente"
}

func kindLabel(k string) string {
	switch k {
	case "praise":
		return "elogio"
	case "improvement":
		return "melhoria"
	}
	return k
}

func severityLabel(s string) string {
	switch s {
	case "major":
		return "importante"
	case "minor":
		return "menor"
	case "info":
		return "info"
	}
	return s
}

func finalizeHint(r *review.Review) string {
	if r.Status == review.StatusRunning {
		return "aguardando o agente terminar"
	}
	if r.Pending() > 0 {
		return "julgue todos os achados antes de finalizar"
	}
	return "gerar o resumo"
}

// countFiles conta os arquivos da revisão que satisfazem match.
func countFiles(r *review.Review, match func(string) bool) int {
	n := 0
	for _, f := range r.Files {
		if match(f.Path) {
			n++
		}
	}
	return n
}
