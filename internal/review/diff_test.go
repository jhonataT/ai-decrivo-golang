package review

import (
	"fmt"
	"strings"
	"testing"
)

// render descreve as linhas de forma compacta: "+3 texto", "-2 texto", " 4:5 texto".
func render(lines []DiffLine) string {
	var b strings.Builder
	for _, l := range lines {
		switch l.Kind {
		case "hunk":
			b.WriteString("@@\n")
		case "added":
			fmt.Fprintf(&b, "+%d %s\n", l.NewLine, l.Text)
		case "removed":
			fmt.Fprintf(&b, "-%d %s\n", l.OldLine, l.Text)
		default:
			fmt.Fprintf(&b, " %d:%d %s\n", l.OldLine, l.NewLine, l.Text)
		}
	}
	return b.String()
}

func TestParsePatch(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  string
	}{
		{
			name: "arquivo novo: cabeçalho some, numeração começa em 1",
			patch: "diff --git a/x.go b/x.go\nnew file mode 100644\nindex 0000000..e69de29\n" +
				"--- /dev/null\n+++ b/x.go\n@@ -0,0 +1,2 @@\n+package x\n+\n",
			want: "@@\n+1 package x\n+2 \n",
		},
		{
			name: "linha removida que começa com -- não é cabeçalho",
			patch: "--- a/q.sql\n+++ b/q.sql\n@@ -1,2 +1,2 @@\n" +
				"--- comentário antigo\n+-- comentário novo\n select 1;\n",
			want: "@@\n-1 -- comentário antigo\n+1 -- comentário novo\n 2:2 select 1;\n",
		},
		{
			name: "sem newline no fim do arquivo",
			patch: "@@ -3,1 +3,1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file\n",
			want: "@@\n-3 a\n+3 b\n",
		},
		{
			name:  "dois hunks mantêm a numeração de cada um",
			patch: "@@ -1,1 +1,1 @@\n x\n@@ -10,1 +10,2 @@\n y\n+z\n",
			want:  "@@\n 1:1 x\n@@\n 10:10 y\n+11 z\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(ParsePatch(tt.patch)); got != tt.want {
				t.Errorf("\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
