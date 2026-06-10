package engine

import (
	"time"

	"github.com/google/uuid"
)

// Source represents the protocol or interface where the task originated.
type Source string

const (
	SourceHTTP  Source = "http"
	SourceGRPC  Source = "grpc"
	SourceWS    Source = "websocket"
	SourceREPL  Source = "repl"
)

// Priority indicates the urgency of a task.
type Priority int

const (
	PriorityLow      Priority = 0
	PriorityNormal   Priority = 1
	PriorityHigh     Priority = 2
	PriorityCritical Priority = 3
)

// EngineTask represents a unit of work to be processed by the engine.
type EngineTask struct {
	ID             string
	Source         Source
	SessionID      string
	Message        string
	Metadata       map[string]string
	Priority       Priority
	Timeout        time.Duration
	MaxTokens      int
	PreferredModel string
	CreatedAt      time.Time
}

// NewEngineTask creates a new EngineTask with required fields.
func NewEngineTask(source Source, message string) *EngineTask {
	return &EngineTask{
		ID:        uuid.New().String(),
		Source:    source,
		Message:   message,
		Metadata:  make(map[string]string),
		Priority:  PriorityNormal,
		CreatedAt: time.Now(),
	}
}

// WithSession sets the session ID and returns the task for chaining.
func (t *EngineTask) WithSession(id string) *EngineTask {
	t.SessionID = id
	return t
}

// WithMetadata adds a key-value pair to metadata and returns the task for chaining.
func (t *EngineTask) WithMetadata(k, v string) *EngineTask {
	if t.Metadata == nil {
		t.Metadata = make(map[string]string)
	}
	t.Metadata[k] = v
	return t
}

// WithPriority sets the priority level and returns the task for chaining.
func (t *EngineTask) WithPriority(p Priority) *EngineTask {
	t.Priority = p
	return t
}

// WithTimeout sets the max execution time and returns the task for chaining.
func (t *EngineTask) WithTimeout(d time.Duration) *EngineTask {
	t.Timeout = d
	return t
}

// WithTokenBudget sets the token budget and returns the task for chaining.
func (t *EngineTask) WithTokenBudget(n int) *EngineTask {
	t.MaxTokens = n
	return t
}