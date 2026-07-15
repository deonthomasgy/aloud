package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxPageUploadBytes = 100 << 20

// --- Projects CRUD ---

func (s *server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.listProjects()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not list projects: "+err.Error())
		return
	}
	if projects == nil {
		projects = []Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title  string  `json:"title"`
		Voice  string  `json:"voice"`
		Speed  float64 `json:"speed"`
		Format string  `json:"format"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		body.Title = "Untitled project"
	}
	if body.Voice == "" || !isValidVoice(body.Voice) {
		body.Voice = DefaultVoice
	}
	if body.Speed < 0.5 || body.Speed > 2.0 {
		body.Speed = 1.0
	}
	if _, ok := allowedFormats[body.Format]; !ok {
		body.Format = "mp3"
	}
	p, err := s.store.createProject(body.Title, body.Voice, body.Speed, body.Format)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not create project: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := s.store.getProject(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.deleteProject(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not delete project: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Page upload (images, PDFs, or text) ---

func (s *server) handleUploadPages(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.store.getProject(projectID); err != nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPageUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, status, "could not parse upload: "+err.Error())
		return
	}
	defer r.MultipartForm.RemoveAll()

	created := 0
	defer func() {
		if created > 0 {
			go s.ocrPendingPages(projectID)
		}
	}()

	// Raw text → one page, split into paragraphs immediately (no OCR).
	if text := strings.TrimSpace(r.FormValue("text")); text != "" {
		pos, _ := s.store.nextPagePosition(projectID)
		pageID, err := s.store.createPage(projectID, pos, "", "text", "done")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not create page: "+err.Error())
			return
		}
		if err := s.store.replaceParagraphs(pageID, splitParagraphs(text)); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not save paragraphs: "+err.Error())
			return
		}
		created++
	}

	// Image files → one page each, OCR'd in the background.
	files := r.MultipartForm.File["images"]
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(f, 25<<20))
		f.Close()
		if len(data) == 0 {
			continue
		}

		pos, _ := s.store.nextPagePosition(projectID)
		pageID, err := s.store.createPage(projectID, pos, "", "image", "pending")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not create page: "+err.Error())
			return
		}

		ext := imageExt(fh.Filename, data)
		path := filepath.Join(s.cfg.dataDir, "uploads", strconv.FormatInt(projectID, 10), fmt.Sprintf("%d%s", pageID, ext))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			if err := os.WriteFile(path, data, 0o644); err == nil {
				_ = s.store.setPageImagePath(pageID, path)
			}
		}
		created++
	}

	// PDF files → rasterized PNG pages, then the same OCR path as images.
	for _, fh := range r.MultipartForm.File["pdf"] {
		f, err := fh.Open()
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not open PDF: "+err.Error())
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxPDFBytes+1))
		closeErr := f.Close()
		if readErr != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read PDF: "+readErr.Error())
			return
		}
		if closeErr != nil {
			writeJSONError(w, http.StatusBadRequest, "could not close PDF: "+closeErr.Error())
			return
		}
		if len(data) > maxPDFBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("PDF exceeds the %d MB limit", maxPDFBytes>>20))
			return
		}

		n, err := s.addPDFPages(r.Context(), projectID, data)
		created += n
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("could not add %q: %v", fh.Filename, err))
			return
		}
	}

	if created == 0 {
		writeJSONError(w, http.StatusBadRequest, "no text, images, or PDFs provided")
		return
	}

	p, _ := s.store.getProject(projectID)
	writeJSON(w, http.StatusAccepted, p)
}

func (s *server) addPDFPages(ctx context.Context, projectID int64, data []byte) (int, error) {
	renderCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	rendered, err := rasterizePDF(renderCtx, data)
	if err != nil {
		return 0, err
	}
	defer rendered.Close()

	uploadDir := filepath.Join(s.cfg.dataDir, "uploads", strconv.FormatInt(projectID, 10))
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return 0, fmt.Errorf("create upload directory: %w", err)
	}

	created := 0
	for pageIndex, sourcePath := range rendered.paths {
		source, err := os.Open(sourcePath)
		if err != nil {
			return created, fmt.Errorf("open rendered page %d: %w", pageIndex+1, err)
		}

		destination, err := os.CreateTemp(uploadDir, "pdf-page-*.png")
		if err != nil {
			source.Close()
			return created, fmt.Errorf("create page image %d: %w", pageIndex+1, err)
		}
		destinationPath := destination.Name()

		_, copyErr := io.Copy(destination, source)
		sourceCloseErr := source.Close()
		destinationCloseErr := destination.Close()
		if copyErr != nil || sourceCloseErr != nil || destinationCloseErr != nil {
			_ = os.Remove(destinationPath)
			switch {
			case copyErr != nil:
				err = copyErr
			case sourceCloseErr != nil:
				err = sourceCloseErr
			default:
				err = destinationCloseErr
			}
			return created, fmt.Errorf("save page image %d: %w", pageIndex+1, err)
		}

		position, err := s.store.nextPagePosition(projectID)
		if err != nil {
			_ = os.Remove(destinationPath)
			return created, fmt.Errorf("position page %d: %w", pageIndex+1, err)
		}
		if _, err := s.store.createPage(projectID, position, destinationPath, "image", "pending"); err != nil {
			_ = os.Remove(destinationPath)
			return created, fmt.Errorf("create page %d: %w", pageIndex+1, err)
		}
		created++
	}

	return created, nil
}

// ocrPendingPages OCRs every pending page of a project sequentially, then
// re-sorts the project's pages by their detected printed page number.
func (s *server) ocrPendingPages(projectID int64) {
	ids, err := s.store.pendingPages(projectID)
	if err != nil {
		return
	}
	for _, pageID := range ids {
		s.ocrPage(pageID)
	}
	s.autoOrder(projectID)
}

// autoOrder sorts a project's pages. If every page has a detected printed page
// number, it sorts by that. Otherwise it asks Gemini to order the pages by
// narrative continuity, falling back to page-number sort if that fails.
func (s *server) autoOrder(projectID int64) {
	p, err := s.store.getProject(projectID)
	if err != nil || len(p.Pages) < 2 {
		return
	}

	allNumbered := true
	for _, pg := range p.Pages {
		if pg.PageNumber == nil {
			allNumbered = false
			break
		}
	}
	if allNumbered {
		if err := s.store.reorderByPageNumber(projectID); err != nil {
			log.Printf("reorder(number) project %d: %v", projectID, err)
		}
		return
	}

	// Mixed or no page numbers → order by content flow with Gemini.
	snippets := make([]pageSnippet, 0, len(p.Pages))
	for _, pg := range p.Pages {
		if pg.OCRStatus == "done" && len(pg.Paragraphs) > 0 {
			snippets = append(snippets, snippetFromPage(pg))
		}
	}
	if len(snippets) < 2 {
		_ = s.store.reorderByPageNumber(projectID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	order, err := aiOrderPages(ctx, s.cfg, s.client, snippets)
	if err != nil {
		log.Printf("ai order project %d: %v (falling back to page numbers)", projectID, err)
		_ = s.store.reorderByPageNumber(projectID)
		return
	}
	if err := s.store.reorderPages(projectID, order); err != nil {
		log.Printf("apply ai order project %d: %v", projectID, err)
	}
}

// handleAutoOrder re-runs automatic ordering on demand.
func (s *server) handleAutoOrder(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.autoOrder(projectID)
	p, err := s.store.getProject(projectID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) ocrPage(pageID int64) {
	_ = s.store.setPageOCR(pageID, "processing", "")

	path, err := s.store.getPageImagePath(pageID)
	if err != nil || path == "" {
		_ = s.store.setPageOCR(pageID, "error", "image missing")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		_ = s.store.setPageOCR(pageID, "error", "could not read image")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	mime := http.DetectContentType(data)
	res, err := ocrImage(ctx, s.cfg, s.client, data, mime)
	if err != nil {
		_ = s.store.setPageOCR(pageID, "error", err.Error())
		return
	}
	if err := s.store.replaceParagraphs(pageID, res.Paragraphs); err != nil {
		_ = s.store.setPageOCR(pageID, "error", "could not save paragraphs")
		return
	}
	_ = s.store.setPageNumber(pageID, res.PageNumber)
	_ = s.store.setPageContStart(pageID, res.ContinuesPrev)
	_ = s.store.setPageOCR(pageID, "done", "")
}

// handleReorderPages sets an explicit page order from a client-provided id list.
func (s *server) handleReorderPages(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Order []int64 `json:"order"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.reorderPages(projectID, body.Order); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not reorder: "+err.Error())
		return
	}
	p, err := s.store.getProject(projectID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleUpdateParagraph edits a paragraph's text (e.g. to fix an OCR mistake or
// remove emphasis caps) and clears its cached audio.
func (s *server) handleUpdateParagraph(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<18)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" {
		writeJSONError(w, http.StatusBadRequest, "text is empty")
		return
	}
	if err := s.store.updateParagraphText(id, body.Text); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not update paragraph: "+err.Error())
		return
	}
	pa, err := s.store.getParagraph(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "paragraph not found")
		return
	}
	writeJSON(w, http.StatusOK, pa)
}

// --- Paragraph speech (lazy, cached) ---

type speechResponse struct {
	AudioURL   string   `json:"audioUrl"`
	Timestamps []WordTS `json:"timestamps"`
}

func (s *server) handleParagraphSpeech(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	para, err := s.store.getParagraph(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "paragraph not found")
		return
	}

	voice, speed, format, err := s.store.projectSettingsForParagraph(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Optional per-request overrides.
	var body struct {
		Voice  string  `json:"voice"`
		Speed  float64 `json:"speed"`
		Format string  `json:"format"`
		Force  bool    `json:"force"` // bypass cache and re-synthesize
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body)
	if body.Voice != "" && isValidVoice(body.Voice) {
		voice = body.Voice
	}
	if body.Speed >= 0.5 && body.Speed <= 2.0 {
		speed = body.Speed
	}
	if _, okF := allowedFormats[body.Format]; okF {
		format = body.Format
	}
	if _, okF := allowedFormats[format]; !okF {
		format = "mp3"
	}

	// What we actually send to the TTS engine: normalized so emphasis words in
	// ALL CAPS are spoken naturally instead of being spelled out letter by letter.
	speechText := normalizeForSpeech(para.Text)

	cacheName := audioCacheName(speechText, voice, speed, format)
	cachePath := filepath.Join(s.cfg.dataDir, "audio", cacheName)

	// Serve from cache if the audio file already exists (unless force re-synth).
	if !body.Force && para.HasAudio && para.AudioPath == cachePath {
		if _, statErr := os.Stat(cachePath); statErr == nil {
			ts := s.cachedTimestamps(id)
			writeJSON(w, http.StatusOK, speechResponse{AudioURL: "/api/audio/" + cacheName, Timestamps: ts})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	audio, timestamps, err := synthesizeCaptioned(ctx, s.cfg, s.client, speechText, voice, format, speed)
	if err != nil {
		_ = s.store.setParagraphAudioError(id)
		writeJSONError(w, http.StatusBadGateway, "synthesis failed: "+err.Error())
		return
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not create audio dir")
		return
	}
	if err := os.WriteFile(cachePath, audio, 0o644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not write audio")
		return
	}
	if err := s.store.setParagraphAudio(id, cachePath, timestamps); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not record audio: "+err.Error())
		return
	}

	if timestamps == nil {
		timestamps = []WordTS{}
	}
	writeJSON(w, http.StatusOK, speechResponse{AudioURL: "/api/audio/" + cacheName, Timestamps: timestamps})
}

// handleSpeak synthesizes arbitrary text (e.g. a paragraph that flows across a
// page break, joined into one) with word timestamps, cached on disk by content.
func (s *server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text   string  `json:"text"`
		Voice  string  `json:"voice"`
		Speed  float64 `json:"speed"`
		Format string  `json:"format"`
		Force  bool    `json:"force"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeJSONError(w, http.StatusBadRequest, "text is empty")
		return
	}
	voice := body.Voice
	if voice == "" || !isValidVoice(voice) {
		voice = DefaultVoice
	}
	speed := body.Speed
	if speed < 0.5 || speed > 2.0 {
		speed = 1.0
	}
	format := body.Format
	if _, ok := allowedFormats[format]; !ok {
		format = "mp3"
	}

	speechText := normalizeForSpeech(text)
	name := audioCacheName(speechText, voice, speed, format)
	cachePath := filepath.Join(s.cfg.dataDir, "audio", name)
	tsPath := cachePath + ".json"

	if !body.Force {
		if _, err := os.Stat(cachePath); err == nil {
			if raw, err := os.ReadFile(tsPath); err == nil {
				var ts []WordTS
				_ = json.Unmarshal(raw, &ts)
				writeJSON(w, http.StatusOK, speechResponse{AudioURL: "/api/audio/" + name, Timestamps: ts})
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	audio, timestamps, err := synthesizeCaptioned(ctx, s.cfg, s.client, speechText, voice, format, speed)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "synthesis failed: "+err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not create audio dir")
		return
	}
	_ = os.WriteFile(cachePath, audio, 0o644)
	if timestamps == nil {
		timestamps = []WordTS{}
	}
	tsJSON, _ := json.Marshal(timestamps)
	_ = os.WriteFile(tsPath, tsJSON, 0o644)
	writeJSON(w, http.StatusOK, speechResponse{AudioURL: "/api/audio/" + name, Timestamps: timestamps})
}

func (s *server) cachedTimestamps(paragraphID int64) []WordTS {
	raw, err := s.store.getParagraphTimestamps(paragraphID)
	if err != nil || raw == "" {
		return []WordTS{}
	}
	var ts []WordTS
	if err := json.Unmarshal([]byte(raw), &ts); err != nil {
		return []WordTS{}
	}
	return ts
}

// handleAudio serves a cached audio file by name.
func (s *server) handleAudio(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !safeAudioName.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.dataDir, "audio", name)
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	if ct, ok := allowedFormats[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, path)
}

// handlePageImage serves the original uploaded image for a page (thumbnail/preview).
func (s *server) handlePageImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	path, err := s.store.getPageImagePath(id)
	if err != nil || path == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

// --- helpers ---

var safeAudioName = regexp.MustCompile(`^[a-f0-9]{64}\.[a-z0-9]{2,4}$`)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func pathID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid "+key)
		return 0, false
	}
	return id, true
}

// audioCacheName builds a content-addressed filename for a synthesis request.
func audioCacheName(text, voice string, speed float64, format string) string {
	key := fmt.Sprintf("%s|%s|%.2f|%s", text, voice, speed, format)
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x.%s", sum, format)
}

// splitParagraphs splits raw text into paragraphs on blank lines.
func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	blocks := regexp.MustCompile(`\n[ \t]*\n`).Split(text, -1)
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if t := strings.TrimSpace(b); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		if t := strings.TrimSpace(text); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// imageExt picks a file extension from the upload filename or sniffed content type.
func imageExt(filename string, data []byte) string {
	if e := strings.ToLower(filepath.Ext(filename)); e != "" {
		switch e {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".heic":
			return e
		}
	}
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}
