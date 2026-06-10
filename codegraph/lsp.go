package codegraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrLSPNotAvailable = errors.New("gopls not available")
	ErrLSPNotInit      = errors.New("LSP client not initialized")
)

// LSPClient integrates with gopls (Go Language Server) for IDE-level code intelligence
type LSPClient struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	rootDir      string
	mu           sync.Mutex
	requestID    int
	initialized  bool
	available    bool
}

// Location represents a code location
type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// ReferenceInfo represents a symbol reference
type ReferenceInfo struct {
	Location    Location `json:"location"`
	SymbolName  string   `json:"symbolName"`
	PackagePath string   `json:"packagePath"`
}

// HoverInfo represents hover information for a symbol
type HoverInfo struct {
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
	TypeDef   string `json:"typeDef"`
}

// LSPRequest represents a JSON-RPC 2.0 request
type LSPRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// LSPResponse represents a JSON-RPC 2.0 response
type LSPResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *LSPError       `json:"error,omitempty"`
}

// LSPError represents a JSON-RPC error
type LSPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeParams for LSP initialize request
type InitializeParams struct {
	RootUri      string                 `json:"rootUri"`
	Capabilities map[string]interface{} `json:"capabilities"`
}

// TextDocumentIdentifier identifies a text document
type TextDocumentIdentifier struct {
	Uri string `json:"uri"`
}

// Position represents a position in a text document
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// TextDocumentPositionParams for textDocument requests
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// LSPLocation is the LSP location format
type LSPLocation struct {
	Uri   string   `json:"uri"`
	Range LSPRange `json:"range"`
}

// LSPRange represents a range in a text document
type LSPRange struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// LSPHoverResponse represents a hover response
type LSPHoverResponse struct {
	Contents *LSPHoverContent `json:"contents"`
	Range    *LSPRange        `json:"range,omitempty"`
}

// LSPHoverContent represents hover content
type LSPHoverContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// SymbolInformation represents workspace symbol information
type SymbolInformation struct {
	Name          string       `json:"name"`
	Kind          int          `json:"kind"`
	Location      LSPLocation  `json:"location"`
	ContainerName string       `json:"containerName,omitempty"`
}

// ReferenceParams for references request
type ReferenceParams struct {
	TextDocumentPositionParams
	Context struct {
		IncludeDeclaration bool `json:"includeDeclaration"`
	} `json:"context"`
}

// NewLSPClient creates a new LSP client
func NewLSPClient(rootDir string) *LSPClient {
	return &LSPClient{
		rootDir:   rootDir,
		requestID: 0,
	}
}

// Initialize starts gopls and sends initialize request with rootDir
func (l *LSPClient) Initialize(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if gopls is available
	_, err := exec.LookPath("gopls")
	if err != nil {
		l.available = false
		return ErrLSPNotAvailable
	}

	l.available = true

	// Start gopls
	l.cmd = exec.CommandContext(ctx, "gopls", "serve")
	l.cmd.Dir = l.rootDir

	stdin, err := l.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := l.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	l.stdin = stdin
	l.stdout = stdout

	// Start the process
	if err := l.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start gopls: %w", err)
	}

	// Convert rootDir to URI
	rootURI := pathToURI(l.rootDir)

	// Send initialize request
	initParams := InitializeParams{
		RootUri: rootURI,
		Capabilities: map[string]interface{}{
			"textDocument": map[string]interface{}{
				"hover": map[string]interface{}{
					"contentFormat": []string{"plaintext", "markdown"},
				},
			},
			"workspace": map[string]interface{}{
				"symbol": map[string]interface{}{
					"symbolKind": map[string]interface{}{
						"valueSet": []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26},
					},
				},
			},
		},
	}

	var result map[string]interface{}
	if err := l.sendRequest("initialize", initParams, &result); err != nil {
		l.Shutdown()
		return fmt.Errorf("failed to initialize gopls: %w", err)
	}

	// Send initialized notification
	if err := l.sendNotification("initialized"); err != nil {
		l.Shutdown()
		return fmt.Errorf("failed to send initialized notification: %w", err)
	}

	l.initialized = true
	return nil
}

// Shutdown sends shutdown + exit
func (l *LSPClient) Shutdown() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.initialized {
		return nil
	}

	// Send shutdown request
	var result interface{}
	_ = l.sendRequest("shutdown", nil, &result)

	// Send exit notification
	_ = l.sendNotification("exit")

	// Close pipes
	if l.stdin != nil {
		_ = l.stdin.Close()
	}
	if l.stdout != nil {
		_ = l.stdout.Close()
	}

	// Wait for process to exit
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Wait()
	}

	l.initialized = false
	return nil
}

// Definition returns the definition location of a symbol at the given position
func (l *LSPClient) Definition(ctx context.Context, file string, line, col int) ([]Location, error) {
	if !l.available || !l.initialized {
		return nil, ErrLSPNotAvailable
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{
			Uri: pathToURI(file),
		},
		Position: Position{
			Line:      line - 1, // LSP uses 0-based indexing
			Character: col - 1,
		},
	}

	var result interface{}
	if err := l.sendRequest("textDocument/definition", params, &result); err != nil {
		return nil, fmt.Errorf("definition request failed: %w", err)
	}

	return l.parseLocations(result)
}

// References returns all references to a symbol at the given position
func (l *LSPClient) References(ctx context.Context, file string, line, col int) ([]ReferenceInfo, error) {
	if !l.available || !l.initialized {
		return nil, ErrLSPNotAvailable
	}

	params := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{
				Uri: pathToURI(file),
			},
			Position: Position{
				Line:      line - 1,
				Character: col - 1,
			},
		},
	}
	params.Context.IncludeDeclaration = false

	var result interface{}
	if err := l.sendRequest("textDocument/references", params, &result); err != nil {
		return nil, fmt.Errorf("references request failed: %w", err)
	}

	return l.parseReferenceInfos(result)
}

// Hover returns hover information for a symbol at the given position
func (l *LSPClient) Hover(ctx context.Context, file string, line, col int) (*HoverInfo, error) {
	if !l.available || !l.initialized {
		return nil, ErrLSPNotAvailable
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{
			Uri: pathToURI(file),
		},
		Position: Position{
			Line:      line - 1,
			Character: col - 1,
		},
	}

	var result LSPHoverResponse
	if err := l.sendRequest("textDocument/hover", params, &result); err != nil {
		return nil, fmt.Errorf("hover request failed: %w", err)
	}

	if result.Contents == nil {
		return &HoverInfo{}, nil
	}

	// Parse hover content
	content := result.Contents.Value
	signature, doc, typeDef := parseHoverContent(content)

	return &HoverInfo{
		Signature: signature,
		Doc:       doc,
		TypeDef:   typeDef,
	}, nil
}

// Symbols searches for symbols in the workspace
func (l *LSPClient) Symbols(ctx context.Context, query string) ([]ReferenceInfo, error) {
	if !l.available || !l.initialized {
		return nil, ErrLSPNotAvailable
	}

	params := map[string]interface{}{
		"query": query,
	}

	var result []SymbolInformation
	if err := l.sendRequest("workspace/symbol", params, &result); err != nil {
		return nil, fmt.Errorf("workspace/symbol request failed: %w", err)
	}

	refs := make([]ReferenceInfo, 0, len(result))
	for _, sym := range result {
		loc, err := l.locationFromLSPLocation(sym.Location)
		if err != nil {
			continue
		}
		refs = append(refs, ReferenceInfo{
			Location:    loc,
			SymbolName:  sym.Name,
			PackagePath: sym.ContainerName,
		})
	}

	return refs, nil
}

// Implementations returns all implementations of an interface at the given position
func (l *LSPClient) Implementations(ctx context.Context, file string, line, col int) ([]Location, error) {
	if !l.available || !l.initialized {
		return nil, ErrLSPNotAvailable
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{
			Uri: pathToURI(file),
		},
		Position: Position{
			Line:      line - 1,
			Character: col - 1,
		},
	}

	var result interface{}
	if err := l.sendRequest("textDocument/implementation", params, &result); err != nil {
		return nil, fmt.Errorf("implementation request failed: %w", err)
	}

	return l.parseLocations(result)
}

// sendRequest sends a JSON-RPC request and waits for response
func (l *LSPClient) sendRequest(method string, params interface{}, result interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.requestID++

	req := LSPRequest{
		Jsonrpc: "2.0",
		ID:      l.requestID,
		Method:  method,
		Params:  params,
	}

	// Marshal and send request
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(reqBytes))
	if _, err := l.stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := l.stdin.Write(reqBytes); err != nil {
		return fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	resp, err := l.readResponse()
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	if result != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// sendNotification sends a JSON-RPC notification (no response expected)
func (l *LSPClient) sendNotification(method string) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(reqBytes))
	if _, err := l.stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := l.stdin.Write(reqBytes); err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}

	return nil
}

// readResponse reads and parses an LSP response
func (l *LSPClient) readResponse() (*LSPResponse, error) {
	// Read headers
	var headers []byte
	buf := make([]byte, 1)
	for {
		n, err := l.stdout.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %w", err)
		}
		if n == 0 {
			continue
		}

		headers = append(headers, buf[0])
		if len(headers) >= 4 && bytes.Equal(headers[len(headers)-4:], []byte("\r\n\r\n")) {
			break
		}
	}

	// Parse Content-Length
	headerStr := string(headers)
	contentLen := 0
	for _, line := range strings.Split(headerStr, "\r\n") {
		if strings.HasPrefix(line, "Content-Length:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				lenStr := strings.TrimSpace(parts[1])
				contentLen, _ = strconv.Atoi(lenStr)
			}
		}
	}

	if contentLen == 0 {
		return nil, errors.New("no Content-Length in response")
	}

	// Read body
	body := make([]byte, contentLen)
	n, err := io.ReadFull(l.stdout, body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	if n != contentLen {
		return nil, fmt.Errorf("incomplete read: expected %d, got %d", contentLen, n)
	}

	// Parse response
	var resp LSPResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// parseLocations converts LSP locations to our Location format
func (l *LSPClient) parseLocations(result interface{}) ([]Location, error) {
	// Result can be a single location or an array of locations
	locations := make([]LSPLocation, 0)

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	// Try to unmarshal as array first
	if err := json.Unmarshal(resultBytes, &locations); err != nil {
		// Try single location
		var singleLoc LSPLocation
		if err := json.Unmarshal(resultBytes, &singleLoc); err == nil {
			locations = []LSPLocation{singleLoc}
		} else {
			// Try as LocationLink array (alternative format)
			var links []map[string]interface{}
			if err := json.Unmarshal(resultBytes, &links); err == nil {
				for _, link := range links {
					if uri, ok := link["targetUri"].(string); ok {
						if rangeVal, ok := link["targetRange"].(map[string]interface{}); ok {
							locations = append(locations, LSPLocation{
								Uri:   uri,
								Range: l.parseRange(rangeVal),
							})
						}
					}
				}
			}
		}
	}

	// Convert to our format
	resultLocs := make([]Location, 0, len(locations))
	for _, loc := range locations {
		parsed, err := l.locationFromLSPLocation(loc)
		if err != nil {
			continue
		}
		resultLocs = append(resultLocs, parsed)
	}

	return resultLocs, nil
}

// parseReferenceInfos converts LSP locations to ReferenceInfo format
func (l *LSPClient) parseReferenceInfos(result interface{}) ([]ReferenceInfo, error) {
	locations, err := l.parseLocations(result)
	if err != nil {
		return nil, err
	}

	refs := make([]ReferenceInfo, 0, len(locations))
	for _, loc := range locations {
		refs = append(refs, ReferenceInfo{
			Location: loc,
		})
	}

	return refs, nil
}

// locationFromLSPLocation converts LSP location to our Location format
func (l *LSPClient) locationFromLSPLocation(loc LSPLocation) (Location, error) {
	path, err := uriToPath(loc.Uri)
	if err != nil {
		return Location{}, err
	}

	return Location{
		File:   path,
		Line:   loc.Range.Start.Line + 1, // Convert to 1-based
		Column: loc.Range.Start.Character + 1,
	}, nil
}

// parseRange converts a map to LSPRange
func (l *LSPClient) parseRange(rangeVal map[string]interface{}) LSPRange {
	startPos := Position{}
	endPos := Position{}

	if start, ok := rangeVal["start"].(map[string]interface{}); ok {
		if line, ok := start["line"].(float64); ok {
			startPos.Line = int(line)
		}
		if char, ok := start["character"].(float64); ok {
			startPos.Character = int(char)
		}
	}

	if end, ok := rangeVal["end"].(map[string]interface{}); ok {
		if line, ok := end["line"].(float64); ok {
			endPos.Line = int(line)
		}
		if char, ok := end["character"].(float64); ok {
			endPos.Character = int(char)
		}
	}

	return LSPRange{
		Start: startPos,
		End:   endPos,
	}
}

// parseHoverContent extracts signature, doc, and type from hover content
func parseHoverContent(content string) (signature, doc, typeDef string) {
	lines := strings.Split(content, "\n")
	
	// First line is usually the signature/type
	if len(lines) > 0 {
		signature = strings.TrimSpace(lines[0])
	}
	
	// Look for documentation
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "/*") {
			doc += trimmed + "\n"
		}
	}
	
	// Try to extract type definition from signature
	parts := strings.SplitN(signature, " ", 2)
	if len(parts) > 1 {
		typeDef = parts[1]
	}
	
	return signature, strings.TrimSpace(doc), typeDef
}

// pathToURI converts a file path to a file:// URI
func pathToURI(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	
	// Convert to URI format
	uri := "file://"
	if strings.HasPrefix(absPath, "/") {
		// Unix path
		uri += absPath
	} else {
		// Windows path
		uri += "/" + strings.ReplaceAll(absPath, "\\", "/")
	}
	
	return uri
}

// uriToPath converts a file:// URI to a file path
func uriToPath(uri string) (string, error) {
	if !strings.HasPrefix(uri, "file://") {
		return "", fmt.Errorf("invalid file URI: %s", uri)
	}
	
	path := uri[7:] // Remove "file://" prefix
	
	// Parse as URL to handle encoding
	u, err := url.Parse(uri)
	if err == nil {
		path = u.Path
	}
	
	// Convert back to platform-specific path
	path = filepath.FromSlash(path)
	
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("URI is not absolute: %s", uri)
	}
	
	return path, nil
}

// IsAvailable returns true if gopls is available
func (l *LSPClient) IsAvailable() bool {
	return l.available
}

// IsInitialized returns true if LSP client is initialized
func (l *LSPClient) IsInitialized() bool {
	return l.initialized
}