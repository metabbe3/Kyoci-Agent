// Package hardware detects host system specs (CPU, RAM, OS, GPU) so the
// dashboard can recommend models that fit the machine running Kyoci.
package hardware

import (
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/mem"
)

// Specs is the detected host configuration. Fields are best-effort — a failed
// detection leaves the field zero-valued and appends to Warnings rather than
// failing the whole call. This way the UI always gets *something* to render.
type Specs struct {
	OS        string `json:"os"`         // runtime.GOOS: darwin, linux, windows
	Arch      string `json:"arch"`       // runtime.GOARCH: arm64, amd64
	ChipModel string `json:"chip_model"` // "Apple M3 Pro", "Intel...", "" if unknown
	CPUCount  int    `json:"cpu_count"`  // logical cores
	RAMGB     int    `json:"ram_gb"`     // total RAM (Apple Silicon: unified memory pool)
	GPUModel  string `json:"gpu_model"`  // NVIDIA model, "" if no NVIDIA GPU
	VRAMGB    int    `json:"vram_gb"`    // NVIDIA VRAM, 0 if none
	Hostname  string `json:"hostname"`
	IsMac     bool   `json:"is_mac"`
	IsAppleSilicon bool `json:"is_apple_silicon"`

	// Warnings collects non-fatal detection failures so the UI can surface them.
	Warnings []string `json:"warnings,omitempty"`
}

// Detect returns the host's specs. Never returns nil. RAM and CPU detection use
// gopsutil (cross-platform); chip model uses sysctl on macOS; NVIDIA detection
// shells out to nvidia-smi (skipped silently if not installed).
func Detect() (*Specs, error) {
	s := &Specs{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCount: runtime.NumCPU(),
		IsMac:    runtime.GOOS == "darwin",
		IsAppleSilicon: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64",
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		s.RAMGB = int(vm.Total / (1024 * 1024 * 1024))
	} else {
		s.Warnings = append(s.Warnings, "RAM detection failed: "+err.Error())
	}

	if host, err := hostname(); err == nil {
		s.Hostname = host
	}

	if s.IsMac {
		if chip, err := darwinChipModel(); err == nil {
			s.ChipModel = chip
		} else {
			s.Warnings = append(s.Warnings, "chip detection failed: "+err.Error())
		}
	} else {
		if chip, err := genericCPUModel(); err == nil && chip != "" {
			s.ChipModel = chip
		}
	}

	if gpu, vram, err := nvidiaGPU(); err == nil && gpu != "" {
		s.GPUModel = gpu
		s.VRAMGB = vram
	}

	return s, nil
}

// EffectiveMemoryGB returns the RAM available for model sizing. On Apple
// Silicon this is the unified memory pool; on NVIDIA hosts we take the larger
// of system RAM and VRAM (discrete VRAM is the binding constraint for CUDA
// inference, but on macOS there's no separate VRAM).
func (s *Specs) EffectiveMemoryGB() int {
	if s.VRAMGB > s.RAMGB {
		return s.VRAMGB
	}
	return s.RAMGB
}

// AppleChipFamily extracts "M1", "M2 Pro", "M3 Max" etc. from the full chip
// string. Returns "" on non-Apple chips.
func (s *Specs) AppleChipFamily() string {
	if !s.IsAppleSilicon {
		return ""
	}
	parts := strings.Fields(s.ChipModel)
	if len(parts) < 2 || parts[0] != "Apple" {
		return ""
	}
	family := parts[1]
	if len(parts) >= 3 {
		switch parts[2] {
		case "Pro", "Max", "Ultra":
			family += " " + parts[2]
		}
	}
	return family
}
