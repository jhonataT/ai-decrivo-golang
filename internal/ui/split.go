package ui

import (
	"strings"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// splitRow é uma linha da visão lado a lado: antes à esquerda, depois à direita.
// Um lado nil vira uma célula vazia (linha que só existe do outro lado).
type splitRow struct {
	Hunk string // cabeçalho "@@ ... @@"; quando preenchido, Old e New são nil
	Old  *review.DiffLine
	New  *review.DiffLine

	// Destaque do que mudou dentro da linha; nil quando não há par ou não compensa.
	OldSegs, NewSegs []seg
}

// splitRows pareia cada bloco de linhas removidas com o bloco de adicionadas
// que vem logo depois, como o Git faz: a 1ª removida fica ao lado da 1ª
// adicionada, e assim por diante. Contexto aparece nos dois lados.
func splitRows(lines []review.DiffLine) []splitRow {
	var rows []splitRow
	var removed, added []*review.DiffLine

	flush := func() {
		for i := 0; i < max(len(removed), len(added)); i++ {
			var row splitRow
			if i < len(removed) {
				row.Old = removed[i]
			}
			if i < len(added) {
				row.New = added[i]
			}
			if row.Old != nil && row.New != nil {
				row.OldSegs, row.NewSegs = intraline(row.Old.Text, row.New.Text)
			}
			rows = append(rows, row)
		}
		removed, added = removed[:0], added[:0]
	}

	// &lines[i] aponta para o elemento do slice original, sem copiar a linha.
	for i := range lines {
		l := &lines[i]
		switch l.Kind {
		case "removed":
			if len(added) > 0 { // um novo bloco de remoção começa depois de adições
				flush()
			}
			removed = append(removed, l)
		case "added":
			added = append(added, l)
		case "hunk":
			flush()
			rows = append(rows, splitRow{Hunk: l.Text})
		default:
			flush()
			rows = append(rows, splitRow{Old: l, New: l})
		}
	}
	flush()
	return rows
}

// lineNo devolve o número da linha no lado pedido (vazio se não houver).
func lineNo(l *review.DiffLine, old bool) string {
	if old {
		return num(l.OldLine)
	}
	return num(l.NewLine)
}

// span é um pedaço de texto de um comentário; Code marca trechos entre crases.
type span struct {
	Text string
	Code bool
}

// codeSpans separa `trechos de código` do texto comum, como no markdown.
// Com número ímpar de crases, a última fica como texto normal.
func codeSpans(s string) []span {
	parts := strings.Split(s, "`")
	if len(parts)%2 == 0 {
		last := len(parts) - 1
		parts[last-1] += "`" + parts[last]
		parts = parts[:last]
	}
	out := make([]span, 0, len(parts))
	for i, p := range parts {
		if p != "" {
			out = append(out, span{Text: p, Code: i%2 == 1})
		}
	}
	return out
}

// fileStats conta as linhas adicionadas e removidas de um arquivo.
func fileStats(f review.FileChange) (adds, dels int) {
	for _, l := range f.Lines {
		switch l.Kind {
		case "added":
			adds++
		case "removed":
			dels++
		}
	}
	return adds, dels
}
