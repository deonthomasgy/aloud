package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	maxPDFBytes = 50 << 20
	maxPDFPages = 200
	pdfDPI      = 150
)

type renderedPDF struct {
	paths []string
	dir   string
}

func (p *renderedPDF) Close() error {
	return os.RemoveAll(p.dir)
}

// rasterizePDF renders a PDF into ordered PNG page images. The caller must
// close the returned value when it has finished copying the page images.
func rasterizePDF(ctx context.Context, data []byte) (*renderedPDF, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("PDF is empty")
	}
	if len(data) > maxPDFBytes {
		return nil, fmt.Errorf("PDF exceeds the %d MB limit", maxPDFBytes>>20)
	}

	header := data
	if len(header) > 1024 {
		header = header[:1024]
	}
	if !bytes.Contains(header, []byte("%PDF-")) {
		return nil, fmt.Errorf("file is not a valid PDF")
	}

	dir, err := os.MkdirTemp("", "aloud-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create PDF workspace: %w", err)
	}
	cleanup := func(err error) (*renderedPDF, error) {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	inputPath := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(inputPath, data, 0o600); err != nil {
		return cleanup(fmt.Errorf("write PDF: %w", err))
	}

	outputPrefix := filepath.Join(dir, "page")
	var stderr bytes.Buffer
	cmd := exec.CommandContext(
		ctx,
		"pdftoppm",
		"-png",
		"-r", strconv.Itoa(pdfDPI),
		"-f", "1",
		"-l", strconv.Itoa(maxPDFPages+1),
		inputPath,
		outputPrefix,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return cleanup(fmt.Errorf("render PDF: %w", ctxErr))
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return cleanup(fmt.Errorf("render PDF: %s", detail))
	}

	paths, err := filepath.Glob(outputPrefix + "-*.png")
	if err != nil {
		return cleanup(fmt.Errorf("list rendered PDF pages: %w", err))
	}
	sort.Slice(paths, func(i, j int) bool {
		return pdfPageNumber(paths[i], outputPrefix) < pdfPageNumber(paths[j], outputPrefix)
	})

	if len(paths) == 0 {
		return cleanup(fmt.Errorf("PDF contains no pages"))
	}
	if len(paths) > maxPDFPages {
		return cleanup(fmt.Errorf("PDF exceeds the %d-page limit", maxPDFPages))
	}

	return &renderedPDF{paths: paths, dir: dir}, nil
}

func pdfPageNumber(path, prefix string) int {
	number := strings.TrimSuffix(strings.TrimPrefix(path, prefix+"-"), ".png")
	n, err := strconv.Atoi(number)
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return n
}
