// Package jsonstore guarda cada item (revisão de PR, mapeamento de dívidas)
// num arquivo JSON separado, servindo de banco local. Implementa review.Store
// e debt.Store.
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

// Store guarda itens do tipo T. No arquivo, o item fica sob a chave key
// ({"version": 1, "review": {...}}), ao lado da versão do schema.
type Store[T any] struct {
	dir string
	key string
	id  func(T) string
}

var _ review.Store = (*Store[review.Review])(nil)

// DefaultDir é %APPDATA%\Decrivo\<name> no Windows (equivalentes no macOS/Linux).
func DefaultDir(name string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("pasta de configuração do usuário: %w", err)
	}
	return filepath.Join(base, "Decrivo", name), nil
}

// New cria o store na pasta dir; id devolve o ID do item, que vira o nome do arquivo.
func New[T any](dir, key string, id func(T) string) (*Store[T], error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("criar %s: %w", dir, err)
	}
	return &Store[T]{dir: dir, key: key, id: id}, nil
}

// Reviews é o store das revisões de PR.
func Reviews(dir string) (*Store[review.Review], error) {
	return New(dir, "review", func(r review.Review) string { return r.ID })
}

func (s *Store[T]) Dir() string { return s.dir }

// O ID vira nome de arquivo: aceita só caracteres seguros para não escapar da pasta.
var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Save grava num arquivo temporário e depois renomeia por cima do definitivo.
// O rename é atômico: se o app cair no meio, fica o arquivo antigo inteiro,
// nunca um JSON cortado pela metade.
func (s *Store[T]) Save(item T) error {
	id := s.id(item)
	if !safeID.MatchString(id) {
		return fmt.Errorf("id inválido para arquivo: %q", id)
	}
	data, err := json.MarshalIndent(map[string]any{"version": schemaVersion, s.key: item}, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, id+".*.tmp")
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
	return os.Rename(tmp.Name(), filepath.Join(s.dir, id+".json"))
}

// LoadAll lê todos os arquivos da pasta. Um arquivo corrompido não impede os
// outros de carregar: os erros são juntados com errors.Join e devolvidos
// junto com o que deu para ler.
func (s *Store[T]) LoadAll() ([]T, error) {
	paths, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}

	var items []T
	var errs []error
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		item, err := s.decode(data)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(p), err))
			continue
		}
		items = append(items, item)
	}
	return items, errors.Join(errs...)
}

func (s *Store[T]) decode(data []byte) (T, error) {
	var item T
	var f map[string]json.RawMessage
	if err := json.Unmarshal(data, &f); err != nil {
		return item, err
	}
	var version int
	if err := json.Unmarshal(f["version"], &version); err != nil {
		return item, fmt.Errorf("versão: %w", err)
	}
	if version != schemaVersion {
		return item, fmt.Errorf("versão %d não suportada", version)
	}
	raw, ok := f[s.key]
	if !ok {
		return item, fmt.Errorf("chave %q ausente", s.key)
	}
	err := json.Unmarshal(raw, &item)
	return item, err
}
