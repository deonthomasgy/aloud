package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// store wraps the SQLite database holding projects, pages, and paragraphs.
type store struct {
	db *sql.DB
}

// Project is a reading project: a book (or document) broken into pages.
type Project struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Voice     string  `json:"voice"`
	Speed     float64 `json:"speed"`
	Format    string  `json:"format"`
	CreatedAt string  `json:"created_at"`
	PageCount int     `json:"page_count"`
	Pages     []Page  `json:"pages"`
}

// Page is one uploaded image (or a text blob) within a project.
type Page struct {
	ID         int64       `json:"id"`
	ProjectID  int64       `json:"project_id"`
	Position   int         `json:"position"`
	ImagePath  string      `json:"-"`
	HasImage   bool        `json:"has_image"`
	PageNumber *int        `json:"page_number"` // printed page number detected by OCR, if any
	ContStart  *bool       `json:"cont_start"`  // OCR: page begins mid-paragraph (continues previous page); nil = unknown
	SourceType string      `json:"source_type"` // "image" | "text"
	OCRStatus  string      `json:"ocr_status"`  // pending | processing | done | error
	OCRError   string      `json:"ocr_error,omitempty"`
	CreatedAt  string      `json:"created_at"`
	Paragraphs []Paragraph `json:"paragraphs"`
}

// Paragraph is a single block of text within a page, with optional cached audio.
type Paragraph struct {
	ID          int64  `json:"id"`
	PageID      int64  `json:"page_id"`
	Position    int    `json:"position"`
	Text        string `json:"text"`
	AudioStatus string `json:"audio_status"` // none | done | error
	AudioPath   string `json:"-"`
	HasAudio    bool   `json:"has_audio"`
}

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite + WAL: single writer keeps things simple
	s := &store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS projects (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  title      TEXT NOT NULL,
  voice      TEXT NOT NULL,
  speed      REAL NOT NULL DEFAULT 1.0,
  format     TEXT NOT NULL DEFAULT 'mp3',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pages (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  position    INTEGER NOT NULL,
  image_path  TEXT NOT NULL DEFAULT '',
  page_number INTEGER,
  cont_start  INTEGER,
  source_type TEXT NOT NULL,
  ocr_status  TEXT NOT NULL DEFAULT 'pending',
  ocr_error   TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pages_project ON pages(project_id, position);

CREATE TABLE IF NOT EXISTS paragraphs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  page_id         INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  position        INTEGER NOT NULL,
  text            TEXT NOT NULL,
  audio_status    TEXT NOT NULL DEFAULT 'none',
  audio_path      TEXT NOT NULL DEFAULT '',
  timestamps_json TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_paragraphs_page ON paragraphs(page_id, position);
`)
	if err != nil {
		return err
	}
	// Best-effort column adds for databases created before these columns existed.
	for _, col := range []string{
		`ALTER TABLE pages ADD COLUMN page_number INTEGER`,
		`ALTER TABLE pages ADD COLUMN cont_start INTEGER`,
	} {
		if _, e := s.db.Exec(col); e != nil && !strings.Contains(e.Error(), "duplicate column") {
			// ignore: column already exists on fresh schemas
		}
	}
	return nil
}

func nowStr() string { return time.Now().UTC().Format(time.RFC3339) }

// --- Projects ---

func (s *store) createProject(title, voice string, speed float64, format string) (*Project, error) {
	res, err := s.db.Exec(
		`INSERT INTO projects (title, voice, speed, format, created_at) VALUES (?, ?, ?, ?, ?)`,
		title, voice, speed, format, nowStr())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.getProject(id)
}

func (s *store) listProjects() ([]Project, error) {
	rows, err := s.db.Query(`
SELECT p.id, p.title, p.voice, p.speed, p.format, p.created_at,
       (SELECT COUNT(*) FROM pages WHERE project_id = p.id)
FROM projects p ORDER BY p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Title, &p.Voice, &p.Speed, &p.Format, &p.CreatedAt, &p.PageCount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *store) getProject(id int64) (*Project, error) {
	var p Project
	err := s.db.QueryRow(
		`SELECT id, title, voice, speed, format, created_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Title, &p.Voice, &p.Speed, &p.Format, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	pages, err := s.pagesForProject(id)
	if err != nil {
		return nil, err
	}
	if pages == nil {
		pages = []Page{}
	}
	p.Pages = pages
	return &p, nil
}

func (s *store) deleteProject(id int64) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// --- Pages ---

func (s *store) pagesForProject(projectID int64) ([]Page, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, position, image_path, page_number, cont_start, source_type, ocr_status, ocr_error, created_at
		 FROM pages WHERE project_id = ? ORDER BY position, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []Page
	for rows.Next() {
		var pg Page
		var pageNum, contStart sql.NullInt64
		if err := rows.Scan(&pg.ID, &pg.ProjectID, &pg.Position, &pg.ImagePath, &pageNum, &contStart,
			&pg.SourceType, &pg.OCRStatus, &pg.OCRError, &pg.CreatedAt); err != nil {
			return nil, err
		}
		if pageNum.Valid {
			n := int(pageNum.Int64)
			pg.PageNumber = &n
		}
		if contStart.Valid {
			b := contStart.Int64 != 0
			pg.ContStart = &b
		}
		pg.HasImage = pg.ImagePath != ""
		pages = append(pages, pg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range pages {
		paras, err := s.paragraphsForPage(pages[i].ID)
		if err != nil {
			return nil, err
		}
		pages[i].Paragraphs = paras
	}
	return pages, nil
}

// nextPagePosition returns the position to assign to a newly added page.
func (s *store) nextPagePosition(projectID int64) (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(position) FROM pages WHERE project_id = ?`, projectID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return int(n.Int64) + 1, nil
}

func (s *store) createPage(projectID int64, position int, imagePath, sourceType, ocrStatus string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO pages (project_id, position, image_path, source_type, ocr_status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, position, imagePath, sourceType, ocrStatus, nowStr())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *store) setPageImagePath(pageID int64, path string) error {
	_, err := s.db.Exec(`UPDATE pages SET image_path = ? WHERE id = ?`, path, pageID)
	return err
}

// pendingPages returns the IDs of pages still awaiting OCR for a project, in order.
func (s *store) pendingPages(projectID int64) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT id FROM pages WHERE project_id = ? AND ocr_status = 'pending' ORDER BY position, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *store) setPageContStart(pageID int64, cont *bool) error {
	if cont == nil {
		_, err := s.db.Exec(`UPDATE pages SET cont_start = NULL WHERE id = ?`, pageID)
		return err
	}
	v := 0
	if *cont {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE pages SET cont_start = ? WHERE id = ?`, v, pageID)
	return err
}

func (s *store) setPageNumber(pageID int64, n *int) error {
	if n == nil {
		_, err := s.db.Exec(`UPDATE pages SET page_number = NULL WHERE id = ?`, pageID)
		return err
	}
	_, err := s.db.Exec(`UPDATE pages SET page_number = ? WHERE id = ?`, *n, pageID)
	return err
}

// reorderByPageNumber sorts a project's pages by their detected printed page
// number (ascending). Pages without a detected number keep their relative order
// and are placed after the numbered ones. Positions are rewritten 0..N-1.
func (s *store) reorderByPageNumber(projectID int64) error {
	type row struct {
		id  int64
		num sql.NullInt64
		pos int
	}
	rows, err := s.db.Query(`SELECT id, page_number, position FROM pages WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.num, &r.pos); err != nil {
			rows.Close()
			return err
		}
		rs = append(rs, r)
	}
	rows.Close()

	sort.SliceStable(rs, func(i, j int) bool {
		ai, aj := rs[i].num.Valid, rs[j].num.Valid
		if ai != aj {
			return ai // numbered pages first
		}
		if ai && aj && rs[i].num.Int64 != rs[j].num.Int64 {
			return rs[i].num.Int64 < rs[j].num.Int64
		}
		return rs[i].pos < rs[j].pos // stable fallback on existing order
	})
	ordered := make([]int64, len(rs))
	for i, r := range rs {
		ordered[i] = r.id
	}
	return s.applyOrder(ordered)
}

// applyOrder rewrites page positions to match the given ordered id slice.
func (s *store) applyOrder(orderedIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range orderedIDs {
		if _, err := tx.Exec(`UPDATE pages SET position = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// reorderPages sets explicit positions from a client-provided ordered id list,
// ignoring any ids that don't belong to the project.
func (s *store) reorderPages(projectID int64, orderedIDs []int64) error {
	valid, err := s.pageIDSet(projectID)
	if err != nil {
		return err
	}
	final := make([]int64, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if valid[id] {
			final = append(final, id)
			delete(valid, id)
		}
	}
	// Append any pages the client omitted, preserving them at the end.
	for id := range valid {
		final = append(final, id)
	}
	return s.applyOrder(final)
}

func (s *store) pageIDSet(projectID int64) (map[int64]bool, error) {
	rows, err := s.db.Query(`SELECT id FROM pages WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = true
	}
	return set, rows.Err()
}

func (s *store) setPageOCR(pageID int64, status, errMsg string) error {
	_, err := s.db.Exec(`UPDATE pages SET ocr_status = ?, ocr_error = ? WHERE id = ?`, status, errMsg, pageID)
	return err
}

func (s *store) getPageImagePath(pageID int64) (string, error) {
	var p string
	err := s.db.QueryRow(`SELECT image_path FROM pages WHERE id = ?`, pageID).Scan(&p)
	return p, err
}

// --- Paragraphs ---

func (s *store) paragraphsForPage(pageID int64) ([]Paragraph, error) {
	rows, err := s.db.Query(
		`SELECT id, page_id, position, text, audio_status, audio_path
		 FROM paragraphs WHERE page_id = ? ORDER BY position, id`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Paragraph
	for rows.Next() {
		var pa Paragraph
		if err := rows.Scan(&pa.ID, &pa.PageID, &pa.Position, &pa.Text, &pa.AudioStatus, &pa.AudioPath); err != nil {
			return nil, err
		}
		pa.HasAudio = pa.AudioStatus == "done" && pa.AudioPath != ""
		out = append(out, pa)
	}
	return out, rows.Err()
}

func (s *store) replaceParagraphs(pageID int64, texts []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM paragraphs WHERE page_id = ?`, pageID); err != nil {
		return err
	}
	for i, t := range texts {
		if _, err := tx.Exec(
			`INSERT INTO paragraphs (page_id, position, text) VALUES (?, ?, ?)`,
			pageID, i, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// updateParagraphText changes a paragraph's text and clears its cached audio so
// it is re-synthesized on next play.
func (s *store) updateParagraphText(id int64, text string) error {
	_, err := s.db.Exec(
		`UPDATE paragraphs SET text = ?, audio_status = 'none', audio_path = '', timestamps_json = '' WHERE id = ?`,
		text, id)
	return err
}

func (s *store) getParagraph(id int64) (*Paragraph, error) {
	var pa Paragraph
	err := s.db.QueryRow(
		`SELECT id, page_id, position, text, audio_status, audio_path
		 FROM paragraphs WHERE id = ?`, id).
		Scan(&pa.ID, &pa.PageID, &pa.Position, &pa.Text, &pa.AudioStatus, &pa.AudioPath)
	if err != nil {
		return nil, err
	}
	pa.HasAudio = pa.AudioStatus == "done" && pa.AudioPath != ""
	return &pa, nil
}

// getParagraphTimestamps returns the cached word timestamps JSON for a paragraph.
func (s *store) getParagraphTimestamps(id int64) (string, error) {
	var ts string
	err := s.db.QueryRow(`SELECT timestamps_json FROM paragraphs WHERE id = ?`, id).Scan(&ts)
	return ts, err
}

func (s *store) setParagraphAudio(id int64, audioPath string, timestamps []WordTS) error {
	tsJSON, _ := json.Marshal(timestamps)
	_, err := s.db.Exec(
		`UPDATE paragraphs SET audio_status = 'done', audio_path = ?, timestamps_json = ? WHERE id = ?`,
		audioPath, string(tsJSON), id)
	return err
}

func (s *store) setParagraphAudioError(id int64) error {
	_, err := s.db.Exec(`UPDATE paragraphs SET audio_status = 'error' WHERE id = ?`, id)
	return err
}

// projectVoiceSettings returns the default voice/speed/format for the project owning a paragraph.
func (s *store) projectSettingsForParagraph(paragraphID int64) (voice string, speed float64, format string, err error) {
	err = s.db.QueryRow(`
SELECT pr.voice, pr.speed, pr.format
FROM paragraphs pa
JOIN pages pg ON pg.id = pa.page_id
JOIN projects pr ON pr.id = pg.project_id
WHERE pa.id = ?`, paragraphID).Scan(&voice, &speed, &format)
	if err != nil {
		return "", 0, "", fmt.Errorf("paragraph %d: %w", paragraphID, err)
	}
	return voice, speed, format, nil
}
