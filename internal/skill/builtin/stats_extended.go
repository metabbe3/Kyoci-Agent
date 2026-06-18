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
// Extended statistics skills — variance, sample stddev, percentile, quartile.
//
// These complement math_extended.go's StatsSkill (which computes a summary
// block: count/mean/median/stddev/variance/mode/percentiles using the
// nearest-rank method and POPULATION stddev). Each skill here is focused on a
// single value and uses linear interpolation (NumPy default, a.k.a. "Type 7" /
// Hyndman-Fan method 7) for percentiles and quartiles, which matches the
// convention used by pandas.describe() and most spreadsheet engines.
//
// Match keywords are deliberately specific to avoid colliding with StatsSkill,
// which already matches "stats", "statistics", "percentile", "stddev", and
// "standard deviation". We only claim queries that name a specific stat:
//   - "variance"
//   - "stddev sample" / "sample stddev" / "standard deviation sample"
//   - "percentile <N>:" (the leading "<N>:" distinguishes us from StatsSkill
//     when the orchestrator prefers the more-specific matcher)
//   - "quartile" / "quartiles"
// =====================================================================================

// ---- variance (population) ----

type VarianceSkill struct{ *kyoci.BaseSkill }

func NewVarianceSkill() *VarianceSkill {
	return &VarianceSkill{BaseSkill: kyoci.NewBaseSkill(
		"variance", "Population variance of a list of numbers (divides by n)",
		[]string{"variance", "population variance", "variance of"},
	)}
}
func (s *VarianceSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "variance")
}
func (s *VarianceSkill) Execute(_ context.Context, q string) (string, error) {
	nums, err := parseNumberList(extractPayload(q))
	if err != nil || len(nums) == 0 {
		return "", fmt.Errorf("no numbers found for variance")
	}
	n := float64(len(nums))
	sum := 0.0
	for _, v := range nums {
		sum += v
	}
	mean := sum / n
	v := 0.0
	for _, x := range nums {
		d := x - mean
		v += d * d
	}
	v /= n
	return formatStat(v), nil
}

// ---- stddev_sample (Bessel's correction, n-1) ----

type StddevSampleSkill struct{ *kyoci.BaseSkill }

func NewStddevSampleSkill() *StddevSampleSkill {
	return &StddevSampleSkill{BaseSkill: kyoci.NewBaseSkill(
		"stddev_sample", "Sample standard deviation of a list (Bessel's correction, n-1)",
		[]string{"stddev sample", "sample stddev", "standard deviation sample", "sample standard deviation"},
	)}
}
func (s *StddevSampleSkill) Match(q string) bool {
	q = strings.ToLower(q)
	// Careful: do NOT match bare "stddev" or "standard deviation" — those are
	// claimed by StatsSkill (population stddev summary). We only claim the
	// sample variants.
	return strings.Contains(q, "stddev sample") ||
		strings.Contains(q, "sample stddev") ||
		strings.Contains(q, "standard deviation sample") ||
		strings.Contains(q, "sample standard deviation")
}
func (s *StddevSampleSkill) Execute(_ context.Context, q string) (string, error) {
	nums, err := parseNumberList(extractPayload(q))
	if err != nil || len(nums) < 2 {
		return "", fmt.Errorf("need at least 2 numbers for sample stddev")
	}
	n := float64(len(nums))
	sum := 0.0
	for _, v := range nums {
		sum += v
	}
	mean := sum / n
	v := 0.0
	for _, x := range nums {
		d := x - mean
		v += d * d
	}
	// Sample variance: divide by n-1 (Bessel's correction).
	v /= (n - 1)
	return formatStat(math.Sqrt(v)), nil
}

// ---- percentile (linear interpolation) ----

// percentileValueRe captures the percentile number from "percentile 90:". A
// leading sign is accepted so the skill still matches invalid inputs like
// "percentile -10:" (the range check happens in Execute, not Match).
var percentileValueRe = regexp.MustCompile(`percentile\s+(-?\d+(?:\.\d+)?)`)

type PercentileSkill struct{ *kyoci.BaseSkill }

func NewPercentileSkill() *PercentileSkill {
	return &PercentileSkill{BaseSkill: kyoci.NewBaseSkill(
		"percentile", "Compute the p-th percentile of a list using linear interpolation. Usage: 'percentile 90: 1 2 ... 10'",
		[]string{"percentile"},
	)}
}
func (s *PercentileSkill) Match(q string) bool {
	q = strings.ToLower(q)
	// Only claim percentile queries that name a specific percentile value
	// (e.g. "percentile 90:") so we don't shadow StatsSkill's general
	// percentile summary path on bare "percentile" queries.
	if !strings.Contains(q, "percentile") {
		return false
	}
	return percentileValueRe.MatchString(q)
}
func (s *PercentileSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	m := percentileValueRe.FindStringSubmatch(low)
	if m == nil {
		return "", fmt.Errorf("specify a percentile value, e.g. 'percentile 90: ...'")
	}
	p, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid percentile value: %w", err)
	}
	if p < 0 || p > 100 {
		return "", fmt.Errorf("percentile must be between 0 and 100")
	}
	nums, err := parseNumberList(extractPayload(q))
	if err != nil || len(nums) == 0 {
		return "", fmt.Errorf("no numbers found for percentile")
	}
	sort.Float64s(nums)
	return formatStat(linearPercentile(nums, p)), nil
}

// ---- quartile (linear interpolation) ----

type QuartileSkill struct{ *kyoci.BaseSkill }

func NewQuartileSkill() *QuartileSkill {
	return &QuartileSkill{BaseSkill: kyoci.NewBaseSkill(
		"quartile", "Compute Q1, Q2 (median), Q3 of a list using linear interpolation (Type 7 / NumPy default)",
		[]string{"quartile", "quartiles"},
	)}
}
func (s *QuartileSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "quartile") || strings.Contains(q, "quartiles")
}
func (s *QuartileSkill) Execute(_ context.Context, q string) (string, error) {
	nums, err := parseNumberList(extractPayload(q))
	if err != nil || len(nums) == 0 {
		return "", fmt.Errorf("no numbers found for quartile")
	}
	sort.Float64s(nums)
	q1 := linearPercentile(nums, 25)
	q2 := linearPercentile(nums, 50)
	q3 := linearPercentile(nums, 75)
	return fmt.Sprintf("Q1: %s\nQ2: %s\nQ3: %s", formatStat(q1), formatStat(q2), formatStat(q3)), nil
}

// =====================================================================================
// Helpers.
// =====================================================================================

// linearPercentile computes the p-th percentile of a SORTED slice using linear
// interpolation (NumPy's default, a.k.a. "Type 7" / Hyndman-Fan method 7). This
// is the convention used by pandas.describe() and most spreadsheet engines.
//
// Given sorted data x[0..n-1], the rank is r = p/100 * (n - 1). The percentile
// is the linear interpolation between x[floor(r)] and x[ceil(r)] weighted by
// the fractional part of r.
func linearPercentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	rank := p / 100.0 * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// formatStat formats a numeric stat result. Integers print without decimals;
// non-integers print with up to 4 decimal places, trailing zeros stripped.
func formatStat(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	// Integer-valued results print without a decimal point (e.g. "2" not "2.0000").
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	// Otherwise 4 decimal places, trimming trailing zeros (e.g. 1.5811).
	s := strconv.FormatFloat(v, 'f', 4, 64)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		end := len(s)
		for end > i+1 && s[end-1] == '0' {
			end--
		}
		s = s[:end]
	}
	return s
}
