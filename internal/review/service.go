package review

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DiffSource interface {
	// Head resolve a branch para o SHA do commit.
	Head(ctx context.Context, repoPath, branch string) (string, error)
	Changes(ctx context.Context, repoPath, base, branch string) ([]FileChange, error)
}

// Store guarda as revisões fora da memória. O Service não sabe se é JSON,
// banco ou outra coisa: só depende deste contrato.
type Store interface {
	Save(r Review) error
	LoadAll() ([]Review, error)
}

type Service struct {
	diffs DiffSource
	db    Store // pode ser nil: aí nada é persistido (útil em testes)

	mu      sync.RWMutex
	reviews map[string]*Review
	seq     int
	current string // ID da revisão mais recente
	notify  func(reviewID string)
}

func NewService(d DiffSource, db Store) *Service {
	return &Service{diffs: d, db: db, reviews: map[string]*Review{}}
}

func (s *Service) OnChange(fn func(string)) { s.notify = fn }

// Load carrega as revisões salvas. Arquivos com problema não impedem os
// outros de carregar: o erro devolvido junta todos (errors.Join).
func (s *Service) Load() error {
	if s.db == nil {
		return nil
	}
	revs, loadErr := s.db.LoadAll()

	s.mu.Lock()
	defer s.mu.Unlock()
	var newest time.Time
	for i := range revs {
		rev := &revs[i]
		for j := range rev.Files {
			rev.Files[j].Lines = ParsePatch(rev.Files[j].Patch)
		}
		// O app fechou no meio da análise: o agente não vai voltar para
		// terminar, então libera os vereditos sobre o que já chegou.
		if rev.Status == StatusRunning {
			rev.Status = StatusReady
			rev.Interrupted = true
		}
		s.reviews[rev.ID] = rev
		if n, err := strconv.Atoi(strings.TrimPrefix(rev.ID, "rev-")); err == nil {
			s.seq = max(s.seq, n)
		}
		if rev.CreatedAt.After(newest) {
			newest, s.current = rev.CreatedAt, rev.ID
		}
	}
	return loadErr
}

// update aplica fn numa CÓPIA da revisão, salva a cópia e só então troca a
// original. Se fn ou o salvamento falharem, a memória fica como estava: o
// que a tela mostra nunca diverge do que está no disco (copy-on-write).
func (s *Service) update(reviewID string, fn func(r *Review) error) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.reviews[reviewID]
	if !ok {
		return Review{}, ErrNotFound
	}
	next := cur.snapshot()
	if err := fn(&next); err != nil {
		return Review{}, err
	}
	next.UpdatedAt = time.Now()
	if err := s.save(next); err != nil {
		return Review{}, err
	}
	s.reviews[reviewID] = &next
	return next.snapshot(), nil
}

// save roda com o lock seguro: assim dois salvamentos da mesma revisão nunca
// terminam fora de ordem (o mais antigo sobrescrevendo o mais novo).
func (s *Service) save(r Review) error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Save(r); err != nil {
		return fmt.Errorf("salvar revisão %s: %w", r.ID, err)
	}
	return nil
}

func (s *Service) Start(ctx context.Context, repoPath, base, branch string) (*Review, error) {
	// O diff sai do SHA, não do nome da branch: se ela andar no meio do
	// caminho, os achados continuam batendo com o commit registrado.
	head, err := s.diffs.Head(ctx, repoPath, branch)
	if err != nil {
		return nil, err
	}
	files, err := s.diffs.Changes(ctx, repoPath, base, head)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("nenhuma alteração entre %s e %s", base, branch)
	}

	s.mu.Lock()
	now := time.Now()
	rev := &Review{
		ID:   fmt.Sprintf("rev-%d", s.seq+1),
		Repo: repoPath, Base: base, Branch: branch,
		HeadSHA:   head,
		Status:    StatusRunning,
		Files:     files,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.save(*rev); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.seq++
	s.reviews[rev.ID] = rev
	s.current = rev.ID
	s.mu.Unlock()

	s.fire(rev.ID)
	return rev, nil
}

func (s *Service) AddFinding(reviewID string, f Finding) (string, error) {
	_, err := s.update(reviewID, func(rev *Review) error {
		if rev.Status != StatusRunning {
			return fmt.Errorf("%w (status: %s)", ErrNotRunning, rev.Status)
		}
		if _, ok := rev.File(f.File); !ok {
			return fmt.Errorf("%w: %s", ErrFileNotInDiff, f.File)
		}
		if f.Line > 0 && !rev.hasLine(f.File, f.Line) {
			return fmt.Errorf("%w: %s:%d", ErrLineNotInDiff, f.File, f.Line)
		}
		f.ID = fmt.Sprintf("f%d", len(rev.Findings)+1)
		f.Verdict = VerdictPending
		rev.Findings = append(rev.Findings, f)
		return nil
	})
	if err != nil {
		return "", err
	}
	s.fire(reviewID)
	return f.ID, nil
}

// SetFileContext registra o que o arquivo faz no projeto.
func (s *Service) SetFileContext(reviewID, path, text string) error {
	text = strings.TrimSpace(text)
	_, err := s.update(reviewID, func(rev *Review) error {
		if rev.Status != StatusRunning {
			return fmt.Errorf("%w (status: %s)", ErrNotRunning, rev.Status)
		}
		for i := range rev.Files {
			if rev.Files[i].Path == path {
				rev.Files[i].Context = text
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrFileNotInDiff, path)
	})
	if err == nil {
		s.fire(reviewID)
	}
	return err
}

func (s *Service) SetVerdict(reviewID, findingID string, v Verdict) error {
	_, err := s.update(reviewID, func(rev *Review) error {
		if rev.Status != StatusReady {
			return fmt.Errorf("%w (status: %s)", ErrNotReady, rev.Status)
		}
		f := rev.finding(findingID)
		if f == nil {
			return ErrNotFound
		}
		// Desfazer um ajuste devolve o texto original do agente.
		if v == VerdictPending && f.Adjusted {
			f.Title, f.Body = f.OrigTitle, f.OrigBody
			f.Adjusted, f.OrigTitle, f.OrigBody = false, "", ""
		}
		f.Verdict = v
		return nil
	})
	return err
}

// Adjust troca o texto do agente pelo do revisor e aprova o achado.
func (s *Service) Adjust(reviewID, findingID, title, body string) error {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" || body == "" {
		return ErrEmptyText
	}
	_, err := s.update(reviewID, func(rev *Review) error {
		if rev.Status != StatusReady {
			return fmt.Errorf("%w (status: %s)", ErrNotReady, rev.Status)
		}
		f := rev.finding(findingID)
		if f == nil {
			return ErrNotFound
		}
		// Num segundo ajuste, o original continua sendo o texto do agente.
		if !f.Adjusted {
			f.OrigTitle, f.OrigBody = f.Title, f.Body
		}
		f.Title, f.Body = title, body
		f.Adjusted = true
		f.Verdict = VerdictAccepted
		return nil
	})
	return err
}

// Finish é o que o agente entrega ao terminar a análise.
type Finish struct {
	Summary        string
	Recommendation Recommendation
	Reason         string
}

// MarkReady encerra a fase do agente: a revisão passa a aguardar os vereditos.
func (s *Service) MarkReady(reviewID string, fin Finish) error {
	fin.Summary, fin.Reason = strings.TrimSpace(fin.Summary), strings.TrimSpace(fin.Reason)
	if fin.Summary == "" {
		return ErrEmptySummary
	}
	if !fin.Recommendation.Valid() {
		return fmt.Errorf("%w (recebido: %q)", ErrInvalidRecommendation, fin.Recommendation)
	}
	_, err := s.update(reviewID, func(rev *Review) error {
		if rev.Status != StatusRunning {
			return fmt.Errorf("%w (status: %s)", ErrNotRunning, rev.Status)
		}
		rev.Status = StatusReady
		rev.ChangeSummary = fin.Summary
		rev.Recommendation = fin.Recommendation
		rev.RecommendationReason = fin.Reason
		return nil
	})
	if err == nil {
		s.fire(reviewID)
	}
	return err
}

// Finalize fecha a revisão com o veredito geral de quem revisou e devolve o
// resumo em markdown. Com publish, o agente fica liberado a publicar no PR.
func (s *Service) Finalize(reviewID string, decision Recommendation, publish bool) (string, error) {
	rev, err := s.update(reviewID, func(rev *Review) error { return rev.Finalize(decision, publish) })
	if err != nil {
		return "", err
	}
	s.fire(reviewID)
	return rev.Summary(), nil
}

// MarkPublished registra que o agente publicou a revisão no PR, para que ela
// não seja publicada de novo.
func (s *Service) MarkPublished(reviewID, url string) error {
	url = strings.TrimSpace(url)
	_, err := s.update(reviewID, func(rev *Review) error {
		if err := rev.CanPublish(); err != nil {
			return err
		}
		rev.PublishedURL = url
		rev.PublishedAt = time.Now()
		return nil
	})
	if err == nil {
		s.fire(reviewID)
	}
	return err
}

// Get devolve uma cópia da revisão, segura para ler fora do lock.
func (s *Service) Get(reviewID string) (Review, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rev, ok := s.reviews[reviewID]
	if !ok {
		return Review{}, ErrNotFound
	}
	return rev.snapshot(), nil
}

// Current devolve uma cópia da revisão mais recente, se houver.
func (s *Service) Current() (Review, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rev, ok := s.reviews[s.current]
	if !ok {
		return Review{}, false
	}
	return rev.snapshot(), true
}

// List devolve cópias de todas as revisões, da mais recente para a mais antiga.
func (s *Service) List() []Review {
	s.mu.RLock()
	out := make([]Review, 0, len(s.reviews))
	for _, rev := range s.reviews {
		out = append(out, rev.snapshot())
	}
	s.mu.RUnlock()

	slices.SortFunc(out, func(a, b Review) int { return cmp.Compare(b.CreatedAt.UnixNano(), a.CreatedAt.UnixNano()) })
	return out
}

// snapshot copia a Review. Findings e Files são clonados porque as alterações
// mexem em seus elementos; Lines nunca muda depois de calculado e é compartilhado.
func (r *Review) snapshot() Review {
	c := *r
	c.Findings = slices.Clone(r.Findings)
	c.Files = slices.Clone(r.Files)
	return c
}

func (s *Service) fire(id string) {
	if s.notify != nil {
		s.notify(id)
	}
}

