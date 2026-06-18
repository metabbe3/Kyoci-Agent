package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Web / HTTP skill tests — 8 skills (user_agent_parse, url_query_extract,
// url_query_build, http_status_lookup, mime_boundary_generate,
// content_disposition_parse, etag_generate, range_parse).
// =====================================================================================

func TestUserAgentParseSkill(t *testing.T) {
	runSkillCases(t, "user_agent_parse", NewUserAgentParseSkill(), []skillCase{
		{
			name:        "chrome on macos",
			query:       "user agent parse: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			shouldMatch: true,
			want:        "Browser: Chrome 120.0.0.0",
			wantErr:     false,
		},
		{
			name:        "firefox on windows",
			query:       "parse user agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
			shouldMatch: true,
			want:        "Firefox",
			wantErr:     false,
		},
		{
			name:        "safari on iphone",
			query:       "user agent parse: Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			shouldMatch: true,
			want:        "Device: Mobile",
			wantErr:     false,
		},
		{
			name:        "negative: parse url",
			query:       "parse url: https://example.com",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
	})

	t.Run("chrome_macos_full_output", func(t *testing.T) {
		s := NewUserAgentParseSkill()
		out, err := s.Execute(context.Background(),
			"user agent parse: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		for _, want := range []string{"Chrome 120.0.0.0", "macOS 10.15.7", "Desktop"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q in output, got:\n%s", want, out)
			}
		}
	})

	t.Run("android_device_is_mobile", func(t *testing.T) {
		s := NewUserAgentParseSkill()
		out, err := s.Execute(context.Background(),
			"user agent parse: Mozilla/5.0 (Linux; Android 13; SM-S901B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "Android 13") {
			t.Errorf("expected Android 13 in output, got: %s", out)
		}
		if !strings.Contains(out, "Mobile") {
			t.Errorf("expected Mobile device, got: %s", out)
		}
	})
}

func TestURLQueryExtractSkill(t *testing.T) {
	runSkillCases(t, "url_query_extract", NewURLQueryExtractSkill(), []skillCase{
		{name: "two params", query: "url query extract: https://example.com/?foo=bar&baz=qux", shouldMatch: true, want: "foo=bar", wantErr: false},
		{name: "extract query synonym", query: "extract query: https://example.com/?a=1&b=2", shouldMatch: true, want: "b=2", wantErr: false},
		{name: "no query params", query: "url query extract: https://example.com/", shouldMatch: true, want: "", wantErr: true},
		{name: "url encoded value", query: "url query extract: https://example.com/?q=hello%20world", shouldMatch: true, want: "q=hello world", wantErr: false},
		{name: "negative: build url", query: "url query build: https://example.com/ | a=1", shouldMatch: false, want: "", wantErr: false},
	})
}

func TestURLQueryBuildSkill(t *testing.T) {
	runSkillCases(t, "url_query_build", NewURLQueryBuildSkill(), []skillCase{
		{name: "pipe separated params", query: "url query build: https://example.com/ | foo=bar, baz=qux", shouldMatch: true, want: "https://example.com/?baz=qux&foo=bar", wantErr: false},
		{name: "single param", query: "url query build: https://example.com/path | name=value", shouldMatch: true, want: "https://example.com/path?name=value", wantErr: false},
		{name: "negative: extract query", query: "url query extract: https://example.com/?a=1", shouldMatch: false, want: "", wantErr: false},
	})

	t.Run("alphabetized_output", func(t *testing.T) {
		s := NewURLQueryBuildSkill()
		out, err := s.Execute(context.Background(),
			"url query build: https://example.com/ | zebra=1, apple=2, mango=3")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "https://example.com/?apple=2&mango=3&zebra=1"
		if out != want {
			t.Errorf("got %q, want %q", out, want)
		}
	})

	t.Run("preserves_path", func(t *testing.T) {
		s := NewURLQueryBuildSkill()
		out, err := s.Execute(context.Background(),
			"url query build: https://api.example.com/v1/users | q=hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.HasPrefix(out, "https://api.example.com/v1/users?") {
			t.Errorf("unexpected URL shape: %s", out)
		}
	})
}

func TestHTTPStatusLookupSkill(t *testing.T) {
	runSkillCases(t, "http_status_lookup", NewHTTPStatusLookupSkill(), []skillCase{
		{name: "404", query: "http status: 404", shouldMatch: true, want: "404 Not Found (Client Error)", wantErr: false},
		{name: "200", query: "http status: 200", shouldMatch: true, want: "200 OK (Success)", wantErr: false},
		{name: "301", query: "http status: 301", shouldMatch: true, want: "301 Moved Permanently (Redirection)", wantErr: false},
		{name: "500", query: "http status: 500", shouldMatch: true, want: "500 Internal Server Error (Server Error)", wantErr: false},
		{name: "429", query: "http status: 429", shouldMatch: true, want: "429 Too Many Requests", wantErr: false},
		{name: "100", query: "http status: 100", shouldMatch: true, want: "100 Continue (Informational)", wantErr: false},
		{name: "418 teapot", query: "http status: 418", shouldMatch: true, want: "I'm a Teapot", wantErr: false},
		{name: "lookup synonym", query: "http status lookup 503", shouldMatch: true, want: "Service Unavailable", wantErr: false},
		{name: "status code synonym", query: "status code 422", shouldMatch: true, want: "Unprocessable Entity", wantErr: false},
		{name: "unknown code in range", query: "http status: 299", shouldMatch: true, want: "Unknown Status", wantErr: false},
		{name: "no code", query: "http status: nothing", shouldMatch: true, want: "", wantErr: true},
		{name: "negative: etag", query: "etag: hello world", shouldMatch: false, want: "", wantErr: false},
	})
}

func TestMIMEBoundaryGenerateSkill(t *testing.T) {
	s := NewMIMEBoundaryGenerateSkill()

	t.Run("match", func(t *testing.T) {
		if !s.Match("mime boundary generate") {
			t.Error("expected match for 'mime boundary generate'")
		}
		if !s.Match("boundary generate") {
			t.Error("expected match for 'boundary generate'")
		}
		if s.Match("generate uuid") {
			t.Error("did not expect match for unrelated query")
		}
	})

	t.Run("boundary_format", func(t *testing.T) {
		out, err := s.Execute(context.Background(), "mime boundary generate")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(out) != 30 {
			t.Errorf("expected 30-char boundary, got %d (%q)", len(out), out)
		}
		if !strings.HasPrefix(out, "--") {
			t.Errorf("expected leading dashes, got %q", out)
		}
		body := out[2:]
		for _, r := range body {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				t.Errorf("non-alphanumeric char %q in boundary %q", r, out)
			}
		}
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := map[string]bool{}
		for i := 0; i < 50; i++ {
			out, err := s.Execute(context.Background(), "mime boundary generate")
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if seen[out] {
				t.Fatalf("collision after %d runs: %s", i+1, out)
			}
			seen[out] = true
		}
	})
}

func TestContentDispositionParseSkill(t *testing.T) {
	runSkillCases(t, "content_disposition_parse", NewContentDispositionParseSkill(), []skillCase{
		{name: "attachment with filename", query: `content disposition parse: attachment; filename="example.csv"`, shouldMatch: true, want: "Type: attachment\nFilename: example.csv", wantErr: false},
		{name: "inline no filename", query: "content disposition parse: inline", shouldMatch: true, want: "Type: inline", wantErr: false},
		{name: "form-data", query: `content disposition parse: form-data; name="file"`, shouldMatch: true, want: "Type: form-data", wantErr: false},
		{name: "negative: range parse", query: "range parse: bytes=0-499", shouldMatch: false, want: "", wantErr: false},
	})

	t.Run("filename_star_ext_value", func(t *testing.T) {
		s := NewContentDispositionParseSkill()
		out, err := s.Execute(context.Background(),
			"content disposition parse: attachment; filename*=utf-8''na%C3%AFve.txt")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "Filename: naïve.txt") {
			t.Errorf("expected decoded Filename: naïve.txt in output, got: %s", out)
		}
	})
}

func TestETagGenerateSkill(t *testing.T) {
	s := NewETagGenerateSkill()

	t.Run("match", func(t *testing.T) {
		if !s.Match("etag: hello world") {
			t.Error("expected match for 'etag:'")
		}
		if !s.Match("generate etag") {
			t.Error("expected match for 'generate etag'")
		}
		if s.Match("http status 200") {
			t.Error("did not expect match for unrelated query")
		}
	})

	t.Run("deterministic_quoted_hex", func(t *testing.T) {
		out1, err := s.Execute(context.Background(), "etag: hello world")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		out2, err := s.Execute(context.Background(), "etag: hello world")
		if err != nil {
			t.Fatalf("Execute second call: %v", err)
		}
		if out1 != out2 {
			t.Errorf("ETag must be deterministic: %q vs %q", out1, out2)
		}
		if len(out1) != 34 {
			t.Errorf("expected 34-char output (quote + 32 hex + quote), got %d: %q", len(out1), out1)
		}
		if !strings.HasPrefix(out1, "\"") || !strings.HasSuffix(out1, "\"") {
			t.Errorf("ETag must be quoted: %q", out1)
		}
		body := out1[1 : len(out1)-1]
		for _, r := range body {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				t.Errorf("non-hex char %q in ETag body %q", r, body)
			}
		}
	})

	t.Run("different_inputs_differ", func(t *testing.T) {
		a, _ := s.Execute(context.Background(), "etag: foo")
		b, _ := s.Execute(context.Background(), "etag: bar")
		if a == b {
			t.Errorf("ETags for different inputs must differ: %q", a)
		}
	})

	t.Run("empty_input_errors", func(t *testing.T) {
		_, err := s.Execute(context.Background(), "etag")
		if err == nil {
			t.Error("expected error for empty ETag input")
		}
	})
}

func TestRangeParseSkill(t *testing.T) {
	runSkillCases(t, "range_parse", NewRangeParseSkill(), []skillCase{
		{name: "closed range", query: "range parse: bytes=0-499", shouldMatch: true, want: "Start: 0\nEnd: 499", wantErr: false},
		{name: "suffix range", query: "range parse: bytes=-500", shouldMatch: true, want: "Start: 500 (suffix)", wantErr: false},
		{name: "open ended", query: "range parse: bytes=500-", shouldMatch: true, want: "Start: 500\nEnd: EOF", wantErr: false},
		{name: "http range synonym", query: "http range: bytes=100-200", shouldMatch: true, want: "Start: 100\nEnd: 200", wantErr: false},
		{name: "with Range prefix", query: "range parse: Range: bytes=0-99", shouldMatch: true, want: "Start: 0\nEnd: 99", wantErr: false},
		{name: "invalid range start>end", query: "range parse: bytes=500-100", shouldMatch: true, want: "", wantErr: true},
		{name: "non-bytes unit errors", query: "range parse: items=0-499", shouldMatch: true, want: "", wantErr: true},
		{name: "negative: etag query", query: "etag: bytes=0-499", shouldMatch: false, want: "", wantErr: false},
	})
}
