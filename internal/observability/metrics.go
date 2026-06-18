package observability

import (
	otelmetric "go.opentelemetry.io/otel/metric"
)

// InstrumentSet holds the process-wide OTel instruments. Fields are never nil:
// before Setup (or with metrics disabled) they are OTel no-op instruments, so
// callers can record unconditionally. Access via the Instruments() accessor.
type InstrumentSet struct {
	AgentExecutions    otelmetric.Int64Counter
	LLMRequestDuration otelmetric.Float64Histogram
	ToolCalls          otelmetric.Int64Counter
	HITLApprovals      otelmetric.Int64Counter
	MemoryOps          otelmetric.Int64Counter
}

func newInstruments(m otelmetric.Meter) *InstrumentSet {
	// OTel returns a usable no-op instrument (never nil) if creation fails; the
	// error is routed through the registered OTel error handler.
	exec, _ := m.Int64Counter("agent_executions_total",
		otelmetric.WithDescription("Number of agent task executions"))
	dur, _ := m.Float64Histogram("llm_request_duration_seconds",
		otelmetric.WithDescription("LLM provider request duration"), otelmetric.WithUnit("s"))
	tools, _ := m.Int64Counter("tool_calls_total",
		otelmetric.WithDescription("Number of tool invocations"))
	hitl, _ := m.Int64Counter("hitl_approvals_total",
		otelmetric.WithDescription("Number of HITL approval decisions"))
	mem, _ := m.Int64Counter("memory_ops_total",
		otelmetric.WithDescription("Number of memory operations"))
	return &InstrumentSet{
		AgentExecutions:    exec,
		LLMRequestDuration: dur,
		ToolCalls:          tools,
		HITLApprovals:      hitl,
		MemoryOps:          mem,
	}
}
