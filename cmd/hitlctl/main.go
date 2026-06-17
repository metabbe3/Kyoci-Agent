// Command hitlctl is the operator-side client for the Kyoci HITL gRPC service.
//
// It subscribes to HelpRequests emitted by the orchestrator when an agent
// exhausts its retry budget on a VERIFY-bearing task. Two modes:
//
//   - interactive (default): prints each HelpRequest to stdout and prompts
//     the operator for a hint on stdin. The hint is submitted back via gRPC.
//
//   - auto (--auto --hint-file=path): reads the hint from a file and submits
//     it on the FIRST HelpRequest received, then exits. Used by grade_level_4.sh
//     for hands-off CI runs.
//
// Usage:
//
//	go run ./cmd/hitlctl                          # interactive, default addr
//	go run ./cmd/hitlctl --addr=localhost:50052  # explicit addr
//	go run ./cmd/hitlctl --auto --hint-file=benchmarks/hint_level_4.txt
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/metabbe3/Kyoci-Agent/internal/hitl/pb"
)

func main() {
	addr := flag.String("addr", "localhost:50052", "HITL gRPC server address")
	autoMode := flag.Bool("auto", false, "automated mode: submit hint from --hint-file on the first HelpRequest and exit")
	hintFile := flag.String("hint-file", "", "path to a file containing the hint (auto mode only)")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if *autoMode && *hintFile == "" {
		fmt.Fprintln(os.Stderr, "error: --auto requires --hint-file=path")
		os.Exit(2)
	}

	var presetHint string
	if *autoMode {
		data, err := os.ReadFile(*hintFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot read hint file %q: %v\n", *hintFile, err)
			os.Exit(1)
		}
		presetHint = string(data)
		slog.Info("loaded hint from file", "path", *hintFile, "len", len(presetHint))
	}

	// Dial the gRPC server. We retry the dial for up to 30s so the script
	// can launch hitlctl slightly before the server is ready.
	conn, err := dialWithRetry(*addr, 30*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot connect to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer conn.Close()
	client := pb.NewHITLServiceClient(conn)
	slog.Info("connected to HITL gRPC server", "addr", *addr)

	// Cancel on Ctrl-C / SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigCh
		slog.Info("signal received, shutting down", "signal", s)
		cancel()
	}()

	stream, err := client.SubscribeHelpRequests(ctx, &pb.SubscribeRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: SubscribeHelpRequests failed: %v\n", err)
		os.Exit(1)
	}
	slog.Info("subscribed; waiting for HelpRequests", "auto_mode", *autoMode)

	stdinReader := bufio.NewReader(os.Stdin)

	for {
		req, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("context cancelled, exiting")
				return
			}
			if err == io.EOF {
				slog.Info("stream closed by server")
				return
			}
			fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
			os.Exit(1)
		}

		printHelpRequest(req)

		// Get the hint: from file (auto) or stdin (interactive).
		var hint string
		if *autoMode {
			hint = presetHint
			fmt.Printf("[auto] submitting hint (%d bytes) for task %s\n", len(hint), req.GetTaskId())
		} else {
			fmt.Print("\nEnter hint (Ctrl-D to skip this request): ")
			line, err := stdinReader.ReadString('\n')
			if err == io.EOF {
				fmt.Println("\n(skipped)")
				continue
			}
			hint = line
		}

		// Submit the hint.
		submitCtx, submitCancel := context.WithTimeout(ctx, 10*time.Second)
		resp, err := client.SubmitHint(submitCtx, &pb.HintSubmission{
			TaskId: req.GetTaskId(),
			Hint:   hint,
		})
		submitCancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "SubmitHint error: %v\n", err)
			continue
		}
		if resp.GetAccepted() {
			fmt.Printf("[ok] hint accepted for task %s\n\n", req.GetTaskId())
		} else {
			fmt.Printf("[!] hint NOT accepted (no agent waiting?) for task %s\n\n", req.GetTaskId())
		}

		if *autoMode {
			slog.Info("auto mode: hint submitted, exiting")
			return
		}
	}
}

// printHelpRequest formats a HelpRequest for the operator terminal.
func printHelpRequest(req *pb.HelpRequest) {
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("🔔 HelpRequest  task=%s  role=%s  attempt=%d\n",
		req.GetTaskId(), req.GetRole(), req.GetAttempt())
	emittedAt := time.Unix(req.GetEmittedAtUnix(), 0).Format(time.RFC3339)
	fmt.Printf("   emitted: %s\n", emittedAt)
	if req.GetQuestion() != "" {
		fmt.Printf("   question: %s\n", indent(req.GetQuestion(), "   "))
	}
	if req.GetLastError() != "" {
		fmt.Printf("   last error:\n%s\n", indent(truncate(req.GetLastError(), 800), "     "))
	}
	if len(req.GetAttemptedFixes()) > 0 {
		fmt.Println("   attempted fixes:")
		for _, a := range req.GetAttemptedFixes() {
			fmt.Printf("     - %s\n", a)
		}
	}
	fmt.Println("──────────────────────────────────────────────────────────")
}

// dialWithRetry retries the gRPC dial until success or deadline.
func dialWithRetry(addr string, timeout time.Duration) (*grpc.ClientConn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timeout after %v", timeout)
	}
	return nil, lastErr
}

// indent prefixes each line of s with prefix.
func indent(s, prefix string) string {
	out := ""
	for i, line := range splitLines(s) {
		if i > 0 {
			out += "\n"
		}
		out += prefix + line
	}
	return out
}

// splitLines splits s on \n without introducing a trailing empty element.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}

// truncate clips s to max chars with a "...(truncated)" marker.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
