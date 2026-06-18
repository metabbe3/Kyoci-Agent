package recommend

// Tier is a single step in the local-model sizing ladder. MinRAM is the
// comfortable RAM floor (in GB) assuming Q4_K_M quantization with ~25%
// headroom for the KV cache and OS overhead.
//
// These numbers are conservative — power users can override via the Settings
// tab. The table is the de-facto consensus from Ollama's minimum-requirements
// table and community sizing guides; refine as newer quantization formats
// (e.g. Q5_K, IQ4) become standard.
type Tier struct {
	MinRAM      int    `json:"min_ram"`
	Model       string `json:"model"`
	ContextLen  int    `json:"context_len"`
	Note        string `json:"note"`
}

// tiers is ordered ascending by MinRAM. Recommend() walks it and classifies
// each tier against the host's effective memory.
var tiers = []Tier{
	{MinRAM: 8, Model: "qwen2.5:3b", ContextLen: 32_000, Note: "Lightweight, fits 8GB comfortably."},
	{MinRAM: 16, Model: "llama3.1:8b", ContextLen: 128_000, Note: "Balanced default for 16GB."},
	{MinRAM: 32, Model: "qwen2.5:14b", ContextLen: 128_000, Note: "Strong coding model for 32GB."},
	{MinRAM: 64, Model: "qwen2.5:32b", ContextLen: 128_000, Note: "Flagship-tier local on 64GB."},
	{MinRAM: 96, Model: "llama3.3:70b", ContextLen: 128_000, Note: "Frontier-class local on 96GB+."},
}
