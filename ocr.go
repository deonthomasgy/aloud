package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ocrPrompt instructs Gemini to transcribe a book page into clean, ordered paragraphs.
const ocrPrompt = `You are transcribing a photograph of a single page from a book.
Transcribe ALL readable text exactly as written, preserving the original reading order and wording.
Split the text into natural paragraphs as they appear on the page.
Keep chapter or section titles as their own separate paragraph (do not drop them).
Also find the printed page number (usually a bare number in the top or bottom margin) and return it as "page_number"; use null if there is no visible page number.
Set "continues_previous_page" to true if the FIRST line of body text on this page is the continuation of a sentence or paragraph from a previous page — i.e. it begins mid-sentence or with a non-indented, lower-case continuation rather than a fresh paragraph, heading, or capitalized sentence start. Otherwise set it to false.
Do NOT include the page number or repeated running headers/footers in the paragraphs.
If a paragraph is split across the top/bottom of the page, join it into one paragraph.
If the page contains no readable text, return an empty paragraphs list.
Return JSON of the form {"page_number": 12, "continues_previous_page": false, "paragraphs": ["...", "..."]}.`

// geminiRequest is the generateContent request body.
type geminiRequest struct {
	Contents         []geminiContent       `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64
}

type geminiGenerationConfig struct {
	ResponseMimeType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ocrResult is the structured transcription of one page image.
type ocrResult struct {
	PageNumber    *int
	ContinuesPrev *bool
	Paragraphs    []string
}

// ocrImage sends an image to Gemini and returns its text split into ordered
// paragraphs plus any detected printed page number.
func ocrImage(ctx context.Context, cfg config, client *http.Client, imageBytes []byte, mime string) (ocrResult, error) {
	if cfg.geminiKey == "" {
		return ocrResult{}, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{
				{Text: ocrPrompt},
				{InlineData: &geminiInlineData{MimeType: mime, Data: base64.StdEncoding.EncodeToString(imageBytes)}},
			},
		}},
		GenerationConfig: geminiGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"page_number":             map[string]any{"type": "integer", "nullable": true},
					"continues_previous_page": map[string]any{"type": "boolean"},
					"paragraphs": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"required": []string{"paragraphs"},
			},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ocrResult{}, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		cfg.geminiModel, cfg.geminiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ocrResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return ocrResult{}, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var gr geminiResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return ocrResult{}, fmt.Errorf("gemini: bad response (%d): %s", resp.StatusCode, truncate(string(raw), 300))
	}
	if gr.Error != nil {
		return ocrResult{}, fmt.Errorf("gemini error: %s", gr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return ocrResult{}, fmt.Errorf("gemini returned %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return ocrResult{}, fmt.Errorf("gemini: empty response")
	}

	var parsed struct {
		PageNumber    *int     `json:"page_number"`
		ContinuesPrev *bool    `json:"continues_previous_page"`
		Paragraphs    []string `json:"paragraphs"`
	}
	// Gemini 3.x may emit multiple parts (e.g. a thoughtSignature part with no
	// text); concatenate every text part to get the JSON payload.
	var sb strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	text := sb.String()
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return ocrResult{}, fmt.Errorf("gemini: could not parse paragraphs JSON: %w", err)
	}

	// Clean up: trim and drop empties.
	out := make([]string, 0, len(parsed.Paragraphs))
	for _, p := range parsed.Paragraphs {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return ocrResult{PageNumber: parsed.PageNumber, ContinuesPrev: parsed.ContinuesPrev, Paragraphs: out}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
