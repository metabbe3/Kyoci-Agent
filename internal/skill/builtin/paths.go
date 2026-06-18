package builtin

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Path-manipulation skills — pure string operations on paths. NO filesystem access.
// These skills clean, join, split, inspect, and classify path strings using only
// the standard library (path/filepath, mime). Nothing here touches disk.
//
// Covered:
//   - filepath_normalize / filepath_join / filepath_dir / filepath_base
//   - filepath_ext / filepath_stem
//   - mime_from_ext / ext_from_mime
//   - path_is_absolute / path_is_relative
// =====================================================================================

// extractPathOperand pulls the path operand out of a natural-language query.
// It first tries the standard colon split (so "filepath base: /x/y" → "/x/y").
// If the verb has no trailing colon (e.g. "base name of /x/y") it falls back
// to stripVerb for each of the given verb/alias candidates. Quotes are removed.
// Returns "" when no operand is present.
func extractPathOperand(q string, verbs ...string) string {
	// Primary: colon-delimited payload. extractPayload already trims and returns
	// "" if nothing follows the colon.
	if p := quoteStripped(extractPayload(q)); p != "" {
		return p
	}
	// Fallback: strip any of the verb aliases and use what remains.
	for _, v := range verbs {
		if p := quoteStripped(stripVerb(q, v)); p != "" && p != ":" {
			return p
		}
	}
	return ""
}

// ---- filepath_normalize ----

type FilepathNormalizeSkill struct{ *kyoci.BaseSkill }

func NewFilepathNormalizeSkill() *FilepathNormalizeSkill {
	return &FilepathNormalizeSkill{BaseSkill: kyoci.NewBaseSkill(
		"filepath_normalize", "Clean a path: collapse separators, resolve . and ..",
		[]string{"filepath normalize", "normalize path", "clean path"},
	)}
}

func (s *FilepathNormalizeSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "filepath normalize") ||
		strings.Contains(low, "normalize filepath") ||
		strings.Contains(low, "clean path")
}

func (s *FilepathNormalizeSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPathOperand(q, "filepath normalize", "normalize filepath", "clean path")
	if in == "" {
		return "", fmt.Errorf("no path to normalize")
	}
	return filepath.Clean(in), nil
}

// ---- filepath_join ----

type FilepathJoinSkill struct{ *kyoci.BaseSkill }

func NewFilepathJoinSkill() *FilepathJoinSkill {
	return &FilepathJoinSkill{BaseSkill: kyoci.NewBaseSkill(
		"filepath join", "Join path components with the OS-correct separator",
		[]string{"filepath join", "join path", "join paths"},
	)}
}

func (s *FilepathJoinSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "filepath join") ||
		strings.Contains(low, "join filepath") ||
		strings.Contains(low, "join path") ||
		strings.Contains(low, "join paths")
}

func (s *FilepathJoinSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPathOperand(q, "filepath join", "join filepath", "join path", "join paths")
	if payload == "" {
		return "", fmt.Errorf("no path components to join")
	}
	// Components are comma-separated. Each may carry surrounding whitespace/quotes.
	rawParts := strings.Split(payload, ",")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		cleaned := strings.TrimSpace(quoteStripped(p))
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("no non-empty path components to join")
	}
	return filepath.Join(parts...), nil
}

// ---- filepath_dir ----

type FilepathDirSkill struct{ *kyoci.BaseSkill }

func NewFilepathDirSkill() *FilepathDirSkill {
	return &FilepathDirSkill{BaseSkill: kyoci.NewBaseSkill(
		"filepath_dir", "Return the directory portion of a path",
		[]string{"filepath dir", "directory of", "dirname"},
	)}
}

func (s *FilepathDirSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "filepath dir") ||
		strings.Contains(low, "directory of") ||
		strings.Contains(low, "dirname")
}

func (s *FilepathDirSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "filepath dir"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "directory of"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "dirname"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no path given")
	}
	return filepath.Dir(in), nil
}

// ---- filepath_base ----

type FilepathBaseSkill struct{ *kyoci.BaseSkill }

func NewFilepathBaseSkill() *FilepathBaseSkill {
	return &FilepathBaseSkill{BaseSkill: kyoci.NewBaseSkill(
		"filepath_base", "Return the base (last) element of a path",
		[]string{"filepath base", "basename", "base name of"},
	)}
}

func (s *FilepathBaseSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "filepath base") ||
		strings.Contains(low, "basename") ||
		strings.Contains(low, "base name of")
}

func (s *FilepathBaseSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "filepath base"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "basename"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "base name of"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no path given")
	}
	return filepath.Base(in), nil
}

// ---- filepath_ext ----

type FilepathExtSkill struct{ *kyoci.BaseSkill }

func NewFilepathExtSkill() *FilepathExtSkill {
	return &FilepathExtSkill{BaseSkill: kyoci.NewBaseSkill(
		"filepath_ext", "Return the file extension of a path (including the dot)",
		[]string{"filepath ext", "extension of", "file extension"},
	)}
}

func (s *FilepathExtSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "filepath ext") ||
		strings.Contains(low, "extension of") ||
		strings.Contains(low, "file extension")
}

func (s *FilepathExtSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "filepath ext"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "extension of"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "file extension"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no path given")
	}
	ext := filepath.Ext(in)
	if ext == "" {
		return "(no extension)", nil
	}
	return ext, nil
}

// ---- filepath_stem ----

type FilepathStemSkill struct{ *kyoci.BaseSkill }

func NewFilepathStemSkill() *FilepathStemSkill {
	return &FilepathStemSkill{BaseSkill: kyoci.NewBaseSkill(
		"filepath_stem", "Return the base name without its extension",
		[]string{"filepath stem", "stem of", "filename without extension"},
	)}
}

func (s *FilepathStemSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "filepath stem") ||
		strings.Contains(low, "stem of") ||
		strings.Contains(low, "filename without extension")
}

func (s *FilepathStemSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "filepath stem"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "stem of"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "filename without extension"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no path given")
	}
	base := filepath.Base(in)
	// strings.TrimSuffix removes only the final extension. For a tarball like
	// "archive.tar.gz" the stem is "archive.tar" — matches Python's pathlib.
	return strings.TrimSuffix(base, filepath.Ext(base)), nil
}

// ---- mime_from_ext ----

type MIMEFromExtSkill struct{ *kyoci.BaseSkill }

func NewMIMEFromExtSkill() *MIMEFromExtSkill {
	return &MIMEFromExtSkill{BaseSkill: kyoci.NewBaseSkill(
		"mime_from_ext", "Look up the MIME type for a file extension",
		[]string{"mime from ext", "mime type for", "content type for"},
	)}
}

func (s *MIMEFromExtSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "mime from ext") ||
		strings.Contains(low, "mime type for") ||
		strings.Contains(low, "content type for")
}

func (s *MIMEFromExtSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "mime from ext"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "mime type for"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "content type for"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no extension given")
	}
	// Ensure a leading dot for the lookup; "txt" and ".txt" both work.
	ext := strings.TrimSpace(in)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	mt := mime.TypeByExtension(ext)
	if mt == "" {
		return "application/octet-stream", nil
	}
	return mt, nil
}

// ---- ext_from_mime ----

// mimeToExts maps the common MIME types the brief calls out to their canonical
// file extensions. The first entry of each list is the most common / preferred.
var mimeToExts = map[string][]string{
	"text/plain":          {".txt", ".text"},
	"text/html":           {".html", ".htm"},
	"text/css":            {".css"},
	"text/javascript":     {".js"},
	"application/json":    {".json"},
	"image/png":           {".png"},
	"image/jpeg":          {".jpg", ".jpeg"},
	"image/gif":           {".gif"},
	"image/svg+xml":       {".svg"},
	"application/pdf":     {".pdf"},
	"audio/mpeg":          {".mp3"},
	"video/mp4":           {".mp4"},
	"text/csv":            {".csv"},
	"application/xml":     {".xml"},
	"application/yaml":    {".yaml", ".yml"},
	"application/x-yaml":  {".yaml", ".yml"},
	"application/zip":     {".zip"},
	"application/gzip":    {".gz"},
	"application/x-gzip":  {".gz"},
	"application/x-tar":   {".tar"},
}

type ExtFromMIMESkill struct{ *kyoci.BaseSkill }

func NewExtFromMIMESkill() *ExtFromMIMESkill {
	return &ExtFromMIMESkill{BaseSkill: kyoci.NewBaseSkill(
		"ext_from_mime", "Return common file extensions for a MIME type",
		[]string{"ext from mime", "extension for mime", "extensions for mime"},
	)}
}

func (s *ExtFromMIMESkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "ext from mime") ||
		strings.Contains(low, "extension for mime") ||
		strings.Contains(low, "extensions for mime")
}

func (s *ExtFromMIMESkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "ext from mime"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "extension for mime"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "extensions for mime"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no MIME type given")
	}
	// Strip any "; charset=..." parameter so "text/plain; charset=utf-8" also matches.
	mt := strings.TrimSpace(strings.ToLower(in))
	if i := strings.Index(mt, ";"); i >= 0 {
		mt = strings.TrimSpace(mt[:i])
	}
	exts, ok := mimeToExts[mt]
	if !ok {
		return "", fmt.Errorf("no known extension for MIME type %q", mt)
	}
	return strings.Join(exts, ", "), nil
}

// ---- path_is_absolute ----

type PathIsAbsoluteSkill struct{ *kyoci.BaseSkill }

func NewPathIsAbsoluteSkill() *PathIsAbsoluteSkill {
	return &PathIsAbsoluteSkill{BaseSkill: kyoci.NewBaseSkill(
		"path_is_absolute", "Check whether a path is absolute",
		[]string{"path is absolute", "is absolute path", "absolute path check"},
	)}
}

func (s *PathIsAbsoluteSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "path is absolute") ||
		strings.Contains(low, "is absolute path") ||
		strings.Contains(low, "absolute path check")
}

func (s *PathIsAbsoluteSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "path is absolute"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "is absolute path"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "absolute path check"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no path given")
	}
	if filepath.IsAbs(in) {
		return "true", nil
	}
	return "false", nil
}

// ---- path_is_relative ----

type PathIsRelativeSkill struct{ *kyoci.BaseSkill }

func NewPathIsRelativeSkill() *PathIsRelativeSkill {
	return &PathIsRelativeSkill{BaseSkill: kyoci.NewBaseSkill(
		"path_is_relative", "Check whether a path is relative (i.e. not absolute)",
		[]string{"path is relative", "is relative path", "relative path check"},
	)}
}

func (s *PathIsRelativeSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "path is relative") ||
		strings.Contains(low, "is relative path") ||
		strings.Contains(low, "relative path check")
}

func (s *PathIsRelativeSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(stripVerb(q, "path is relative"))
	if in == "" {
		in = quoteStripped(stripVerb(q, "is relative path"))
	}
	if in == "" {
		in = quoteStripped(stripVerb(q, "relative path check"))
	}
	if in == "" {
		in = quoteStripped(extractPayload(q))
	}
	if in == "" {
		return "", fmt.Errorf("no path given")
	}
	if filepath.IsAbs(in) {
		return "false", nil
	}
	return "true", nil
}
