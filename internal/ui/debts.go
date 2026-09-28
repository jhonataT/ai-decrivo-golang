package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jhonataT/ai-decrivo-golang/internal/debt"
	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

func (h *Handler) routeDebts() {
	h.mux.HandleFunc("GET /ui/debts", h.debtsHome)
	h.mux.HandleFunc("GET /ui/debts/current", h.debtsCurrent)
	h.mux.HandleFunc("GET /ui/debts/history", h.debtsHistory)
	h.mux.HandleFunc("GET /ui/debts/{id}", h.scanPage)
	h.mux.HandleFunc("GET /ui/debts/{id}/summary", h.scanSummary)
	h.mux.HandleFunc("POST /ui/debts/{id}/finalize", h.scanFinalize)
	h.mux.HandleFunc("POST /ui/debts/{id}/items/{did}/accept", h.debtVerdict(review.VerdictAccepted))
	h.mux.HandleFunc("POST /ui/debts/{id}/items/{did}/reject", h.debtVerdict(review.VerdictRejected))
	h.mux.HandleFunc("POST /ui/debts/{id}/items/{did}/reset", h.debtVerdict(review.VerdictPending))
	h.mux.HandleFunc("GET /ui/debts/{id}/items/{did}", h.debtItem)
	h.mux.HandleFunc("GET /ui/debts/{id}/items/{did}/edit", h.debtEdit)
	h.mux.HandleFunc("POST /ui/debts/{id}/items/{did}/adjust", h.debtAdjust)
}

func (h *Handler) debtsHome(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.debts.Current()
	if !ok {
		render(w, r, DebtsHome(nil))
		return
	}
	render(w, r, DebtsHome(&sc))
}

// debtsCurrent responde ao polling do DebtsEmpty: continua vazio até surgir um mapeamento.
func (h *Handler) debtsCurrent(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.debts.Current()
	if !ok {
		render(w, r, DebtsEmpty())
		return
	}
	render(w, r, ScanPage(&sc))
}

func (h *Handler) debtsHistory(w http.ResponseWriter, r *http.Request) {
	render(w, r, ScanHistory(h.debts.List()))
}

// scanPage também é o polling da ScanBar enquanto o agente mapeia.
func (h *Handler) scanPage(w http.ResponseWriter, r *http.Request) {
	sc, err := h.debts.Get(r.PathValue("id"))
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, ScanPage(&sc))
}

func (h *Handler) scanSummary(w http.ResponseWriter, r *http.Request) {
	sc, err := h.debts.Get(r.PathValue("id"))
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, ScanSummary(&sc))
}

func (h *Handler) scanFinalize(w http.ResponseWriter, r *http.Request) {
	sc, err := h.debts.Finalize(r.PathValue("id"), r.FormValue("project"), r.FormValue("publish") != "")
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, ScanSummary(&sc))
}

func (h *Handler) debtVerdict(v review.Verdict) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.debts.SetVerdict(r.PathValue("id"), r.PathValue("did"), v); err != nil {
			render(w, r, ErrorBox(err.Error()))
			return
		}
		h.debtItem(w, r)
	}
}

// debtItem devolve o card e, fora do alvo (oob), a barra e a lista de dívidas.
func (h *Handler) debtItem(w http.ResponseWriter, r *http.Request) {
	sc, d, err := h.loadDebt(r)
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, DebtCard(&sc, d))
	render(w, r, ScanBar(&sc, true))
	render(w, r, DebtNav(&sc, true))
}

func (h *Handler) debtEdit(w http.ResponseWriter, r *http.Request) {
	sc, d, err := h.loadDebt(r)
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, DebtEdit(&sc, d, d.Text, ""))
}

func (h *Handler) debtAdjust(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	t := debt.Text{
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		Reason:      r.FormValue("reason"),
		Labels:      r.Form["labels"],
	}
	// Aceita "1,5" além de "1.5".
	effort, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.FormValue("effort")), ",", "."), 64)
	t.EffortHours = effort
	if err != nil {
		err = debt.ErrInvalidEffort
	} else {
		err = h.debts.Adjust(r.PathValue("id"), r.PathValue("did"), t)
	}
	if err == nil {
		h.debtItem(w, r)
		return
	}
	// Erro de validação: reabre o formulário com o que foi digitado.
	sc, d, loadErr := h.loadDebt(r)
	if loadErr != nil {
		render(w, r, ErrorBox(loadErr.Error()))
		return
	}
	render(w, r, DebtEdit(&sc, d, t, err.Error()))
}

func (h *Handler) loadDebt(r *http.Request) (debt.Scan, debt.Debt, error) {
	sc, err := h.debts.Get(r.PathValue("id"))
	if err != nil {
		return debt.Scan{}, debt.Debt{}, err
	}
	d, ok := sc.Debt(r.PathValue("did"))
	if !ok {
		return debt.Scan{}, debt.Debt{}, review.ErrNotFound
	}
	return sc, d, nil
}
