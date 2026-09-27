package ui

import (
	"path"
	"strings"
)

// Padrões usados pelos filtros "esconder testes" e "esconder gerados".
// É uma heurística por nome: cobre as convenções mais comuns de Go e JS/TS.
var (
	testSuffixes = []string{"_test.go", "_spec.rb", "_test.py", ".snap"}
	testInfixes  = []string{".test.", ".spec.", ".e2e.", ".stories."}
	testDirs     = []string{"test", "tests", "__tests__", "spec", "specs", "testdata", "e2e", "__mocks__", "mocks"}

	genSuffixes = []string{"_templ.go", ".pb.go", "_gen.go", ".gen.go", "_string.go", ".min.js", ".min.css", ".map", ".d.ts"}
	genDirs     = []string{"vendor", "dist", "build", "generated", "node_modules", "wailsjs"}
)

func isTestFile(p string) bool {
	p = strings.ToLower(p)
	name := path.Base(p)
	if strings.HasPrefix(name, "test_") {
		return true
	}
	return hasSuffix(name, testSuffixes) || hasInfix(name, testInfixes) || inDir(p, testDirs)
}

func isGeneratedFile(p string) bool {
	p = strings.ToLower(p)
	name := path.Base(p)
	if strings.HasPrefix(name, "mock_") || strings.Contains(name, ".generated.") {
		return true
	}
	return hasSuffix(name, genSuffixes) || inDir(p, genDirs)
}

func hasSuffix(s string, list []string) bool {
	for _, x := range list {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

func hasInfix(s string, list []string) bool {
	for _, x := range list {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// inDir diz se algum diretório do caminho (não o nome do arquivo) está na lista.
func inDir(p string, dirs []string) bool {
	parts := strings.Split(path.Dir(p), "/")
	for _, part := range parts {
		for _, d := range dirs {
			if part == d {
				return true
			}
		}
	}
	return false
}
