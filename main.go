// Command invtts is a small Go web app that converts pasted text to speech
// using the Kokoro-82M model (https://kokorottsai.com) via a self-hosted
// OpenAI-compatible TTS endpoint such as Kokoro-FastAPI or docker-kokoro.
//
// It serves a single-page UI and proxies synthesis requests to the configured
// Kokoro endpoint, so the browser never needs to know the upstream URL or key.
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

// config holds runtime settings, all overridable via environment variables.
type config struct {
	addr        string // HTTP listen address, e.g. ":8080"
	baseURL     string // Kokoro OpenAI-compatible base URL, e.g. http://localhost:8880/v1
	apiKey      string // API key sent as Bearer token (Kokoro ignores it but OpenAI clients require one)
	model       string // model name passed upstream
	maxChars    int    // reject requests longer than this
	dataDir     string // where the SQLite DB, uploads, and cached audio live
	geminiKey   string // Google Gemini API key for OCR
	geminiModel string // Gemini model used for OCR
}

func loadConfig() config {
	return config{
		addr:        env("PORT_ADDR", ":"+env("PORT", "8080")),
		baseURL:     strings.TrimRight(env("KOKORO_BASE_URL", "http://alpha-old:8880/v1"), "/"),
		apiKey:      env("KOKORO_API_KEY", "not-needed"),
		model:       env("KOKORO_MODEL", "kokoro"),
		maxChars:    envInt("KOKORO_MAX_CHARS", 50000),
		dataDir:     env("DATA_DIR", "/data"),
		geminiKey:   env("GEMINI_API_KEY", ""),
		geminiModel: env("GEMINI_MODEL", "gemini-3.5-flash"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ttsRequest is the JSON the browser posts to /api/tts.
type ttsRequest struct {
	Text   string  `json:"text"`
	Voice  string  `json:"voice"`
	Format string  `json:"format"`
	Speed  float64 `json:"speed"`
}

// openAITTSRequest is the payload sent upstream to /v1/audio/speech.
type openAITTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed"`
}

var allowedFormats = map[string]string{
	"mp3":  "audio/mpeg",
	"wav":  "audio/wav",
	"opus": "audio/opus",
	"flac": "audio/flac",
	"aac":  "audio/aac",
	"pcm":  "audio/pcm",
}

func main() {
	cfg := loadConfig()

	if err := os.MkdirAll(cfg.dataDir, 0o755); err != nil {
		log.Fatalf("could not create data dir %q: %v", cfg.dataDir, err)
	}
	st, err := openStore(filepath.Join(cfg.dataDir, "invtts.db"))
	if err != nil {
		log.Fatalf("could not open database: %v", err)
	}

	srv := &server{cfg: cfg, client: &http.Client{Timeout: 5 * time.Minute}, store: st}

	mux := http.NewServeMux()
	// Static SPA + simple TTS API (original).
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/health", srv.handleHealth)
	mux.HandleFunc("/api/voices", srv.handleVoices)
	mux.HandleFunc("/api/tts", srv.handleTTS)

	// Projects API.
	mux.HandleFunc("GET /api/projects", srv.handleListProjects)
	mux.HandleFunc("POST /api/projects", srv.handleCreateProject)
	mux.HandleFunc("GET /api/projects/{id}", srv.handleGetProject)
	mux.HandleFunc("DELETE /api/projects/{id}", srv.handleDeleteProject)
	mux.HandleFunc("POST /api/projects/{id}/pages", srv.handleUploadPages)
	mux.HandleFunc("POST /api/projects/{id}/reorder", srv.handleReorderPages)
	mux.HandleFunc("POST /api/projects/{id}/auto-order", srv.handleAutoOrder)
	mux.HandleFunc("PUT /api/paragraphs/{id}", srv.handleUpdateParagraph)
	mux.HandleFunc("POST /api/paragraphs/{id}/speech", srv.handleParagraphSpeech)
	mux.HandleFunc("POST /api/speak", srv.handleSpeak)
	mux.HandleFunc("GET /api/pages/{id}/image", srv.handlePageImage)
	mux.HandleFunc("GET /api/audio/{file}", srv.handleAudio)

	log.Printf("invtts listening on %s", cfg.addr)
	log.Printf("Kokoro endpoint: %s (model %q)", cfg.baseURL, cfg.model)
	log.Printf("Data dir: %s · Gemini OCR model: %s (key set: %v)", cfg.dataDir, cfg.geminiModel, cfg.geminiKey != "")
	if err := http.ListenAndServe(cfg.addr, mux); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	cfg    config
	client *http.Client
	store  *store
}

// staticExts are file types served directly from the embedded web/ dir.
var staticContentTypes = map[string]string{
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".ico":  "image/x-icon",
	".json": "application/json",
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Serve a real embedded asset if one matches; otherwise fall back to the
	// SPA shell so client-side hash routing works on any path.
	clean := path.Clean(r.URL.Path)
	if clean != "/" && !strings.HasPrefix(clean, "/api/") {
		if data, err := webFS.ReadFile("web" + clean); err == nil {
			if ct, ok := staticContentTypes[strings.ToLower(filepath.Ext(clean))]; ok {
				w.Header().Set("Content-Type", ct)
			}
			_, _ = w.Write(data)
			return
		}
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "ui not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	upReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.baseURL+"/models", nil)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	upReq.Header.Set("Authorization", "Bearer "+s.cfg.apiKey)

	resp, err := s.client.Do(upReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "unavailable",
			"error":  "could not reach Kokoro endpoint at " + s.cfg.baseURL + ": " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "unavailable",
			"error":  fmt.Sprintf("Kokoro returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg))),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"endpoint": s.cfg.baseURL,
		"model":    s.cfg.model,
	})
}

func (s *server) handleVoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"default": DefaultVoice,
		"groups":  VoiceGroups,
	})
}

func (s *server) handleTTS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	var req ttsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeJSONError(w, http.StatusBadRequest, "text is empty")
		return
	}
	if len(req.Text) > s.cfg.maxChars {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("text too long (%d chars, max %d)", len(req.Text), s.cfg.maxChars))
		return
	}
	if req.Voice == "" {
		req.Voice = DefaultVoice
	}
	if !isValidVoice(req.Voice) {
		writeJSONError(w, http.StatusBadRequest, "unknown voice: "+req.Voice)
		return
	}
	if req.Format == "" {
		req.Format = "mp3"
	}
	contentType, ok := allowedFormats[req.Format]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "unsupported format: "+req.Format)
		return
	}
	if req.Speed == 0 {
		req.Speed = 1.0
	}
	if req.Speed < 0.5 || req.Speed > 2.0 {
		writeJSONError(w, http.StatusBadRequest, "speed must be between 0.5 and 2.0")
		return
	}

	upstream := openAITTSRequest{
		Model:          s.cfg.model,
		Input:          req.Text,
		Voice:          req.Voice,
		ResponseFormat: req.Format,
		Speed:          req.Speed,
	}
	body, _ := json.Marshal(upstream)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.baseURL+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Authorization", "Bearer "+s.cfg.apiKey)

	resp, err := s.client.Do(upReq)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway,
			"could not reach Kokoro endpoint at "+s.cfg.baseURL+": "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		writeJSONError(w, http.StatusBadGateway,
			fmt.Sprintf("Kokoro returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg))))
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("stream to client failed: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
