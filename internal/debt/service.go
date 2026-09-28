package debt

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// Source é o acesso ao git de que o mapeamento precisa.
type Source interface {
	Head(ctx context.Context, repoPath, ref string) (string, error)
	ChangedFiles(ctx context.Context, repoPath, base, branch string) ([]string, error)
	ReadFile(ctx context.Context, repoPath, commit, path string) (string, error)
}

type Store interface {
	Save(s Scan) error
	LoadAll() ([]Scan, error)
}

// snippetContext é quantas linhas aparecem antes e depois do trecho;
// snippetMax limita o tamanho do trecho guardado.
const (
	snippetContext = 2
	snippetMax     = 40
)

// Service segue o mesmo desenho do review.Service: cópia, salva, troca.
type Service struct {
	git Source
	db  Store // pode ser nil: aí nada é persistido (útil em testes)

	mu      sync.RWMutex
	scans   map[string]*Scan
	seq     int
	current string
	notify  func(scanID string)
}

func NewService(git Source, db Store) *Service {
	return &Service{git: git, db: db, scans: map[string]*Scan{}}
}

func (s *Service) OnChange(fn func(string)) { s.notify = fn }

// Load carrega os mapeamentos salvos; um mapeamento que estava rodando
// quando o app fechou vira "interrompido" e libera os vereditos.
func (s *Service) Load() error {
	if s.db == nil {
		return nil
	}
	scans, loadErr := s.db.LoadAll()

	s.mu.Lock()
	defer s.mu.Unlock()
	var newest time.Time
	for i := range scans {
		sc := &scans[i]
		if sc.Status == review.StatusRunning {
			sc.Status = review.StatusReady
			sc.Interrupted = true
		}
		s.scans[sc.ID] = sc
		if n, err := strconv.Atoi(strings.TrimPrefix(sc.ID, "debt-")); err == nil {
			s.seq = max(s.seq, n)
		}
		if sc.CreatedAt.After(newest) {
			newest, s.current = sc.CreatedAt, sc.ID
		}
	}
	return loadErr
}

func (s *Service) update(scanID string, fn func(sc *Scan) error) (Scan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.scans[scanID]
	if !ok {
		return Scan{}, review.ErrNotFound
	}
	next := cur.snapshot()
	if err := fn(&next); err != nil {
		return Scan{}, err
	}
	next.UpdatedAt = time.Now()
	if err := s.save(next); err != nil {
		return Scan{}, err
	}
	s.scans[scanID] = &next
	return next.snapshot(), nil
}

func (s *Service) save(sc Scan) error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Save(sc); err != nil {
		return fmt.Errorf("salvar mapeamento %s: %w", sc.ID, err)
	}
	return nil
}

type StartInput struct {
	Repo     string
	Base     string
	Branch   string // vazio: projeto inteiro
	Labels   []Label
	Projects []Project
}

func (s *Service) Start(ctx context.Context, in StartInput) (*Scan, error) {
	in.Base, in.Branch = strings.TrimSpace(in.Base), strings.TrimSpace(in.Branch)
	if in.Base == "" {
		return nil, fmt.Errorf("informe a branch base")
	}
	ref := in.Base
	if in.Branch != "" {
		ref = in.Branch
	}
	commit, err := s.git.Head(ctx, in.Repo, ref)
	if err != nil {
		return nil, err
	}
	var files []string
	if in.Branch != "" {
		files, err = s.git.ChangedFiles(ctx, in.Repo, in.Base, commit)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("nenhuma alteração entre %s e %s", in.Base, in.Branch)
		}
	}

	s.mu.Lock()
	now := time.Now()
	sc := &Scan{
		ID:   fmt.Sprintf("debt-%d", s.seq+1),
		Repo: in.Repo, Base: in.Base, Branch: in.Branch,
		Commit:    commit,
		Files:     files,
		Status:    review.StatusRunning,
		Labels:    in.Labels,
		Projects:  in.Projects,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.save(*sc); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.seq++
	s.scans[sc.ID] = sc
	s.current = sc.ID
	s.mu.Unlock()

	s.fire(sc.ID)
	return sc, nil
}

// AddDebt valida a dívida contra o commit mapeado e guarda o trecho de código,
// para a tela e o histórico não dependerem do repositório depois.
func (s *Service) AddDebt(ctx context.Context, scanID string, d Debt) (string, error) {
	sc, err := s.Get(scanID)
	if err != nil {
		return "", err
	}
	if !sc.inScope(d.File) {
		return "", fmt.Errorf("%w: %s", ErrFileNotInScope, d.File)
	}
	content, err := s.git.ReadFile(ctx, sc.Repo, sc.Commit, d.File)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrFileNotFound, d.File)
	}
	if err := setRange(&d, content); err != nil {
		return "", err
	}

	sc, err = s.update(scanID, func(sc *Scan) error {
		if sc.Status != review.StatusRunning {
			return fmt.Errorf("%w (status: %s)", review.ErrNotRunning, sc.Status)
		}
		t, err := sc.validate(trimText(d.Text))
		if err != nil {
			return err
		}
		d.Text = t
		d.ID = fmt.Sprintf("d%d", len(sc.Debts)+1)
		d.Verdict = review.VerdictPending
		d.Adjusted, d.Orig, d.IssueURL = false, nil, ""
		sc.Debts = append(sc.Debts, d)
		return nil
	})
	if err != nil {
		return "", err
	}
	s.fire(scanID)
	return sc.Debts[len(sc.Debts)-1].ID, nil
}

// setRange confere as linhas contra o arquivo e recorta o trecho.
func setRange(d *Debt, content string) error {
	d.Snippet = nil
	if d.StartLine == 0 && d.EndLine == 0 {
		return nil
	}
	if d.EndLine == 0 {
		d.EndLine = d.StartLine
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if d.StartLine < 1 || d.EndLine < d.StartLine || d.EndLine > len(lines) {
		return fmt.Errorf("%w: %s:%d-%d (o arquivo tem %d linhas)", ErrLineOutOfRange, d.File, d.StartLine, d.EndLine, len(lines))
	}
	from := max(d.StartLine-snippetContext, 1)
	to := min(d.EndLine+snippetContext, len(lines), from+snippetMax-1)
	for n := from; n <= to; n++ {
		d.Snippet = append(d.Snippet, Line{N: n, Text: strings.TrimSuffix(lines[n-1], "\r")})
	}
	return nil
}

func trimText(t Text) Text {
	t.Title = strings.TrimSpace(t.Title)
	t.Description = strings.TrimSpace(t.Description)
	t.Reason = strings.TrimSpace(t.Reason)
	return t
}

// MarkReady encerra a fase do agente: o mapeamento passa a aguardar os vereditos.
func (s *Service) MarkReady(scanID, summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ErrEmptySummary
	}
	_, err := s.update(scanID, func(sc *Scan) error {
		if sc.Status != review.StatusRunning {
			return fmt.Errorf("%w (status: %s)", review.ErrNotRunning, sc.Status)
		}
		sc.Status = review.StatusReady
		sc.Summary = summary
		return nil
	})
	if err == nil {
		s.fire(scanID)
	}
	return err
}

func (s *Service) SetVerdict(scanID, debtID string, v review.Verdict) error {
	_, err := s.update(scanID, func(sc *Scan) error {
		if sc.Status != review.StatusReady {
			return fmt.Errorf("%w (status: %s)", review.ErrNotReady, sc.Status)
		}
		d := sc.debt(debtID)
		if d == nil {
			return review.ErrNotFound
		}
		// Desfazer um ajuste devolve o texto original do agente.
		if v == review.VerdictPending && d.Adjusted {
			d.Text = *d.Orig
			d.Adjusted, d.Orig = false, nil
		}
		d.Verdict = v
		return nil
	})
	return err
}

// Adjust troca o texto do agente pelo de quem revisa e aceita a dívida.
func (s *Service) Adjust(scanID, debtID string, t Text) error {
	_, err := s.update(scanID, func(sc *Scan) error {
		if sc.Status != review.StatusReady {
			return fmt.Errorf("%w (status: %s)", review.ErrNotReady, sc.Status)
		}
		d := sc.debt(debtID)
		if d == nil {
			return review.ErrNotFound
		}
		t, err := sc.validate(trimText(t))
		if err != nil {
			return err
		}
		// Num segundo ajuste, o original continua sendo o texto do agente.
		if !d.Adjusted {
			orig := d.Text
			orig.Labels = slices.Clone(d.Labels)
			d.Orig = &orig
		}
		d.Text = t
		d.Adjusted = true
		d.Verdict = review.VerdictAccepted
		return nil
	})
	return err
}

// Finalize fecha o mapeamento. project é o título de um dos projetos
// encontrados (ou vazio); publish libera o agente a criar as issues.
func (s *Service) Finalize(scanID, project string, publish bool) (Scan, error) {
	sc, err := s.update(scanID, func(sc *Scan) error { return sc.Finalize(project, publish) })
	if err != nil {
		return Scan{}, err
	}
	s.fire(scanID)
	return sc, nil
}

// MarkPublished grava a issue criada para a dívida. É por dívida porque as
// issues são criadas uma a uma: se o agente parar no meio, a próxima
// execução só cria as que faltam.
func (s *Service) MarkPublished(scanID, debtID, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return ErrEmptyIssueURL
	}
	_, err := s.update(scanID, func(sc *Scan) error {
		if err := sc.CanPublish(); err != nil {
			return err
		}
		d := sc.debt(debtID)
		switch {
		case d == nil:
			return review.ErrNotFound
		case d.Verdict != review.VerdictAccepted:
			return fmt.Errorf("%w: %s", ErrNotAccepted, debtID)
		case d.IssueURL != "":
			return fmt.Errorf("%w: %s", ErrAlreadyIssued, d.IssueURL)
		}
		d.IssueURL = url
		return nil
	})
	if err == nil {
		s.fire(scanID)
	}
	return err
}

func (s *Service) Get(scanID string) (Scan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc, ok := s.scans[scanID]
	if !ok {
		return Scan{}, review.ErrNotFound
	}
	return sc.snapshot(), nil
}

func (s *Service) Current() (Scan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc, ok := s.scans[s.current]
	if !ok {
		return Scan{}, false
	}
	return sc.snapshot(), true
}

// List devolve cópias de todos os mapeamentos, do mais recente para o mais antigo.
func (s *Service) List() []Scan {
	s.mu.RLock()
	out := make([]Scan, 0, len(s.scans))
	for _, sc := range s.scans {
		out = append(out, sc.snapshot())
	}
	s.mu.RUnlock()

	slices.SortFunc(out, func(a, b Scan) int { return cmp.Compare(b.CreatedAt.UnixNano(), a.CreatedAt.UnixNano()) })
	return out
}

// snapshot copia o Scan. Debts é clonado, e as labels de cada dívida também,
// porque o ajuste troca o slice inteiro; Snippet nunca muda e é compartilhado.
func (sc *Scan) snapshot() Scan {
	c := *sc
	c.Debts = slices.Clone(sc.Debts)
	for i := range c.Debts {
		c.Debts[i].Labels = slices.Clone(c.Debts[i].Labels)
	}
	return c
}

func (s *Service) fire(id string) {
	if s.notify != nil {
		s.notify(id)
	}
}
