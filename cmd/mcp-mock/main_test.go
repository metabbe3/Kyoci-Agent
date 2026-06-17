package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// Mock MCP Server — JSON-RPC 2.0 over stdio
//
// These tests verify the mock server that the L3 benchmark connects to. The
// server speaks the same wire protocol as the real MCP client
// (internal/mcp/transport.go): one JSON-RPC message per line, newline-
// terminated, all logging to stderr.
//
// The server exposes one tool: fetch_user_schema, which returns a fixed JSON
// schema describing the User model. The benchmark's Session B asks the agent
// to call this tool and generate Go structs from the returned fields.
// =============================================================================

// parseResp reads one JSON-RPC response line from the buffer.
func parseResp(t *testing.T, buf *bytes.Buffer) map[string]json.RawMessage {
	t.Helper()
	line, err := buf.ReadBytes('\n')
	if err != nil {
		t.Fatalf("no response line in buffer: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(line, &m); err != nil {
		t.Fatalf("response not valid JSON: %v\nraw: %s", err, line)
	}
	return m
}

func TestHandleLine_Initialize(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`), &buf)

	resp := parseResp(t, &buf)
	if string(resp["jsonrpc"]) != `"2.0"` {
		t.Errorf("jsonrpc = %s, want \"2.0\"", resp["jsonrpc"])
	}
	if string(resp["id"]) != `1` {
		t.Errorf("id = %s, want 1", resp["id"])
	}
	if _, ok := resp["error"]; ok {
		t.Errorf("unexpected error: %s", resp["error"])
	}
	// Result must be an InitializeResult with the protocol version.
	var result struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		Capabilities    map[string]interface{} `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v\nraw: %s", err, resp["result"])
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocolVersion = %q, want \"2024-11-05\"", result.ProtocolVersion)
	}
	if result.ServerInfo.Name == "" {
		t.Errorf("serverInfo.name empty")
	}
}

func TestHandleLine_ToolsList(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`), &buf)

	resp := parseResp(t, &buf)
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("failed to unmarshal tools/list result: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "fetch_user_schema" {
		t.Errorf("tool name = %q, want \"fetch_user_schema\"", result.Tools[0].Name)
	}
}

func TestHandleLine_ToolsCall_FetchUserSchema(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"fetch_user_schema"}}`), &buf)

	resp := parseResp(t, &buf)
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("failed to unmarshal tools/call result: %v", err)
	}
	if result.IsError {
		t.Errorf("tool call returned isError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("content type = %q, want \"text\"", result.Content[0].Type)
	}
	// The text must be the User schema JSON with the three expected fields.
	text := result.Content[0].Text
	for _, field := range []string{"uuid", "email_address", "created_at"} {
		if !strings.Contains(text, field) {
			t.Errorf("schema text missing field %q;\ntext: %s", field, text)
		}
	}
}

func TestHandleLine_ToolsCall_UnknownTool(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nonexistent"}}`), &buf)

	resp := parseResp(t, &buf)
	var rpcErr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp["error"], &rpcErr); err != nil {
		t.Fatalf("expected error in response, got result: %s", resp["result"])
	}
	if rpcErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601 (MethodNotFound)", rpcErr.Code)
	}
}

func TestHandleLine_UnknownMethod(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(`{"jsonrpc":"2.0","id":5,"method":"resources/read"}`), &buf)

	resp := parseResp(t, &buf)
	var rpcErr struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(resp["error"], &rpcErr); err != nil {
		t.Fatalf("expected error for unknown method, got result: %s", resp["result"])
	}
	if rpcErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", rpcErr.Code)
	}
}

func TestHandleLine_MalformedJSON(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(`{not valid json`), &buf)

	resp := parseResp(t, &buf)
	var rpcErr struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(resp["error"], &rpcErr); err != nil {
		t.Fatalf("expected parse error, got result: %s", resp["result"])
	}
	if rpcErr.Code != -32700 {
		t.Errorf("error code = %d, want -32700 (ParseError)", rpcErr.Code)
	}
}

func TestHandleLine_EmptyLine_NoResponse(t *testing.T) {
	var buf bytes.Buffer
	handleLine([]byte(``), &buf)
	handleLine([]byte("   \n"), &buf)
	if buf.Len() != 0 {
		t.Errorf("empty/whitespace lines should produce no response; got: %q", buf.String())
	}
}
