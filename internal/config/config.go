package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/apperr"
	"gopkg.in/yaml.v3"
)

// ==============================================================================
// Config Types
// ==============================================================================

// ServerConfig holds server configuration settings.
// This includes gRPC and REST server ports and TLS settings.
type ServerConfig struct {
	// GRPCPort is the port number for the gRPC server
	// Default: 50051
	GRPCPort int `yaml:"grpc_port" env:"KYOCI_GRPC_PORT"`

	// RESTPort is the port number for the REST API server
	// Default: 8080
	RESTPort int `yaml:"rest_port" env:"KYOCI_REST_PORT"`

	// TLSEnabled indicates whether TLS is enabled for the server
	// Default: false
	TLSEnabled bool `yaml:"tls_enabled" env:"KYOCI_TLS_ENABLED"`

	// TLSCertFile is the path to the TLS certificate file
	// Required if TLSEnabled is true
	TLSCertFile string `yaml:"tls_cert_file" env:"KYOCI_TLS_CERT_FILE"`

	// TLSKeyFile is the path to the TLS private key file
	// Required if TLSEnabled is true
	TLSKeyFile string `yaml:"tls_key_file" env:"KYOCI_TLS_KEY_FILE"`

	// AgentGRPCPort is the TCP port for the optional AgentService gRPC API
	// (Execute/ExecuteStream/GetStatus). 0 (default) means the server is NOT
	// started — the HTTP API and the benchmark suite are unaffected. Setting a
	// positive port enables the gRPC surface alongside the HTTP server.
	AgentGRPCPort int `yaml:"agent_grpc_port" env:"KYOCI_AGENT_GRPC_PORT"`
}

// GetGRPCPort returns the gRPC port.
func (sc *ServerConfig) GetGRPCPort() int {
	return sc.GRPCPort
}

// GetRESTPort returns the REST port.
func (sc *ServerConfig) GetRESTPort() int {
	return sc.RESTPort
}

// GetTLSEnabled returns whether TLS is enabled.
func (sc *ServerConfig) GetTLSEnabled() bool {
	return sc.TLSEnabled
}

// GetTLSCertFile returns the TLS cert file path.
func (sc *ServerConfig) GetTLSCertFile() string {
	return sc.TLSCertFile
}

// GetTLSKeyFile returns the TLS key file path.
func (sc *ServerConfig) GetTLSKeyFile() string {
	return sc.TLSKeyFile
}

// LogConfig holds logging configuration settings.
type LogConfig struct {
	// Level is the logging level (debug, info, warn, error)
	// Default: info
	Level string `yaml:"level" env:"KYOCI_LOG_LEVEL"`

	// Format is the log output format (json, text)
	// Default: json
	Format string `yaml:"format" env:"KYOCI_LOG_FORMAT"`

	// PerRunEnabled controls whether each orchestrated agent run also writes
	// a JSON-lines trace to logs/<YYYY-MM-DD>/run_<task_id>.log in addition
	// to stdout. Server runtime logs still go to stdout (12-factor); these
	// per-run files are job artifacts, not server logs.
	// Default: true
	PerRunEnabled bool `yaml:"per_run_enabled" env:"KYOCI_LOGGING_PER_RUN_ENABLED"`

	// PerRunDir is the root directory for per-run log files. The actual files
	// land under <PerRunDir>/<YYYY-MM-DD>/. Default: "logs"
	PerRunDir string `yaml:"per_run_dir" env:"KYOCI_LOGGING_PER_RUN_DIR"`
}

// TasksConfig configures the per-task workspace layout. Each orchestrated agent
// run that produces user-facing files writes them under <Dir>/<task_id>/.
type TasksConfig struct {
	// Dir is the root directory for per-task workspaces. Default: "tasks".
	Dir string `yaml:"dir" env:"KYOCI_TASKS_DIR"`

	// RetentionDays is informational — the number of days completed task
	// folders are kept before manual cleanup. No automatic deletion is
	// performed; this field exists so external retention scripts can read it.
	// Default: 30. 0 = retain forever.
	RetentionDays int `yaml:"retention_days" env:"KYOCI_TASKS_RETENTION_DAYS"`
}

// GetLevel returns the log level.
func (lc *LogConfig) GetLevel() string {
	return lc.Level
}

// GetFormat returns the log format.
func (lc *LogConfig) GetFormat() string {
	return lc.Format
}

// MemoryConfig holds memory storage configuration settings.
type MemoryConfig struct {
	// DBPath is the path to the SQLite database file
	// Default: ./data/memory.db
	DBPath string `yaml:"db_path" env:"KYOCI_DB_PATH"`

	// CompactionThreshold is the memory usage ratio (0.0-1.0) at which compaction triggers
	// Default: 0.75
	CompactionThreshold float64 `yaml:"compaction_threshold" env:"KYOCI_COMPACTION_THRESHOLD"`

	// MaxShortTermTokens is the maximum number of tokens to keep in short-term memory
	// Default: 4000
	MaxShortTermTokens int `yaml:"max_shortterm_tokens" env:"KYOCI_MAX_SHORTTERM_TOKENS"`
}

// GetDBPath returns the database path.
func (mc *MemoryConfig) GetDBPath() string {
	return mc.DBPath
}

// GetCompactionThreshold returns the compaction threshold.
func (mc *MemoryConfig) GetCompactionThreshold() float64 {
	return mc.CompactionThreshold
}

// GetMaxShortTermTokens returns the max short-term tokens.
func (mc *MemoryConfig) GetMaxShortTermTokens() int {
	return mc.MaxShortTermTokens
}

// PoolConfig holds worker pool configuration settings.
type PoolConfig struct {
	// Workers is the number of concurrent worker goroutines
	// Default: 4
	Workers int `yaml:"workers" env:"KYOCI_WORKERS"`

	// QueueSize is the maximum number of tasks in the queue before blocking
	// Default: 100
	QueueSize int `yaml:"queue_size" env:"KYOCI_QUEUE_SIZE"`
}

// GetWorkers returns the worker count.
func (pc *PoolConfig) GetWorkers() int {
	return pc.Workers
}

// GetQueueSize returns the queue size.
func (pc *PoolConfig) GetQueueSize() int {
	return pc.QueueSize
}

// SecurityConfig holds security configuration settings.
type SecurityConfig struct {
	// AuthEnabled indicates whether authentication is required
	// Default: false
	AuthEnabled bool `yaml:"auth_enabled" env:"KYOCI_AUTH_ENABLED"`

	// RateLimit is the maximum number of requests per minute per client
	// Default: 100
	RateLimit int `yaml:"rate_limit" env:"KYOCI_RATE_LIMIT"`

	// TLSEnabled indicates whether TLS is required for connections
	// Default: false
	TLSEnabled bool `yaml:"tls_enabled" env:"KYOCI_SECURITY_TLS_ENABLED"`
}

// GetAuthEnabled returns whether auth is enabled.
func (sc *SecurityConfig) GetAuthEnabled() bool {
	return sc.AuthEnabled
}

// GetRateLimit returns the rate limit.
func (sc *SecurityConfig) GetRateLimit() int {
	return sc.RateLimit
}

// GetTLSEnabled returns whether security TLS is enabled.
func (sc *SecurityConfig) GetTLSEnabled() bool {
	return sc.TLSEnabled
}

// TelegramConfig holds configuration for the Telegram bot gateway.
type TelegramConfig struct {
	// Enabled controls whether the Telegram gateway is active
	Enabled bool `yaml:"enabled" env:"KYOCI_TELEGRAM_ENABLED"`

	// Token is the Telegram Bot API token from @BotFather
	Token string `yaml:"token" env:"KYOCI_TELEGRAM_TOKEN"`

	// AllowedUsers is a comma-separated list of Telegram user IDs allowed to use the bot
	// Empty = allow all users
	AllowedUsers string `yaml:"allowed_users" env:"KYOCI_TELEGRAM_ALLOWED_USERS"`

	// PollTimeout is the long-polling timeout in seconds (default: 30)
	PollTimeout int `yaml:"poll_timeout" env:"KYOCI_TELEGRAM_POLL_TIMEOUT"`
}

// GetEnabled returns whether Telegram gateway is enabled.
func (tc *TelegramConfig) GetEnabled() bool {
	return tc.Enabled
}

// GetToken returns the Telegram bot token.
func (tc *TelegramConfig) GetToken() string {
	return tc.Token
}

// ProviderConfig holds configuration for a specific LLM provider.
type ProviderConfig struct {
	// BaseURL is the base URL for the provider's API
	BaseURL string `yaml:"base_url" env:"KYOCI_PROVIDER_BASE_URL"`

	// APIKey is the API key for authenticating with the provider
	APIKey string `yaml:"api_key" env:"KYOCI_PROVIDER_API_KEY"`

	// DefaultModel is the default model to use for this provider
	DefaultModel string `yaml:"default_model"`

	// MaxRetries is the maximum number of retry attempts for failed requests
	// Default: 3
	MaxRetries int `yaml:"max_retries" env:"KYOCI_PROVIDER_MAX_RETRIES"`

	// Timeout is the timeout duration for API requests in seconds
	// Default: 120
	Timeout int `yaml:"timeout" env:"KYOCI_PROVIDER_TIMEOUT"`

	// Enabled indicates whether this provider is active and can be used
	// Default: false
	Enabled bool `yaml:"enabled" env:"KYOCI_PROVIDER_ENABLED"`
}

// GetBaseURL returns the base URL.
func (pc *ProviderConfig) GetBaseURL() string {
	return pc.BaseURL
}

// GetAPIKey returns the API key.
func (pc *ProviderConfig) GetAPIKey() string {
	return pc.APIKey
}

// GetDefaultModel returns the default model.
func (pc *ProviderConfig) GetDefaultModel() string {
	return pc.DefaultModel
}

// GetMaxRetries returns the max retries.
func (pc *ProviderConfig) GetMaxRetries() int {
	return pc.MaxRetries
}

// GetTimeout returns the timeout as time.Duration.
func (pc *ProviderConfig) GetTimeout() time.Duration {
	if pc.Timeout <= 0 {
		return 120 * time.Second
	}
	return time.Duration(pc.Timeout) * time.Second
}

// GetEnabled returns whether the provider is enabled.
func (pc *ProviderConfig) GetEnabled() bool {
	return pc.Enabled
}

// RoleConfig holds configuration for a specific agent role.
type RoleConfig struct {
	// SystemPrompt is the system prompt that defines the role's behavior
	SystemPrompt string `yaml:"system_prompt"`

	// Tools is the list of tool names available to this role
	Tools []string `yaml:"tools"`

	// PreferredProvider is the preferred LLM provider for this role
	PreferredProvider string `yaml:"preferred_provider"`

	// Model overrides the provider's default model for this role
	// Example: "qwen2.5-coder:14b" for coding roles, "" for provider default
	Model string `yaml:"model"`

	// MaxIterations is the maximum number of iterations for tasks in this role
	// Default: 10
	MaxIterations int `yaml:"max_iterations" env:"KYOCI_ROLE_MAX_ITERATIONS"`
}

// GetModel returns the model override (empty string = use provider default).
func (rc *RoleConfig) GetModel() string {
	return rc.Model
}

// GetSystemPrompt returns the system prompt.
func (rc *RoleConfig) GetSystemPrompt() string {
	return rc.SystemPrompt
}

// GetTools returns the tools list.
func (rc *RoleConfig) GetTools() []string {
	return rc.Tools
}

// GetPreferredProvider returns the preferred provider.
func (rc *RoleConfig) GetPreferredProvider() string {
	return rc.PreferredProvider
}

// GetMaxIterations returns the max iterations.
func (rc *RoleConfig) GetMaxIterations() int {
	return rc.MaxIterations
}

// Config is the main configuration structure for the Kyoci Agent.
// It contains all configuration sections and provides methods for loading,
// validating, and accessing configuration values.
type Config struct {
	// Server holds server configuration
	Server ServerConfig `yaml:"server"`

	// Logging holds logging configuration
	Logging LogConfig `yaml:"logging"`

	// Memory holds memory storage configuration
	Memory MemoryConfig `yaml:"memory"`

	// Pool holds worker pool configuration
	Pool PoolConfig `yaml:"pool"`

	// Security holds security configuration
	Security SecurityConfig `yaml:"security"`

	// Providers is a map of provider name to provider configuration
	Providers map[string]ProviderConfig `yaml:"providers"`

	// Roles is a map of role name to role configuration
	Roles map[string]*RoleConfig `yaml:"roles"`

	// Telegram holds Telegram bot gateway configuration
	Telegram TelegramConfig `yaml:"telegram"`

	// MCP holds Model Context Protocol server configurations
	MCP MCPConfig `yaml:"mcp"`

	// Webhook holds webhook gateway configuration
	Webhook WebhookConfig `yaml:"webhook"`

	// Agent holds agent-level configuration (thinking system, etc.)
	Agent AgentConfig `yaml:"agent"`

	// PromptSkill holds configuration for the prompt-skill knowledge layer
	// (markdown workflow bundles injected into the system prompt per task).
	PromptSkill PromptSkillConfig `yaml:"prompt_skills"`

	// HITL holds Human-In-The-Loop configuration. When Enabled, the server
	// starts a gRPC server (port HITL.Port) that operator clients subscribe
	// to. The orchestrator emits HelpRequests when it exhausts its retry
	// budget on tasks carrying a VERIFY: directive.
	HITL HITLConfig `yaml:"hitl"`

	// Tasks holds per-task workspace configuration. Each orchestrated run
	// that writes user-facing files lands them under Tasks.Dir/<task_id>/.
	Tasks TasksConfig `yaml:"tasks"`

	// AgentsDir is the path to the markdown-driven agent definitions.
	// The orchestrator's loader walks this dir for *.md files at startup and
	// registers each as an agent. Default "agents" at the repo root.
	// Env override: KYOCI_AGENTS_DIR.
	AgentsDir string `yaml:"agents_dir"`
}

// HITLConfig configures the Human-In-The-Loop fallback subsystem.
type HITLConfig struct {
	// Enabled controls whether the gRPC HITL server is started. Default: false.
	Enabled bool `yaml:"enabled" env:"KYOCI_HITL_ENABLED"`

	// Port is the TCP port the HITL gRPC server listens on. Default: 50052.
	Port int `yaml:"port" env:"KYOCI_HITL_PORT"`

	// RequestTimeout is how long (in seconds) the orchestrator blocks waiting
	// for an operator hint after emitting a HelpRequest. Default: 300 (5 min).
	RequestTimeout int `yaml:"request_timeout" env:"KYOCI_HITL_REQUEST_TIMEOUT"`
}

// PromptSkillConfig configures the prompt-skill subsystem. Skills are markdown
// files with YAML frontmatter discovered at startup; relevant ones are matched
// per task and their bodies injected into the system prompt.
type PromptSkillConfig struct {
	// Enabled controls whether the prompt-skill injector is wired into the
	// agent's context injector chain. When false, no skill content is injected.
	Enabled bool `yaml:"enabled" env:"KYOCI_PROMPT_SKILLS_ENABLED"`

	// Dir is the filesystem path walked recursively for *.md skill files.
	// A missing dir is tolerated (empty registry) so the agent still boots.
	// Default: "data/skills"
	Dir string `yaml:"dir" env:"KYOCI_PROMPT_SKILLS_DIR"`

	// MaxSkillsPerTask caps how many matched skills are injected for a single
	// task, to keep the system prompt bounded. Default: 4
	MaxSkillsPerTask int `yaml:"max_skills_per_task"`

	// MaxTotalChars caps the total injected body bytes across all matched
	// skills for one task. Default: 12000
	MaxTotalChars int `yaml:"max_total_chars"`
}

// MCPServerConfig holds configuration for a single MCP server.
type MCPServerConfig struct {
	// Command is the executable to run (e.g. "npx", "python3")
	Command string `yaml:"command"`
	// Args are command-line arguments
	Args []string `yaml:"args"`
	// Env are environment variables for the subprocess
	Env map[string]string `yaml:"env"`
	// Enabled controls whether this server is started
	Enabled bool `yaml:"enabled"`
}

// MCPConfig holds Model Context Protocol configuration.
type MCPConfig struct {
	// Servers is a map of server name to server config
	Servers map[string]MCPServerConfig `yaml:"servers"`
}

// WebhookConfig holds webhook gateway configuration.
type WebhookConfig struct {
	// Enabled controls whether the webhook endpoint is active
	Enabled bool `yaml:"enabled"`
	// Secret is the bearer token for webhook auth
	Secret string `yaml:"secret"`
}

// ThinkingConfig holds configuration for the hybrid thinking state machine
// (Assess → Plan → Execute → Verify → Reflect → Done). These knobs control
// loop budgets and escalation thresholds that govern how the agent reasons
// about complex tasks on small models.
type ThinkingConfig struct {
	// Enabled switches the agent from free-ReAct to the thinking state machine.
	// Defaults to false in PR #1 for bisectability; flip to true in PR #2.
	Enabled bool `yaml:"enabled" env:"KYOCI_THINKING_ENABLED"`

	// ToolBudget is the maximum number of tool calls allowed in a single
	// thinking loop before the agent is forced into Reflect or honest termination.
	// Default: 15
	ToolBudget int `yaml:"tool_budget" env:"KYOCI_THINKING_TOOL_BUDGET"`

	// MaxReflections is the hard cap on Reflect state visits per task.
	// After this many reflections, the agent terminates honestly with what it has.
	// Default: 3
	MaxReflections int `yaml:"max_reflections" env:"KYOCI_THINKING_MAX_REFLECTIONS"`

	// MaxReplans is the hard cap on replanning (Plan state re-entry from Reflect).
	// Prevents endless plan thrashing. Default: 2
	MaxReplans int `yaml:"max_replans" env:"KYOCI_THINKING_MAX_REPLANS"`

	// ConfidenceThreshold is the minimum Assess confidence (0.0-1.0) required
	// for the fast path (skip Plan, go straight to Execute). Below this,
	// complex tasks escalate to the full Plan loop.
	// Default: 0.7
	ConfidenceThreshold float64 `yaml:"confidence_threshold" env:"KYOCI_THINKING_CONFIDENCE_THRESHOLD"`

	// FewShot controls whether the worked example is prepended to the system
	// prompt. The one-shot example anchors JSON output format on small models.
	// Default: true
	FewShot bool `yaml:"few_shot" env:"KYOCI_THINKING_FEW_SHOT"`
}

// AgentConfig holds agent-level configuration that is not role-specific.
type AgentConfig struct {
	// Thinking holds the hybrid thinking state machine configuration.
	Thinking ThinkingConfig `yaml:"thinking"`

	// Orchestration holds the Orchestrator-Worker pipeline configuration.
	// When Enabled, agents route through the Planner → Dispatcher → Workers →
	// Synthesizer pipeline instead of the thinking state machine or legacy
	// ReAct loop. This is the recommended default for multi-step tasks on
	// 14B models — each LLM call gets exactly one focused job.
	Orchestration OrchestrationConfig `yaml:"orchestration"`
}

// OrchestrationConfig holds configuration for the Orchestrator-Worker pipeline.
// Each LLM call in the pipeline has exactly one job (decompose, execute-one-step,
// or synthesize), which is the capability envelope a 14B model handles reliably.
type OrchestrationConfig struct {
	// Enabled routes agents through executeOrchestrated() instead of the
	// thinking state machine or legacy ReAct loop.
	Enabled bool `yaml:"enabled" env:"KYOCI_ORCHESTRATION_ENABLED"`

	// MaxSteps caps the number of steps the planner may emit. Beyond this the
	// plan is usually over-decomposed and the synthesizer struggles to use it.
	// Default: 6
	MaxSteps int `yaml:"max_steps" env:"KYOCI_ORCHESTRATION_MAX_STEPS"`

	// MaxParallel bounds concurrent worker goroutines. Default: 3
	MaxParallel int `yaml:"max_parallel" env:"KYOCI_ORCHESTRATION_MAX_PARALLEL"`

	// WorkerMaxIterations is the per-worker ReAct iteration cap. Default: 8
	WorkerMaxIterations int `yaml:"worker_max_iterations" env:"KYOCI_ORCHESTRATION_WORKER_MAX_ITERATIONS"`

	// WorkerMaxToolCalls is the per-worker tool-call budget. Default: 8
	WorkerMaxToolCalls int `yaml:"worker_max_tool_calls" env:"KYOCI_ORCHESTRATION_WORKER_MAX_TOOL_CALLS"`

	// MaxRetries caps how many times the orchestrator retries a task whose
	// VERIFY directive exits non-zero before falling back to HITL. 0 disables
	// the retry loop entirely (legacy single-shot behavior). Default: 0.
	// The L4 benchmark sets this to 2.
	MaxRetries int `yaml:"max_retries" env:"KYOCI_ORCHESTRATION_MAX_RETRIES"`
}

// ==============================================================================
// Config Load and Validation
// ==============================================================================

// Default returns a new Config with sensible default values.
// This includes pre-configured provider defaults and role defaults.
func Default() *Config {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort:   50051,
			RESTPort:   8080,
			TLSEnabled: false,
		},
		Logging: LogConfig{
			Level:         "info",
			Format:        "json",
			PerRunEnabled: true,
			PerRunDir:     "logs",
		},
		Tasks: TasksConfig{
			Dir:           "tasks",
			RetentionDays: 30,
		},
		AgentsDir: "agents",
		Memory: MemoryConfig{
			DBPath:              "./data/memory.db",
			CompactionThreshold: 0.75,
			MaxShortTermTokens:  4000,
		},
		Pool: PoolConfig{
			Workers:   4,
			QueueSize: 100,
		},
		Security: SecurityConfig{
			AuthEnabled: false,
			RateLimit:   100,
			TLSEnabled:  false,
		},
		Providers: make(map[string]ProviderConfig),
		Roles:     make(map[string]*RoleConfig),
		Agent: AgentConfig{
			Thinking: ThinkingConfig{
				Enabled:             false, // PR #1 default; flip to true in PR #2
				ToolBudget:          15,
				MaxReflections:      3,
				MaxReplans:          2,
				ConfidenceThreshold: 0.7,
				FewShot:             true,
			},
			Orchestration: OrchestrationConfig{
				// Default OFF here; config/default.yaml flips it ON so the
				// orchestrator-worker pipeline is the shipping execution path.
				Enabled:             false,
				MaxSteps:            6,
				MaxParallel:         3,
				WorkerMaxIterations: 8,
				WorkerMaxToolCalls:  8,
				MaxRetries:          0, // opt-in via config
			},
		},
		PromptSkill: PromptSkillConfig{
			Enabled:          true,
			Dir:              "data/skills",
			MaxSkillsPerTask: 4,
			MaxTotalChars:    12000,
		},
		HITL: HITLConfig{
			Enabled:        false,
			Port:           50052,
			RequestTimeout: 300,
		},
	}

	// Apply provider defaults
	for name, defaults := range providerDefaults {
		cfg.Providers[name] = defaults
	}

	// Role defaults are no longer seeded here. Agents are markdown-driven —
	// operators define them in agents/*.md and the orchestrator's loader
	// registers them at startup. cfg.Roles stays empty by default and the
	// YAML loader below no longer merges legacy role overrides either.

	return cfg
}

// Load loads the configuration from a YAML file at the given path.
// It merges defaults with the file contents, then applies environment variable overrides.
// Returns an error if the file cannot be read, parsed, or validated.
func Load(path string) (*Config, error) {
	slog.Info("Loading configuration", "path", path)

	// Start with defaults
	cfg := Default()

	// Read the configuration file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("Configuration file not found, using defaults", "path", path)
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal YAML into the default config
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Legacy role-merge shim removed. Roles are now markdown-driven; the
	// YAML `roles:` section, if present, is preserved on cfg.Roles for
	// backward-compat reads but no longer seeded from a Go map.

	slog.Info("Configuration loaded from file", "path", path)

	// Apply environment variable overrides
	if err := cfg.applyEnvOverrides(); err != nil {
		return nil, fmt.Errorf("failed to apply env overrides: %w", err)
	}

	// Validate the final configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	slog.Info("Configuration validated successfully")
	return cfg, nil
}

// Validate validates all configuration values and returns an error if any are
// invalid. Errors are typed apperr errors (Kind=Invalid) with stable codes so
// the API layer can map them to HTTP 400 via apperr.CodeToHTTP. The human
// messages are unchanged from the original fmt.Errorf forms for log/test
// substring compatibility.
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return apperr.Newf("config.grpc_port", apperr.KindInvalid, "invalid grpc_port: must be between 1 and 65535")
	}
	if c.Server.RESTPort <= 0 || c.Server.RESTPort > 65535 {
		return apperr.Newf("config.rest_port", apperr.KindInvalid, "invalid rest_port: must be between 1 and 65535")
	}
	if c.Server.TLSEnabled {
		if c.Server.TLSCertFile == "" {
			return apperr.Newf("config.tls_cert_file", apperr.KindInvalid, "tls_enabled is true but tls_cert_file is not specified")
		}
		if c.Server.TLSKeyFile == "" {
			return apperr.Newf("config.tls_key_file", apperr.KindInvalid, "tls_enabled is true but tls_key_file is not specified")
		}
	}

	// Validate logging config
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return apperr.Newf("config.log_level", apperr.KindInvalid, "invalid log_level: must be one of debug, info, warn, error")
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		return apperr.Newf("config.log_format", apperr.KindInvalid, "invalid log_format: must be json or text")
	}
	if c.Logging.PerRunEnabled && c.Logging.PerRunDir == "" {
		return apperr.Newf("config.logging.per_run_dir", apperr.KindInvalid, "logging.per_run_dir cannot be empty when per_run_enabled is true")
	}
	if c.Tasks.Dir == "" {
		return apperr.Newf("config.tasks.dir", apperr.KindInvalid, "tasks.dir cannot be empty")
	}
	if c.Tasks.RetentionDays < 0 {
		return apperr.Newf("config.tasks.retention_days", apperr.KindInvalid, "tasks.retention_days must be non-negative")
	}

	// Validate memory config
	if c.Memory.DBPath == "" {
		return apperr.Newf("config.db_path", apperr.KindInvalid, "db_path cannot be empty")
	}
	if c.Memory.CompactionThreshold < 0.0 || c.Memory.CompactionThreshold > 1.0 {
		return apperr.Newf("config.compaction_threshold", apperr.KindInvalid, "compaction_threshold must be between 0.0 and 1.0")
	}
	if c.Memory.MaxShortTermTokens < 0 {
		return apperr.Newf("config.max_shortterm_tokens", apperr.KindInvalid, "max_shortterm_tokens must be non-negative")
	}

	// Validate pool config
	if c.Pool.Workers <= 0 {
		return apperr.Newf("config.workers", apperr.KindInvalid, "workers must be positive")
	}
	if c.Pool.QueueSize <= 0 {
		return apperr.Newf("config.queue_size", apperr.KindInvalid, "queue_size must be positive")
	}

	// Validate security config
	if c.Security.RateLimit <= 0 {
		return apperr.Newf("config.rate_limit", apperr.KindInvalid, "rate_limit must be positive")
	}

	// Validate providers
	for name, provider := range c.Providers {
		if provider.Enabled {
			if provider.BaseURL == "" {
				return apperr.Newf("config.provider_base_url", apperr.KindInvalid, "provider %s is enabled but has no base_url", name)
			}
			if provider.APIKey == "" && !isLocalProviderName(name) {
				// Local OpenAI-compatible servers (Ollama, LM Studio) don't
				// require an API key. Anything else (cloud APIs) does.
				return apperr.Newf("config.provider_api_key", apperr.KindInvalid, "provider %s is enabled but has no api_key", name)
			}
			if provider.DefaultModel == "" {
				return apperr.Newf("config.provider_default_model", apperr.KindInvalid, "provider %s is enabled but has no default_model", name)
			}
			if provider.MaxRetries < 0 {
				return apperr.Newf("config.provider_max_retries", apperr.KindInvalid, "provider %s has invalid max_retries: must be non-negative", name)
			}
			if provider.Timeout <= 0 {
				return apperr.Newf("config.provider_timeout", apperr.KindInvalid, "provider %s has invalid timeout: must be positive", name)
			}
		}
	}

	// Validate roles
	for name, role := range c.Roles {
		if role.SystemPrompt == "" {
			return apperr.Newf("config.role_system_prompt", apperr.KindInvalid, "role %s has empty system_prompt", name)
		}
		if role.MaxIterations <= 0 {
			return apperr.Newf("config.role_max_iterations", apperr.KindInvalid, "role %s has invalid max_iterations: must be positive", name)
		}
		if role.PreferredProvider != "" {
			if _, exists := c.Providers[role.PreferredProvider]; !exists {
				return apperr.Newf("config.role_unknown_provider", apperr.KindInvalid, "role %s references unknown provider %s", name, role.PreferredProvider)
			}
		}
	}

	// Validate thinking config
	if c.Agent.Thinking.ToolBudget <= 0 {
		return apperr.Newf("config.thinking_tool_budget", apperr.KindInvalid, "agent.thinking.tool_budget must be positive")
	}
	if c.Agent.Thinking.MaxReflections < 0 {
		return apperr.Newf("config.thinking_max_reflections", apperr.KindInvalid, "agent.thinking.max_reflections must be non-negative")
	}
	if c.Agent.Thinking.MaxReplans < 0 {
		return apperr.Newf("config.thinking_max_replans", apperr.KindInvalid, "agent.thinking.max_replans must be non-negative")
	}
	if c.Agent.Thinking.ConfidenceThreshold < 0.0 || c.Agent.Thinking.ConfidenceThreshold > 1.0 {
		return apperr.Newf("config.thinking_confidence_threshold", apperr.KindInvalid, "agent.thinking.confidence_threshold must be between 0.0 and 1.0")
	}

	// Validate prompt-skill config
	if c.PromptSkill.Enabled {
		if c.PromptSkill.MaxSkillsPerTask <= 0 {
			return apperr.Newf("config.prompt_skills_max", apperr.KindInvalid, "prompt_skills.max_skills_per_task must be positive when enabled")
		}
		if c.PromptSkill.MaxTotalChars <= 0 {
			return apperr.Newf("config.prompt_skills_chars", apperr.KindInvalid, "prompt_skills.max_total_chars must be positive when enabled")
		}
	}

	return nil
}

// ==============================================================================
// Environment Variable Overrides
// ==============================================================================

// applyEnvOverrides applies environment variable overrides to the configuration.
func (c *Config) applyEnvOverrides() error {
	slog.Info("Applying environment variable overrides")

	// Server overrides
	if v := os.Getenv("KYOCI_GRPC_PORT"); v != "" {
		c.Server.GRPCPort = parseIntEnv(v, c.Server.GRPCPort)
		slog.Info("Override applied", "setting", "grpc_port", "value", c.Server.GRPCPort)
	}
	if v := os.Getenv("KYOCI_REST_PORT"); v != "" {
		c.Server.RESTPort = parseIntEnv(v, c.Server.RESTPort)
		slog.Info("Override applied", "setting", "rest_port", "value", c.Server.RESTPort)
	}
	if v := os.Getenv("KYOCI_TLS_ENABLED"); v != "" {
		c.Server.TLSEnabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "tls_enabled", "value", c.Server.TLSEnabled)
	}
	if v := os.Getenv("KYOCI_TLS_CERT_FILE"); v != "" {
		c.Server.TLSCertFile = v
		slog.Info("Override applied", "setting", "tls_cert_file", "value", v)
	}
	if v := os.Getenv("KYOCI_TLS_KEY_FILE"); v != "" {
		c.Server.TLSKeyFile = v
		slog.Info("Override applied", "setting", "tls_key_file", "value", v)
	}
	if v := os.Getenv("KYOCI_AGENT_GRPC_PORT"); v != "" {
		c.Server.AgentGRPCPort = parseIntEnv(v, c.Server.AgentGRPCPort)
		slog.Info("Override applied", "setting", "agent_grpc_port", "value", c.Server.AgentGRPCPort)
	}

	// Logging overrides
	if v := os.Getenv("KYOCI_LOG_LEVEL"); v != "" {
		c.Logging.Level = v
		slog.Info("Override applied", "setting", "log_level", "value", v)
	}
	if v := os.Getenv("KYOCI_LOG_FORMAT"); v != "" {
		c.Logging.Format = v
		slog.Info("Override applied", "setting", "log_format", "value", v)
	}
	if v := os.Getenv("KYOCI_LOGGING_PER_RUN_ENABLED"); v != "" {
		c.Logging.PerRunEnabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "logging.per_run_enabled", "value", c.Logging.PerRunEnabled)
	}
	if v := os.Getenv("KYOCI_LOGGING_PER_RUN_DIR"); v != "" {
		c.Logging.PerRunDir = v
		slog.Info("Override applied", "setting", "logging.per_run_dir", "value", v)
	}

	// Tasks overrides
	if v := os.Getenv("KYOCI_TASKS_DIR"); v != "" {
		c.Tasks.Dir = v
		slog.Info("Override applied", "setting", "tasks.dir", "value", v)
	}
	if v := os.Getenv("KYOCI_TASKS_RETENTION_DAYS"); v != "" {
		c.Tasks.RetentionDays = parseIntEnv(v, c.Tasks.RetentionDays)
		slog.Info("Override applied", "setting", "tasks.retention_days", "value", c.Tasks.RetentionDays)
	}

	// AgentsDir override
	if v := os.Getenv("KYOCI_AGENTS_DIR"); v != "" {
		c.AgentsDir = v
		slog.Info("Override applied", "setting", "agents_dir", "value", v)
	}

	// Memory overrides
	if v := os.Getenv("KYOCI_DB_PATH"); v != "" {
		c.Memory.DBPath = v
		slog.Info("Override applied", "setting", "db_path", "value", v)
	}
	if v := os.Getenv("KYOCI_COMPACTION_THRESHOLD"); v != "" {
		c.Memory.CompactionThreshold = parseFloatEnv(v, c.Memory.CompactionThreshold)
		slog.Info("Override applied", "setting", "compaction_threshold", "value", c.Memory.CompactionThreshold)
	}
	if v := os.Getenv("KYOCI_MAX_SHORTTERM_TOKENS"); v != "" {
		c.Memory.MaxShortTermTokens = parseIntEnv(v, c.Memory.MaxShortTermTokens)
		slog.Info("Override applied", "setting", "max_shortterm_tokens", "value", c.Memory.MaxShortTermTokens)
	}

	// Pool overrides
	if v := os.Getenv("KYOCI_WORKERS"); v != "" {
		c.Pool.Workers = parseIntEnv(v, c.Pool.Workers)
		slog.Info("Override applied", "setting", "workers", "value", c.Pool.Workers)
	}
	if v := os.Getenv("KYOCI_QUEUE_SIZE"); v != "" {
		c.Pool.QueueSize = parseIntEnv(v, c.Pool.QueueSize)
		slog.Info("Override applied", "setting", "queue_size", "value", c.Pool.QueueSize)
	}

	// Security overrides
	if v := os.Getenv("KYOCI_AUTH_ENABLED"); v != "" {
		c.Security.AuthEnabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "auth_enabled", "value", c.Security.AuthEnabled)
	}
	if v := os.Getenv("KYOCI_RATE_LIMIT"); v != "" {
		c.Security.RateLimit = parseIntEnv(v, c.Security.RateLimit)
		slog.Info("Override applied", "setting", "rate_limit", "value", c.Security.RateLimit)
	}
	if v := os.Getenv("KYOCI_SECURITY_TLS_ENABLED"); v != "" {
		c.Security.TLSEnabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "security_tls_enabled", "value", c.Security.TLSEnabled)
	}

	// Provider-specific overrides
	for name := range c.Providers {
		prefix := fmt.Sprintf("KYOCI_PROVIDER_%s_", strings.ToUpper(name))
		provider := c.Providers[name]

		if v := os.Getenv(prefix + "API_KEY"); v != "" {
			provider.APIKey = v
			slog.Info("Override applied", "setting", prefix+"API_KEY", "provider", name)
		}
		if v := os.Getenv(prefix + "BASE_URL"); v != "" {
			provider.BaseURL = v
			slog.Info("Override applied", "setting", prefix+"BASE_URL", "provider", name)
		}
		if v := os.Getenv(prefix + "DEFAULT_MODEL"); v != "" {
			provider.DefaultModel = v
			slog.Info("Override applied", "setting", prefix+"DEFAULT_MODEL", "provider", name)
		}
		if v := os.Getenv(prefix + "MAX_RETRIES"); v != "" {
			provider.MaxRetries = parseIntEnv(v, provider.MaxRetries)
			slog.Info("Override applied", "setting", prefix+"MAX_RETRIES", "provider", name, "value", provider.MaxRetries)
		}
		if v := os.Getenv(prefix + "TIMEOUT"); v != "" {
			provider.Timeout = parseIntEnv(v, provider.Timeout)
			slog.Info("Override applied", "setting", prefix+"TIMEOUT", "provider", name, "value", provider.Timeout)
		}
		if v := os.Getenv(prefix + "ENABLED"); v != "" {
			provider.Enabled = parseBoolEnv(v)
			slog.Info("Override applied", "setting", prefix+"ENABLED", "provider", name, "value", provider.Enabled)
		}

		c.Providers[name] = provider
	}

	// Thinking config overrides
	if v := os.Getenv("KYOCI_THINKING_ENABLED"); v != "" {
		c.Agent.Thinking.Enabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "thinking.enabled", "value", c.Agent.Thinking.Enabled)
	}
	if v := os.Getenv("KYOCI_THINKING_TOOL_BUDGET"); v != "" {
		c.Agent.Thinking.ToolBudget = parseIntEnv(v, c.Agent.Thinking.ToolBudget)
		slog.Info("Override applied", "setting", "thinking.tool_budget", "value", c.Agent.Thinking.ToolBudget)
	}
	if v := os.Getenv("KYOCI_THINKING_MAX_REFLECTIONS"); v != "" {
		c.Agent.Thinking.MaxReflections = parseIntEnv(v, c.Agent.Thinking.MaxReflections)
		slog.Info("Override applied", "setting", "thinking.max_reflections", "value", c.Agent.Thinking.MaxReflections)
	}
	if v := os.Getenv("KYOCI_THINKING_MAX_REPLANS"); v != "" {
		c.Agent.Thinking.MaxReplans = parseIntEnv(v, c.Agent.Thinking.MaxReplans)
		slog.Info("Override applied", "setting", "thinking.max_replans", "value", c.Agent.Thinking.MaxReplans)
	}
	if v := os.Getenv("KYOCI_THINKING_CONFIDENCE_THRESHOLD"); v != "" {
		c.Agent.Thinking.ConfidenceThreshold = parseFloatEnv(v, c.Agent.Thinking.ConfidenceThreshold)
		slog.Info("Override applied", "setting", "thinking.confidence_threshold", "value", c.Agent.Thinking.ConfidenceThreshold)
	}
	if v := os.Getenv("KYOCI_THINKING_FEW_SHOT"); v != "" {
		c.Agent.Thinking.FewShot = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "thinking.few_shot", "value", c.Agent.Thinking.FewShot)
	}

	// Orchestration config overrides
	if v := os.Getenv("KYOCI_ORCHESTRATION_ENABLED"); v != "" {
		c.Agent.Orchestration.Enabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "orchestration.enabled", "value", c.Agent.Orchestration.Enabled)
	}
	if v := os.Getenv("KYOCI_ORCHESTRATION_MAX_STEPS"); v != "" {
		c.Agent.Orchestration.MaxSteps = parseIntEnv(v, c.Agent.Orchestration.MaxSteps)
		slog.Info("Override applied", "setting", "orchestration.max_steps", "value", c.Agent.Orchestration.MaxSteps)
	}
	if v := os.Getenv("KYOCI_ORCHESTRATION_MAX_PARALLEL"); v != "" {
		c.Agent.Orchestration.MaxParallel = parseIntEnv(v, c.Agent.Orchestration.MaxParallel)
		slog.Info("Override applied", "setting", "orchestration.max_parallel", "value", c.Agent.Orchestration.MaxParallel)
	}
	if v := os.Getenv("KYOCI_ORCHESTRATION_WORKER_MAX_ITERATIONS"); v != "" {
		c.Agent.Orchestration.WorkerMaxIterations = parseIntEnv(v, c.Agent.Orchestration.WorkerMaxIterations)
		slog.Info("Override applied", "setting", "orchestration.worker_max_iterations", "value", c.Agent.Orchestration.WorkerMaxIterations)
	}
	if v := os.Getenv("KYOCI_ORCHESTRATION_WORKER_MAX_TOOL_CALLS"); v != "" {
		c.Agent.Orchestration.WorkerMaxToolCalls = parseIntEnv(v, c.Agent.Orchestration.WorkerMaxToolCalls)
		slog.Info("Override applied", "setting", "orchestration.worker_max_tool_calls", "value", c.Agent.Orchestration.WorkerMaxToolCalls)
	}
	if v := os.Getenv("KYOCI_ORCHESTRATION_MAX_RETRIES"); v != "" {
		c.Agent.Orchestration.MaxRetries = parseIntEnv(v, c.Agent.Orchestration.MaxRetries)
		slog.Info("Override applied", "setting", "orchestration.max_retries", "value", c.Agent.Orchestration.MaxRetries)
	}

	// HITL config overrides
	if v := os.Getenv("KYOCI_HITL_ENABLED"); v != "" {
		c.HITL.Enabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "hitl.enabled", "value", c.HITL.Enabled)
	}
	if v := os.Getenv("KYOCI_HITL_PORT"); v != "" {
		c.HITL.Port = parseIntEnv(v, c.HITL.Port)
		slog.Info("Override applied", "setting", "hitl.port", "value", c.HITL.Port)
	}
	if v := os.Getenv("KYOCI_HITL_REQUEST_TIMEOUT"); v != "" {
		c.HITL.RequestTimeout = parseIntEnv(v, c.HITL.RequestTimeout)
		slog.Info("Override applied", "setting", "hitl.request_timeout", "value", c.HITL.RequestTimeout)
	}

	// Prompt-skill config overrides
	if v := os.Getenv("KYOCI_PROMPT_SKILLS_ENABLED"); v != "" {
		c.PromptSkill.Enabled = parseBoolEnv(v)
		slog.Info("Override applied", "setting", "prompt_skills.enabled", "value", c.PromptSkill.Enabled)
	}
	if v := os.Getenv("KYOCI_PROMPT_SKILLS_DIR"); v != "" {
		c.PromptSkill.Dir = v
		slog.Info("Override applied", "setting", "prompt_skills.dir", "value", c.PromptSkill.Dir)
	}
	if v := os.Getenv("KYOCI_PROMPT_SKILLS_MAX_PER_TASK"); v != "" {
		c.PromptSkill.MaxSkillsPerTask = parseIntEnv(v, c.PromptSkill.MaxSkillsPerTask)
		slog.Info("Override applied", "setting", "prompt_skills.max_skills_per_task", "value", c.PromptSkill.MaxSkillsPerTask)
	}
	if v := os.Getenv("KYOCI_PROMPT_SKILLS_MAX_CHARS"); v != "" {
		c.PromptSkill.MaxTotalChars = parseIntEnv(v, c.PromptSkill.MaxTotalChars)
		slog.Info("Override applied", "setting", "prompt_skills.max_total_chars", "value", c.PromptSkill.MaxTotalChars)
	}

	return nil
}

// ==============================================================================
// Thread-Safe Getters for Config
// ==============================================================================

// GetServer returns a copy of the server configuration.
func (c *Config) GetServer() ServerConfig {
	return c.Server
}

// GetLogging returns a copy of the logging configuration.
func (c *Config) GetLogging() LogConfig {
	return c.Logging
}

// GetMemory returns a copy of the memory configuration.
func (c *Config) GetMemory() MemoryConfig {
	return c.Memory
}

// GetPool returns a copy of the pool configuration.
func (c *Config) GetPool() PoolConfig {
	return c.Pool
}

// GetSecurity returns a copy of the security configuration.
func (c *Config) GetSecurity() SecurityConfig {
	return c.Security
}

// GetProvider returns a copy of the provider configuration for the given name.
// Returns false if the provider does not exist.
func (c *Config) GetProvider(name string) (ProviderConfig, bool) {
	provider, exists := c.Providers[name]
	return provider, exists
}

// GetAllProviders returns a copy of all provider configurations.
func (c *Config) GetAllProviders() map[string]ProviderConfig {
	result := make(map[string]ProviderConfig, len(c.Providers))
	for k, v := range c.Providers {
		result[k] = v
	}
	return result
}

// GetRole returns a copy of the role configuration for the given name.
// Returns false if the role does not exist. Safe to call on a Config whose
// Roles map is nil or empty (the default after the markdown-driven migration —
// agents are seeded by the orchestrator's loader, not by Default()).
func (c *Config) GetRole(name string) (RoleConfig, bool) {
	if c == nil || c.Roles == nil {
		return RoleConfig{}, false
	}
	role, exists := c.Roles[name]
	if !exists || role == nil {
		return RoleConfig{}, false
	}
	return *role, exists
}

// GetAllRoles returns a copy of all role configurations.
func (c *Config) GetAllRoles() map[string]RoleConfig {
	result := make(map[string]RoleConfig, len(c.Roles))
	for k, v := range c.Roles {
		result[k] = *v
	}
	return result
}

// ==============================================================================
// Helper Functions for Env Parsing
// ==============================================================================

// parseIntEnv parses an integer from an environment variable string.
func parseIntEnv(s string, defaultValue int) int {
	var value int
	_, err := fmt.Sscanf(s, "%d", &value)
	if err != nil {
		return defaultValue
	}
	return value
}

// parseFloatEnv parses a float64 from an environment variable string.
func parseFloatEnv(s string, defaultValue float64) float64 {
	var value float64
	_, err := fmt.Sscanf(s, "%f", &value)
	if err != nil {
		return defaultValue
	}
	return value
}

// parseBoolEnv parses a boolean from an environment variable string.
// Accepts: true, false, 1, 0 (case-insensitive)
func parseBoolEnv(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// parseDurationEnv parses a duration from an environment variable string.
// Accepts Go duration format (e.g., "30s", "5m", "1h")
func parseDurationEnv(s string, defaultValue time.Duration) time.Duration {
	duration, err := time.ParseDuration(s)
	if err != nil {
		return defaultValue
	}
	return duration
}

// isLocalProviderName returns true for OpenAI-compatible local servers that
// don't require an API key. Mirrors the auth switch in internal/llm/providers.go
// — keep the two lists in sync. Lives here (not in internal/llm) to avoid an
// import cycle.
func isLocalProviderName(name string) bool {
	switch name {
	case "ollama", "lmstudio":
		return true
	}
	return false
}
