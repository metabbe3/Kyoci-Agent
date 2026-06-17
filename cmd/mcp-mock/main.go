// Command mcp-mock is a minimal stdio MCP server for benchmark L3.
//
// It speaks JSON-RPC 2.0 over stdin/stdout (one message per line) and exposes
// a single tool, fetch_user_schema, which returns the canonical User model:
//
//	{"model":"User","fields":["uuid","email_address","created_at"],"strict_typing":true}
//
// The Kyoci MCP client (internal/mcp/transport.go) connects to this process,
// calls initialize → tools/list → tools/call, and exposes the tool to the
// agent as kyoci_fetch_user_schema. All diagnostic output goes to stderr;
// stdout is reserved exclusively for JSON-RPC responses.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// userSchemaJSON is the fixed response returned by fetch_user_schema.
const userSchemaJSON = `{"model":"User","fields":["uuid","email_address","created_at"],"strict_typing":true}`

// rpcReq is the inbound JSON-RPC 2.0 request.
type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResp is the outbound JSON-RPC 2.0 response.
type rpcResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcErr     `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	log.SetOutput(os.Stderr) // stdout is JSON-RPC only
	log.SetFlags(0)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		handleLine(line, os.Stdout)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("stdin scanner error: %v", err)
	}
}

// handleLine parses one JSON-RPC request line and writes one response line
// to w (newline-terminated). Blank or whitespace-only lines produce no
// output — this matches the client's one-request-one-response contract.
func handleLine(line []byte, w io.Writer) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return
	}

	var req rpcReq
	if err := json.Unmarshal(line, &req); err != nil {
		writeResp(w, rpcResp{JSONRPC: "2.0", Error: &rpcErr{Code: -32700, Message: "Parse error"}})
		return
	}

	resp := dispatch(req)
	resp.JSONRPC = "2.0"
	resp.ID = req.ID
	writeResp(w, resp)
}

// dispatch routes the request to the appropriate handler.
func dispatch(req rpcReq) rpcResp {
	switch req.Method {
	case "initialize":
		return rpcResp{Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo": map[string]interface{}{
				"name":    "mcp-mock",
				"version": "1.0.0",
			},
		}}

	case "tools/list":
		return rpcResp{Result: map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "fetch_user_schema",
					"description": "MANDATORY TOOL. Returns the exact JSON schema required to build the User struct (fields: uuid, email_address, created_at; strict_typing: true). You MUST use this tool instead of searching files or using memory when asked for user schemas.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
						"required":   []string{},
					},
				},
			},
		}}

	case "tools/call":
		var p struct {
			Name string `json:"name"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &p)
		}
		if p.Name != "fetch_user_schema" {
			return rpcResp{Error: &rpcErr{Code: -32601, Message: fmt.Sprintf("unknown tool: %s", p.Name)}}
		}
		return rpcResp{Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": userSchemaJSON},
			},
			"isError": false,
		}}

	default:
		return rpcResp{Error: &rpcErr{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}}
	}
}

// writeResp serializes one response as JSON + newline to w.
func writeResp(w io.Writer, resp rpcResp) {
	b, err := json.Marshal(resp)
	if err != nil {
		// Should never happen for our simple structs; fall back to a static error.
		fmt.Fprintf(w, `{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal marshal error"}}`+"\n")
		return
	}
	b = append(b, '\n')
	_, _ = w.Write(b)
}
