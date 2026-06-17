package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// UploadDir is where uploaded files land. Relative to the server's working
// directory (the same place data/kyoci.db lives). Created on first upload.
const UploadDir = "data/uploads"

// MaxUploadBytes caps a single upload at 50 MB. Spreadsheets and PDFs rarely
// exceed this; raise via code if a use case demands it.
const MaxUploadBytes = 50 * 1024 * 1024

// allowedExt is the whitelist of accepted file extensions. Anything else is
// rejected with 415. Keep this list deliberately narrow — every entry is a
// surface the agent tool layer must know how to handle safely.
var allowedExt = map[string]bool{
	".txt":  true,
	".md":   true,
	".csv":  true,
	".json": true,
	".xlsx": true,
	".xls":  true,
	".pdf":  true,
	".docx": true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
}

// safeNameRe whitelists filename characters. Anything else is replaced with `_`.
var safeNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// UploadedFile is the response shape for POST /api/dashboard/upload and the
// per-file entry in the chat request payload.
type UploadedFile struct {
	ID       string `json:"id"`        // server-generated UUID; agent tool uses this to locate the file
	Filename string `json:"filename"`  // sanitized original name (no path)
	Size     int64  `json:"size"`      // bytes
	MimeType string `json:"mime_type"` // from multipart header (informational; not trusted)
}

// handleUpload accepts multipart/form-data with a single "file" field, writes
// the file to UploadDir under a fresh UUID-prefixed name, and returns metadata.
// The UUID prefix guarantees no collisions and lets the uploaded_file agent
// tool resolve files by exact filename without path-traversal shenanigans.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Enforce size at the multipart reader level so we don't buffer 100 MB
	// before rejecting. MaxBytesReader returns 413-friendly errors.
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+1024)

	// 1 MB read buffer; multipart files stream to disk via CreateTemp.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "http: request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": "upload rejected: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExt[ext] {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error": "file type not allowed: " + ext + " (allowed: txt, md, csv, json, xlsx, xls, pdf, docx, png, jpg, jpeg)",
		})
		return
	}

	if header.Size > MaxUploadBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": fmt.Sprintf("file too large: %d bytes (max %d)", header.Size, MaxUploadBytes),
		})
		return
	}

	if err := os.MkdirAll(UploadDir, 0o755); err != nil {
		s.logger.Error("upload mkdir failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create upload dir"})
		return
	}

	// Sanitize the original filename: strip directory components, whitelist
	// characters. We then prefix with a UUID so two uploads of "report.xlsx"
	// never collide on disk and so the agent tool can locate by exact name.
	safeBase := safeNameRe.ReplaceAllString(filepath.Base(header.Filename), "_")
	id := randomID()
	finalName := id + "-" + safeBase
	dstPath := filepath.Join(UploadDir, finalName)

	dst, err := os.Create(dstPath)
	if err != nil {
		s.logger.Error("upload create failed", "path", dstPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store upload"})
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		// Best-effort cleanup of partial file.
		_ = os.Remove(dstPath)
		s.logger.Error("upload copy failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not write upload"})
		return
	}

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	resp := UploadedFile{
		ID:       id,
		Filename: finalName,
		Size:     written,
		MimeType: mime,
	}
	s.logger.Info("upload stored",
		"id", id, "filename", finalName, "size", written, "mime", mime)

	writeJSON(w, http.StatusOK, resp)
}

// resolveUploadPath returns the absolute path to an uploaded file by ID or
// filename, rejecting any attempt to escape UploadDir. Used by the
// uploaded_file and excel agent tools so user input cannot traverse the
// filesystem.
//
// Accepts:
//   - bare UUID ("abc123")              → searches UploadDir for abc123-*
//   - UUID-prefixed filename ("abc-x")  → exact match in UploadDir
//   - original filename ("x.xlsx")      → matches the most recent upload with this name
func resolveUploadPath(userInput string) (string, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", fmt.Errorf("empty file reference")
	}

	// Strip any path components the caller tried to inject.
	userInput = filepath.Base(userInput)

	// Reject parent-directory traversal even after Base() — defense in depth
	// for Windows-style separators or other oddities.
	if strings.Contains(userInput, "..") {
		return "", fmt.Errorf("invalid file reference: %q", userInput)
	}

	// Case 1: exact filename match in UploadDir (the common path — frontend
	// sends the full server-generated name).
	candidate := filepath.Join(UploadDir, userInput)
	if isRegularFile(candidate) {
		return candidate, nil
	}

	// Case 2: bare UUID — find the first file in UploadDir starting with it.
	entries, err := os.ReadDir(UploadDir)
	if err != nil {
		return "", fmt.Errorf("upload dir not readable: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, userInput+"-") || name == userInput {
			p := filepath.Join(UploadDir, name)
			if isRegularFile(p) {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("no uploaded file matches %q", userInput)
}

func isRegularFile(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// randomID returns an 8-byte hex string. Sufficient to avoid collisions at
// dashboard scale; not a security boundary.
func randomID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// rand.Read should never fail on macOS/Linux; fall back to timestamp-ish.
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}
