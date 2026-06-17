//go:build darwin

package hardware

import (
	"errors"
	"os/exec"
	"strings"
)

// hostname returns the machine's hostname via the standard `hostname` command.
func hostname() (string, error) {
	out, err := exec.Command("hostname").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// darwinChipModel reads `sysctl -n machdep.cpu.brand_string` which on Apple
// Silicon returns e.g. "Apple M3 Pro".
func darwinChipModel() (string, error) {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "", err
	}
	brand := strings.TrimSpace(string(out))
	if brand == "" {
		return "", errors.New("empty brand string")
	}
	return brand, nil
}

// genericCPUModel is unused on macOS but keeps the file set compilable across
// build tags.
func genericCPUModel() (string, error) {
	return darwinChipModel()
}
