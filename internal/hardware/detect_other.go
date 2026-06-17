//go:build !darwin

package hardware

import (
	"runtime"
	"strings"
)

func hostname() (string, error) {
	// gopsutil's host module adds another dep; defer to /etc/hostname-style via
	// the environment which works on Linux/Windows. Empty hostname is non-fatal.
	return "host", nil
}

// darwinChipModel is unused off-macOS but keeps the file set compilable.
func darwinChipModel() (string, error) {
	return "", nil
}

// genericCPUModel returns a best-effort CPU identifier on non-Apple platforms.
// Linux: /proc/cpuinfo "model name"; Windows: empty (defer to OS reporting).
func genericCPUModel() (string, error) {
	if runtime.GOOS == "linux" {
		if out, err := readFirstLine("/proc/cpuinfo", "model name"); err == nil {
			return out, nil
		}
	}
	return "", nil
}

// readFirstLine scans a small file for a key:value line and returns the value.
func readFirstLine(path, key string) (string, error) {
	data, err := readFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":")), nil
		}
	}
	return "", nil
}

// readFile is a thin wrapper kept here so the darwin build doesn't need os.
func readFile(path string) ([]byte, error) {
	return readFileOS(path)
}
