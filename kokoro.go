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

// WordTS is a single word with its spoken start/end time in seconds.
// Field names match Kokoro's /dev/captioned_speech response.
type WordTS struct {
	Word      string  `json:"word"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// captionedRequest mirrors the body the macOS app sends (KokoroClient.swift).
type captionedRequest struct {
	Model                string         `json:"model"`
	Input                string         `json:"input"`
	Voice                string         `json:"voice"`
	ResponseFormat       string         `json:"response_format"`
	Speed                float64        `json:"speed"`
	Stream               bool           `json:"stream"`
	NormalizationOptions map[string]any `json:"normalization_options"`
}

// rawTimestamp accepts both {start_time,end_time} and {start,end} shapes.
type rawTimestamp struct {
	Word      string   `json:"word"`
	StartTime *float64 `json:"start_time"`
	EndTime   *float64 `json:"end_time"`
	Start     *float64 `json:"start"`
	End       *float64 `json:"end"`
}

type captionedResponse struct {
	Audio      string         `json:"audio"` // base64
	Timestamps []rawTimestamp `json:"timestamps"`
}

// synthesizeCaptioned calls Kokoro's captioned endpoint and returns audio bytes
// plus word timestamps. On any failure it falls back to plain speech (no timestamps).
func synthesizeCaptioned(ctx context.Context, cfg config, client *http.Client, text, voice, format string, speed float64) ([]byte, []WordTS, error) {
	audio, ts, err := captioned(ctx, cfg, client, text, voice, format, speed)
	if err == nil {
		return audio, ts, nil
	}
	// Fallback: plain /v1/audio/speech (existing proxy path), no timestamps.
	audio, ferr := plainSpeech(ctx, cfg, client, text, voice, format, speed)
	if ferr != nil {
		return nil, nil, fmt.Errorf("captioned failed (%v) and fallback failed: %w", err, ferr)
	}
	return audio, nil, nil
}

func captioned(ctx context.Context, cfg config, client *http.Client, text, voice, format string, speed float64) ([]byte, []WordTS, error) {
	reqBody := captionedRequest{
		Model:                cfg.model,
		Input:                text,
		Voice:                voice,
		ResponseFormat:       format,
		Speed:                speed,
		Stream:               false,
		NormalizationOptions: map[string]any{"normalize": false},
	}
	body, _ := json.Marshal(reqBody)

	// captioned_speech lives at the server root (sibling of /v1), e.g. .../dev/captioned_speech
	url := captionedURL(cfg.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("captioned_speech returned %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var cr captionedResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, nil, fmt.Errorf("captioned_speech: bad JSON: %w", err)
	}
	audio, err := base64.StdEncoding.DecodeString(cr.Audio)
	if err != nil {
		return nil, nil, fmt.Errorf("captioned_speech: bad base64 audio: %w", err)
	}

	ts := make([]WordTS, 0, len(cr.Timestamps))
	for _, r := range cr.Timestamps {
		start := firstNonNil(r.StartTime, r.Start)
		end := firstNonNil(r.EndTime, r.End)
		if end == 0 {
			end = start
		}
		ts = append(ts, WordTS{Word: r.Word, StartTime: start, EndTime: end})
	}
	return audio, ts, nil
}

// plainSpeech posts to /v1/audio/speech and returns raw audio bytes.
func plainSpeech(ctx context.Context, cfg config, client *http.Client, text, voice, format string, speed float64) ([]byte, error) {
	upstream := openAITTSRequest{
		Model:          cfg.model,
		Input:          text,
		Voice:          voice,
		ResponseFormat: format,
		Speed:          speed,
	}
	body, _ := json.Marshal(upstream)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.baseURL+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audio/speech returned %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return raw, nil
}

// captionedURL derives the /dev/captioned_speech URL from the OpenAI-style base
// URL (which ends in /v1), since the captioned endpoint sits at the server root.
func captionedURL(baseURL string) string {
	root := strings.TrimSuffix(baseURL, "/v1")
	return root + "/dev/captioned_speech"
}

func firstNonNil(vals ...*float64) float64 {
	for _, v := range vals {
		if v != nil {
			return *v
		}
	}
	return 0
}
