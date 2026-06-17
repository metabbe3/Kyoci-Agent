package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Time skill tests — 6 skills (now, time_parse, time_format, time_diff,
// cron_next, epoch_convert). Time-dependent tests verify format, not exact
// timestamps.
// =====================================================================================

func TestNowSkill(t *testing.T) {
	skill := NewNowSkill()
	if !skill.Match("now") {
		t.Error("expected match for 'now'")
	}
	if !skill.Match("current time") {
		t.Error("expected match for 'current time'")
	}
	out, err := skill.Execute(context.Background(), "now")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should contain ISO 8601 format and a unix timestamp.
	if !strings.Contains(out, "T") {
		t.Errorf("expected ISO 8601 timestamp, got %q", out)
	}
	if !strings.Contains(out, "unix:") {
		t.Errorf("expected unix: field, got %q", out)
	}
}

func TestTimeParseSkill(t *testing.T) {
	skill := NewTimeParseSkill()
	if !skill.Match("parse time 2024-01-15") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "parse time 2024-01-15")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("expected date in output, got %q", out)
	}
}

func TestTimeFormatSkill(t *testing.T) {
	skill := NewTimeFormatSkill()
	if !skill.Match("format time 2024-01-15T00:00:00Z|2006/01/02") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "format time 2024-01-15T00:00:00Z|2006/01/02")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "2024/01/15") {
		t.Errorf("expected reformatted date, got %q", out)
	}
}

func TestTimeDiffSkill(t *testing.T) {
	skill := NewTimeDiffSkill()
	if !skill.Match("time diff 2024-01-01T00:00:00Z 2024-01-11T00:00:00Z") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "time diff 2024-01-01T00:00:00Z 2024-01-11T00:00:00Z")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 10-day diff = 240 hours = 864000 seconds.
	if !strings.Contains(out, "10d") && !strings.Contains(out, "864000") {
		t.Errorf("expected 10-day difference, got %q", out)
	}
}

func TestCronNextSkill(t *testing.T) {
	skill := NewCronNextSkill()
	if !skill.Match("cron_next */5 * * * *") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "cron_next */5 * * * *")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should list 5 next runs.
	if !strings.Contains(out, "+1:") || !strings.Contains(out, "+5:") {
		t.Errorf("expected 5 next-run lines, got %q", out)
	}
}

func TestEpochConvertSkill(t *testing.T) {
	skill := NewEpochConvertSkill()
	if !skill.Match("epoch convert 1700000000") {
		t.Error("expected match for epoch seconds")
	}
	// Direction 1: epoch → ISO
	out, err := skill.Execute(context.Background(), "epoch convert 1700000000")
	if err != nil {
		t.Fatalf("Execute epoch→ISO: %v", err)
	}
	if !strings.Contains(out, "2023") {
		t.Errorf("1700000000 should be in 2023, got %q", out)
	}

	// Direction 2: ISO → epoch
	out2, err := skill.Execute(context.Background(), "epoch convert 2023-11-14T22:13:20Z")
	if err != nil {
		t.Fatalf("Execute ISO→epoch: %v", err)
	}
	if !strings.Contains(out2, "1700000000") {
		t.Errorf("2023-11-14 should be ~1700000000, got %q", out2)
	}
}
