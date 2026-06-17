package builtin

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// =====================================================================================
// Generator skill tests — 10 skills (uuid_v4/v7, nanoid, guid, random_int,
// random_string, random_bytes, nonce, fake_name, fake_email). Output is
// random — tests verify format/length, not specific values.
// =====================================================================================

func TestUUIDV4Skill(t *testing.T) {
	skill := NewUUIDV4Skill()
	if !skill.Match("generate uuid v4") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "generate uuid v4")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !uuidV4Re.MatchString(out) {
		t.Errorf("expected UUID v4 format, got %q", out)
	}
	// Two calls should produce different UUIDs.
	out2, _ := skill.Execute(context.Background(), "generate uuid v4")
	if out == out2 {
		t.Error("expected different UUIDs on successive calls")
	}
}

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUIDV7Skill(t *testing.T) {
	skill := NewUUIDV7Skill()
	if !skill.Match("generate uuid v7") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "generate uuid v7")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// v7 UUIDs have '7' as the first char of the third group.
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !re.MatchString(out) {
		t.Errorf("expected UUID v7 format, got %q", out)
	}
}

func TestNanoidSkill(t *testing.T) {
	skill := NewNanoidSkill()
	if !skill.Match("generate nanoid") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "generate nanoid")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out) != 21 {
		t.Errorf("default nanoid length is 21, got %d (%q)", len(out), out)
	}
	// Custom length.
	out2, _ := skill.Execute(context.Background(), "generate nanoid 32")
	if len(out2) != 32 {
		t.Errorf("custom nanoid length 32 expected, got %d (%q)", len(out2), out2)
	}
}

func TestGUIDSkill(t *testing.T) {
	skill := NewGUIDSkill()
	if !skill.Match("generate guid") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "generate guid")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Microsoft-style: {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX} upper.
	re := regexp.MustCompile(`^\{[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}\}$`)
	if !re.MatchString(out) {
		t.Errorf("expected braced uppercase GUID, got %q", out)
	}
}

func TestRandomIntSkill(t *testing.T) {
	skill := NewRandomIntSkill()
	if !skill.Match("random int 1-10") {
		t.Error("expected match")
	}
	for i := 0; i < 20; i++ {
		out, err := skill.Execute(context.Background(), "random int 1-10")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		n := parseIntStr(out)
		if n < 1 || n > 10 {
			t.Errorf("random int out of [1,10]: %d", n)
		}
	}
}

func TestRandomStringSkill(t *testing.T) {
	skill := NewRandomStringSkill()
	if !skill.Match("random string 16") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "random string 16")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out) != 16 {
		t.Errorf("expected 16 chars, got %d (%q)", len(out), out)
	}
	// Hex charset option (size goes at the end so parseIntSuffix catches it).
	out2, _ := skill.Execute(context.Background(), "random string hex 8")
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(out2) {
		t.Errorf("expected 8 hex chars, got %q", out2)
	}
}

func TestRandomBytesSkill(t *testing.T) {
	skill := NewRandomBytesSkill()
	if !skill.Match("random bytes 16") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "random bytes 16")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 16 bytes = 32 hex chars.
	if len(out) != 32 {
		t.Errorf("expected 32 hex chars for 16 bytes, got %d (%q)", len(out), out)
	}
	if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(out) {
		t.Errorf("expected hex output, got %q", out)
	}
}

func TestNonceSkill(t *testing.T) {
	skill := NewNonceSkill()
	if !skill.Match("generate nonce") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "generate nonce")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 16 random bytes → ~22 base64URL chars (no padding).
	if len(out) < 20 || len(out) > 24 {
		t.Errorf("expected ~22 chars, got %d (%q)", len(out), out)
	}
}

func TestFakeNameSkill(t *testing.T) {
	skill := NewFakeNameSkill()
	if !skill.Match("fake name") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "fake name")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Format: "Firstname Lastname"
	parts := strings.Fields(out)
	if len(parts) != 2 {
		t.Errorf("expected 'First Last', got %q", out)
	}
}

func TestFakeEmailSkill(t *testing.T) {
	skill := NewFakeEmailSkill()
	if !skill.Match("fake email") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "fake email")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "@") {
		t.Errorf("expected email format with @, got %q", out)
	}
}
