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

// =====================================================================================
// Date/Time skills — extensions to the existing TimeSkill. Specific operations
// (now, parse, format, diff, cron_next, epoch_convert) so the orchestrator
// can fast-path them instead of hitting the general time skill.
// =====================================================================================

// ---- now ----

type NowSkill struct{ *kyoci.BaseSkill }

func NewNowSkill() *NowSkill {
	return &NowSkill{BaseSkill: kyoci.NewBaseSkill(
		"now", "Current UTC time in ISO 8601, with optional timezone",
		[]string{"now", "current time", "what time", "what is the time"},
	)}
}
func (s *NowSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return q == "now" || strings.HasPrefix(q, "now ") || strings.Contains(q, "current time") ||
		strings.Contains(q, "what time is it") || strings.Contains(q, "what is the time")
}
func (s *NowSkill) Execute(_ context.Context, q string) (string, error) {
	loc := time.UTC
	// Optional timezone name after the colon or in the query.
	if tzName := extractTZ(q); tzName != "" {
		if l, err := time.LoadLocation(tzName); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%s\nunix: %d", now.Format(time.RFC3339), now.Unix()), nil
}

func extractTZ(q string) string {
	re := regexp.MustCompile(`(?i)\b(in|tz|timezone)\s+([A-Za-z/_]+)`)
	m := re.FindStringSubmatch(q)
	if m == nil {
		return ""
	}
	return m[2]
}

// ---- time_parse ----

type TimeParseSkill struct{ *kyoci.BaseSkill }

func NewTimeParseSkill() *TimeParseSkill {
	return &TimeParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"time_parse", "Parse a date string in common formats and emit ISO 8601",
		[]string{"parse time", "parse date", "time parse"},
	)}
}
func (s *TimeParseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "parse time") || strings.Contains(q, "parse date") ||
		strings.Contains(q, "time parse")
}
func (s *TimeParseSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload breaks on colons inside ISO timestamps; strip the verb.
	in := strings.TrimSpace(stripVerb(q, "parse time"))
	if in == "" {
		in = strings.TrimSpace(stripVerb(q, "parse date"))
	}
	if in == "" {
		return "", fmt.Errorf("no date to parse")
	}
	layouts := []string{
		time.RFC3339, time.RFC3339Nano, time.RFC1123, time.RFC1123Z,
		time.RFC822, time.RFC822Z, time.ANSIC, time.UnixDate,
		"2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05",
		"01/02/2006", "01/02/2006 15:04:05", "02-Jan-2006", "Jan 2, 2006",
		"January 2, 2006", "2006/01/02", "2006-01-02 15:04:05.000",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, in); err == nil {
			return fmt.Sprintf("parsed: %s\nunix: %d", t.UTC().Format(time.RFC3339), t.Unix()), nil
		}
	}
	// Try as unix epoch seconds / millis.
	if n, err := strconv.ParseInt(in, 10, 64); err == nil {
		var t time.Time
		if n > 1e12 {
			t = time.UnixMilli(n)
		} else {
			t = time.Unix(n, 0)
		}
		return fmt.Sprintf("parsed (epoch): %s\nunix: %d", t.UTC().Format(time.RFC3339), t.Unix()), nil
	}
	return "", fmt.Errorf("unrecognized date format: %s", in)
}

// ---- time_format ----

type TimeFormatSkill struct{ *kyoci.BaseSkill }

func NewTimeFormatSkill() *TimeFormatSkill {
	return &TimeFormatSkill{BaseSkill: kyoci.NewBaseSkill(
		"time_format", "Format an ISO date using a Go time layout. Usage: 'time_format 2006-01-02T15:04:05Z Jan 2, 2006'",
		[]string{"format time", "format date", "time format"},
	)}
}
func (s *TimeFormatSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "format time") || strings.Contains(q, "format date") ||
		strings.Contains(q, "time_format")
}
func (s *TimeFormatSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload splits at the first ':' (inside the ISO timestamp). Strip
	// the verb ourselves and split on '|' or newline.
	payload := stripVerb(q, "format time")
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(payload, "\n", 2)
	}
	if len(parts) < 2 {
		return "", fmt.Errorf("usage: time_format <iso-date> <go-layout>")
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
	if err != nil {
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02", time.RFC1123} {
			if t2, err2 := time.Parse(layout, strings.TrimSpace(parts[0])); err2 == nil {
				t = t2
				err = nil
				break
			}
		}
		if err != nil {
			return "", fmt.Errorf("invalid date: %w", err)
		}
	}
	layout := strings.TrimSpace(parts[1])
	return t.Format(layout), nil
}

// ---- time_diff ----

type TimeDiffSkill struct{ *kyoci.BaseSkill }

func NewTimeDiffSkill() *TimeDiffSkill {
	return &TimeDiffSkill{BaseSkill: kyoci.NewBaseSkill(
		"time_diff", "Human-readable duration between two ISO dates. Usage: 'time_diff 2024-01-01T00:00:00Z 2024-12-31T00:00:00Z'",
		[]string{"time diff", "time difference", "duration between", "elapsed between"},
	)}
}
func (s *TimeDiffSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "time diff") || strings.Contains(q, "time difference") ||
		strings.Contains(q, "duration between") || strings.Contains(q, "elapsed between")
}
func (s *TimeDiffSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload breaks on the ':' inside ISO timestamps. Strip the verb.
	payload := stripVerb(q, "time diff")
	parts := strings.Fields(payload)
	if len(parts) < 2 {
		return "", fmt.Errorf("need two dates")
	}
	t1, err1 := parseAnyTime(parts[0])
	t2, err2 := parseAnyTime(parts[1])
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("invalid date(s): %v, %v", err1, err2)
	}
	d := t2.Sub(t1)
	if d < 0 {
		d = -d
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	var b strings.Builder
	fmt.Fprintf(&b, "total seconds: %d\n", int(d.Seconds()))
	fmt.Fprintf(&b, "duration: %dd %dh %dm\n", days, hours, mins)
	fmt.Fprintf(&b, "human: %s\n", humanDuration(d))
	return strings.TrimRight(b.String(), "\n"), nil
}

func parseAnyTime(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339, "2006-01-02 15:04:05", "2006-01-02", time.RFC1123,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n), nil
		}
		return time.Unix(n, 0), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized format: %s", s)
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

// ---- cron_next ----

type CronNextSkill struct{ *kyoci.BaseSkill }

func NewCronNextSkill() *CronNextSkill {
	return &CronNextSkill{BaseSkill: kyoci.NewBaseSkill(
		"cron_next", "Compute the next N runs of a cron expression. Usage: 'cron_next */5 * * * *' (5 next runs)",
		[]string{"cron next", "next cron", "cron_next", "when next does cron"},
	)}
}
func (s *CronNextSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "cron next") || strings.Contains(q, "next cron") ||
		strings.Contains(q, "cron_next") || strings.Contains(q, "when next does cron")
}
func (s *CronNextSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload isn't useful here — the cron expr has no colon. Strip
	// the verb manually and pull the next 5 whitespace-separated tokens.
	payload := stripVerb(q, "cron_next")
	if payload == q {
		payload = stripVerb(q, "next cron")
	}
	fields := strings.Fields(payload)
	if len(fields) < 5 {
		return "", fmt.Errorf("need a 5-field cron expression")
	}
	expr := strings.Join(fields[:5], " ")
	schedule, err := parseCronExpr(expr)
	if err != nil {
		return "", err
	}
	now := time.Now()
	var b strings.Builder
	for i := 0; i < 5; i++ {
		next := schedule.Next(now)
		fmt.Fprintf(&b, "+%d: %s\n", i+1, next.Format("Mon Jan 2 15:04:05 2006"))
		now = next
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// parseCronExpr is a minimal 5-field cron parser (min hour dom month dow).
// Supports: *, */N, N, comma-lists, ranges (a-b). Does NOT support names
// (JAN, MON) or 7-field extensions. Returns a CronSchedule whose Next(t)
// computes the next fire time after t.
func parseCronExpr(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("need 5 fields (min hour dom month dow); got %d", len(fields))
	}
	ranges := []struct{ min, max int }{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	var sch CronSchedule
	for i, f := range fields {
		bits, err := parseCronField(f, ranges[i].min, ranges[i].max)
		if err != nil {
			return nil, fmt.Errorf("field %d (%q): %w", i, f, err)
		}
		sch.bits[i] = bits
	}
	return &sch, nil
}

type CronSchedule struct {
	bits [5]uint64 // bitmask of allowed values per field
}

func parseCronField(field string, min, max int) (uint64, error) {
	var bits uint64
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			for v := min; v <= max; v++ {
				bits |= 1 << uint(v)
			}
			continue
		}
		// */N
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return 0, fmt.Errorf("invalid step: %s", part)
			}
			for v := min; v <= max; v += step {
				bits |= 1 << uint(v)
			}
			continue
		}
		// a-b or a-b/N
		if strings.Contains(part, "-") {
			rng := strings.SplitN(part, "-", 2)
			a, err1 := strconv.Atoi(rng[0])
			b, err2 := strconv.Atoi(rng[1])
			if err1 != nil || err2 != nil || a < min || b > max || a > b {
				return 0, fmt.Errorf("invalid range: %s", part)
			}
			for v := a; v <= b; v++ {
				bits |= 1 << uint(v)
			}
			continue
		}
		// single value
		v, err := strconv.Atoi(part)
		if err != nil || v < min || v > max {
			return 0, fmt.Errorf("invalid value: %s", part)
		}
		bits |= 1 << uint(v)
	}
	return bits, nil
}

// Next returns the next time AFTER t that matches the cron schedule.
// Iterates minute-by-minute (max 366*24*60 iterations = ~526k for "never matches").
func (s *CronSchedule) Next(t time.Time) time.Time {
	t = t.Add(time.Minute).Truncate(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{} // no match within a year
}

func (s *CronSchedule) matches(t time.Time) bool {
	return s.bits[0]&(1<<uint(t.Minute())) != 0 &&
		s.bits[1]&(1<<uint(t.Hour())) != 0 &&
		s.bits[2]&(1<<uint(t.Day())) != 0 &&
		s.bits[3]&(1<<uint(int(t.Month()))) != 0 &&
		s.bits[4]&(1<<uint(int(t.Weekday()))) != 0
}

// ---- epoch_convert ----

type EpochConvertSkill struct{ *kyoci.BaseSkill }

func NewEpochConvertSkill() *EpochConvertSkill {
	return &EpochConvertSkill{BaseSkill: kyoci.NewBaseSkill(
		"epoch_convert", "Convert Unix epoch (s or ms) ↔ ISO 8601",
		[]string{"epoch convert", "convert epoch", "epoch to", "to epoch"},
	)}
}
func (s *EpochConvertSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "epoch convert") || strings.Contains(q, "convert epoch") ||
		strings.Contains(q, "epoch to") || strings.Contains(q, "to epoch") ||
		strings.Contains(q, "unix to iso") || strings.Contains(q, "iso to unix")
}
func (s *EpochConvertSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload strips "convert " but not "epoch " prefix. Strip both verbs.
	payload := strings.TrimSpace(stripVerb(q, "epoch convert"))
	if payload == q {
		payload = strings.TrimSpace(stripVerb(q, "convert epoch"))
	}
	// Direction 1: epoch → ISO.
	if n, err := strconv.ParseInt(payload, 10, 64); err == nil {
		var t time.Time
		if n > 1e12 {
			t = time.UnixMilli(n)
		} else {
			t = time.Unix(n, 0)
		}
		return fmt.Sprintf("iso: %s\nunix_seconds: %d", t.UTC().Format(time.RFC3339), t.Unix()), nil
	}
	// Direction 2: ISO → epoch.
	t, err := parseAnyTime(payload)
	if err != nil {
		return "", fmt.Errorf("not an epoch or ISO date: %w", err)
	}
	return fmt.Sprintf("unix_seconds: %d\nunix_millis: %d\niso: %s",
		t.Unix(), t.UnixMilli(), t.UTC().Format(time.RFC3339)), nil
}
