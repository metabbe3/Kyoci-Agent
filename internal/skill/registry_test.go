package skill

import (
	"context"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"

	"github.com/metabbe3/Kyoci-Agent/internal/skill/builtin"
)

// TestRegistry tests the registry functionality.
func TestRegistry(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	// Test registering a skill
	mathSkill := builtin.NewMathSkill()
	if err := registry.Register(mathSkill); err != nil {
		t.Fatalf("Failed to register math skill: %v", err)
	}

	// Test matching
	skill, ok := registry.Match("calculate 2+2")
	if !ok {
		t.Error("Expected match for math query")
	}
	if skill == nil {
		t.Fatal("Matched skill is nil")
	}

	// Test listing
	infos := registry.List()
	if len(infos) == 0 {
		t.Error("Expected at least one skill in list")
	}

	found := false
	for _, info := range infos {
		if info.Name == "math" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected math skill in list")
	}

	// Test executing by name
	result, err := registry.Execute(ctx, "math", "calculate 2+2")
	if err != nil {
		t.Errorf("Failed to execute math skill: %v", err)
	}
	if result == "" {
		t.Error("Expected non-empty result")
	}
}

// TestRegisterBuiltin tests registering all built-in skills.
//
// The catalog is grouped into ~12 categories totalling 240 skills. This
// test verifies (a) the total count meets the floor (>=100), (b) every
// category is present, and (c) one representative skill from each category
// is registered.
func TestRegisterBuiltin(t *testing.T) {
	registry := NewRegistry()

	if err := registry.RegisterBuiltin(); err != nil {
		t.Fatalf("Failed to register built-in skills: %v", err)
	}

	infos := registry.List()

	// Floor: catalog must ship 100+ skills.
	if len(infos) < 100 {
		t.Errorf("Expected ≥100 built-in skills, got %d", len(infos))
	}
	t.Logf("registered %d built-in skills", len(infos))

	// Original 20 — must all still be present.
	originals := []string{
		"math", "time", "hash", "uuid", "encode", "convert",
		"color", "regex", "jsonfmt", "sqlfmt", "diff",
		"jwt", "qr", "password", "charset", "cron",
		"subnet", "lorem", "markdown", "emojinfo",
	}
	for _, name := range originals {
		if !hasSkill(infos, name) {
			t.Errorf("Expected original skill %q not found", name)
		}
	}

	// Representative skills from each new category.
	categoryReps := []string{
		// encoding
		"base64_encode", "url_decode", "hex_decode", "unicode_escape",
		// hashing
		"md5", "sha256", "bcrypt_hash", "aes_encrypt",
		// security
		"password_strength", "secret_redact", "hash_identify", "cve_parse",
		// datafmt
		"yaml_to_json", "csv_to_json", "toml_to_json", "env_to_json",
		// text
		"slugify", "case_convert", "levenshtein", "regex_replace",
		// generators
		"uuid_v4", "uuid_v7", "nanoid", "random_string",
		// net
		"ip_validate", "mac_lookup", "dns_lookup", "cidr_merge",
		// color
		"hex_to_rgb", "contrast_ratio", "palette_complementary",
		// math
		"stats", "gcd", "is_prime", "factorial", "base_convert", "percentage",
		// time
		"now", "time_diff", "cron_next", "epoch_convert",
		// markdown
		"markdown_toc", "markdown_strip", "markdown_link_extract",
	}
	for _, name := range categoryReps {
		if !hasSkill(infos, name) {
			t.Errorf("Expected category skill %q not found", name)
		}
	}
}

// hasSkill reports whether infos contains a skill with the given name.
func hasSkill(infos []kyoci.SkillInfo, name string) bool {
	for _, info := range infos {
		if info.Name == name {
			return true
		}
	}
	return false
}

// TestMathSkill tests the math skill.
func TestMathSkill(t *testing.T) {
	mathSkill := builtin.NewMathSkill()
	ctx := context.Background()

	testCases := []struct {
		name     string
		query    string
		shouldMatch bool
		contains string
	}{
		{"simple addition", "calculate 2+2", true, "4.00"},
		{"subtraction", "calculate 10-3", true, "7.00"},
		{"multiplication", "calculate 5*6", true, "30.00"},
		{"division", "calculate 20/4", true, "5.00"},
		{"percentage", "what is 15% of 200", true, "30.00"},
		{"square root", "sqrt 144", true, "12.00"},
		{"exponent", "2 ^ 8", true, "256.00"},
		{"complex expression", "calculate (10+5)*3", true, "45.00"},
		{"no match", "hello world", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if mathSkill.Match(tc.query) != tc.shouldMatch {
				t.Errorf("Expected match=%v for query %q", tc.shouldMatch, tc.query)
			}

			if tc.shouldMatch {
				result, err := mathSkill.Execute(ctx, tc.query)
				if err != nil {
					t.Errorf("Execute failed: %v", err)
				}
				if tc.contains != "" && !contains(result, tc.contains) {
					t.Errorf("Expected result to contain %q, got %q", tc.contains, result)
				}
			}
		})
	}
}

// TestTimeSkill tests the (legacy) general time skill.
//
// After catalog expansion, the legacy skill DEFERS to specific time skills
// (now, time_parse, time_format, time_diff, cron_next, epoch_convert) when
// their tighter patterns fire. It still catches generic "current time" /
// "what time" / "today" phrasings.
func TestTimeSkill(t *testing.T) {
	timeSkill := builtin.NewTimeSkill()
	ctx := context.Background()

	testCases := []struct {
		name        string
		query       string
		shouldMatch bool
		contains    []string
	}{
		{"current time", "what time is it", true, []string{"Current time"}},
		{"current date", "current date", true, []string{"Current date"}},
		{"unix timestamp", "unix timestamp", true, []string{"Unix timestamp"}},
		// Bare "time" no longer matches — the legacy skill now requires a
		// more specific phrase to avoid shadowing the new time skills.
		{"no match for bare time", "time", false, []string{}},
		// "now" defers to NowSkill.
		{"now (deferred)", "now", false, []string{}},
		{"no match", "hello world", false, []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if timeSkill.Match(tc.query) != tc.shouldMatch {
				t.Errorf("Expected match=%v for query %q", tc.shouldMatch, tc.query)
			}

			if tc.shouldMatch {
				result, err := timeSkill.Execute(ctx, tc.query)
				if err != nil {
					t.Errorf("Execute failed: %v", err)
				}
				for _, exp := range tc.contains {
					if !contains(result, exp) {
						t.Errorf("Expected result to contain %q, got %q", exp, result)
					}
				}
			}
		})
	}
}

// TestHashSkill tests the (legacy) general hash skill.
//
// After the catalog expansion, the legacy hash skill DEFERS to the specific
// md5/sha1/sha256 skills for any query that names the algorithm explicitly.
// It now only matches generic "hash this" / "hash of" phrasings.
func TestHashSkill(t *testing.T) {
	hashSkill := builtin.NewHashSkill()
	ctx := context.Background()

	testCases := []struct {
		name        string
		query       string
		shouldMatch bool
		contains    string
	}{
		// Explicit algorithm names now defer to the specific skills.
		{"md5 (deferred to specific md5 skill)", "md5 hello", false, ""},
		{"sha1 (deferred)", "sha1 hello", false, ""},
		{"sha256 (deferred)", "sha256 hello", false, ""},
		// Generic hash phrasings still match the legacy skill.
		{"hash this", "hash this test", true, "MD5:"},
		{"hash of", "hash of test", true, "MD5:"},
		{"no match", "hello world", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if hashSkill.Match(tc.query) != tc.shouldMatch {
				t.Errorf("Expected match=%v for query %q", tc.shouldMatch, tc.query)
			}

			if tc.shouldMatch {
				result, err := hashSkill.Execute(ctx, tc.query)
				if err != nil {
					t.Errorf("Execute failed: %v", err)
				}
				if tc.contains != "" && !contains(result, tc.contains) {
					t.Errorf("Expected result to contain %q, got %q", tc.contains, result)
				}
			}
		})
	}
}

// TestUUIDSkill tests the (legacy) general UUID skill.
//
// After catalog expansion, the legacy skill DEFERS to uuid_v4 / uuid_v7 when
// the user specifies a version, and to the dedicated GUIDSkill for Microsoft-
// style GUIDs. It still matches generic "generate uuid" / "new uuid".
func TestUUIDSkill(t *testing.T) {
	uuidSkill := builtin.NewUUIDSkill()
	ctx := context.Background()

	testCases := []struct {
		name        string
		query       string
		shouldMatch bool
		contains    string
	}{
		{"generate uuid", "generate uuid", true, "UUID:"},
		{"new uuid", "new uuid", true, "UUID:"},
		{"random uuid", "random uuid", true, "UUID:"},
		// "guid" now defers to the dedicated GUIDSkill.
		{"guid (deferred)", "guid", false, ""},
		// Versioned UUIDs defer to the specific skills.
		{"uuid v4 (deferred)", "generate uuid v4", false, ""},
		{"uuid v7 (deferred)", "generate uuid v7", false, ""},
		{"no match", "hello world", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if uuidSkill.Match(tc.query) != tc.shouldMatch {
				t.Errorf("Expected match=%v for query %q", tc.shouldMatch, tc.query)
			}

			if tc.shouldMatch {
				result, err := uuidSkill.Execute(ctx, tc.query)
				if err != nil {
					t.Errorf("Execute failed: %v", err)
				}
				if tc.contains != "" && !contains(result, tc.contains) {
					t.Errorf("Expected result to contain %q, got %q", tc.contains, result)
				}
			}
		})
	}
}

// TestEncodeSkill tests the (legacy) general encode skill.
//
// After catalog expansion, the legacy skill DEFERS to the specific
// base64_encode/base64_decode/url_encode/url_decode/etc. skills for any
// encoding-specific phrasing. It still catches generic "encode"/"decode"
// without a specific prefix.
func TestEncodeSkill(t *testing.T) {
	encodeSkill := builtin.NewEncodeSkill()
	ctx := context.Background()

	testCases := []struct {
		name        string
		query       string
		shouldMatch bool
		contains    string
	}{
		// Specific encoding phrasings now defer to dedicated skills.
		{"base64 encode (deferred)", "base64 encode hello", false, ""},
		{"base64 decode (deferred)", "base64 decode aGVsbG8=", false, ""},
		{"url encode (deferred)", "url encode hello world", false, ""},
		{"url decode (deferred)", "url decode hello%20world", false, ""},
		// json format is still handled by the legacy skill (jsonfmt).
		{"json format", "json format {\"key\":\"value\"}", true, "Formatted JSON:"},
		{"no match", "hello world", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if encodeSkill.Match(tc.query) != tc.shouldMatch {
				t.Errorf("Expected match=%v for query %q", tc.shouldMatch, tc.query)
			}

			if tc.shouldMatch {
				result, err := encodeSkill.Execute(ctx, tc.query)
				if err != nil {
					t.Errorf("Execute failed: %v", err)
				}
				if tc.contains != "" && !contains(result, tc.contains) {
					t.Errorf("Expected result to contain %q, got %q", tc.contains, result)
				}
			}
		})
	}
}

// TestConvertSkill tests the unit conversion skill.
func TestConvertSkill(t *testing.T) {
	convertSkill := builtin.NewConvertSkill()
	ctx := context.Background()

	testCases := []struct {
		name        string
		query       string
		shouldMatch bool
		contains    string
	}{
		{"temperature f to c", "100 f to c", true, "f = "},
		{"temperature c to f", "0 c to f", true, "c = "},
		{"length km to mi", "1 km to mi", true, "km = "},
		{"length mi to km", "1 mi to km", true, "mi = "},
		{"weight kg to lb", "1 kg to lb", true, "kg = "},
		{"weight lb to kg", "1 lb to kg", true, "lb = "},
		{"storage gb to mb", "1 gb to mb", true, "gb = "},
		{"storage mb to gb", "1024 mb to gb", true, "mb = "},
		{"no match", "hello world", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if convertSkill.Match(tc.query) != tc.shouldMatch {
				t.Errorf("Expected match=%v for query %q", tc.shouldMatch, tc.query)
			}

			if tc.shouldMatch {
				result, err := convertSkill.Execute(ctx, tc.query)
				if err != nil {
					t.Errorf("Execute failed: %v", err)
				}
				if tc.contains != "" && !contains(result, tc.contains) {
					t.Errorf("Expected result to contain %q, got %q", tc.contains, result)
				}
			}
		})
	}
}

// TestNoMatch tests queries that shouldn't match any skill.
func TestNoMatch(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterBuiltin()

	queries := []string{
		"hello world",
		"what's the weather",
		"tell me a joke",
		"who are you",
		"write a story",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			skill, ok := registry.Match(query)
			if ok {
				t.Errorf("Unexpected match for query %q (matched skill: %s)", query, skill.Name())
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0)
}

// Simple substring search
func findSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestConcurrentAccess tests that the registry is thread-safe.
func TestConcurrentAccess(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterBuiltin()

	done := make(chan bool)

	// Launch goroutines to test concurrent access
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				registry.Match("calculate 2+2")
				registry.List()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to finish
	for i := 0; i < 10; i++ {
		<-done
	}
}