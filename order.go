package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// pageSnippet is the text Gemini sees when ordering pages by content flow.
type pageSnippet struct {
	ID   int64
	Head string // text at the start of the page
	Tail string // text at the end of the page
}

const orderPrompt = `You are given several pages from a book or document, in arbitrary order.
Each page has an ID, the text at its START, and the text at its END.
Determine the correct reading order so the prose flows continuously: the END of one page should read naturally into the START of the next (a sentence cut off at the bottom of one page continues at the top of the next; topics and narrative progress logically).
Return JSON {"order": [ids...]} listing every page ID exactly once in the correct reading sequence.`

// aiOrderPages asks Gemini to order pages by narrative continuity, returning the
// ordered page IDs. Used when printed page numbers are missing.
func aiOrderPages(ctx context.Context, cfg config, client *http.Client, snippets []pageSnippet) ([]int64, error) {
	if cfg.geminiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	var b strings.Builder
	b.WriteString(orderPrompt)
	b.WriteString("\n\n")
	for _, s := range snippets {
		fmt.Fprintf(&b, "PAGE ID %d\nSTART: %s\nEND: %s\n\n", s.ID, s.Head, s.Tail)
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: b.String()}}}},
		GenerationConfig: geminiGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"order": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "integer"},
					},
				},
				"required": []string{"order"},
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		cfg.geminiModel, cfg.geminiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini order request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var gr geminiResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, fmt.Errorf("gemini order: bad response (%d)", resp.StatusCode)
	}
	if gr.Error != nil {
		return nil, fmt.Errorf("gemini order error: %s", gr.Error.Message)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini order: empty response")
	}
	var sb strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	var parsed struct {
		Order []int64 `json:"order"`
	}
	if err := json.Unmarshal([]byte(sb.String()), &parsed); err != nil {
		return nil, fmt.Errorf("gemini order: could not parse JSON: %w", err)
	}
	if len(parsed.Order) == 0 {
		return nil, fmt.Errorf("gemini order: empty order")
	}
	return parsed.Order, nil
}

// snippet builds a head/tail excerpt from a page's paragraphs.
func snippetFromPage(pg Page) pageSnippet {
	full := ""
	for i, pa := range pg.Paragraphs {
		if i > 0 {
			full += " "
		}
		full += pa.Text
	}
	full = strings.Join(strings.Fields(full), " ") // collapse whitespace
	const n = 240
	head := full
	if len(head) > n {
		head = head[:n]
	}
	tail := full
	if len(tail) > n {
		tail = tail[len(tail)-n:]
	}
	return pageSnippet{ID: pg.ID, Head: head, Tail: tail}
}
