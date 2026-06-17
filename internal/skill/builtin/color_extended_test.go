package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Color skill tests — 8 skills (hex↔rgb, hex↔hsl, contrast_ratio,
// color_blend, palette_analogous, palette_complementary).
// =====================================================================================

func TestHexToRGBSkill(t *testing.T) {
	skill := NewHexToRGBSkill()
	if !skill.Match("hex to rgb: #ff0000") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "hex to rgb: #ff0000")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "rgb(255, 0, 0)") {
		t.Errorf("red hex should map to rgb(255,0,0), got %q", out)
	}
}

func TestRGBToHexSkill(t *testing.T) {
	skill := NewRGBToHexSkill()
	if !skill.Match("rgb to hex: rgb(0, 128, 255)") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "rgb to hex: rgb(0, 128, 255)")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.EqualFold(out, "#0080FF") {
		t.Errorf("expected #0080FF, got %q", out)
	}
}

func TestHexToHSLSkill(t *testing.T) {
	skill := NewHexToHSLSkill()
	if !skill.Match("hex to hsl: #ff0000") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "hex to hsl: #ff0000")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Pure red = hsl(0, 100%, 50%)
	if !strings.Contains(out, "hsl(0,") || !strings.Contains(out, "100%") {
		t.Errorf("expected hsl(0, 100%%, 50%%), got %q", out)
	}
}

func TestHSLToHexSkill(t *testing.T) {
	skill := NewHSLToHexSkill()
	if !skill.Match("hsl to hex: hsl(0, 100%, 50%)") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "hsl to hex: hsl(0, 100%, 50%)")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.EqualFold(out, "#FF0000") {
		t.Errorf("hsl(0,100%%,50%%) is red #FF0000, got %q", out)
	}
}

func TestContrastRatioSkill(t *testing.T) {
	skill := NewContrastRatioSkill()
	if !skill.Match("contrast ratio between #fff #000") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "contrast ratio between #fff #000")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// White-on-black contrast = 21:1, AAA pass.
	if !strings.Contains(out, "ratio:") {
		t.Errorf("expected ratio field, got %q", out)
	}
	if !strings.Contains(out, "21") {
		t.Errorf("expected ratio ~21 for white/black, got %q", out)
	}
	if !strings.Contains(out, "AAA") {
		t.Errorf("expected AAA verdict for max contrast, got %q", out)
	}
}

func TestColorBlendSkill(t *testing.T) {
	skill := NewColorBlendSkill()
	if !skill.Match("blend #000 #fff 0.5") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "blend #000 #fff 0.5")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 50% blend of black + white = gray. Integer math gives 127 = 0x7F.
	up := strings.ToUpper(out)
	if !strings.Contains(up, "#7F7F7F") && !strings.Contains(up, "#808080") {
		t.Errorf("expected ~#7F7F7F or #808080 (gray), got %q", out)
	}
}

func TestPaletteAnalogousSkill(t *testing.T) {
	skill := NewPaletteAnalogousSkill()
	if !skill.Match("analogous palette: #ff0000") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "analogous palette: #ff0000")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	colors := strings.Fields(out)
	if len(colors) != 5 {
		t.Errorf("expected 5 analogous colors, got %d (%q)", len(colors), out)
	}
	// Middle color should be the input.
	if !strings.EqualFold(colors[2], "#FF0000") {
		t.Errorf("middle color should be the input #FF0000, got %q", colors[2])
	}
}

func TestPaletteComplementarySkill(t *testing.T) {
	skill := NewPaletteComplementarySkill()
	if !skill.Match("complementary palette: #ff0000") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "complementary palette: #ff0000")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "base:") {
		t.Errorf("expected 'base:' field, got %q", out)
	}
	if !strings.Contains(out, "complement:") {
		t.Errorf("expected 'complement:' field, got %q", out)
	}
	// Red's complement is cyan #00FFFF.
	if !strings.Contains(strings.ToUpper(out), "#00FFFF") {
		t.Errorf("expected complement #00FFFF, got %q", out)
	}
}
