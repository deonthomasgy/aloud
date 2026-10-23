package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBinaryJSONPayloads(t *testing.T) {
	data := []byte{0, 1, 2, 255, 128}
	raw, err := json.Marshal(geminiInlineData{MimeType: "image/png", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err = json.Unmarshal(raw, &got); err != nil || got["data"] != base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("payload=%s error=%v", raw, err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"audio": data, "timestamps": []any{map[string]any{"word": "one", "start": 0, "end": 1}}})
	}))
	defer upstream.Close()
	audio, ts, err := captioned(context.Background(), config{baseURL: upstream.URL + "/v1"}, upstream.Client(), "one", "voice", "wav", 1)
	if err != nil || !bytes.Equal(audio, data) || len(ts) != 1 {
		t.Fatalf("audio=%v timestamps=%v error=%v", audio, ts, err)
	}
}
