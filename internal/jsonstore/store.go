// Package jsonstore guarda cada revisão num arquivo JSON separado, servindo
// de banco local. Implementa review.Store.
package jsonstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// schemaVersion permite migrar o formato no futuro sem perder o histórico.
const schemaVersion = 1

type file struct {
	Version int           `json:"version"`
	Review  review.Review `json:"review"`
}

type Store struct {
	dir string
}

var _ review.Store = (*Store)(nil)

// DefaultDir é %APPDATA%\Decrivo\reviews no Windows (equivalentes no macOS/Linux).
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("pasta de configuração do usuário: %w", err)
	}
	return filepath.Join(base, "Decrivo", "reviews"), nil
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("criar %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

// O ID vira nome de arquivo: aceita só caracteres seguros para não escapar da pasta.
var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Save grava num arquivo temporário e depois renomeia por cima do definitivo.
// O rename é atômico: se o app cair no meio, fica o arquivo antigo inteiro,
// nunca um JSON cortado pela metade.
func (s *Store) Save(r review.Review) error {
	if !safeID.MatchString(r.ID) {
		return fmt.Errorf("id de revisão inválido para arquivo: %q", r.ID)
	}
	data, err := json.MarshalIndent(file{Version: schemaVersion, Review: r}, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, r.ID+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // sem efeito se o rename deu certo

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.dir, r.ID+".json"))
}

// LoadAll lê todos os arquivos da pasta. Um arquivo corrompido não impede os
// outros de carregar: os erros são juntados com errors.Join e devolvidos
// junto com o que deu para ler.
func (s *Store) LoadAll() ([]review.Review, error) {
	paths, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}

	var revs []review.Review
	var errs []error
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var f file
		if err := json.Unmarshal(data, &f); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(p), err))
			continue
		}
		if f.Version != schemaVersion {
			errs = append(errs, fmt.Errorf("%s: versão %d não suportada", filepath.Base(p), f.Version))
			continue
		}
		revs = append(revs, f.Review)
	}
	return revs, errors.Join(errs...)
}
