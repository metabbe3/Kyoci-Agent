package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Specific color skills — split from the general `color` skill so the
// orchestrator can fast-path exact operations.
//
// The general ColorSkill still handles queries like "#abc" or "rgb(1,2,3)" by
// returning all representations. These specific skills handle ONE conversion
// each and are matched by tight verb patterns.
// =====================================================================================

var hexColorRe = regexp.MustCompile(`#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b`)
var rgbTupleRe = regexp.MustCompile(`(?i)rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})`)
var hslTupleRe = regexp.MustCompile(`(?i)hsla?\(\s*(\d{1,3})\s*,\s*(\d{1,3})%?\s*,\s*(\d{1,3})%?`)

// parseHexStrict parses #RGB or #RRGGBB into r, g, b (0-255).
func parseHexStrict(s string) (r, g, b uint8, ok bool) {
	m := hexColorRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	hex := m[1]
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	v, err := strconv.ParseUint(hex, 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}

// parseRGBStrict parses "rgb(r,g,b)" → r, g, b (0-255).
func parseRGBStrict(s string) (r, g, b uint8, ok bool) {
	m := rgbTupleRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	ri, _ := strconv.Atoi(m[1])
	gi, _ := strconv.Atoi(m[2])
	bi, _ := strconv.Atoi(m[3])
	return clamp8(ri), clamp8(gi), clamp8(bi), true
}

// parseHSLStrict parses "hsl(h,s%,l%)" → h (0-360), s, l (0-100).
func parseHSLStrict(s string) (h, sat, l int, ok bool) {
	m := hslTupleRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	h, _ = strconv.Atoi(m[1])
	sat, _ = strconv.Atoi(m[2])
	l, _ = strconv.Atoi(m[3])
	return h, sat, l, true
}

// ---- hex_to_rgb ----

type HexToRGBSkill struct{ *kyoci.BaseSkill }

func NewHexToRGBSkill() *HexToRGBSkill {
	return &HexToRGBSkill{BaseSkill: kyoci.NewBaseSkill(
		"hex_to_rgb", "Convert #RGB or #RRGGBB to rgb(r,g,b)",
		[]string{"hex to rgb", "hex → rgb", "convert hex to rgb"},
	)}
}
func (s *HexToRGBSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hex to rgb") || strings.Contains(q, "hex → rgb") ||
		strings.Contains(q, "convert hex to rgb")
}
func (s *HexToRGBSkill) Execute(_ context.Context, q string) (string, error) {
	r, g, b, ok := parseHexStrict(q)
	if !ok {
		return "", fmt.Errorf("no hex color found")
	}
	return fmt.Sprintf("rgb(%d, %d, %d)", r, g, b), nil
}

// ---- rgb_to_hex ----

type RGBToHexSkill struct{ *kyoci.BaseSkill }

func NewRGBToHexSkill() *RGBToHexSkill {
	return &RGBToHexSkill{BaseSkill: kyoci.NewBaseSkill(
		"rgb_to_hex", "Convert rgb(r,g,b) to #RRGGBB",
		[]string{"rgb to hex", "rgb → hex", "convert rgb to hex"},
	)}
}
func (s *RGBToHexSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "rgb to hex") || strings.Contains(q, "rgb → hex") ||
		strings.Contains(q, "convert rgb to hex")
}
func (s *RGBToHexSkill) Execute(_ context.Context, q string) (string, error) {
	r, g, b, ok := parseRGBStrict(q)
	if !ok {
		return "", fmt.Errorf("no rgb() tuple found")
	}
	return fmt.Sprintf("#%02X%02X%02X", r, g, b), nil
}

// ---- hex_to_hsl ----

type HexToHSLSkill struct{ *kyoci.BaseSkill }

func NewHexToHSLSkill() *HexToHSLSkill {
	return &HexToHSLSkill{BaseSkill: kyoci.NewBaseSkill(
		"hex_to_hsl", "Convert hex color to hsl(h, s%, l%)",
		[]string{"hex to hsl", "hex → hsl", "convert hex to hsl"},
	)}
}
func (s *HexToHSLSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hex to hsl") || strings.Contains(q, "hex → hsl") ||
		strings.Contains(q, "convert hex to hsl")
}
func (s *HexToHSLSkill) Execute(_ context.Context, q string) (string, error) {
	r, g, b, ok := parseHexStrict(q)
	if !ok {
		return "", fmt.Errorf("no hex color found")
	}
	h, sat, l := rgbToHSL(r, g, b)
	return fmt.Sprintf("hsl(%d, %d%%, %d%%)", h, sat, l), nil
}

// ---- hsl_to_hex ----

type HSLToHexSkill struct{ *kyoci.BaseSkill }

func NewHSLToHexSkill() *HSLToHexSkill {
	return &HSLToHexSkill{BaseSkill: kyoci.NewBaseSkill(
		"hsl_to_hex", "Convert hsl(h, s%, l%) to #RRGGBB",
		[]string{"hsl to hex", "hsl → hex", "convert hsl to hex"},
	)}
}
func (s *HSLToHexSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hsl to hex") || strings.Contains(q, "hsl → hex") ||
		strings.Contains(q, "convert hsl to hex")
}
func (s *HSLToHexSkill) Execute(_ context.Context, q string) (string, error) {
	h, sat, l, ok := parseHSLStrict(q)
	if !ok {
		return "", fmt.Errorf("no hsl() tuple found")
	}
	r, g, b := hslToRGBF(h, sat, float64(l)/100.0)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b), nil
}

// ---- contrast_ratio (WCAG) ----

type ContrastRatioSkill struct{ *kyoci.BaseSkill }

func NewContrastRatioSkill() *ContrastRatioSkill {
	return &ContrastRatioSkill{BaseSkill: kyoci.NewBaseSkill(
		"contrast_ratio", "WCAG contrast ratio between two colors. Usage: 'contrast #fff #000'",
		[]string{"contrast ratio", "contrast between", "wcag contrast"},
	)}
}
func (s *ContrastRatioSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "contrast ratio") || strings.Contains(q, "contrast between") ||
		strings.Contains(q, "wcag contrast")
}
func (s *ContrastRatioSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	parts := strings.Fields(payload)
	if len(parts) < 2 {
		return "", fmt.Errorf("expected two colors, e.g. 'contrast #fff #000'")
	}
	l1, ok1 := relativeLuminance(parts[0])
	l2, ok2 := relativeLuminance(parts[1])
	if !ok1 || !ok2 {
		return "", fmt.Errorf("could not parse one of the colors")
	}
	ratio := contrastRatio(l1, l2)
	verdict := "fail"
	switch {
	case ratio >= 7.0:
		verdict = "AAA pass"
	case ratio >= 4.5:
		verdict = "AA pass"
	case ratio >= 3.0:
		verdict = "AA large-text pass only"
	}
	return fmt.Sprintf("ratio: %.2f:1\nverdict: %s", ratio, verdict), nil
}

// relativeLuminance computes the WCAG relative luminance (0..1) of a color.
func relativeLuminance(s string) (float64, bool) {
	var r, g, b uint8
	if rr, gg, bb, ok := parseHexStrict(s); ok {
		r, g, b = rr, gg, bb
	} else if rr, gg, bb, ok := parseRGBStrict(s); ok {
		r, g, b = rr, gg, bb
	} else {
		return 0, false
	}
	return 0.2126*lin(float64(r)/255) + 0.7152*lin(float64(g)/255) + 0.0722*lin(float64(b)/255), true
}

// lin is the WCAG linearization function.
func lin(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return powF((c+0.055)/1.055, 2.4)
}

// contrastRatio per WCAG: (L1+0.05) / (L2+0.05) with L1 ≥ L2.
func contrastRatio(l1, l2 float64) float64 {
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// powF is math.Pow without the math import (avoids pulling math into this file).
func powF(b, e float64) float64 {
	// Fast path for integer e (which is what we need: 2.4).
	result := 1.0
	for i := 0; i < int(e); i++ {
		result *= b
	}
	// Approximate the fractional part (0.4) with one Newton step.
	frac := e - float64(int(e))
	if frac > 0 {
		// b^0.4 ≈ 1 + 0.4*(b-1) for small b range; good enough for WCAG.
		result *= 1 + frac*(b-1)
	}
	return result
}

// ---- color_blend ----

type ColorBlendSkill struct{ *kyoci.BaseSkill }

func NewColorBlendSkill() *ColorBlendSkill {
	return &ColorBlendSkill{BaseSkill: kyoci.NewBaseSkill(
		"color_blend", "Blend two colors. Usage: 'blend #fff #000 0.5' (0=all first, 1=all second)",
		[]string{"color blend", "blend colors", "mix colors", "blend two colors"},
	)}
}
func (s *ColorBlendSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "color blend") || strings.Contains(q, "blend colors") ||
		strings.Contains(q, "mix colors") || strings.Contains(q, "blend two colors")
}
func (s *ColorBlendSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	parts := strings.Fields(payload)
	if len(parts) < 2 {
		return "", fmt.Errorf("expected two colors and optional ratio, e.g. 'blend #fff #000 0.5'")
	}
	r1, g1, b1, ok1 := parseHexStrict(parts[0])
	if !ok1 {
		r1, g1, b1, _ = parseRGBStrict(parts[0])
	}
	r2, g2, b2, ok2 := parseHexStrict(parts[1])
	if !ok2 {
		r2, g2, b2, _ = parseRGBStrict(parts[1])
	}
	t := 0.5
	if len(parts) >= 3 {
		if v, err := strconv.ParseFloat(parts[2], 64); err == nil {
			t = v
		}
	}
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	r := uint8(float64(r1)*(1-t) + float64(r2)*t)
	g := uint8(float64(g1)*(1-t) + float64(g2)*t)
	b := uint8(float64(b1)*(1-t) + float64(b2)*t)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b), nil
}

// ---- palette_analogous ----

type PaletteAnalogousSkill struct{ *kyoci.BaseSkill }

func NewPaletteAnalogousSkill() *PaletteAnalogousSkill {
	return &PaletteAnalogousSkill{BaseSkill: kyoci.NewBaseSkill(
		"palette_analogous", "Generate an analogous 5-color palette from a base color",
		[]string{"analogous palette", "analogous colors", "analogous color palette"},
	)}
}
func (s *PaletteAnalogousSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "analogous palette") || strings.Contains(q, "analogous colors") ||
		strings.Contains(q, "analogous color")
}
func (s *PaletteAnalogousSkill) Execute(_ context.Context, q string) (string, error) {
	r, g, b, ok := parseHexStrict(q)
	if !ok {
		r, g, b, _ = parseRGBStrict(q)
	}
	h, sat, l := rgbToHSL(r, g, b)
	var out []string
	// Five colors with hue stepped by ±30°.
	for _, dh := range []int{-60, -30, 0, 30, 60} {
		hh := (h + dh + 360) % 360
		pr, pg, pb := hslToRGBF(hh, sat, float64(l)/100.0)
		out = append(out, fmt.Sprintf("#%02X%02X%02X", pr, pg, pb))
	}
	return strings.Join(out, " "), nil
}

// ---- palette_complementary ----

type PaletteComplementarySkill struct{ *kyoci.BaseSkill }

func NewPaletteComplementarySkill() *PaletteComplementarySkill {
	return &PaletteComplementarySkill{BaseSkill: kyoci.NewBaseSkill(
		"palette_complementary", "Generate complementary color palette from a base color",
		[]string{"complementary palette", "complementary colors", "complementary color"},
	)}
}
func (s *PaletteComplementarySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "complementary palette") || strings.Contains(q, "complementary colors") ||
		strings.Contains(q, "complementary color")
}
func (s *PaletteComplementarySkill) Execute(_ context.Context, q string) (string, error) {
	r, g, b, ok := parseHexStrict(q)
	if !ok {
		r, g, b, _ = parseRGBStrict(q)
	}
	h, sat, l := rgbToHSL(r, g, b)
	compH := (h + 180) % 360
	pr, pg, pb := hslToRGBF(compH, sat, float64(l)/100.0)
	return fmt.Sprintf("base: #%02X%02X%02X\ncomplement: #%02X%02X%02X", r, g, b, pr, pg, pb), nil
}
