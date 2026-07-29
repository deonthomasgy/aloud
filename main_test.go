package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHandleTTSNormalizesTextBeforeSendingItUpstream(t *testing.T) {
	var upstreamRequest openAITTSRequest
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&upstreamRequest); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(strings.NewReader("audio")),
		}, nil
	})}
	srv := &server{
		cfg: config{
			baseURL:  "http://kokoro.test/v1",
			apiKey:   "test-key",
			model:    "kokoro",
			maxChars: 50000,
		},
		client: client,
	}

	body, err := json.Marshal(ttsRequest{
		Text:   "**Use `self_assessment` & manager`s review.**",
		Format: "mp3",
	})
	if err != nil {
		t.Fatalf("encode TTS request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/tts", strings.NewReader(string(body)))
	response := httptest.NewRecorder()

	srv.handleTTS(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("handleTTS status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	want := "Use self assessment and manager's review."
	if upstreamRequest.Input != want {
		t.Fatalf("upstream input = %q, want %q", upstreamRequest.Input, want)
	}
}
