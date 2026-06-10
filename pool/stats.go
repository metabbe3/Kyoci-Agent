package pool

import (
	"context"
	"runtime"
	"time"
)

// Stats contains memory and runtime statistics
type Stats struct {
	runtime.MemStats
	ToolCount       int
	GoroutineCount  int
	SessionCount    int
}

// GetStats collects current memory and runtime statistics
func GetStats() Stats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return Stats{
		MemStats:       m,
		GoroutineCount: runtime.NumGoroutine(),
	}
}

// Summary returns a one-line summary of statistics
func (s Stats) Summary() string {
	return "Mem: " + formatBytes(s.Alloc) +
		" / " + formatBytes(s.Sys) +
		", Goroutines: " + itoa(s.GoroutineCount) +
		", Tools: " + itoa(s.ToolCount) +
		", Sessions: " + itoa(s.SessionCount)
}

// StartMonitor runs a background goroutine that periodically collects stats
// and sends them to the returned channel. Cancel the context to stop.
func StartMonitor(ctx context.Context, interval time.Duration) <-chan Stats {
	ch := make(chan Stats)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ch <- GetStats()
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// formatBytes formats a byte count as human-readable (KB, MB, GB)
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return itoa(int(b)) + "B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return itoa(int(b/div)) + "KMGT"[exp:exp+1] + "B"
}

// itoa converts int to string without fmt.Sprintf
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}