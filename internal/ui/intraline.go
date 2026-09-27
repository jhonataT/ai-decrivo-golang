package ui

import (
	"strings"
	"unicode"
)

// seg é um pedaço de uma linha de código; Changed marca o que mudou em
// relação à linha pareada do outro lado.
type seg struct {
	Text    string
	Changed bool
}

// Limites para não destacar quando não compensa: linhas enormes (a tabela
// do LCS cresce com n*m) ou tão diferentes que o destaque vira ruído.
const (
	maxLCSCells   = 40_000
	minSimilarity = 0.4
)

// intraline compara duas linhas palavra a palavra e devolve os segmentos de
// cada lado. Devolve nil, nil quando o destaque não ajuda.
func intraline(before, after string) (oldSegs, newSegs []seg) {
	a, b := tokenize(before), tokenize(after)
	if len(a) == 0 || len(b) == 0 || len(a)*len(b) > maxLCSCells {
		return nil, nil
	}

	keepA, keepB, common := lcs(a, b)
	if float64(2*common)/float64(len(a)+len(b)) < minSimilarity {
		return nil, nil
	}
	return segments(a, keepA), segments(b, keepB)
}

// tokenize quebra a linha em palavras (letras, dígitos e _), blocos de
// espaço e símbolos soltos: "a.b(c)" vira ["a", ".", "b", "(", "c", ")"].
func tokenize(s string) []string {
	var toks []string
	runes := []rune(s)
	for i := 0; i < len(runes); {
		j := i + 1
		switch {
		case isWord(runes[i]):
			for j < len(runes) && isWord(runes[j]) {
				j++
			}
		case unicode.IsSpace(runes[i]):
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
		}
		toks = append(toks, string(runes[i:j]))
		i = j
	}
	return toks
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// lcs calcula a maior subsequência comum entre a e b e marca, em cada lado,
// quais tokens fazem parte dela (keep = não mudou).
func lcs(a, b []string) (keepA, keepB []bool, common int) {
	n, m := len(a), len(b)
	// dp[i][j] = tamanho da LCS entre a[i:] e b[j:]
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	keepA, keepB = make([]bool, n), make([]bool, m)
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			keepA[i], keepB[j] = true, true
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return keepA, keepB, dp[0][0]
}

// segments junta tokens vizinhos com o mesmo estado. Um espaço entre dois
// trechos alterados entra no destaque, para "int // centavos" virar um
// bloco só em vez de três.
func segments(toks []string, keep []bool) []seg {
	changed := make([]bool, len(toks))
	for i := range toks {
		changed[i] = !keep[i]
	}
	for i := 1; i < len(toks)-1; i++ {
		if strings.TrimSpace(toks[i]) == "" && changed[i-1] && changed[i+1] {
			changed[i] = true
		}
	}

	var out []seg
	for i, t := range toks {
		if n := len(out); n > 0 && out[n-1].Changed == changed[i] {
			out[n-1].Text += t
			continue
		}
		out = append(out, seg{Text: t, Changed: changed[i]})
	}
	return out
}

// OnlySpace diz se o trecho alterado é só espaço (ex.: realinhamento do gofmt).
func (s seg) OnlySpace() bool { return s.Changed && strings.TrimSpace(s.Text) == "" }
