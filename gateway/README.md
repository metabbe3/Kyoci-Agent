# Gateway Package - Production Routing Layer

## Overview

The `gateway` package provides a production-grade routing layer for the AI agent system with three core components:

1. **Circuit Breaker** - Thread-safe failure detection and recovery
2. **Tiered Router** - Cascading tier-based provider routing with fallback
3. **DAG Executor** - Parallel and sequential task execution with dependency management

## Components

### Circuit Breaker (`circuit.go`)

A thread-safe circuit breaker implementation using only standard library.

**Features:**
- Three states: Closed, HalfOpen, Open
- Configurable failure threshold (default: 5)
- Configurable success threshold for recovery (default: 3)
- Configurable timeout for open state (default: 30s)
- Functional options pattern for configuration
- State change callbacks
- Manual state control (Reset, ForceOpen, ForceClosed)

**Key Types:**
- `State` - Circuit breaker state enum
- `CircuitBreaker` - Main circuit breaker struct
- `CBOption` - Functional option type

**Key Functions:**
- `NewCircuitBreaker(name string, opts ...CBOption) *CircuitBreaker`
- `WithFailureThreshold(n int) CBOption`
- `WithSuccessThreshold(n int) CBOption`
- `WithTimeout(d time.Duration) CBOption`
- `WithStateChangeCallback(fn func(name string, from, to State)) CBOption`

**Key Methods:**
- `Execute(fn func() (interface{}, error)) (interface{}, error)` - Execute with circuit protection
- `State() State` - Get current state
- `Stats() (state State, failures, successes int)` - Get statistics
- `Reset()` - Reset to closed state
- `ForceOpen()` - Force open state
- `ForceClosed()` - Force closed state

### Tiered Router (`tiered_router.go`)

Cascading tiered router for provider selection with circuit breaker integration.

**Features:**
- Three tiers: Tier0 (Code/Deterministic), Tier1 (Cheap AI/Local), Tier2 (Complex AI/Cloud)
- Per-provider circuit breakers
- Configurable tier timeouts:
  - Tier0: 1s
  - Tier1: 3s
  - Tier2: 30s
- Automatic provider selection based on availability
- Fallback routing to lower tiers
- Dynamic provider management

**Key Types:**
- `Tier` - Tier enum (Tier0, Tier1, Tier2)
- `Provider` - Provider configuration
- `TieredRouter` - Main router struct
- `NoProviderAvailableError` - Error for unavailable providers

**Key Functions:**
- `NewTieredRouter(cfg *config.Config) *TieredRouter`

**Key Methods:**
- `Route(level int) (*Provider, error)` - Get provider for tier
- `RouteWithFallback(level int) (*Provider, Tier, error)` - Route with tier fallback
- `ReportFailure(name string)` - Report provider failure
- `RecordSuccess(name string)` - Record provider success
- `AvailableTiers() []Tier` - Get available tiers
- `GetCircuitBreaker(name string) *CircuitBreaker` - Get provider circuit breaker
- `GetProvider(name string) (*Provider, bool)` - Get provider by name
- `AddProvider(tier Tier, provider Provider)` - Add provider
- `RemoveProvider(name string) bool` - Remove provider
- `GetProvidersInTier(level int) []Provider` - Get all providers in tier

### DAG Executor (`dag_executor.go`)

Execute tasks as DAG or parallel list with dependency management.

**Features:**
- Three execution modes: PARALLEL, SEQUENTIAL, DAG
- Built-in worker pool for concurrent execution
- Task dependency resolution
- Per-task timeout support
- Tier fallback on failure
- Topological execution for DAG mode
- JSON plan parsing and validation

**Key Types:**
- `DAGTask` - Task definition with dependencies
- `DAGPlan` - Plan of tasks with execution mode
- `TaskResult` - Result of task execution
- `WorkerPool` - Internal worker pool for parallel execution
- `DAGExecutor` - Main executor struct

**Key Functions:**
- `NewDAGExecutor(maxParallel int, timeout time.Duration) *DAGExecutor`
- `ParseDAGPlan(jsonData []byte) (DAGPlan, error)`

**Key Methods:**
- `Execute(ctx context.Context, plan DAGPlan) []TaskResult` - Execute the plan
- `Shutdown()` - Shutdown the executor

**Plan Structure:**
```json
{
  "plan_id": "string",
  "execution_mode": "PARALLEL|SEQUENTIAL|DAG",
  "tasks": [
    {
      "step": 1,
      "service_target": "service-name",
      "rpc_method": "method-name",
      "payload": {...},
      "tier_fallback": 1,
      "dependencies": []
    }
  ]
}
```

## Usage Examples

### Circuit Breaker

```go
cb := gateway.NewCircuitBreaker(
    "my-service",
    gateway.WithFailureThreshold(5),
    gateway.WithSuccessThreshold(3),
    gateway.WithTimeout(30*time.Second),
    gateway.WithStateChangeCallback(func(name string, from, to gateway.State) {
        log.Printf("Circuit %s: %v -> %v", name, from, to)
    }),
)

result, err := cb.Execute(func() (interface{}, error) {
    // Execute protected operation
    return callService()
})

if errors.Is(err, gateway.ErrCircuitOpen) {
    // Circuit is open, use fallback
}
```

### Tiered Router

```go
cfg := config.DefaultConfig()
tr := gateway.NewTieredRouter(cfg)

// Get provider for Tier2
provider, err := tr.Route(int(gateway.Tier2))
if err != nil {
    // No provider available
}

// Route with fallback to lower tiers
provider, actualTier, err := tr.RouteWithFallback(int(gateway.Tier2))
if err != nil {
    // All tiers unavailable
}

// Report status
tr.RecordSuccess(provider.Name)
// or
tr.ReportFailure(provider.Name)
```

### DAG Executor

```go
ctx := context.Background()
executor := gateway.NewDAGExecutor(10, 30*time.Second)
defer executor.Shutdown()

// Create plan
plan := gateway.DAGPlan{
    PlanID:        "my-plan",
    ExecutionMode: "PARALLEL",
    Tasks: []gateway.DAGTask{
        {
            Step:          1,
            ServiceTarget: "service-a",
            RPCMethod:     "method-a",
            Payload:       json.RawMessage(`{"key":"value"}`),
            TierFallback:  1,
            Dependencies:  []int{},
        },
        {
            Step:          2,
            ServiceTarget: "service-b",
            RPCMethod:     "method-b",
            TierFallback:  1,
            Dependencies:  []int{1}, // Depends on step 1
        },
    },
}

// Execute
results := executor.Execute(ctx, plan)

for _, result := range results {
    if result.Success {
        log.Printf("Task %d succeeded: %v", result.Step, result.Result)
    } else {
        log.Printf("Task %d failed: %v", result.Step, result.Error)
    }
}
```

### Parse JSON Plan

```go
jsonData := []byte(`{
    "plan_id": "test-plan",
    "execution_mode": "PARALLEL",
    "tasks": [...]
}`)

plan, err := gateway.ParseDAGPlan(jsonData)
if err != nil {
    log.Fatal(err)
}

// Validate
if err := plan.Validate(); err != nil {
    log.Fatal(err)
}
```

## Architecture

### Integration Points

1. **Circuit Breaker** → **Tiered Router**
   - Each provider has its own circuit breaker
   - Router checks circuit state before selecting provider

2. **Tiered Router** → **DAG Executor**
   - Task execution can request specific tier
   - Fallback tier can be configured per task

3. **Config** → **Tiered Router**
   - Provider configuration from `config.Config`
   - Automatic tier assignment based on provider name

### Thread Safety

All components are thread-safe:
- CircuitBreaker uses `sync.Mutex`
- TieredRouter uses `sync.RWMutex` for concurrent reads
- DAGExecutor uses `sync.WaitGroup` and worker pool

### Error Handling

- Circuit breaker returns `ErrCircuitOpen` when open
- Tiered router returns `NoProviderAvailableError` when no providers
- DAG executor returns errors in `TaskResult` for each task

## Testing

Run tests with:
```bash
go test ./gateway/...
```

Run benchmarks:
```bash
go test -bench=. ./gateway/...
```

## Dependencies

- `github.com/nicholas/ai-agent/config` - Configuration
- Standard library only (sync, time, context, encoding/json, errors)

## Migration Notes

The following files are deprecated:
- `circuit_breaker.go` - Use `circuit.go` instead
- `dag.go` - Use `dag_executor.go` instead

The deprecated files are kept for backwards compatibility only.