package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// TimeSkill handles time-related queries.
type TimeSkill struct {
	*kyoci.BaseSkill
}

// NewTimeSkill creates a new time skill.
func NewTimeSkill() *TimeSkill {
	return &TimeSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"time",
			"Returns current time, date, or Unix timestamp",
			[]string{"time", "current time", "what time", "date", "current date", "today", "unix timestamp", "timestamp"},
		),
	}
}

// Match checks if the query is asking for time information.
//
// Defers to the specific time skills (now, time_parse, time_format, time_diff,
// cron_next, epoch_convert) when their patterns would fire. The legacy `time`
// skill catches generic "what time" / "current date" / "unix timestamp"
// phrasings that no specific skill claims.
func (s *TimeSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	// Defer to specific skills.
	deferPhrases := []string{
		"what time is it now", "what is the time", "current time in",
		"parse time", "parse date", "time parse",
		"format time", "format date", "time format",
		"time diff", "time difference", "duration between", "elapsed between",
		"cron next", "next cron", "cron_next",
		"epoch convert", "convert epoch", "epoch to", "to epoch",
		"unix to iso", "iso to unix",
	}
	for _, p := range deferPhrases {
		if strings.Contains(queryLower, p) {
			return false
		}
	}
	// Bare "now" — defer to NowSkill.
	if queryLower == "now" {
		return false
	}
	// Note: "date" as a substring false-positives inside "validate", "candidate",
	// "update", etc. Use word-bounded phrases instead.
	timeKeywords := []string{
		"current time", "what time", "clock", "current date",
		"what date", "today", "unix timestamp", "timestamp", "epoch",
		" time", "the time", "this time",
	}
	for _, keyword := range timeKeywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// Execute returns the requested time information.
func (s *TimeSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(query)
	now := time.Now()

	// Check for unix timestamp
	if strings.Contains(queryLower, "unix") || strings.Contains(queryLower, "timestamp") || strings.Contains(queryLower, "epoch") {
		timestamp := now.Unix()
		millis := now.UnixMilli()
		return fmt.Sprintf("Unix timestamp: %d\nUnix milliseconds: %d", timestamp, millis), nil
	}

	// Check for date only
	if strings.Contains(queryLower, "date") && !strings.Contains(queryLower, "time") {
		return fmt.Sprintf("Current date: %s", now.Format("2006-01-02 Monday")), nil
	}

	// Check for time only
	if strings.Contains(queryLower, "time") && !strings.Contains(queryLower, "date") {
		return fmt.Sprintf("Current time: %s", now.Format("15:04:05")), nil
	}

	// Default: return both time and date
	return fmt.Sprintf("Current time: %s\nCurrent date: %s\nUnix timestamp: %d",
		now.Format("15:04:05"),
		now.Format("2006-01-02 Monday"),
		now.Unix(),
	), nil
}