package builtin

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Math skills — extensions to the existing MathSkill. These handle specific
// operations (stats, gcd, prime, factorial, base_convert, etc.) so the
// orchestrator can fast-path them. The general MathSkill keeps the
// arithmetic-evaluator path.
// =====================================================================================

// ---- stats ----

type StatsSkill struct{ *kyoci.BaseSkill }

func NewStatsSkill() *StatsSkill {
	return &StatsSkill{BaseSkill: kyoci.NewBaseSkill(
		"stats", "Compute statistics (mean, median, mode, p50/p90/p95/p99, stddev, variance) for a list",
		[]string{"stats", "statistics", "percentile", "stddev", "standard deviation"},
	)}
}
func (s *StatsSkill) Match(q string) bool {
	q = strings.ToLower(q)
	if strings.Contains(q, "stats for") || strings.Contains(q, "statistics for") ||
		strings.Contains(q, "percentile") || strings.Contains(q, "stddev") ||
		strings.Contains(q, "standard deviation") {
		return true
	}
	return strings.HasPrefix(q, "stats ") || strings.HasPrefix(q, "statistics ")
}
func (s *StatsSkill) Execute(_ context.Context, q string) (string, error) {
	nums, err := parseNumberList(extractPayload(q))
	if err != nil {
		return "", err
	}
	if len(nums) == 0 {
		return "", fmt.Errorf("no numbers found")
	}
	sort.Float64s(nums)
	n := float64(len(nums))
	sum := 0.0
	for _, v := range nums {
		sum += v
	}
	mean := sum / n
	variance := 0.0
	for _, v := range nums {
		diff := v - mean
		variance += diff * diff
	}
	variance /= n
	stddev := math.Sqrt(variance)
	mode := computeMode(nums)
	var b strings.Builder
	fmt.Fprintf(&b, "count: %d\n", len(nums))
	fmt.Fprintf(&b, "sum: %g\n", sum)
	fmt.Fprintf(&b, "mean: %g\n", mean)
	fmt.Fprintf(&b, "median: %g\n", percentile(nums, 50))
	fmt.Fprintf(&b, "mode: %s\n", mode)
	fmt.Fprintf(&b, "min: %g\n", nums[0])
	fmt.Fprintf(&b, "max: %g\n", nums[len(nums)-1])
	fmt.Fprintf(&b, "p90: %g\n", percentile(nums, 90))
	fmt.Fprintf(&b, "p95: %g\n", percentile(nums, 95))
	fmt.Fprintf(&b, "p99: %g\n", percentile(nums, 99))
	fmt.Fprintf(&b, "stddev: %g\n", stddev)
	fmt.Fprintf(&b, "variance: %g\n", variance)
	return strings.TrimRight(b.String(), "\n"), nil
}

func parseNumberList(s string) ([]float64, error) {
	var out []float64
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == ';'
	})
	for _, f := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// percentile computes the p-th percentile from a SORTED list (nearest-rank method).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// computeMode returns the most-frequent value(s) as a comma-separated string.
func computeMode(sorted []float64) string {
	if len(sorted) == 0 {
		return ""
	}
	bestCount := 1
	best := []float64{sorted[0]}
	cur := sorted[0]
	curCount := 1
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == cur {
			curCount++
			continue
		}
		if curCount > bestCount {
			bestCount = curCount
			best = []float64{cur}
		} else if curCount == bestCount {
			best = append(best, cur)
		}
		cur = sorted[i]
		curCount = 1
	}
	if curCount > bestCount {
		best = []float64{cur}
	} else if curCount == bestCount {
		best = append(best, cur)
	}
	parts := make([]string, len(best))
	for i, v := range best {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return strings.Join(parts, ", ")
}

// ---- gcd / lcm ----

type GCDSkill struct{ *kyoci.BaseSkill }

func NewGCDSkill() *GCDSkill {
	return &GCDSkill{BaseSkill: kyoci.NewBaseSkill(
		"gcd", "Greatest common divisor of two or more integers",
		[]string{"gcd", "greatest common divisor", "hcf", "highest common factor"},
	)}
}
func (s *GCDSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.HasPrefix(q, "gcd ") || strings.Contains(q, "gcd of") ||
		strings.Contains(q, "greatest common divisor") || strings.Contains(q, "hcf of") ||
		strings.Contains(q, "highest common factor")
}
func (s *GCDSkill) Execute(_ context.Context, q string) (string, error) {
	nums, err := parseIntList(extractPayload(q))
	if err != nil || len(nums) < 2 {
		return "", fmt.Errorf("need at least 2 integers")
	}
	result := absInt(nums[0])
	for _, n := range nums[1:] {
		result = gcdInt(result, absInt(n))
	}
	return fmt.Sprintf("%d", result), nil
}

type LCMSkill struct{ *kyoci.BaseSkill }

func NewLCMSkill() *LCMSkill {
	return &LCMSkill{BaseSkill: kyoci.NewBaseSkill(
		"lcm", "Least common multiple of two or more integers",
		[]string{"lcm", "least common multiple"},
	)}
}
func (s *LCMSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.HasPrefix(q, "lcm ") || strings.Contains(q, "lcm of") ||
		strings.Contains(q, "least common multiple")
}
func (s *LCMSkill) Execute(_ context.Context, q string) (string, error) {
	nums, err := parseIntList(extractPayload(q))
	if err != nil || len(nums) < 2 {
		return "", fmt.Errorf("need at least 2 integers")
	}
	result := absInt(nums[0])
	for _, n := range nums[1:] {
		n = absInt(n)
		result = result / gcdInt(result, n) * n
	}
	return fmt.Sprintf("%d", result), nil
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func parseIntList(s string) ([]int, error) {
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == ';'
	}) {
		v, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// ---- is_prime / prime_factors ----

type IsPrimeSkill struct{ *kyoci.BaseSkill }

func NewIsPrimeSkill() *IsPrimeSkill {
	return &IsPrimeSkill{BaseSkill: kyoci.NewBaseSkill(
		"is_prime", "Check if a number is prime",
		[]string{"is prime", "prime check", "is_prime", "primality"},
	)}
}
func (s *IsPrimeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "is prime") || strings.Contains(q, "prime check") ||
		strings.Contains(q, "is_prime") || strings.Contains(q, "primality")
}
func (s *IsPrimeSkill) Execute(_ context.Context, q string) (string, error) {
	// Pull the integer via regex — extractPayload leaves "is prime 17" as-is
	// (no colon, no stopword match), which strconv.Atoi can't parse.
	re := regexp.MustCompile(`-?\d+`)
	m := re.FindString(q)
	if m == "" {
		return "", fmt.Errorf("invalid integer")
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return "", fmt.Errorf("invalid integer: %w", err)
	}
	if isPrime(n) {
		return fmt.Sprintf("%d is prime", n), nil
	}
	return fmt.Sprintf("%d is composite", n), nil
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n < 4 {
		return true
	}
	if n%2 == 0 {
		return false
	}
	for i := 3; i*i <= n; i += 2 {
		if n%i == 0 {
			return false
		}
	}
	return true
}

type PrimeFactorsSkill struct{ *kyoci.BaseSkill }

func NewPrimeFactorsSkill() *PrimeFactorsSkill {
	return &PrimeFactorsSkill{BaseSkill: kyoci.NewBaseSkill(
		"prime_factors", "Prime factorization of an integer",
		[]string{"prime factors", "prime factorization", "factorize"},
	)}
}
func (s *PrimeFactorsSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "prime factors") || strings.Contains(q, "prime factorization") ||
		strings.Contains(q, "factorize")
}
func (s *PrimeFactorsSkill) Execute(_ context.Context, q string) (string, error) {
	re := regexp.MustCompile(`-?\d+`)
	m := re.FindString(q)
	if m == "" {
		return "", fmt.Errorf("no integer found")
	}
	n, err := strconv.Atoi(m)
	if err != nil || n < 2 {
		return "", fmt.Errorf("need integer ≥ 2")
	}
	var factors []string
	for d := 2; d*d <= n; d++ {
		for n%d == 0 {
			factors = append(factors, strconv.Itoa(d))
			n /= d
		}
	}
	if n > 1 {
		factors = append(factors, strconv.Itoa(n))
	}
	return strings.Join(factors, " × "), nil
}

// ---- factorial ----

type FactorialSkill struct{ *kyoci.BaseSkill }

func NewFactorialSkill() *FactorialSkill {
	return &FactorialSkill{BaseSkill: kyoci.NewBaseSkill(
		"factorial", "Factorial n! (0-20 supported; overflow beyond)",
		[]string{"factorial", "n fact", "n!"},
	)}
}
func (s *FactorialSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "factorial") || strings.Contains(q, "n fact") ||
		strings.Contains(q, "n!")
}
func (s *FactorialSkill) Execute(_ context.Context, q string) (string, error) {
	re := regexp.MustCompile(`-?\d+`)
	m := re.FindString(q)
	if m == "" {
		return "", fmt.Errorf("no integer found")
	}
	n, err := strconv.Atoi(m)
	if err != nil || n < 0 {
		return "", fmt.Errorf("need non-negative integer")
	}
	if n > 20 {
		return "", fmt.Errorf("factorial too large (max 20 to fit int64)")
	}
	result := int64(1)
	for i := 2; i <= n; i++ {
		result *= int64(i)
	}
	return fmt.Sprintf("%d", result), nil
}

// ---- base_convert ----

type BaseConvertSkill struct{ *kyoci.BaseSkill }

func NewBaseConvertSkill() *BaseConvertSkill {
	return &BaseConvertSkill{BaseSkill: kyoci.NewBaseSkill(
		"base_convert", "Convert between bases. Usage: 'base_convert 255 dec hex' or 'base_convert ff hex dec'",
		[]string{"base convert", "convert base", "bin to dec", "dec to hex", "hex to bin"},
	)}
}
func (s *BaseConvertSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "base convert") || strings.Contains(q, "base_convert") ||
		strings.Contains(q, "convert base") ||
		strings.Contains(q, "bin to dec") || strings.Contains(q, "dec to hex") ||
		strings.Contains(q, "hex to bin") || strings.Contains(q, "dec to bin") ||
		strings.Contains(q, "bin to hex") || strings.Contains(q, "hex to dec") ||
		strings.Contains(q, "oct to dec") || strings.Contains(q, "dec to oct")
}
func (s *BaseConvertSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	// Common named pairs first.
	pairs := []struct{ from, to string; fromBase, toBase int }{
		{"bin", "dec", 2, 10}, {"dec", "bin", 10, 2},
		{"hex", "dec", 16, 10}, {"dec", "hex", 10, 16},
		{"oct", "dec", 8, 10}, {"dec", "oct", 10, 8},
		{"bin", "hex", 2, 16}, {"hex", "bin", 16, 2},
	}
	for _, p := range pairs {
		needle := p.from + " to " + p.to
		if strings.Contains(low, needle) {
			payload := extractPayload(q)
			payload = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(payload, needle, "")))
			return convertBase(payload, p.fromBase, p.toBase)
		}
	}
	// Generic format: "base_convert <value> <from> <to>".
	fields := strings.Fields(extractPayload(q))
	if len(fields) >= 3 {
		val, fromName, toName := fields[0], fields[1], fields[2]
		from, err := baseFromName(fromName)
		if err != nil {
			return "", err
		}
		to, err := baseFromName(toName)
		if err != nil {
			return "", err
		}
		return convertBase(val, from, to)
	}
	return "", fmt.Errorf("usage: base_convert <value> <from> <to> (bases: bin, oct, dec, hex)")
}

func baseFromName(name string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bin", "binary", "b", "2":
		return 2, nil
	case "oct", "octal", "o", "8":
		return 8, nil
	case "dec", "decimal", "d", "10":
		return 10, nil
	case "hex", "hexadecimal", "h", "16":
		return 16, nil
	}
	return 0, fmt.Errorf("unknown base name: %s", name)
}

func convertBase(val string, from, to int) (string, error) {
	val = strings.TrimSpace(strings.ToLower(val))
	n, err := strconv.ParseInt(val, from, 64)
	if err != nil {
		return "", fmt.Errorf("invalid value for base %d: %w", from, err)
	}
	return strconv.FormatInt(n, to), nil
}

// ---- round_sig ----

type RoundSigSkill struct{ *kyoci.BaseSkill }

func NewRoundSigSkill() *RoundSigSkill {
	return &RoundSigSkill{BaseSkill: kyoci.NewBaseSkill(
		"round_sig", "Round to N significant figures. Usage: 'round_sig 3.14159 3'",
		[]string{"round sig", "round to sig", "significant figures", "round_sig"},
	)}
}
func (s *RoundSigSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "round sig") || strings.Contains(q, "round to sig") ||
		strings.Contains(q, "significant figures") || strings.Contains(q, "round_sig")
}
func (s *RoundSigSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload leaves the skill name in place; pull numeric tokens via regex.
	numRe := regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	nums := numRe.FindAllString(q, -1)
	if len(nums) < 2 {
		return "", fmt.Errorf("usage: round_sig <value> <sig-figs>")
	}
	v, err := strconv.ParseFloat(nums[0], 64)
	if err != nil {
		return "", fmt.Errorf("invalid value: %w", err)
	}
	sig, err := strconv.Atoi(nums[len(nums)-1])
	if err != nil || sig < 1 {
		return "", fmt.Errorf("invalid sig-figs (need positive integer)")
	}
	if v == 0 {
		return "0", nil
	}
	shift := sig - 1 - int(math.Floor(math.Log10(math.Abs(v))))
	factor := math.Pow(10, float64(shift))
	rounded := math.Round(v*factor) / factor
	return strconv.FormatFloat(rounded, 'g', sig, 64), nil
}

// ---- units_convert ----

type UnitsConvertSkill struct{ *kyoci.BaseSkill }

func NewUnitsConvertSkill() *UnitsConvertSkill {
	return &UnitsConvertSkill{BaseSkill: kyoci.NewBaseSkill(
		"units_convert", "Convert between common units (bytes, length, temperature, weight)",
		[]string{"units convert", "unit conversion", "convert units", "b to kb", "c to f", "kg to lb"},
	)}
}
func (s *UnitsConvertSkill) Match(q string) bool {
	q = strings.ToLower(q)
	if strings.Contains(q, "units convert") || strings.Contains(q, "convert units") {
		return true
	}
	// Common unit pairs — keep these tight so they don't false-positive on
	// "rgb to hex" (b to...) or "dec to hex" (which belongs to base_convert).
	pairs := []string{
		"bytes to kb", "byte to kb", "bytes to mb", "byte to mb",
		"kb to mb", "mb to gb", "gb to tb", "tb to pb",
		" c to f", " f to c", "celsius to fahrenheit", "fahrenheit to celsius",
		"m to ft", "ft to m", "m to mile", "mile to m", "km to mile", "mile to km",
		"kg to lb", "lb to kg", "g to oz", "oz to g",
	}
	for _, p := range pairs {
		if strings.Contains(q, p) {
			return true
		}
	}
	return false
}
func (s *UnitsConvertSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	payload := extractPayload(q)
	// Bytes.
	for _, p := range []struct{ from, to string; factor float64 }{
		{"b", "kb", 1024}, {"bytes", "kb", 1024},
		{"kb", "mb", 1024}, {"mb", "gb", 1024}, {"gb", "tb", 1024},
	} {
		needle := p.from + " to " + p.to
		if strings.Contains(low, needle) {
			n := extractFirstNumber(payload)
			if n < 0 {
				return "", fmt.Errorf("no number found")
			}
			return strconv.FormatFloat(n/p.factor, 'g', -1, 64), nil
		}
	}
	// Temperature.
	if strings.Contains(low, "c to f") || strings.Contains(low, "celsius to fahrenheit") {
		c := extractFirstNumber(payload)
		return strconv.FormatFloat(c*9/5+32, 'g', -1, 64), nil
	}
	if strings.Contains(low, "f to c") || strings.Contains(low, "fahrenheit to celsius") {
		f := extractFirstNumber(payload)
		return strconv.FormatFloat((f-32)*5/9, 'g', -1, 64), nil
	}
	// Length.
	lengthConv := []struct {
		needle string
		factor float64
	}{
		{"m to ft", 3.28084},
		{"ft to m", 0.3048},
		{"km to mile", 0.621371},
		{"mile to km", 1.60934},
		{"m to mile", 0.000621371},
		{"mile to m", 1609.34},
	}
	for _, c := range lengthConv {
		if strings.Contains(low, c.needle) {
			n := extractFirstNumber(payload)
			return strconv.FormatFloat(n*c.factor, 'g', -1, 64), nil
		}
	}
	// Weight.
	weightConv := []struct {
		needle string
		factor float64
	}{
		{"kg to lb", 2.20462},
		{"lb to kg", 0.453592},
		{"g to oz", 0.035274},
		{"oz to g", 28.3495},
	}
	for _, c := range weightConv {
		if strings.Contains(low, c.needle) {
			n := extractFirstNumber(payload)
			return strconv.FormatFloat(n*c.factor, 'g', -1, 64), nil
		}
	}
	return "", fmt.Errorf("unsupported conversion (try: b to kb, kb to mb, c to f, f to c, m to ft, kg to lb)")
}

func extractFirstNumber(s string) float64 {
	var num strings.Builder
	started := false
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			num.WriteRune(r)
			started = true
		} else if started {
			break
		}
	}
	v, _ := strconv.ParseFloat(num.String(), 64)
	return v
}

// ---- currency_format ----

type CurrencyFormatSkill struct{ *kyoci.BaseSkill }

func NewCurrencyFormatSkill() *CurrencyFormatSkill {
	return &CurrencyFormatSkill{BaseSkill: kyoci.NewBaseSkill(
		"currency_format", "Format a number as currency (locale-style, no FX rates)",
		[]string{"currency format", "format currency", "format as usd", "format as eur"},
	)}
}
func (s *CurrencyFormatSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "currency format") || strings.Contains(q, "format currency") ||
		strings.Contains(q, "format as usd") || strings.Contains(q, "format as eur") ||
		strings.Contains(q, "format as jpy") || strings.Contains(q, "format as gbp")
}
func (s *CurrencyFormatSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	amount := extractFirstNumber(extractPayload(q))
	symbol := "$"
	decimals := 2
	if strings.Contains(low, "eur") {
		symbol = "€"
	} else if strings.Contains(low, "gbp") {
		symbol = "£"
	} else if strings.Contains(low, "jpy") {
		symbol = "¥"
		decimals = 0
	} else if strings.Contains(low, "cny") || strings.Contains(low, "rmb") {
		symbol = "¥"
	} else if strings.Contains(low, "idr") {
		symbol = "Rp"
	}
	return fmt.Sprintf("%s%s", symbol, formatNumber(amount, decimals)), nil
}

func formatNumber(n float64, decimals int) string {
	return strconv.FormatFloat(n, 'f', decimals, 64)
}

// ---- percentage ----

type PercentageSkill struct{ *kyoci.BaseSkill }

func NewPercentageSkill() *PercentageSkill {
	return &PercentageSkill{BaseSkill: kyoci.NewBaseSkill(
		"percentage", "Compute X% of Y, or X is what % of Y. Usage: 'percentage 20 of 80' or 'percentage 16 of 200'",
		[]string{"percentage of", "percent of", "what percentage", "what percent"},
	)}
}
func (s *PercentageSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "percentage of") || strings.Contains(q, "percent of") ||
		strings.Contains(q, "what percentage") || strings.Contains(q, "what percent")
}
func (s *PercentageSkill) Execute(_ context.Context, q string) (string, error) {
	nums, _ := parseIntList(extractPayload(q)) // ints ok but we want floats
	fl, _ := parseNumberList(extractPayload(q))
	if len(fl) < 2 {
		return "", fmt.Errorf("need two numbers: 'percentage X of Y'")
	}
	x, y := fl[0], fl[1]
	_ = nums
	// If X looks like a percentage (<= 100), compute X% of Y.
	if x >= 0 && x <= 100 {
		return fmt.Sprintf("%g%% of %g = %g", x, y, x/100*y), nil
	}
	// Otherwise compute X is what % of Y.
	return fmt.Sprintf("%g is %g%% of %g", x, x/y*100, y), nil
}

// ---- ratio_simplify ----

type RatioSimplifySkill struct{ *kyoci.BaseSkill }

func NewRatioSimplifySkill() *RatioSimplifySkill {
	return &RatioSimplifySkill{BaseSkill: kyoci.NewBaseSkill(
		"ratio_simplify", "Simplify a ratio a:b to its lowest terms",
		[]string{"simplify ratio", "ratio simplify", "reduce ratio"},
	)}
}
func (s *RatioSimplifySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "simplify ratio") || strings.Contains(q, "ratio simplify") ||
		strings.Contains(q, "reduce ratio")
}
func (s *RatioSimplifySkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload splits at first ':' which appears INSIDE the ratio
	// (12:8). Strip the verb ourselves.
	payload := stripVerb(q, "simplify ratio")
	payload = strings.TrimSpace(payload)
	parts := strings.FieldsFunc(payload, func(r rune) bool {
		return r == ':' || r == '/' || r == ' '
	})
	if len(parts) < 2 {
		return "", fmt.Errorf("need ratio like 12:8 or 12/8")
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("invalid integers in ratio")
	}
	if a == 0 || b == 0 {
		return "", fmt.Errorf("ratio parts must be non-zero")
	}
	g := gcdInt(absInt(a), absInt(b))
	return fmt.Sprintf("%d:%d", a/g, b/g), nil
}
