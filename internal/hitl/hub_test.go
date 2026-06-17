package hitl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/metabbe3/Kyoci-Agent/internal/hitl/pb"
)

// TestHub_NoSubscriber verifies the fast-fail path: when no operator is
// subscribed, RequestHelp returns ErrNoSubscriber immediately rather than
// blocking for the full timeout. The orchestrator relies on this to degrade
// cleanly when HITL is unavailable.
func TestHub_NoSubscriber(t *testing.T) {
	hub := NewHub(5*time.Second, nil)
	hint, err := hub.RequestHelp(context.Background(), HelpRequest{
		TaskID:  "task-1",
		Role:    "developer",
		Attempt: 2,
	})
	if !errors.Is(err, ErrNoSubscriber) {
		t.Fatalf("expected ErrNoSubscriber, got %v", err)
	}
	if hint != "" {
		t.Fatalf("expected empty hint, got %q", hint)
	}
}

// TestHub_RoundTrip exercises the full happy path:
//   1. operator subscribes
//   2. agent emits HelpRequest (in goroutine — RequestHelp blocks)
//   3. operator receives the request via the subscriber channel
//   4. operator calls DeliverHint
//   5. RequestHelp returns with the hint
func TestHub_RoundTrip(t *testing.T) {
	hub := NewHub(2*time.Second, nil)
	ch, unsub := hub.Subscribe()
	defer unsub()

	type result struct {
		hint string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		hint, err := hub.RequestHelp(context.Background(), HelpRequest{
			TaskID:   "task-rt",
			Role:     "qa",
			Attempt:  2,
			Question: "stuck on calculator.go",
		})
		resCh <- result{hint, err}
	}()

	// Wait for the HelpRequest to arrive on the subscriber channel.
	select {
	case req := <-ch:
		if req.GetTaskId() != "task-rt" {
			t.Fatalf("expected task-rt, got %q", req.GetTaskId())
		}
		if req.GetQuestion() != "stuck on calculator.go" {
			t.Fatalf("unexpected question: %q", req.GetQuestion())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HelpRequest was not delivered to subscriber within 500ms")
	}

	// Deliver the hint.
	if !hub.DeliverHint("task-rt", "use + not *") {
		t.Fatal("DeliverHint returned false; no waiter registered")
	}

	// RequestHelp should return with the hint.
	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("RequestHelp returned error: %v", res.err)
		}
		if res.hint != "use + not *" {
			t.Fatalf("expected hint 'use + not *', got %q", res.hint)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RequestHelp did not return within 500ms after DeliverHint")
	}
}

// TestHub_Timeout verifies that RequestHelp returns ErrNoHint when no operator
// responds within the timeout.
func TestHub_Timeout(t *testing.T) {
	hub := NewHub(100*time.Millisecond, nil)
	// Register a subscriber so we hit the wait path (not the no-subscriber fast path).
	_, unsub := hub.Subscribe()
	defer unsub()

	start := time.Now()
	hint, err := hub.RequestHelp(context.Background(), HelpRequest{
		TaskID: "task-to",
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrNoHint) {
		t.Fatalf("expected ErrNoHint, got %v (hint=%q)", err, hint)
	}
	if hint != "" {
		t.Fatalf("expected empty hint, got %q", hint)
	}
	// Should have waited ~100ms, not the 5min default.
	if elapsed > 1*time.Second {
		t.Fatalf("RequestHelp waited too long: %v", elapsed)
	}
}

// TestHub_DeliverHint_NoWaiter verifies the false return path when there's no
// registered waiter for the given task ID.
func TestHub_DeliverHint_NoWaiter(t *testing.T) {
	hub := NewHub(1*time.Second, nil)
	if hub.DeliverHint("nonexistent", "hint") {
		t.Fatal("DeliverHint returned true for nonexistent task")
	}
}

// TestHub_MultipleSubscribers verifies the broadcast path: every subscribed
// operator receives every HelpRequest. We only need one to respond, but the
// broadcast lets multiple operator terminals observe traffic.
func TestHub_MultipleSubscribers(t *testing.T) {
	hub := NewHub(2*time.Second, nil)
	ch1, unsub1 := hub.Subscribe()
	defer unsub1()
	ch2, unsub2 := hub.Subscribe()
	defer unsub2()

	done := make(chan string, 1)
	go func() {
		hint, _ := hub.RequestHelp(context.Background(), HelpRequest{
			TaskID: "task-bcast",
		})
		done <- hint
	}()

	// Both subscribers should see the request.
	for i, ch := range []<-chan *pb.HelpRequest{ch1, ch2} {
		select {
		case req := <-ch:
			if req.GetTaskId() != "task-bcast" {
				t.Fatalf("sub %d: wrong task ID %q", i, req.GetTaskId())
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("sub %d did not receive HelpRequest", i)
		}
	}

	hub.DeliverHint("task-bcast", "ok")
	select {
	case hint := <-done:
		if hint != "ok" {
			t.Fatalf("expected hint 'ok', got %q", hint)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RequestHelp did not return after hint")
	}
}

// TestHub_Unsubscribe_Basic verifies that after unsub, the channel is closed
// out of the subscriber map and HasSubscriber reflects the change.
func TestHub_Unsubscribe_Basic(t *testing.T) {
	hub := NewHub(1*time.Second, nil)
	if hub.HasSubscriber() {
		t.Fatal("fresh hub should have no subscribers")
	}
	_, unsub := hub.Subscribe()
	if !hub.HasSubscriber() {
		t.Fatal("expected subscriber after Subscribe")
	}
	unsub()
	if hub.HasSubscriber() {
		t.Fatal("expected no subscriber after unsub")
	}
}

// TestHub_NewTaskID ensures task IDs are unique and well-formed.
func TestHub_NewTaskID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewTaskID()
		if !strings.HasPrefix(id, "task-") {
			t.Fatalf("expected 'task-' prefix, got %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate task ID: %q", id)
		}
		seen[id] = true
	}
}
