package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// cronFieldPattern matches a cron-ish token: digits, *, or */N, or comma/range lists.
var cronFieldPattern = regexp.MustCompile(`^[\d*/,-]+$`)

// CronSkill parses cron expressions and computes the next fire time.
type CronSkill struct {
	*kyoci.BaseSkill
}

// NewCronSkill creates a new cron skill.
func NewCronSkill() *CronSkill {
	return &CronSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"cron",
			"Parse a cron expression and show its schedule; compute the next fire time",
			[]string{"cron", "crontab", "schedule"},
		),
	}
}

// Match checks if the query references cron or looks like a cron expression.
func (s *CronSkill) Match(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(queryLower, "cron") || strings.Contains(queryLower, "crontab") || strings.Contains(queryLower, "schedule") {
		return true
	}
	// Try to detect a 5-field cron-ish pattern.
	fields := strings.Fields(queryLower)
	if len(fields) >= 5 {
		first5 := fields[:5]
		allMatch := true
		for _, f := range first5 {
			if !cronFieldPattern.MatchString(f) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

// Execute parses the cron expression and reports its schedule plus next fire time.
func (s *CronSkill) Execute(ctx context.Context, query string) (string, error) {
	expr := s.extractExpression(query)
	if expr == "" {
		return "", fmt.Errorf("could not find a cron expression in query: %s", query)
	}

	fields := strings.Fields(expr)
	if len(fields) < 5 {
		return "", fmt.Errorf("cron expression must have 5 fields, got %d: %q", len(fields), expr)
	}
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	if err := validateField(minute, 0, 59); err != nil {
		return "", fmt.Errorf("minute: %w", err)
	}
	if err := validateField(hour, 0, 23); err != nil {
		return "", fmt.Errorf("hour: %w", err)
	}
	if err := validateField(dom, 1, 31); err != nil {
		return "", fmt.Errorf("day-of-month: %w", err)
	}
	if err := validateField(month, 1, 12); err != nil {
		return "", fmt.Errorf("month: %w", err)
	}
	if err := validateField(dow, 0, 6); err != nil {
		return "", fmt.Errorf("day-of-week: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Expression: %s\n", strings.Join([]string{minute, hour, dom, month, dow}, " "))
	fmt.Fprintf(&b, "Minute: %s\n", describeField(minute))
	fmt.Fprintf(&b, "Hour: %s\n", describeField(hour))
	fmt.Fprintf(&b, "Day-of-month: %s\n", describeField(dom))
	fmt.Fprintf(&b, "Month: %s\n", describeField(month))
	fmt.Fprintf(&b, "Day-of-week: %s\n", describeField(dow))

	next, err := nextFireTime(minute, hour, dom, month, dow, time.Now())
	if err != nil {
		return b.String(), nil
	}
	fmt.Fprintf(&b, "Next fire: %s", next.Format("2006-01-02 15:04:05"))
	return b.String(), nil
}

// extractExpression pulls the cron expression out of the query.
func (s *CronSkill) extractExpression(query string) string {
	query = strings.TrimSpace(query)
	// Strip leading keywords like "cron", "parse cron", "schedule".
	lower := strings.ToLower(query)
	for _, prefix := range []string{"parse cron ", "cron schedule ", "cron expression ", "cron ", "schedule "} {
		if strings.HasPrefix(lower, prefix) {
			query = strings.TrimSpace(query[len(prefix):])
			break
		}
	}
	fields := strings.Fields(query)
	if len(fields) >= 5 && cronFieldPattern.MatchString(fields[0]) {
		return strings.Join(fields[:5], " ")
	}
	// Fallback: look for the first 5-field cron-ish run anywhere.
	for i := 0; i+5 <= len(fields); i++ {
		window := fields[i : i+5]
		ok := true
		for _, f := range window {
			if !cronFieldPattern.MatchString(f) {
				ok = false
				break
			}
		}
		if ok {
			return strings.Join(window, " ")
		}
	}
	return ""
}

// validateField ensures a cron field's numeric values fall in [min,max].
func validateField(field string, min, max int) error {
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			continue
		}
		if strings.HasPrefix(part, "*/") {
			n, err := strconv.Atoi(strings.TrimPrefix(part, "*/"))
			if err != nil || n < 1 {
				return fmt.Errorf("invalid step %q", part)
			}
			continue
		}
		// Handle ranges like 1-5 or 1-5/2.
		rangePart := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			rangePart = part[:idx]
		}
		var lo, hi int
		if strings.Contains(rangePart, "-") {
			ends := strings.SplitN(rangePart, "-", 2)
			v1, err1 := strconv.Atoi(ends[0])
			v2, err2 := strconv.Atoi(ends[1])
			if err1 != nil || err2 != nil {
				return fmt.Errorf("invalid range %q", part)
			}
			lo, hi = v1, v2
		} else {
			v, err := strconv.Atoi(rangePart)
			if err != nil {
				return fmt.Errorf("invalid value %q", part)
			}
			lo, hi = v, v
		}
		if lo < min || lo > max || hi < min || hi > max {
			return fmt.Errorf("value %q out of range [%d,%d]", part, min, max)
		}
	}
	return nil
}

// describeField renders a human-readable summary of a cron field.
func describeField(field string) string {
	field = strings.TrimSpace(field)
	if field == "*" {
		return "every"
	}
	if strings.HasPrefix(field, "*/") {
		return "every " + strings.TrimPrefix(field, "*/") + "th"
	}
	return field
}

// matchField reports whether the given value satisfies the cron field.
func matchField(field string, value, min, max int) bool {
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		step := 1
		base := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			s, err := strconv.Atoi(part[idx+1:])
			if err != nil || s < 1 {
				continue
			}
			step = s
			base = part[:idx]
		}
		lo, hi := min, max
		if base == "*" {
			lo, hi = min, max
		} else if strings.Contains(base, "-") {
			ends := strings.SplitN(base, "-", 2)
			v1, err1 := strconv.Atoi(ends[0])
			v2, err2 := strconv.Atoi(ends[1])
			if err1 != nil || err2 != nil {
				continue
			}
			lo, hi = v1, v2
		} else {
			v, err := strconv.Atoi(base)
			if err != nil {
				continue
			}
			lo, hi = v, v
		}
		if value < lo || value > hi {
			continue
		}
		if step > 1 {
			if (value-lo)%step != 0 {
				continue
			}
		}
		return true
	}
	return false
}

// nextFireTime finds the next minute (after `from`) matching all cron fields.
func nextFireTime(minute, hour, dom, month, dow string, from time.Time) (time.Time, error) {
	// Start at the next whole minute.
	t := from.Add(time.Minute).Truncate(time.Minute)
	limit := from.AddDate(0, 0, 366) // up to 366 days ahead
	for t.Before(limit) {
		if !matchField(month, int(t.Month()), 1, 12) {
			t = t.AddDate(0, 0, 1)
			continue
		}
		if !matchField(dom, t.Day(), 1, 31) {
			t = t.AddDate(0, 0, 1)
			continue
		}
		// Go weekday: Sunday=0.
		if !matchField(dow, int(t.Weekday()), 0, 6) {
			t = t.AddDate(0, 0, 1)
			continue
		}
		if !matchField(hour, t.Hour(), 0, 23) {
			t = t.Add(time.Hour)
			continue
		}
		if !matchField(minute, t.Minute(), 0, 59) {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("no matching fire time within 366 days")
}
