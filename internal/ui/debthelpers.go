package ui

import (
	"slices"
	"strconv"

	"github.com/jhonataT/ai-decrivo-golang/internal/debt"
	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

func countDebtVerdicts(sc *debt.Scan) tally {
	var t tally
	for _, d := range sc.Debts {
		switch {
		case d.Verdict == review.VerdictRejected:
			t.Rejected++
		case d.Verdict == review.VerdictAccepted && d.Adjusted:
			t.Adjusted++
		case d.Verdict == review.VerdictAccepted:
			t.Accepted++
		}
	}
	t.Judged = t.Accepted + t.Adjusted + t.Rejected
	return t
}

func debtVerdictClass(d debt.Debt) string {
	if d.Adjusted {
		return "v-adjusted"
	}
	return "v-" + string(d.Verdict)
}

func debtVerdictLabel(d debt.Debt) string {
	switch {
	case d.Adjusted:
		return "editada e aceita"
	case d.Verdict == review.VerdictAccepted:
		return "aceita"
	case d.Verdict == review.VerdictRejected:
		return "recusada"
	}
	return "pendente"
}

func debtURL(sc *debt.Scan, d debt.Debt) string {
	return "/ui/debts/" + sc.ID + "/items/" + d.ID
}

// scanLocked: vereditos só são aceitos com o mapeamento pronto.
func scanLocked(sc *debt.Scan) bool { return sc.Status != review.StatusReady }

func scanLockHint(sc *debt.Scan) string {
	if sc.Status == review.StatusFinalized {
		return "mapeamento finalizado"
	}
	return "disponível quando o agente terminar"
}

func scanFinalizeHint(sc *debt.Scan) string {
	if sc.Status == review.StatusRunning {
		return "aguardando o agente terminar"
	}
	if sc.Pending() > 0 {
		return "julgue todas as dívidas antes de finalizar"
	}
	return "fechar o mapeamento"
}

func scanStatusLabel(sc *debt.Scan) string {
	switch {
	case sc.Status == review.StatusFinalized:
		return "finalizado"
	case sc.Status == review.StatusRunning:
		return "mapeando"
	case sc.Interrupted:
		return "interrompido"
	}
	return "aguardando vereditos"
}

// issueCounts devolve quantas dívidas aceitas já têm issue e quantas são aceitas.
func issueCounts(sc *debt.Scan) (done, total int) {
	for _, d := range sc.Debts {
		if d.Verdict != review.VerdictAccepted {
			continue
		}
		total++
		if d.IssueURL != "" {
			done++
		}
	}
	return done, total
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func hasString(list []string, s string) bool { return slices.Contains(list, s) }
