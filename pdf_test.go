package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRasterizePDFOrdersPagesNumerically(t *testing.T) {
	binDir := t.TempDir()
	renderer := filepath.Join(binDir, "pdftoppm")
	script := `#!/bin/sh
prefix=""
for arg in "$@"; do prefix="$arg"; done
: > "${prefix}-10.png"
: > "${prefix}-2.png"
: > "${prefix}-1.png"
`
	if err := os.WriteFile(renderer, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	rendered, err := rasterizePDF(context.Background(), []byte("%PDF-1.7\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer rendered.Close()

	got := []int{
		pdfPageNumber(rendered.paths[0], filepath.Join(rendered.dir, "page")),
		pdfPageNumber(rendered.paths[1], filepath.Join(rendered.dir, "page")),
		pdfPageNumber(rendered.paths[2], filepath.Join(rendered.dir, "page")),
	}
	want := []int{1, 2, 10}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("page %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestRasterizePDFRejectsNonPDF(t *testing.T) {
	_, err := rasterizePDF(context.Background(), []byte("not a PDF"))
	if err == nil {
		t.Fatal("expected invalid PDF error")
	}
}
