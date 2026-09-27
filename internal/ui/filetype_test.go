package ui

import "testing"

func TestFileType(t *testing.T) {
	tests := []struct {
		path      string
		test, gen bool
	}{
		{"internal/review/service.go", false, false},
		{"internal/review/service_test.go", true, false},
		{"web/src/cart.spec.ts", true, false},
		{"web/src/Cart.test.tsx", true, false},
		{"web/src/__tests__/cart.ts", true, false},
		{"internal/gitrepo/testdata/sample.patch", true, false},
		{"tests/test_cart.py", true, false},
		{"internal/ui/pages_templ.go", false, true},
		{"api/v1/cart.pb.go", false, true},
		{"frontend/dist/htmx.min.js", false, true},
		{"vendor/github.com/x/y.go", false, true},
		{"internal/store/mock_repo.go", false, true},
		// "latest" contém "test", mas não é teste; "builder" não é "build".
		{"internal/latest/builder.go", false, false},
	}
	for _, tt := range tests {
		if got := isTestFile(tt.path); got != tt.test {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.test)
		}
		if got := isGeneratedFile(tt.path); got != tt.gen {
			t.Errorf("isGeneratedFile(%q) = %v, want %v", tt.path, got, tt.gen)
		}
	}
}
