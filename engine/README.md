# Engine Package

The `engine` package provides a unified processing layer for handling tasks from multiple protocols (HTTP, gRPC, WebSocket, REPL) through a consistent pipeline.

## Components

### EngineTask

Represents a unit of work to be processed by the engine.

```go
task := engine.NewEngineTask(engine.SourceHTTP, "What is 2+2?").
    WithSession("session-123").
    WithMetadata("user_id", "user-456").
    WithPriority(engine.PriorityNormal).
    WithTokenBudget(1024)
```

Fields:
- `ID`: Unique UUID
- `Source`: Protocol origin (HTTP, GRPC, WebSocket, REPL)
- `SessionID`: Conversation session identifier
- `Message`: User input
- `Metadata`: Arbitrary key-value pairs
- `Priority`: Urgency (Low, Normal, High, Critical)
- `Timeout`: Max execution time
- `MaxTokens`: Token budget
- `PreferredModel`: Optional model override
- `CreatedAt`: Creation timestamp

### TaskResult

Represents the outcome of processing an EngineTask.

```go
result := engine.NewTaskResult(task.ID)
result.Message = "The answer is 4"
result.Tier = 1  // cheap AI
result.Duration = 150 * time.Millisecond
```

Fields:
- `TaskID`: Associated task ID
- `Success`: Whether processing succeeded
- `Message`: Response message
- `ModelUsed`: LLM model used
- `TokensIn/Out`: Token counts
- `Tier`: Processing tier (0=code, 1=cheap AI, 2=complex AI)
- `Duration`: Processing time
- `Error`: Error message if failed
- `SubTasks`: Nested results for DAG execution

### Adapters

Convert protocol-specific input into EngineTask.

#### HTTPAdapter
Parses JSON HTTP request bodies:
```json
{
  "session_id": "session-123",
  "message": "Calculate 2+2",
  "mode": "assistant",
  "model": "gpt-4",
  "max_tokens": 1024,
  "temperature": 0.7
}
```

#### GRPCAdapter
Wraps gRPC proto fields:
```json
{
  "session_id": "session-123",
  "message": "Calculate 2+2",
  "user_id": "user-456"
}
```

#### WSAdapter
Parses WebSocket messages:
```json
{
  "type": "message",
  "payload": {
    "message": "Calculate 2+2",
    "session_id": "session-123"
  }
}
```

#### REPLAdapter
Wraps raw string input directly.

### Engine

Coordinates task processing through the pipeline.

```go
engine := engine.NewEngine(
    classifier.NewClassifier(),
    skill.NewRegistry(),
    agent.NewV2(cfg, router, tools),
    pool.NewWorkerPool(4, 100),
    gateway.NewCircuitBreaker("main", gateway.WithFailureThreshold(5)),
    gateway.NewDAGExecutor(4, 30*time.Second),
)

result := engine.Process(ctx, task)
```

## Processing Pipeline

The Engine.Process method implements a 5-step pipeline:

1. **Zero-AI Skill Check**: Try to match and execute a zero-AI skill
   - If matched: return immediately (Tier 0)

2. **Complexity Classification**: Analyze input complexity (Level 1-5)
   - Trivial/Simple: No AI needed
   - Moderate: Light AI processing
   - Complex/Critical: Full AI processing

3. **Timeout Configuration**: Set context timeout based on tier
   - Level 1-2: 30 seconds
   - Level 3: 2 minutes
   - Level 4: 5 minutes
   - Level 5: 10 minutes

4. **Tier Routing**: Route to appropriate processing tier
   - Tier 0: Zero-AI (code execution)
   - Tier 1: Cheap AI (local/small model)
   - Tier 2: Complex AI (best model)

5. **Result Return**: Return TaskResult with metrics
   - Success/failure status
   - Response message
   - Model used
   - Token counts
   - Duration
   - Error details

## Integration

The engine integrates with existing packages:

- `classifier`: Complexity analysis
- `skill`: Zero-AI capabilities
- `agent`: AI processing
- `pool`: Concurrent task execution
- `gateway`: Circuit breaking and DAG execution
- `security`: Input sanitization

## Example Usage

```go
// Create engine
engine := engine.NewEngine(
    classifier.NewClassifier(),
    skillReg,
    agent,
    workerPool,
    circuitBreaker,
    dagExec,
)

// Create task from HTTP request
adapter := engine.NewHTTPAdapter()
task, err := adapter.Adapt(request.Body)
if err != nil {
    // Handle error
}

// Process task
result := engine.Process(ctx, task)

// Handle result
if result.Success {
    fmt.Printf("Response: %s\n", result.Message)
    fmt.Printf("Model: %s, Tier: %d\n", result.ModelUsed, result.Tier)
} else {
    fmt.Printf("Error: %s\n", result.Error)
}
```