package config

import (
	"os"
	"strings"
	"testing"
)

// ==============================================================================
// Test Helpers
// ==============================================================================

// createTestConfigFile creates a temporary YAML config file for testing.
func createTestConfigFile(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatalf("failed to write temp file: %v", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		t.Fatalf("failed to close temp file: %v", err)
	}

	t.Cleanup(func() {
		os.Remove(f.Name())
	})

	return f.Name()
}

// ==============================================================================
// TestDefaultConfig
// ==============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := Default()

	// Test server defaults
	if cfg.Server.GRPCPort != 50051 {
		t.Errorf("Expected GRPCPort 50051, got %d", cfg.Server.GRPCPort)
	}
	if cfg.Server.RESTPort != 8080 {
		t.Errorf("Expected RESTPort 8080, got %d", cfg.Server.RESTPort)
	}
	if cfg.Server.TLSEnabled {
		t.Error("Expected TLSEnabled false, got true")
	}

	// Test logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format json, got %s", cfg.Logging.Format)
	}

	// Test memory defaults
	if cfg.Memory.DBPath != "./data/memory.db" {
		t.Errorf("Expected DBPath ./data/memory.db, got %s", cfg.Memory.DBPath)
	}
	if cfg.Memory.CompactionThreshold != 0.75 {
		t.Errorf("Expected CompactionThreshold 0.75, got %f", cfg.Memory.CompactionThreshold)
	}
	if cfg.Memory.MaxShortTermTokens != 4000 {
		t.Errorf("Expected MaxShortTermTokens 4000, got %d", cfg.Memory.MaxShortTermTokens)
	}

	// Test pool defaults
	if cfg.Pool.Workers != 4 {
		t.Errorf("Expected Workers 4, got %d", cfg.Pool.Workers)
	}
	if cfg.Pool.QueueSize != 100 {
		t.Errorf("Expected QueueSize 100, got %d", cfg.Pool.QueueSize)
	}

	// Test security defaults
	if cfg.Security.AuthEnabled {
		t.Error("Expected AuthEnabled false, got true")
	}
	if cfg.Security.RateLimit != 100 {
		t.Errorf("Expected RateLimit 100, got %d", cfg.Security.RateLimit)
	}
	if cfg.Security.TLSEnabled {
		t.Error("Expected Security TLSEnabled false, got true")
	}

	// Test that all 11 providers are present with correct defaults
	expectedProviders := []string{"openai", "anthropic", "ollama", "gemini", "zai", "groq", "mistral", "deepseek", "together", "fireworks", "xai"}
	for _, name := range expectedProviders {
		provider, exists := cfg.Providers[name]
		if !exists {
			t.Errorf("Expected provider %s to exist", name)
			continue
		}
		if provider.Enabled {
			t.Errorf("Expected provider %s to be disabled by default", name)
		}
		if provider.MaxRetries != 3 {
			t.Errorf("Expected provider %s to have MaxRetries 3, got %d", name, provider.MaxRetries)
		}
		if provider.Timeout != 120 {
			// ollama and groq have custom timeouts
			customTimeouts := map[string]int{"ollama": 180, "groq": 60}
			if expected, ok := customTimeouts[name]; ok {
				if provider.Timeout != expected {
					t.Errorf("Expected provider %s to have Timeout %d, got %v", name, expected, provider.Timeout)
				}
			} else {
				t.Errorf("Expected provider %s to have Timeout 120, got %v", name, provider.Timeout)
			}
		}
		if provider.BaseURL == "" {
			t.Errorf("Expected provider %s to have a BaseURL", name)
		}
	}

	// Roles are no longer seeded by Default() — agents are markdown-driven
	// via agents/*.md and registered by the orchestrator's loader. The
	// cfg.Roles map is intentionally empty here.
	if len(cfg.Roles) != 0 {
		t.Errorf("Default() should return empty Roles map, got %d entries", len(cfg.Roles))
	}
}

// ==============================================================================
// TestLoadFromYAML
// ==============================================================================

func TestLoadFromYAML(t *testing.T) {
	testYAML := `
server:
  grpc_port: 9000
  rest_port: 9001
  tls_enabled: true
  tls_cert_file: /path/to/cert.pem
  tls_key_file: /path/to/key.pem

logging:
  level: debug
  format: text

memory:
  db_path: /custom/path/memory.db
  compaction_threshold: 0.9
  max_shortterm_tokens: 8000

pool:
  workers: 8
  queue_size: 200

security:
  auth_enabled: true
  rate_limit: 50
  tls_enabled: true

providers:
  openai:
    base_url: https://custom.openai.com/v1
    api_key: sk-test123
    default_model: gpt-5
    max_retries: 5
    timeout: 60
    enabled: true

roles:
  developer:
    system_prompt: Custom developer prompt
    tools:
      - custom_tool
    preferred_provider: anthropic
    max_iterations: 20
`

	path := createTestConfigFile(t, testYAML)
	cfg, err := Load(path)

	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify server config loaded
	if cfg.Server.GRPCPort != 9000 {
		t.Errorf("Expected GRPCPort 9000, got %d", cfg.Server.GRPCPort)
	}
	if cfg.Server.RESTPort != 9001 {
		t.Errorf("Expected RESTPort 9001, got %d", cfg.Server.RESTPort)
	}
	if !cfg.Server.TLSEnabled {
		t.Error("Expected TLSEnabled true, got false")
	}
	if cfg.Server.TLSCertFile != "/path/to/cert.pem" {
		t.Errorf("Expected TLS cert /path/to/cert.pem, got %s", cfg.Server.TLSCertFile)
	}

	// Verify logging config loaded
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Expected log format text, got %s", cfg.Logging.Format)
	}

	// Verify memory config loaded
	if cfg.Memory.DBPath != "/custom/path/memory.db" {
		t.Errorf("Expected DBPath /custom/path/memory.db, got %s", cfg.Memory.DBPath)
	}

	// Verify pool config loaded
	if cfg.Pool.Workers != 8 {
		t.Errorf("Expected Workers 8, got %d", cfg.Pool.Workers)
	}

	// Verify security config loaded
	if !cfg.Security.AuthEnabled {
		t.Error("Expected AuthEnabled true, got false")
	}
	if cfg.Security.RateLimit != 50 {
		t.Errorf("Expected RateLimit 50, got %d", cfg.Security.RateLimit)
	}

	// Verify provider config loaded
	provider, exists := cfg.Providers["openai"]
	if !exists {
		t.Fatal("Expected openai provider to exist")
	}
	if provider.BaseURL != "https://custom.openai.com/v1" {
		t.Errorf("Expected base_url https://custom.openai.com/v1, got %s", provider.BaseURL)
	}
	if provider.APIKey != "sk-test123" {
		t.Errorf("Expected api_key sk-test123, got %s", provider.APIKey)
	}
	if !provider.Enabled {
		t.Error("Expected provider enabled true, got false")
	}

	// Verify role config loaded
	role, exists := cfg.Roles["developer"]
	if !exists {
		t.Fatal("Expected developer role to exist")
	}
	if role.SystemPrompt != "Custom developer prompt" {
		t.Errorf("Expected custom system prompt, got %s", role.SystemPrompt)
	}
	if role.MaxIterations != 20 {
		t.Errorf("Expected max_iterations 20, got %d", role.MaxIterations)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")

	if err != nil {
		t.Fatalf("Expected nil error for non-existent file, got %v", err)
	}

	// Should return default config
	if cfg == nil {
		t.Fatal("Expected default config, got nil")
	}

	if cfg.Server.GRPCPort != 50051 {
		t.Error("Expected default config values")
	}
}

// ==============================================================================
// TestEnvOverrides
// ==============================================================================

func TestEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("KYOCI_GRPC_PORT", "7000")
	os.Setenv("KYOCI_LOG_LEVEL", "debug")
	os.Setenv("KYOCI_WORKERS", "16")
	os.Setenv("KYOCI_AUTH_ENABLED", "true")
	os.Setenv("KYOCI_PROVIDER_OPENAI_API_KEY", "sk-env-key")
	os.Setenv("KYOCI_PROVIDER_OPENAI_ENABLED", "true")

	defer func() {
		os.Unsetenv("KYOCI_GRPC_PORT")
		os.Unsetenv("KYOCI_LOG_LEVEL")
		os.Unsetenv("KYOCI_WORKERS")
		os.Unsetenv("KYOCI_AUTH_ENABLED")
		os.Unsetenv("KYOCI_PROVIDER_OPENAI_API_KEY")
		os.Unsetenv("KYOCI_PROVIDER_OPENAI_ENABLED")
	}()

	testYAML := `
server:
  grpc_port: 50051
  rest_port: 8080
  tls_enabled: false

logging:
  level: info
  format: json

pool:
  workers: 4
  queue_size: 100

security:
  auth_enabled: false
  rate_limit: 100

providers:
  openai:
    base_url: https://api.openai.com/v1
    api_key: ""
    default_model: gpt-4o
    max_retries: 3
    timeout: 120
    enabled: false
`

	path := createTestConfigFile(t, testYAML)
	cfg, err := Load(path)

	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify env overrides applied
	if cfg.Server.GRPCPort != 7000 {
		t.Errorf("Expected GRPCPort 7000 from env, got %d", cfg.Server.GRPCPort)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug from env, got %s", cfg.Logging.Level)
	}
	if cfg.Pool.Workers != 16 {
		t.Errorf("Expected Workers 16 from env, got %d", cfg.Pool.Workers)
	}
	if !cfg.Security.AuthEnabled {
		t.Error("Expected AuthEnabled true from env")
	}

	// Verify provider env overrides
	provider := cfg.Providers["openai"]
	if provider.APIKey != "sk-env-key" {
		t.Errorf("Expected API key from env, got %s", provider.APIKey)
	}
	if !provider.Enabled {
		t.Error("Expected provider enabled from env")
	}
}

// ==============================================================================
// TestValidate
// ==============================================================================

func TestValidateValidConfig(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSEnabled = false

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected valid config to pass validation, got error: %v", err)
	}
}

func TestValidateInvalidGRPCPort(t *testing.T) {
	cfg := Default()
	cfg.Server.GRPCPort = 70000 // Invalid port

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for invalid grpc_port")
	} else if !strings.Contains(err.Error(), "grpc_port") {
		t.Errorf("Expected error about grpc_port, got: %v", err)
	}
}

func TestValidateInvalidRESTPort(t *testing.T) {
	cfg := Default()
	cfg.Server.RESTPort = -1 // Invalid port

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for invalid rest_port")
	} else if !strings.Contains(err.Error(), "rest_port") {
		t.Errorf("Expected error about rest_port, got: %v", err)
	}
}

func TestValidateTLSWithoutCert(t *testing.T) {
	cfg := Default()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSCertFile = "" // Missing cert

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for TLS without cert")
	} else if !strings.Contains(err.Error(), "tls_cert_file") {
		t.Errorf("Expected error about tls_cert_file, got: %v", err)
	}
}

func TestValidateInvalidLogLevel(t *testing.T) {
	cfg := Default()
	cfg.Logging.Level = "invalid"

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for invalid log level")
	} else if !strings.Contains(err.Error(), "log_level") {
		t.Errorf("Expected error about log_level, got: %v", err)
	}
}

func TestValidateInvalidCompactionThreshold(t *testing.T) {
	cfg := Default()
	cfg.Memory.CompactionThreshold = 1.5 // Invalid: > 1.0

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for invalid compaction_threshold")
	} else if !strings.Contains(err.Error(), "compaction_threshold") {
		t.Errorf("Expected error about compaction_threshold, got: %v", err)
	}
}

func TestValidateNegativeWorkers(t *testing.T) {
	cfg := Default()
	cfg.Pool.Workers = -1

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for negative workers")
	} else if !strings.Contains(err.Error(), "workers") {
		t.Errorf("Expected error about workers, got: %v", err)
	}
}

func TestValidateEnabledProviderWithoutBaseURL(t *testing.T) {
	cfg := Default()
	provider := cfg.Providers["openai"]
	provider.Enabled = true
	provider.BaseURL = "" // Missing base URL
	cfg.Providers["openai"] = provider

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for enabled provider without base_url")
	} else if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("Expected error about base_url, got: %v", err)
	}
}

func TestValidateEnabledProviderWithoutAPIKey(t *testing.T) {
	cfg := Default()
	provider := cfg.Providers["openai"]
	provider.Enabled = true
	provider.APIKey = "" // Missing API key
	cfg.Providers["openai"] = provider

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for enabled provider without api_key")
	} else if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("Expected error about api_key, got: %v", err)
	}
}

func TestValidateOllamaWithoutAPIKey(t *testing.T) {
	cfg := Default()
	provider := cfg.Providers["ollama"]
	provider.Enabled = true
	provider.APIKey = "" // Ollama doesn't need API key
	provider.DefaultModel = "llama2"
	cfg.Providers["ollama"] = provider

	// Should pass validation - Ollama doesn't require API key
	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected Ollama to validate without API key, got error: %v", err)
	}
}

func TestValidateRoleWithEmptySystemPrompt(t *testing.T) {
	cfg := Default()
	// Roles are not seeded by Default() anymore — agents come from agents/*.md.
	// Add one manually to exercise the validator's per-role empty-prompt check.
	cfg.Roles["developer"] = &RoleConfig{
		SystemPrompt:  "",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for role with empty system_prompt")
	} else if !strings.Contains(err.Error(), "system_prompt") {
		t.Errorf("Expected error about system_prompt, got: %v", err)
	}
}

func TestValidateRoleWithUnknownProvider(t *testing.T) {
	cfg := Default()
	cfg.Roles["developer"] = &RoleConfig{
		SystemPrompt:      "nonempty",
		Tools:             []string{"terminal"},
		MaxIterations:     5,
		PreferredProvider: "nonexistent_provider",
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for role with unknown provider")
	} else if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("Expected error about unknown provider, got: %v", err)
	}
}

// ==============================================================================
// TestThreadSafeGetters
// ==============================================================================

func TestThreadSafeGetters(t *testing.T) {
	cfg := Default()

	// Test server getters
	if cfg.Server.GetGRPCPort() != 50051 {
		t.Errorf("GetGRPCPort() returned wrong value")
	}
	if cfg.Server.GetRESTPort() != 8080 {
		t.Errorf("GetRESTPort() returned wrong value")
	}

	// Test logging getters
	if cfg.Logging.GetLevel() != "info" {
		t.Errorf("GetLevel() returned wrong value")
	}
	if cfg.Logging.GetFormat() != "json" {
		t.Errorf("GetFormat() returned wrong value")
	}

	// Test memory getters
	if cfg.Memory.GetDBPath() != "./data/memory.db" {
		t.Errorf("GetDBPath() returned wrong value")
	}
	if cfg.Memory.GetCompactionThreshold() != 0.75 {
		t.Errorf("GetCompactionThreshold() returned wrong value")
	}

	// Test pool getters
	if cfg.Pool.GetWorkers() != 4 {
		t.Errorf("GetWorkers() returned wrong value")
	}
	if cfg.Pool.GetQueueSize() != 100 {
		t.Errorf("GetQueueSize() returned wrong value")
	}

	// Test security getters
	if cfg.Security.GetRateLimit() != 100 {
		t.Errorf("GetRateLimit() returned wrong value")
	}

	// Test provider getters
	provider, exists := cfg.GetProvider("openai")
	if !exists {
		t.Error("GetProvider() should find openai")
	}
	if provider.GetBaseURL() == "" {
		t.Error("Provider should have base URL")
	}

	// Roles are not seeded by Default() anymore — agents are markdown-driven
	// via agents/*.md. Verify GetRole returns exists=false for unknown names
	// rather than panicking on the empty map.
	if _, exists := cfg.GetRole("developer"); exists {
		t.Error("GetRole() should return exists=false on Default() (no seeded roles)")
	}
}

// ==============================================================================
// Thinking Config (agent.thinking)
// ==============================================================================

// TestDefaultConfig_ThinkingDefaults verifies that Default() returns a
// ThinkingConfig with sensible defaults matching the thinking system's
// internal constants.
func TestDefaultConfig_ThinkingDefaults(t *testing.T) {
	cfg := Default()

	// Enabled defaults to false in PR #1 — flipped to true in PR #2.
	if cfg.Agent.Thinking.Enabled {
		t.Error("Expected Agent.Thinking.Enabled false by default (PR #1), got true")
	}
	if cfg.Agent.Thinking.ToolBudget != 15 {
		t.Errorf("Expected ToolBudget 15, got %d", cfg.Agent.Thinking.ToolBudget)
	}
	if cfg.Agent.Thinking.MaxReflections != 3 {
		t.Errorf("Expected MaxReflections 3, got %d", cfg.Agent.Thinking.MaxReflections)
	}
	if cfg.Agent.Thinking.MaxReplans != 2 {
		t.Errorf("Expected MaxReplans 2, got %d", cfg.Agent.Thinking.MaxReplans)
	}
	if cfg.Agent.Thinking.ConfidenceThreshold != 0.7 {
		t.Errorf("Expected ConfidenceThreshold 0.7, got %f", cfg.Agent.Thinking.ConfidenceThreshold)
	}
	if !cfg.Agent.Thinking.FewShot {
		t.Error("Expected FewShot true by default")
	}
}

// TestLoadFromYAML_ThinkingConfig verifies that the agent.thinking section
// in the YAML file is parsed into ThinkingConfig.
func TestLoadFromYAML_ThinkingConfig(t *testing.T) {
	testYAML := `
agent:
  thinking:
    enabled: true
    tool_budget: 25
    max_reflections: 5
    max_replans: 3
    confidence_threshold: 0.85
    few_shot: false

providers:
  ollama:
    enabled: true
    base_url: "http://localhost:11434/v1"
    default_model: "qwen3.5:9b"
    timeout: 120
    max_retries: 2
`

	path := createTestConfigFile(t, testYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !cfg.Agent.Thinking.Enabled {
		t.Error("Expected Agent.Thinking.Enabled true, got false")
	}
	if cfg.Agent.Thinking.ToolBudget != 25 {
		t.Errorf("Expected ToolBudget 25, got %d", cfg.Agent.Thinking.ToolBudget)
	}
	if cfg.Agent.Thinking.MaxReflections != 5 {
		t.Errorf("Expected MaxReflections 5, got %d", cfg.Agent.Thinking.MaxReflections)
	}
	if cfg.Agent.Thinking.MaxReplans != 3 {
		t.Errorf("Expected MaxReplans 3, got %d", cfg.Agent.Thinking.MaxReplans)
	}
	if cfg.Agent.Thinking.ConfidenceThreshold != 0.85 {
		t.Errorf("Expected ConfidenceThreshold 0.85, got %f", cfg.Agent.Thinking.ConfidenceThreshold)
	}
	if cfg.Agent.Thinking.FewShot {
		t.Error("Expected FewShot false, got true")
	}
}

// TestValidate_InvalidThinkingToolBudget verifies that a negative or zero
// tool_budget fails validation.
func TestValidate_InvalidThinkingToolBudget(t *testing.T) {
	cfg := Default()
	cfg.Agent.Thinking.ToolBudget = 0

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for zero tool_budget")
	} else if !strings.Contains(err.Error(), "tool_budget") {
		t.Errorf("Expected error about tool_budget, got: %v", err)
	}
}

// TestValidate_InvalidThinkingConfidenceThreshold verifies that a
// confidence_threshold outside [0, 1] fails validation.
func TestValidate_InvalidThinkingConfidenceThreshold(t *testing.T) {
	cfg := Default()
	cfg.Agent.Thinking.ConfidenceThreshold = 1.5

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for confidence_threshold > 1.0")
	} else if !strings.Contains(err.Error(), "confidence_threshold") {
		t.Errorf("Expected error about confidence_threshold, got: %v", err)
	}
}

// TestValidate_InvalidThinkingMaxReflections verifies that negative
// max_reflections fails validation.
func TestValidate_InvalidThinkingMaxReflections(t *testing.T) {
	cfg := Default()
	cfg.Agent.Thinking.MaxReflections = -1

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for negative max_reflections")
	} else if !strings.Contains(err.Error(), "max_reflections") {
		t.Errorf("Expected error about max_reflections, got: %v", err)
	}
}

// ==============================================================================
// Prompt Skill Config (prompt_skills)
// ==============================================================================

// TestDefaultConfig_PromptSkillDefaults verifies Default() returns a
// PromptSkillConfig with the expected defaults.
func TestDefaultConfig_PromptSkillDefaults(t *testing.T) {
	cfg := Default()

	if !cfg.PromptSkill.Enabled {
		t.Error("Expected PromptSkill.Enabled true by default")
	}
	if cfg.PromptSkill.Dir != "data/skills" {
		t.Errorf("Expected Dir 'data/skills', got %q", cfg.PromptSkill.Dir)
	}
	if cfg.PromptSkill.MaxSkillsPerTask != 4 {
		t.Errorf("Expected MaxSkillsPerTask 4, got %d", cfg.PromptSkill.MaxSkillsPerTask)
	}
	if cfg.PromptSkill.MaxTotalChars != 12000 {
		t.Errorf("Expected MaxTotalChars 12000, got %d", cfg.PromptSkill.MaxTotalChars)
	}
}

// TestValidate_InvalidPromptSkillMaxSkills verifies a non-positive
// max_skills_per_task fails validation.
func TestValidate_InvalidPromptSkillMaxSkills(t *testing.T) {
	cfg := Default()
	cfg.PromptSkill.MaxSkillsPerTask = 0

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for max_skills_per_task=0")
	} else if !strings.Contains(err.Error(), "max_skills_per_task") {
		t.Errorf("Expected error about max_skills_per_task, got: %v", err)
	}
}

// TestLoadFromYAML_PromptSkillConfig verifies the prompt_skills section parses.
func TestLoadFromYAML_PromptSkillConfig(t *testing.T) {
	testYAML := `
prompt_skills:
  enabled: false
  dir: "/custom/skills"
  max_skills_per_task: 8
  max_total_chars: 5000
`
	path := createTestConfigFile(t, testYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.PromptSkill.Enabled {
		t.Error("Expected Enabled false from YAML")
	}
	if cfg.PromptSkill.Dir != "/custom/skills" {
		t.Errorf("Expected Dir '/custom/skills', got %q", cfg.PromptSkill.Dir)
	}
	if cfg.PromptSkill.MaxSkillsPerTask != 8 {
		t.Errorf("Expected MaxSkillsPerTask 8, got %d", cfg.PromptSkill.MaxSkillsPerTask)
	}
	if cfg.PromptSkill.MaxTotalChars != 5000 {
		t.Errorf("Expected MaxTotalChars 5000, got %d", cfg.PromptSkill.MaxTotalChars)
	}
}