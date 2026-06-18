package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Path-manipulation skill tests — 10 skills (filepath_*, mime_*, path_is_*).
// All skills are pure string operations: no disk access, so behavior is identical
// across platforms EXCEPT for the OS-specific separator emitted by filepath.Join
// and filepath.Clean. Those assertions use filepath-agnostic expectations or
// match the runtime.GOOS-derived separator.
// =====================================================================================

// ---- filepath_normalize ----

func TestFilepathNormalizeSkill(t *testing.T) {
	runSkillCases(t, "filepath_normalize", NewFilepathNormalizeSkill(), []skillCase{
		{"positive: dots resolved", "filepath normalize: /foo/./bar/../baz", true, "/foo/baz", false},
		{"positive: trailing slash cleaned", "filepath normalize: /foo/bar/", true, "", false},
		{"positive: double sep", "filepath normalize: /foo//bar", true, "", false},
		{"positive: clean path alias", "clean path: /a/b/../c", true, "", false},
		{"negative: dir", "filepath dir: /foo/bar", false, "", false},
		{"negative: unrelated", "slugify hello world", false, "", false},
		{"edge: empty operand errors", "filepath normalize:", true, "", true},
	})
}

// ---- filepath_join ----

func TestFilepathJoinSkill(t *testing.T) {
	runSkillCases(t, "filepath_join", NewFilepathJoinSkill(), []skillCase{
		{"positive: three parts", "filepath join: /foo, bar, baz", true, "", false},
		{"positive: two parts", "filepath join: a, b", true, "", false},
		{"positive: single part", "filepath join: lone", true, "lone", false},
		{"positive: join path alias", "join path: a, b, c", true, "", false},
		{"positive: join paths alias", "join paths: x, y", true, "", false},
		{"positive: quoted component", `filepath join: "/foo bar", baz`, true, "", false},
		{"negative: dir verb", "filepath dir: /a/b", false, "", false},
		{"negative: unrelated", "uppercase hello", false, "", false},
		{"edge: empty operand errors", "filepath join:", true, "", true},
	})
}

// filepathJoinPartOk asserts each of the joined components appears in the output
// (separators differ by OS, so we look for the substrings rather than equality).
func filepathJoinPartOk(t *testing.T, out string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(out, p) {
			t.Errorf("join output %q missing part %q", out, p)
		}
	}
}

func TestFilepathJoinSkillOutput(t *testing.T) {
	s := NewFilepathJoinSkill()
	out, err := s.Execute(context.Background(), "filepath join: /foo, bar, baz")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	filepathJoinPartOk(t, out, "foo", "bar", "baz")
}

// ---- filepath_dir ----

func TestFilepathDirSkill(t *testing.T) {
	runSkillCases(t, "filepath_dir", NewFilepathDirSkill(), []skillCase{
		{"positive: with extension", "filepath dir: /foo/bar/baz.txt", true, "", false},
		{"positive: directory of alias", "directory of: /a/b/c", true, "", false},
		{"positive: dirname alias", "dirname: /x/y/z", true, "", false},
		{"negative: base verb", "filepath base: /a/b", false, "", false},
		{"negative: unrelated", "hash this string", false, "", false},
		{"edge: empty operand errors", "filepath dir:", true, "", true},
	})
}

func TestFilepathDirSkillOutput(t *testing.T) {
	s := NewFilepathDirSkill()
	out, err := s.Execute(context.Background(), "filepath dir: /foo/bar/baz.txt")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Both /foo/bar and \foo\bar contain "foo" and "bar" substrings.
	if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
		t.Errorf("Execute = %q, expected to contain foo and bar", out)
	}
}

// ---- filepath_base ----

func TestFilepathBaseSkill(t *testing.T) {
	runSkillCases(t, "filepath_base", NewFilepathBaseSkill(), []skillCase{
		{"positive: with extension", "filepath base: /foo/bar/baz.txt", true, "baz.txt", false},
		{"positive: basename alias", "basename: /a/b/c.txt", true, "c.txt", false},
		{"positive: base name of alias", "base name of: /x/y/z.png", true, "z.png", false},
		{"negative: dir verb", "filepath dir: /a/b", false, "", false},
		{"negative: unrelated", "encode this to base64", false, "", false},
		{"edge: empty operand errors", "filepath base:", true, "", true},
	})
}

// ---- filepath_ext ----

func TestFilepathExtSkill(t *testing.T) {
	runSkillCases(t, "filepath_ext", NewFilepathExtSkill(), []skillCase{
		{"positive: txt", "filepath ext: /foo/bar/baz.txt", true, ".txt", false},
		{"positive: extension of alias", "extension of: archive.tar.gz", true, ".gz", false},
		{"positive: file extension alias", "file extension: image.PNG", true, ".PNG", false},
		{"positive: no extension note", "filepath ext: /foo/bar", true, "(no extension)", false},
		{"negative: stem verb", "filepath stem: /a/b.txt", false, "", false},
		{"negative: unrelated", "convert to snake_case: helloWorld", false, "", false},
		{"edge: empty operand errors", "filepath ext:", true, "", true},
	})
}

// ---- filepath_stem ----

func TestFilepathStemSkill(t *testing.T) {
	runSkillCases(t, "filepath_stem", NewFilepathStemSkill(), []skillCase{
		{"positive: simple", "filepath stem: /foo/bar/baz.txt", true, "baz", false},
		{"positive: stem of alias", "stem of: /x/y/z.png", true, "z", false},
		{"positive: filename without ext alias", "filename without extension: /a/b/c.json", true, "c", false},
		{"positive: double extension keeps first", "filepath stem: archive.tar.gz", true, "archive.tar", false},
		{"positive: no extension", "filepath stem: /foo/bar/README", true, "README", false},
		{"negative: ext verb", "filepath ext: /a/b.txt", false, "", false},
		{"negative: unrelated", "base64 encode: hello", false, "", false},
		{"edge: empty operand errors", "filepath stem:", true, "", true},
	})
}

// ---- mime_from_ext ----

func TestMIMEFromExtSkill(t *testing.T) {
	runSkillCases(t, "mime_from_ext", NewMIMEFromExtSkill(), []skillCase{
		{"positive: txt with dot", "mime from ext: .txt", true, "text/plain", false},
		{"positive: txt without dot", "mime from ext: txt", true, "text/plain", false},
		{"positive: mime type for alias", "mime type for: .json", true, "application/json", false},
		{"positive: content type for alias", "content type for: .html", true, "text/html", false},
		{"positive: unknown falls back", "mime from ext: .zzzqqq", true, "application/octet-stream", false},
		{"negative: ext from mime verb", "ext from mime: text/plain", false, "", false},
		{"negative: unrelated", "url encode: hi there", false, "", false},
		{"edge: empty operand errors", "mime from ext:", true, "", true},
	})
}

// ---- ext_from_mime ----

func TestExtFromMIMESkill(t *testing.T) {
	runSkillCases(t, "ext_from_mime", NewExtFromMIMESkill(), []skillCase{
		{"positive: text/plain", "ext from mime: text/plain", true, ".txt", false},
		{"positive: application/json", "ext from mime: application/json", true, ".json", false},
		{"positive: with charset param", "ext from mime: text/plain; charset=utf-8", true, ".txt", false},
		{"positive: extension for mime alias", "extension for mime: image/png", true, ".png", false},
		{"positive: extensions for mime alias", "extensions for mime: video/mp4", true, ".mp4", false},
		{"positive: image/jpeg returns both", "ext from mime: image/jpeg", true, ".jpg", false},
		{"negative: mime from ext verb", "mime from ext: .txt", false, "", false},
		{"negative: unrelated", "sha256 of hello", false, "", false},
		{"edge: unknown MIME errors", "ext from mime: application/x-totally-fake", true, "", true},
		{"edge: empty operand errors", "ext from mime:", true, "", true},
	})
}

// ---- path_is_absolute ----

func TestPathIsAbsoluteSkill(t *testing.T) {
	runSkillCases(t, "path_is_absolute", NewPathIsAbsoluteSkill(), []skillCase{
		{"positive: posix absolute", "path is absolute: /foo", true, "true", false},
		{"positive: deeper absolute", "path is absolute: /foo/bar/baz", true, "true", false},
		{"positive: relative returns false", "path is absolute: foo/bar", true, "false", false},
		{"positive: is absolute path alias", "is absolute path: /x", true, "true", false},
		{"positive: absolute path check alias", "absolute path check: /etc/hosts", true, "true", false},
		{"negative: relative verb", "path is relative: /foo", false, "", false},
		{"negative: unrelated", "levenshtein distance foo bar", false, "", false},
		{"edge: empty operand errors", "path is absolute:", true, "", true},
	})
}

// ---- path_is_relative ----

func TestPathIsRelativeSkill(t *testing.T) {
	runSkillCases(t, "path_is_relative", NewPathIsRelativeSkill(), []skillCase{
		{"positive: relative true", "path is relative: foo/bar", true, "true", false},
		{"positive: bare name", "path is relative: lone", true, "true", false},
		{"positive: absolute returns false", "path is relative: /foo", true, "false", false},
		{"positive: is relative path alias", "is relative path: a/b", true, "true", false},
		{"positive: relative path check alias", "relative path check: ./x", true, "true", false},
		{"negative: absolute verb", "path is absolute: /foo", false, "", false},
		{"negative: unrelated", "slugify this text", false, "", false},
		{"edge: empty operand errors", "path is relative:", true, "", true},
	})
}
