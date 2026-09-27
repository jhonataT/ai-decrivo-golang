package review

import (
	"regexp"
	"strconv"
	"strings"
)

type DiffLine struct {
	Kind    string // context | added | removed | hunk
	OldLine int
	NewLine int
	Text    string
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func ParsePatch(patch string) []DiffLine {
	var out []DiffLine
	oldN, newN := 0, 0
	// Tudo antes do primeiro "@@" é cabeçalho do git (diff, index, ---, +++,
	// new file mode, rename...). Depois dele, "---" pode ser uma linha
	// removida que começa com "--", então não dá para filtrar por prefixo.
	inHunk := false

	// Sem o TrimSuffix, o "\n" final do patch viraria uma linha de contexto vazia.
	for _, raw := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		switch {
		case strings.HasPrefix(raw, "@@"):
			if m := hunkRe.FindStringSubmatch(raw); m != nil {
				oldN, _ = strconv.Atoi(m[1])
				newN, _ = strconv.Atoi(m[2])
			}
			inHunk = true
			out = append(out, DiffLine{Kind: "hunk", Text: raw})
		case !inHunk, strings.HasPrefix(raw, `\`): // "\ No newline at end of file"
			continue
		case strings.HasPrefix(raw, "+"):
			out = append(out, DiffLine{Kind: "added", NewLine: newN, Text: raw[1:]})
			newN++
		case strings.HasPrefix(raw, "-"):
			out = append(out, DiffLine{Kind: "removed", OldLine: oldN, Text: raw[1:]})
			oldN++
		default:
			text := strings.TrimPrefix(raw, " ")
			out = append(out, DiffLine{Kind: "context", OldLine: oldN, NewLine: newN, Text: text})
			oldN++
			newN++
		}
	}
	return out
}

// File devolve o arquivo alterado com esse caminho, se ele estiver no diff.
func (r *Review) File(path string) (FileChange, bool) {
	for _, f := range r.Files {
		if f.Path == path {
			return f, true
		}
	}
	return FileChange{}, false
}

func (r *Review) hasLine(file string, line int) bool {
	for _, f := range r.Files {
		if f.Path != file {
			continue
		}
		for _, l := range f.Lines {
			if l.NewLine == line && (l.Kind == "added" || l.Kind == "context") {
				return true
			}
		}
	}
	return false
}
