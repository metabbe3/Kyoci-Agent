package builtin

import (
	"context"
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ColorSkill converts colors between HEX, RGB, and HSL representations and
// generates a 5-shade palette by varying HSL lightness.
type ColorSkill struct {
	*kyoci.BaseSkill
	hexPattern *regexp.Regexp
	rgbPattern *regexp.Regexp
}

// NewColorSkill creates a new color skill.
func NewColorSkill() *ColorSkill {
	return &ColorSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"color",
			"Convert colors between HEX, RGB, HSL and generate palettes",
			[]string{"color", "hex to rgb", "rgb to hex", "hsl", "palette", "#", "hex"},
		),
		hexPattern: regexp.MustCompile(`#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b`),
		rgbPattern: regexp.MustCompile(`(?i)rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})`),
	}
}

// Match checks if the query references colors in any supported form.
//
// Defers to the specific color skills (hex_to_rgb, contrast_ratio, color_blend,
// palette_analogous, palette_complementary, etc.) when the query contains an
// operation they own — otherwise the greedy "color" / "hex" / "rgb" keywords
// here would shadow them under non-deterministic map iteration.
func (s *ColorSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	// Defer to specific skills when present.
	for _, specific := range []string{
		"hex to rgb", "hex → rgb", "rgb to hex", "rgb → hex",
		"hex to hsl", "hex → hsl", "hsl to hex", "hsl → hex",
		"contrast ratio", "contrast between",
		"color blend", "blend colors", "mix colors",
		"analogous palette", "analogous colors",
		"complementary palette", "complementary color",
	} {
		if strings.Contains(queryLower, specific) {
			return false
		}
	}
	for _, keyword := range []string{"color", "hex", "rgb", "hsl", "palette"} {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	if strings.Contains(query, "#") {
		if s.hexPattern.MatchString(query) {
			return true
		}
	}
	return false
}

// Execute parses a color from the query and returns all representations plus a palette.
func (s *ColorSkill) Execute(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)

	var r, g, b uint8

	if m := s.rgbPattern.FindStringSubmatch(query); m != nil {
		ri, _ := strconv.Atoi(m[1])
		gi, _ := strconv.Atoi(m[2])
		bi, _ := strconv.Atoi(m[3])
		r = clamp8(ri)
		g = clamp8(gi)
		b = clamp8(bi)
	} else if m := s.hexPattern.FindStringSubmatch(query); m != nil {
		hex := m[1]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		v, err := strconv.ParseUint(hex, 16, 64)
		if err != nil {
			return "", fmt.Errorf("invalid hex color %q: %w", m[0], err)
		}
		r = uint8(v >> 16)
		g = uint8(v >> 8)
		b = uint8(v)
	} else {
		return "", fmt.Errorf("no color found in query (expected #RRGGBB, #RGB, or rgb(r,g,b))")
	}

	h, sat, l := rgbToHSL(r, g, b)
	lPct := float64(l) / 100.0
	hexStr := fmt.Sprintf("#%02X%02X%02X", r, g, b)

	var sb strings.Builder
	fmt.Fprintf(&sb, "HEX: %s\n", hexStr)
	fmt.Fprintf(&sb, "RGB: rgb(%d, %d, %d)\n", r, g, b)
	fmt.Fprintf(&sb, "HSL: hsl(%d, %d%%, %d%%)\n", h, sat, l)
	sb.WriteString("\nPalette (lighter → darker):\n")
	for _, pl := range []float64{0.80, 0.65, lPct, 0.35, 0.20} {
		pr, pg, pb := hslToRGBF(h, sat, pl)
		fmt.Fprintf(&sb, "  #%02X%02X%02X  hsl(%d, %d%%, %d%%)\n", pr, pg, pb, h, sat, int(pl*100+0.5))
	}

	// reference color.Color for interface satisfaction tooling (unused at runtime)
	_ = color.RGBA{R: r, G: g, B: b, A: 255}

	return sb.String(), nil
}

func clamp8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// rgbToHSL converts 8-bit RGB to HSL with h in [0,360), s and l in [0,100].
func rgbToHSL(r, g, b uint8) (int, int, int) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0
	max := max3(rf, gf, bf)
	min := min3(rf, gf, bf)
	l := (max + min) / 2.0
	var s, h float64
	if max != min {
		d := max - min
		if l > 0.5 {
			s = d / (2.0 - max - min)
		} else {
			s = d / (max + min)
		}
		switch max {
		case rf:
			h = (gf - bf) / d
			if gf < bf {
				h += 6
			}
		case gf:
			h = (bf-rf)/d + 2
		case bf:
			h = (rf-gf)/d + 4
		}
		h *= 60
	}
	if h < 0 {
		h += 360
	}
	return int(h + 0.5), int(s*100 + 0.5), int(l*100 + 0.5)
}

// hslToRGBF converts HSL (h in degrees, s in [0,100], l in [0,1]) to 8-bit RGB.
func hslToRGBF(h, s int, l float64) (uint8, uint8, uint8) {
	hf := float64(h) / 360.0
	sf := float64(s) / 100.0

	var r, g, b float64
	if sf == 0 {
		r, g, b = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + sf)
		} else {
			q = l + sf - l*sf
		}
		p := 2*l - q
		r = hueToRGB(p, q, hf+1.0/3.0)
		g = hueToRGB(p, q, hf)
		b = hueToRGB(p, q, hf-1.0/3.0)
	}
	return clamp8f(r * 255), clamp8f(g * 255), clamp8f(b * 255)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

func clamp8f(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func max3(a, b, c float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func min3(a, b, c float64) float64 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
