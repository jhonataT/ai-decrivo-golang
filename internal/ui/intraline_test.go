package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/jhonataT/ai-decrivo-golang/internal/review"
)

// show desenha os segmentos com [colchetes] em volta do que mudou.
func show(segs []seg) string {
	var b strings.Builder
	for _, s := range segs {
		if s.Changed {
			b.WriteString("[" + s.Text + "]")
		} else {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func TestIntraline(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		wantOld       string
		wantNew       string
	}{
		{
			name:    "troca de tipo",
			before:  "\tPrice float64",
			after:   "\tPrice int // centavos",
			wantOld: "\tPrice [float64]",
			wantNew: "\tPrice [int // centavos]",
		},
		{
			name:    "argumento novo",
			before:  "func (c *Cart) Add(it Item) {",
			after:   "func (c *Cart) Add(it Item) error {",
			wantOld: "func (c *Cart) Add(it Item) {",
			wantNew: "func (c *Cart) Add(it Item) [error ]{",
		},
		{
			name:    "expressão alterada",
			before:  "\t\tt += it.Price * float64(it.Qty)",
			after:   "\t\tt += it.Price * it.Qty",
			wantOld: "\t\tt += it.Price * [float64(]it.Qty[)]",
			wantNew: "\t\tt += it.Price * it.Qty",
		},
		{
			name:    "acentos contam como letra",
			before:  `msg := "inválido"`,
			after:   `msg := "inválida"`,
			wantOld: `msg := "[inválido]"`,
			wantNew: `msg := "[inválida]"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, n := intraline(tt.before, tt.after)
			if got := show(o); got != tt.wantOld {
				t.Errorf("antes:\n got %q\nwant %q", got, tt.wantOld)
			}
			if got := show(n); got != tt.wantNew {
				t.Errorf("depois:\n got %q\nwant %q", got, tt.wantNew)
			}
		})
	}
}

// Linhas sem nada em comum não recebem destaque: pintar tudo seria ruído.
func TestIntralineSemRelacao(t *testing.T) {
	o, n := intraline("return t", "c.Discount = 50")
	if o != nil || n != nil {
		t.Fatalf("esperava nil, nil; veio %q / %q", show(o), show(n))
	}
}

// A célula usa white-space: pre-wrap, então qualquer espaço que o templ
// inserir entre os spans apareceria no código. O HTML precisa ser exato.
func TestSideNaoInsereEspacos(t *testing.T) {
	l := &review.DiffLine{Kind: "added", NewLine: 7, Text: "a := b"}
	segs := []seg{{Text: "a := "}, {Text: "b", Changed: true}}

	var b strings.Builder
	if err := side(l, segs, false).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	want := `<td class="code added">a := <span class="hl">b</span></td>`
	if !strings.Contains(b.String(), want) {
		t.Fatalf("HTML inesperado:\n%s\nesperava conter:\n%s", b.String(), want)
	}
}
