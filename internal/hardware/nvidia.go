package hardware

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// nvidiaGPU shells out to `nvidia-smi` to detect an NVIDIA GPU and its VRAM.
// Returns ("", 0, nil) silently when nvidia-smi isn't installed — common on
// macOS and most consumer hardware.
func nvidiaGPU() (model string, vramGB int, err error) {
	bin, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return "", 0, nil // not installed — not an error
	}
	out, err := exec.Command(bin,
		"--query-gpu=name,memory.total",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return "", 0, err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", 0, errors.New("nvidia-smi returned no GPUs")
	}
	// First GPU only for v1 (multi-GPU rigs are rare for this use case).
	first := strings.SplitN(line, "\n", 2)[0]
	fields := strings.Split(first, ",")
	if len(fields) < 2 {
		return "", 0, errors.New("unexpected nvidia-smi output: " + first)
	}
	model = strings.TrimSpace(fields[0])
	vramMiB, parseErr := strconv.Atoi(strings.TrimSpace(fields[1]))
	if parseErr != nil {
		return model, 0, nil
	}
	return model, vramMiB / 1024, nil
}
