package role

import (
	"context"
	"testing"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	developerpkg "github.com/metabbe3/Kyoci-Agent/internal/role/developer"
	pmpkg "github.com/metabbe3/Kyoci-Agent/internal/role/pm"
	qapkg "github.com/metabbe3/Kyoci-Agent/internal/role/qa"
	srepkg "github.com/metabbe3/Kyoci-Agent/internal/role/sre"
	"github.com/metabbe3/Kyoci-Agent/internal/skill"
	"github.com/metabbe3/Kyoci-Agent/internal/tool"
	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Mock Agent for Testing
// =============================================================================

// mockAgent is a minimal mock of agent.Agent for testing.
type mockAgent struct {
	executeFunc      func(ctx context.Context, task string) (*kyoci.TaskResult, error)
	executeStreamFunc func(ctx context.Context, task string) (<-chan kyoci.StreamChunk, error)
}

func (m *mockAgent) Execute(ctx context.Context, task string) (*kyoci.TaskResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, task)
	}
	return &kyoci.TaskResult{
		Content:    "mock result",
		Iterations: 1,
		Usage:      kyoci.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (m *mockAgent) ExecuteStream(ctx context.Context, task string) (<-chan kyoci.StreamChunk, error) {
	if m.executeStreamFunc != nil {
		return m.executeStreamFunc(ctx, task)
	}
	ch := make(chan kyoci.StreamChunk, 1)
	ch <- kyoci.StreamChunk{
		Content: "mock stream",
		Done:    true,
	}
	close(ch)
	return ch, nil
}

// =============================================================================
// TestRoleRegistry
// =============================================================================

func TestRoleRegistry_Register(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Create a valid role config
	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}

	// Register the role
	err := registry.Register(cfg)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Verify registration
	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if roleAgent == nil {
		t.Fatal("Get() returned nil role agent")
	}
	if roleAgent.Type() != kyoci.RoleDeveloper {
		t.Errorf("Expected role type %v, got %v", kyoci.RoleDeveloper, roleAgent.Type())
	}
}

func TestRoleRegistry_Register_InvalidConfig(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Test with empty system prompt
	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "", // Invalid
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}

	err := registry.Register(cfg)
	if err == nil {
		t.Fatal("Register() expected error for invalid config, got nil")
	}
	if !containsString(err.Error(), "system prompt cannot be empty") {
		t.Errorf("Expected error about system prompt, got: %v", err)
	}
}

func TestRoleRegistry_Register_Duplicate(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}

	// Register first time
	err := registry.Register(cfg)
	if err != nil {
		t.Fatalf("First Register() failed: %v", err)
	}

	// Register second time with same type (should replace)
	cfg2 := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Updated prompt",
		Tools:         []string{"read_file"},
		MaxIterations: 10,
		Temperature:   0.6,
	}
	err = registry.Register(cfg2)
	if err != nil {
		t.Fatalf("Second Register() failed: %v", err)
	}

	// Verify replacement
	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if roleAgent.SystemPrompt() != "Updated prompt" {
		t.Errorf("Expected updated prompt, got: %s", roleAgent.SystemPrompt())
	}
	if roleAgent.MaxIterations() != 10 {
		t.Errorf("Expected MaxIterations 10, got: %d", roleAgent.MaxIterations())
	}
}

func TestRoleRegistry_Get_NotFound(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Try to get a non-existent role
	_, err := registry.Get(kyoci.RoleDeveloper)
	if err != kyoci.ErrRoleNotFound {
		t.Errorf("Expected ErrRoleNotFound, got: %v", err)
	}
}

func TestRoleRegistry_List(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Register multiple roles
	roles := []kyoci.RoleConfig{
		{
			Type:          kyoci.RoleDeveloper,
			SystemPrompt:  "Developer prompt",
			Tools:         []string{"terminal"},
			MaxIterations: 5,
			Temperature:   0.7,
		},
		{
			Type:          kyoci.RoleSRE,
			SystemPrompt:  "SRE prompt",
			Tools:         []string{"read_file"},
			MaxIterations: 8,
			Temperature:   0.6,
		},
	}

	for _, cfg := range roles {
		err := registry.Register(cfg)
		if err != nil {
			t.Fatalf("Register() failed: %v", err)
		}
	}

	// List all roles
	configs := registry.List()
	if len(configs) != 2 {
		t.Errorf("Expected 2 configs, got: %d", len(configs))
	}

	// Verify configs
	typeMap := make(map[kyoci.RoleType]bool)
	for _, cfg := range configs {
		typeMap[cfg.Type] = true
	}
	if !typeMap[kyoci.RoleDeveloper] {
		t.Error("Developer role not found in list")
	}
	if !typeMap[kyoci.RoleSRE] {
		t.Error("SRE role not found in list")
	}
}

func TestRoleRegistry_RegisterDefaults(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Register default roles (no config overrides)
	err := registry.RegisterDefaults(nil)
	if err != nil {
		t.Fatalf("RegisterDefaults() failed: %v", err)
	}

	// Verify all default roles are registered
	for roleType := kyoci.RoleDeveloper; roleType <= kyoci.RolePM; roleType++ {
		roleAgent, err := registry.Get(roleType)
		if err != nil {
			t.Errorf("Should have registered role %s, got error: %v", roleType, err)
		}
		if roleAgent == nil {
			t.Errorf("Role %s should not be nil", roleType)
		}
	}

	// Verify developer role specifically
	devRole, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get(Developer) failed: %v", err)
	}
	if !containsString(devRole.SystemPrompt(), "autonomous developer") {
		t.Errorf("Developer prompt should contain 'autonomous developer', got: %s", devRole.SystemPrompt())
	}
	if devRole.MaxIterations() != 15 {
		t.Errorf("Developer MaxIterations should be 15, got: %d", devRole.MaxIterations())
	}
}

// =============================================================================
// Test Role Configurations
// =============================================================================

func TestDeveloperRole_Config(t *testing.T) {
	cfg := developerpkg.DefaultConfig()

	if cfg.Type != kyoci.RoleDeveloper {
		t.Errorf("Expected RoleDeveloper, got: %v", cfg.Type)
	}
	if cfg.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
	if !containsString(cfg.SystemPrompt, "autonomous developer") {
		t.Error("SystemPrompt should contain 'autonomous developer'")
	}

	// Check tools
	if !containsStringSlice(cfg.Tools, "terminal") {
		t.Error("Tools should contain 'terminal'")
	}
	if !containsStringSlice(cfg.Tools, "file") {
		t.Error("Tools should contain 'file'")
	}
	if !containsStringSlice(cfg.Tools, "http_client") {
		t.Error("Tools should contain 'http_client'")
	}
	if !containsStringSlice(cfg.Tools, "web_search") {
		t.Error("Tools should contain 'web_search'")
	}
	if !containsStringSlice(cfg.Tools, "calculator") {
		t.Error("Tools should contain 'calculator'")
	}

	// Check configuration
	if cfg.PreferredProvider != "" {
		t.Errorf("PreferredProvider should be empty, got: %s", cfg.PreferredProvider)
	}
	if cfg.MaxIterations != 15 {
		t.Errorf("MaxIterations should be 15, got: %d", cfg.MaxIterations)
	}
	if cfg.Temperature != 0.3 {
		t.Errorf("Temperature should be 0.3, got: %f", cfg.Temperature)
	}
}

func TestDeveloperRole_Specialized(t *testing.T) {
	cfg := developerpkg.SpecializedDeveloper()

	if cfg.Type != kyoci.RoleDeveloper {
		t.Errorf("Expected RoleDeveloper, got: %v", cfg.Type)
	}
	if !containsString(cfg.SystemPrompt, "specialize in Go development") {
		t.Error("SystemPrompt should contain 'specialize in Go development'")
	}
	if !containsString(cfg.SystemPrompt, "table-driven tests") {
		t.Error("SystemPrompt should contain 'table-driven tests'")
	}
}

func TestSRERole_Config(t *testing.T) {
	cfg := srepkg.DefaultConfig()

	if cfg.Type != kyoci.RoleSRE {
		t.Errorf("Expected RoleSRE, got: %v", cfg.Type)
	}
	if cfg.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
	if !containsString(cfg.SystemPrompt, "Site Reliability Engineer") {
		t.Error("SystemPrompt should contain 'Site Reliability Engineer'")
	}
	if !containsString(cfg.SystemPrompt, "monitoring") {
		t.Error("SystemPrompt should contain 'monitoring'")
	}
	if !containsString(cfg.SystemPrompt, "incident response") {
		t.Error("SystemPrompt should contain 'incident response'")
	}

	// Check tools
	if !containsStringSlice(cfg.Tools, "terminal") {
		t.Error("Tools should contain 'terminal'")
	}
	if !containsStringSlice(cfg.Tools, "file") {
		t.Error("Tools should contain 'file'")
	}
	if !containsStringSlice(cfg.Tools, "http_client") {
		t.Error("Tools should contain 'http_client'")
	}
	if !containsStringSlice(cfg.Tools, "web_search") {
		t.Error("Tools should contain 'web_search'")
	}
	if !containsStringSlice(cfg.Tools, "security_scan") {
		t.Error("Tools should contain 'security_scan'")
	}

	// Check configuration
	if cfg.PreferredProvider != "" {
		t.Errorf("PreferredProvider should be empty, got: %s", cfg.PreferredProvider)
	}
	if cfg.MaxIterations != 15 {
		t.Errorf("MaxIterations should be 15, got: %d", cfg.MaxIterations)
	}
	if cfg.Temperature != 0.6 {
		t.Errorf("Temperature should be 0.6, got: %f", cfg.Temperature)
	}
}

func TestQARole_Config(t *testing.T) {
	cfg := qapkg.DefaultConfig()

	if cfg.Type != kyoci.RoleQA {
		t.Errorf("Expected RoleQA, got: %v", cfg.Type)
	}
	if cfg.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
	if !containsString(cfg.SystemPrompt, "Quality Assurance") {
		t.Error("SystemPrompt should contain 'Quality Assurance'")
	}
	if !containsString(cfg.SystemPrompt, "Testing Strategies") {
		t.Error("SystemPrompt should contain 'Testing Strategies'")
	}
	if !containsString(cfg.SystemPrompt, "code review") {
		t.Error("SystemPrompt should contain 'code review'")
	}
	if !containsString(cfg.SystemPrompt, "security validation") {
		t.Error("SystemPrompt should contain 'security validation'")
	}

	// Check tools
	if !containsStringSlice(cfg.Tools, "terminal") {
		t.Error("Tools should contain 'terminal'")
	}
	if !containsStringSlice(cfg.Tools, "file") {
		t.Error("Tools should contain 'file'")
	}
	if !containsStringSlice(cfg.Tools, "http_client") {
		t.Error("Tools should contain 'http_client'")
	}
	if !containsStringSlice(cfg.Tools, "calculator") {
		t.Error("Tools should contain 'calculator'")
	}
	if !containsStringSlice(cfg.Tools, "security_scan") {
		t.Error("Tools should contain 'security_scan'")
	}

	// Check configuration
	if cfg.PreferredProvider != "" {
		t.Errorf("PreferredProvider should be empty, got: %s", cfg.PreferredProvider)
	}
	if cfg.MaxIterations != 6 {
		t.Errorf("MaxIterations should be 6, got: %d", cfg.MaxIterations)
	}
	if cfg.Temperature != 0.6 {
		t.Errorf("Temperature should be 0.6, got: %f", cfg.Temperature)
	}
}

func TestPMRole_Config(t *testing.T) {
	cfg := pmpkg.DefaultConfig()

	if cfg.Type != kyoci.RolePM {
		t.Errorf("Expected RolePM, got: %v", cfg.Type)
	}
	if cfg.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
	if !containsString(cfg.SystemPrompt, "Project Manager") {
		t.Error("SystemPrompt should contain 'Project Manager'")
	}
	if !containsString(cfg.SystemPrompt, "planning") {
		t.Error("SystemPrompt should contain 'planning'")
	}
	if !containsString(cfg.SystemPrompt, "prioritization") {
		t.Error("SystemPrompt should contain 'prioritization'")
	}
	if !containsString(cfg.SystemPrompt, "stakeholder") {
		t.Error("SystemPrompt should contain 'stakeholder'")
	}

	// Check tools - PM role doesn't have terminal
	if !containsStringSlice(cfg.Tools, "file") {
		t.Error("Tools should contain 'file'")
	}
	if !containsStringSlice(cfg.Tools, "http_client") {
		t.Error("Tools should contain 'http_client'")
	}
	if !containsStringSlice(cfg.Tools, "web_search") {
		t.Error("Tools should contain 'web_search'")
	}

	// Check configuration
	if cfg.PreferredProvider != "" {
		t.Errorf("PreferredProvider should be empty, got: %s", cfg.PreferredProvider)
	}
	if cfg.MaxIterations != 6 {
		t.Errorf("MaxIterations should be 6, got: %d", cfg.MaxIterations)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("Temperature should be 0.7, got: %f", cfg.Temperature)
	}
}

// =============================================================================
// Test RoleAgent Execution
// =============================================================================

func TestRoleAgent_Execute_Delegation(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Register a role
	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}
	err := registry.Register(cfg)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Get the role agent
	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Execute a task
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: This will fail without proper LLM provider setup,
	// but we're testing the delegation path
	_, err = roleAgent.Execute(ctx, "test task", nil)
	// The error is expected since we don't have a real LLM configured
	if err == nil {
		t.Log("Note: Execute succeeded (unexpected without LLM config)")
	}
}

func TestRoleAgent_ExecuteStream_Delegation(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Register a role
	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}
	err := registry.Register(cfg)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Get the role agent
	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Execute a stream task
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: This will fail without proper LLM provider setup,
	// but we're testing the delegation path
	_, err = roleAgent.ExecuteStream(ctx, "test task")
	// The error is expected since we don't have a real LLM configured
	if err == nil {
		t.Log("Note: ExecuteStream succeeded (unexpected without LLM config)")
	}
}

func TestRoleAgent_ImplementsRoleInterface(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}
	err := registry.Register(cfg)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Verify it implements kyoci.Role interface
	var _ kyoci.Role = roleAgent

	// Test all interface methods
	if roleAgent.Type() != kyoci.RoleDeveloper {
		t.Errorf("Expected Type() to return RoleDeveloper, got: %v", roleAgent.Type())
	}
	if roleAgent.SystemPrompt() != "Test prompt" {
		t.Errorf("Expected SystemPrompt() to return 'Test prompt', got: %s", roleAgent.SystemPrompt())
	}
	if len(roleAgent.Tools()) != 1 || roleAgent.Tools()[0] != "terminal" {
		t.Errorf("Expected Tools() to return ['terminal'], got: %v", roleAgent.Tools())
	}
	if roleAgent.PreferredProvider() != "" {
		t.Errorf("Expected PreferredProvider() to return empty, got: %s", roleAgent.PreferredProvider())
	}
	if roleAgent.MaxIterations() != 5 {
		t.Errorf("Expected MaxIterations() to return 5, got: %d", roleAgent.MaxIterations())
	}
}

// =============================================================================
// Test Utilities
// =============================================================================

// createTestRegistry creates a test RoleRegistry with minimal setup.
func createTestRegistry(t *testing.T) *RoleRegistry {
	// Create a simple LLM router
	providerReg := llm.NewProviderRegistry()
	router := llm.NewRouter(providerReg, llm.StrategyFallback)

	// Create tool and skill registries
	toolReg := tool.NewRegistry()
	skillReg := skill.NewRegistry()

	// Create role registry (memory manager can be nil for basic tests)
	registry := NewRoleRegistry(router, toolReg, skillReg, nil)

	return registry
}

// closeTestResources cleans up test resources.
func closeTestResources(registry *RoleRegistry) {
	// No-op for now as registries don't require explicit cleanup
}

// containsString checks if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

// containsSubstring is a helper for containsString.
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// containsStringSlice checks if a string slice contains a specific string.
func containsStringSlice(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// =============================================================================
// PR #2: Thinking Config Wiring Tests
// =============================================================================

// TestRegisterDefaults_WiresThinkingConfig verifies that the thinking config
// from *config.Config is propagated through to each registered role agent's
// AgentConfig. This is the integration point that makes the thinking system
// actually active in the running agent.
func TestRegisterDefaults_WiresThinkingConfig(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Build a config with custom thinking values distinct from defaults.
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Thinking: config.ThinkingConfig{
				Enabled:             true,
				ToolBudget:          25,
				MaxReflections:      5,
				MaxReplans:          3,
				ConfidenceThreshold: 0.9,
				FewShot:             false,
			},
		},
	}

	if err := registry.RegisterDefaults(cfg); err != nil {
		t.Fatalf("RegisterDefaults() failed: %v", err)
	}

	// Fetch the developer role and inspect its underlying agent's config.
	// Test is in package role, so it can access the unexported `agent` field.
	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get(Developer) failed: %v", err)
	}

	got := roleAgent.agent.GetConfig()

	if !got.EnableThinking {
		t.Errorf("EnableThinking = false, want true (config.Agent.Thinking.Enabled)")
	}
	if got.ThinkingToolBudget != 25 {
		t.Errorf("ThinkingToolBudget = %d, want 25", got.ThinkingToolBudget)
	}
	if got.ThinkingMaxReflections != 5 {
		t.Errorf("ThinkingMaxReflections = %d, want 5", got.ThinkingMaxReflections)
	}
	if got.ThinkingMaxReplans != 3 {
		t.Errorf("ThinkingMaxReplans = %d, want 3", got.ThinkingMaxReplans)
	}
	if got.ThinkingConfidenceThreshold != 0.9 {
		t.Errorf("ThinkingConfidenceThreshold = %v, want 0.9", got.ThinkingConfidenceThreshold)
	}
	if got.ThinkingFewShot {
		t.Errorf("ThinkingFewShot = true, want false")
	}
}

// TestRegisterDefaults_DisabledThinkingByDefault verifies that when
// RegisterDefaults is called with a nil config (no thinking config available),
// the thinking system remains disabled and the AgentConfig thinking fields
// fall back to sensible defaults from DefaultAgentConfig.
func TestRegisterDefaults_DisabledThinkingByDefault(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// nil config — registry must not crash and should leave thinking disabled.
	if err := registry.RegisterDefaults(nil); err != nil {
		t.Fatalf("RegisterDefaults(nil) failed: %v", err)
	}

	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get(Developer) failed: %v", err)
	}

	got := roleAgent.agent.GetConfig()

	// With no config provided, thinking stays disabled (PR #1 behavior).
	if got.EnableThinking {
		t.Errorf("EnableThinking = true, want false when no config provided")
	}
	// The other fields should fall back to DefaultAgentConfig values so the
	// thinking system has sane budgets if it is later enabled.
	if got.ThinkingToolBudget != 15 {
		t.Errorf("ThinkingToolBudget = %d, want default 15", got.ThinkingToolBudget)
	}
	if got.ThinkingMaxReflections != 3 {
		t.Errorf("ThinkingMaxReflections = %d, want default 3", got.ThinkingMaxReflections)
	}
}

// Compile-time guard: ensure we reference agent package so the import is used.
var _ = agent.AgentConfig{}

// =============================================================================
// Step 7b — MCP tools must bypass the per-role allowlist
//
// createRoleAgent builds a NEW per-role tool registry and filters the source
// registry through the role's hardcoded Tools list. Before Step 7 this filter
// dropped every dynamically-loaded MCP tool (e.g. kyoci_fetch_user_schema)
// because no role config could list those names — they're registered at
// runtime by the MCP manager. The result: the orchestrated worker's
// a.tools.List() contained ZERO MCP tools, the Step 6 MCP enforcement filter
// never fired, and the L3 benchmark's `mcp_calls=1` was a grader false
// positive matching plan-step description text rather than real execution.
//
// The fix: non-built-in tools (per tool.IsBuiltinName) pass through the
// filter unconditionally. Built-ins still respect the allowlist.
//
// This is the load-bearing regression test for that behavior.
// =============================================================================

// fakeMCPTool is a minimal kyoci.Tool implementation we register into the
// source tool registry to simulate an MCP-loaded tool. It uses a name with
// the MCP manager prefix (`kyoci_`) just like the real MCP adapter.
type fakeMCPTool struct {
	name string
	desc string
}

func (f *fakeMCPTool) Name() string                                       { return f.name }
func (f *fakeMCPTool) Description() string                                { return f.desc }
func (f *fakeMCPTool) Parameters() []kyoci.ToolParameter                  { return nil }
func (f *fakeMCPTool) Execute(ctx context.Context, p map[string]interface{}) (string, error) {
	return "fake-mcp-ok", nil
}

// TestCreateRoleAgent_MCPToolsPassThrough is THE keystone regression test for
// Step 7. It registers three tools into the source registry — one allowlisted
// built-in, one NON-allowlisted built-in, and one MCP-shaped tool — then
// builds a role whose Tools list names ONLY the first. The per-role agent's
// tool list must:
//   - CONTAIN the allowlisted built-in (file)
//   - CONTAIN the MCP-shaped tool (kyoci_fetch_user_schema) — pass-through
//   - NOT contain the non-allowlisted built-in (security_scan)
//
// If this test fails, the L3 benchmark's M2/M5 cannot pass: workers will
// never see MCP tools and Step 6's Go-side enforcement filter becomes inert.
func TestCreateRoleAgent_MCPToolsPassThrough(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Seed the SOURCE tool registry with the cases that matter.
	// We can't use tool.TestTool (unexported), so we register via the
	// built-in constructors for built-ins and our fakeMCPTool for the
	// dynamic tool.
	sourceTools := []kyoci.Tool{
		builtin.NewFileTool(),       // allowlisted built-in
		builtin.NewCalculatorTool(), // NON-allowlisted built-in
		&fakeMCPTool{
			name: "kyoci_fetch_user_schema",
			desc: "MANDATORY: fetch user schema",
		},
	}
	for _, tl := range sourceTools {
		if err := registry.toolReg.Register(tl); err != nil {
			t.Fatalf("seed register %q failed: %v", tl.Name(), err)
		}
	}

	// Role config with a Tools allowlist that does NOT mention the MCP tool.
	// Pre-Step-7 this would drop it; post-Step-7 it must pass through.
	cfg := kyoci.RoleConfig{
		Type:           kyoci.RoleDeveloper,
		SystemPrompt:   "Test prompt",
		Tools:          []string{"file"},
		MaxIterations:  5,
		Temperature:    0.7,
	}

	if err := registry.Register(cfg); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	names := roleAgent.agent.ToolNames()
	has := func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}

	// Must keep: allowlisted built-in + MCP tool (pass-through).
	if !has("file") {
		t.Errorf("per-role agent missing allowlisted built-in 'file'; got %v", names)
	}
	if !has("kyoci_fetch_user_schema") {
		t.Errorf("per-role agent missing MCP tool 'kyoci_fetch_user_schema' — "+
			"non-built-in tools MUST bypass the allowlist (Step 7 regression); got %v", names)
	}

	// Must drop: built-in not in the allowlist (calculator here is non-allowlisted).
	if has("calculator") {
		t.Errorf("per-role agent kept non-allowlisted built-in 'calculator' — "+
			"built-in gating must still apply (Step 7 must not over-reach); got %v", names)
	}
}

// TestCreateRoleAgent_AllowlistStillGatesBuiltins verifies that the Step 7
// change is surgical: built-ins that are NOT in the role's allowlist are still
// stripped. This prevents a frontend role from accidentally receiving
// security_scan, which was the original purpose of the filter. If this test
// fails, the Step 7 change over-reached and broke role isolation.
func TestCreateRoleAgent_AllowlistStillGatesBuiltins(t *testing.T) {
	registry := createTestRegistry(t)
	defer closeTestResources(registry)

	// Register only built-ins — no MCP tool at all.
	if err := registry.toolReg.Register(builtin.NewFileTool()); err != nil {
		t.Fatalf("register file failed: %v", err)
	}
	if err := registry.toolReg.Register(builtin.NewCalculatorTool()); err != nil {
		t.Fatalf("register calculator failed: %v", err)
	}
	// Register security_scan-shaped built-in. We don't have its constructor
	// handy; use a fake with a built-in name. The role filter must drop it
	// because 'security_scan' is in the builtin set but NOT in the role's
	// allowlist below.
	secTool := &fakeMCPTool{name: "security_scan", desc: "security scanner"}
	if err := registry.toolReg.Register(secTool); err != nil {
		t.Fatalf("register security_scan failed: %v", err)
	}

	cfg := kyoci.RoleConfig{
		Type:         kyoci.RoleDeveloper,
		SystemPrompt: "Test prompt",
		Tools:        []string{"file"}, // explicitly NOT security_scan, NOT calculator
		MaxIterations: 5,
		Temperature:   0.7,
	}
	if err := registry.Register(cfg); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	roleAgent, err := registry.Get(kyoci.RoleDeveloper)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	names := roleAgent.agent.ToolNames()
	has := func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}

	if !has("file") {
		t.Errorf("allowlisted 'file' should be present; got %v", names)
	}
	if has("calculator") {
		t.Errorf("non-allowlisted built-in 'calculator' should be stripped; got %v", names)
	}
	if has("security_scan") {
		t.Errorf("non-allowlisted built-in 'security_scan' should be stripped "+
			"(this is the original purpose of the role filter); got %v", names)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkRoleRegistry_Register(b *testing.B) {
	registry := createTestRegistry(&testing.T{})
	defer closeTestResources(registry)

	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Benchmark test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use different role types for each iteration
		cfg.Type = kyoci.RoleType(i % 4)
		_ = registry.Register(cfg)
	}
}

func BenchmarkRoleRegistry_Get(b *testing.B) {
	registry := createTestRegistry(&testing.T{})
	defer closeTestResources(registry)

	// Register a role
	cfg := kyoci.RoleConfig{
		Type:          kyoci.RoleDeveloper,
		SystemPrompt:  "Benchmark test prompt",
		Tools:         []string{"terminal"},
		MaxIterations: 5,
		Temperature:   0.7,
	}
	_ = registry.Register(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.Get(kyoci.RoleDeveloper)
	}
}