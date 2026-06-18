package config

// ==============================================================================
// Provider Defaults
// ==============================================================================

// providerDefaults contains the default configuration for all supported providers.
// These defaults include base URLs, recommended models, timeouts, and retry settings.
// All providers are disabled by default; users must explicitly enable them and add API keys.
var providerDefaults = map[string]ProviderConfig{
	"openai": {
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "",
		DefaultModel: "gpt-4o",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"anthropic": {
		BaseURL:      "https://api.anthropic.com/v1",
		APIKey:       "",
		DefaultModel: "claude-sonnet-4-20250514",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"ollama": {
		BaseURL:      "http://localhost:11434/v1",
		APIKey:       "",
		DefaultModel: "qwen3",
		MaxRetries:   3,
		Timeout:      180,
		Enabled:      false,
	},

	"gemini": {
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
		APIKey:       "",
		DefaultModel: "gemini-2.5-pro-preview-05-21",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"zai": {
		BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
		APIKey:       "",
		DefaultModel: "glm-4.7",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"groq": {
		BaseURL:      "https://api.groq.com/openai/v1",
		APIKey:       "",
		DefaultModel: "llama-3.3-70b-versatile",
		MaxRetries:   3,
		Timeout:      60,
		Enabled:      false,
	},

	"mistral": {
		BaseURL:      "https://api.mistral.ai/v1",
		APIKey:       "",
		DefaultModel: "codestral-latest",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"deepseek": {
		BaseURL:      "https://api.deepseek.com/v1",
		APIKey:       "",
		DefaultModel: "deepseek-coder",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"together": {
		BaseURL:      "https://api.together.ai/v1",
		APIKey:       "",
		DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo-Free",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"fireworks": {
		BaseURL:      "https://api.fireworks.ai/inference/v1",
		APIKey:       "",
		DefaultModel: "accounts/fireworks/models/llama-v3p3-70b-instruct",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},

	"xai": {
		BaseURL:      "https://api.x.ai/v1",
		APIKey:       "",
		DefaultModel: "grok-beta",
		MaxRetries:   3,
		Timeout:      120,
		Enabled:      false,
	},
}
