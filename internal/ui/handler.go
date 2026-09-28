package ui

import (
	"log"
	"net/http"

	"github.com/a-h/templ"

	"github.com/jhonataT/ai-decrivo-golang/internal/debt"
	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

type Handler struct {
	mux   *http.ServeMux
	svc   *review.Service
	debts *debt.Service
}

func NewHandler(svc *review.Service, debts *debt.Service) http.Handler {
	h := &Handler{mux: http.NewServeMux(), svc: svc, debts: debts}
	h.mux.HandleFunc("GET /ui/home", h.home)
	h.mux.HandleFunc("GET /ui/current", h.current)
	h.mux.HandleFunc("GET /ui/reviews/{id}", h.reviewPage)
	h.mux.HandleFunc("GET /ui/reviews/{id}/status", h.status)
	h.mux.HandleFunc("POST /ui/reviews/{id}/findings/{fid}/accept", h.verdict(review.VerdictAccepted))
	h.mux.HandleFunc("POST /ui/reviews/{id}/findings/{fid}/reject", h.verdict(review.VerdictRejected))
	h.mux.HandleFunc("POST /ui/reviews/{id}/findings/{fid}/reset", h.verdict(review.VerdictPending))
	h.mux.HandleFunc("GET /ui/reviews/{id}/findings/{fid}", h.finding)
	h.mux.HandleFunc("GET /ui/reviews/{id}/findings/{fid}/edit", h.editFinding)
	h.mux.HandleFunc("POST /ui/reviews/{id}/findings/{fid}/adjust", h.adjust)
	h.mux.HandleFunc("POST /ui/reviews/{id}/finalize", h.finalize)
	h.mux.HandleFunc("GET /ui/reviews/{id}/summary", h.summary)
	h.mux.HandleFunc("GET /ui/history", h.history)
	h.routeDebts()
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("request:", r.Method, r.URL.Path)
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	rev, ok := h.svc.Current()
	if !ok {
		render(w, r, Home(nil))
		return
	}
	render(w, r, Home(&rev))
}

// current responde ao polling do Empty: continua vazio até surgir uma revisão.
func (h *Handler) current(w http.ResponseWriter, r *http.Request) {
	rev, ok := h.svc.Current()
	if !ok {
		render(w, r, Empty())
		return
	}
	render(w, r, ReviewPage(&rev))
}

func (h *Handler) reviewPage(w http.ResponseWriter, r *http.Request) {
	rev, err := h.svc.Get(r.PathValue("id"))
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, ReviewPage(&rev))
}

// status é o polling da StatusBar enquanto o agente roda; devolve a página inteira.
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	h.reviewPage(w, r)
}

// verdict gera um handler para cada veredito (aceitar, recusar, desfazer).
func (h *Handler) verdict(v review.Verdict) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, fid := r.PathValue("id"), r.PathValue("fid")
		if err := h.svc.SetVerdict(id, fid, v); err != nil {
			render(w, r, ErrorBox(err.Error()))
			return
		}
		h.finding(w, r)
	}
}

// finding devolve o card do achado e, fora do alvo (oob), a barra de status
// e a lista de arquivos, para o progresso refletir o veredito.
func (h *Handler) finding(w http.ResponseWriter, r *http.Request) {
	rev, f, err := h.load(r)
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, FindingCard(&rev, f))
	render(w, r, StatusBar(&rev, true))
	render(w, r, FileNav(&rev, true))
}

func (h *Handler) editFinding(w http.ResponseWriter, r *http.Request) {
	rev, f, err := h.load(r)
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, FindingEdit(&rev, f, f.Title, f.Body, ""))
}

func (h *Handler) adjust(w http.ResponseWriter, r *http.Request) {
	title, body := r.FormValue("title"), r.FormValue("body")
	err := h.svc.Adjust(r.PathValue("id"), r.PathValue("fid"), title, body)
	if err == nil {
		h.finding(w, r)
		return
	}
	// Erro de validação: reabre o formulário com o que foi digitado.
	rev, f, loadErr := h.load(r)
	if loadErr != nil {
		render(w, r, ErrorBox(loadErr.Error()))
		return
	}
	render(w, r, FindingEdit(&rev, f, title, body, err.Error()))
}

// load busca a revisão e o achado indicados na URL.
func (h *Handler) load(r *http.Request) (review.Review, review.Finding, error) {
	rev, err := h.svc.Get(r.PathValue("id"))
	if err != nil {
		return review.Review{}, review.Finding{}, err
	}
	f, ok := rev.Finding(r.PathValue("fid"))
	if !ok {
		return review.Review{}, review.Finding{}, review.ErrNotFound
	}
	return rev, f, nil
}

func (h *Handler) finalize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	decision := review.Recommendation(r.FormValue("decision"))
	md, err := h.svc.Finalize(id, decision, r.FormValue("publish") != "")
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	rev, err := h.svc.Get(id)
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, Summary(&rev, md))
}

// summary reabre o resumo de uma revisão finalizada (vinda do histórico).
func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	rev, err := h.svc.Get(r.PathValue("id"))
	if err != nil {
		render(w, r, ErrorBox(err.Error()))
		return
	}
	render(w, r, Summary(&rev, rev.Summary()))
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	render(w, r, History(h.svc.List()))
}

// render escreve o fragmento com status 200 (o HTMX ignora 4xx/5xx).
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	if err := c.Render(r.Context(), w); err != nil {
		log.Println("render:", err)
	}
}
