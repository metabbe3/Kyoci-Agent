package api

import (
	"bytes"
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebSocket connection types
const (
	WSMessageTypeChat   = "chat"
	WSMessageTypeTool   = "tool"
	WSMessageTypeStatus = "status"
	WSMessageTypeError  = "error"
	WSMessageTypePong   = "pong"
)

// WebSocket message structure
type WSMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// WebSocket connection wrapper
type WSConn struct {
	conn        net.Conn
	writeMu     sync.Mutex
	closeChan   chan struct{}
	sessionID   string
	remoteAddr  string
	lastPing    time.Time
	pingMu      sync.RWMutex
}

// WebSocket manager for tracking connections
type WSManager struct {
	connections sync.Map // map[string]*WSConn
}

var wsManager = &WSManager{}

// handleWebSocketUpgrade handles WebSocket upgrade requests
func (s *ServerV2) handleWebSocketUpgrade(w http.ResponseWriter, r *http.Request) {
	// Validate WebSocket upgrade
	if r.Header.Get("Upgrade") != "websocket" {
		jsonError(w, http.StatusBadRequest, "expected WebSocket upgrade request")
		return
	}

	// Extract WebSocket key
	wsKey := r.Header.Get("Sec-WebSocket-Key")
	if wsKey == "" {
		jsonError(w, http.StatusBadRequest, "missing Sec-WebSocket-Key header")
		return
	}

	// Generate accept key
	acceptKey := computeAcceptKey(wsKey)

	// Send handshake response
	hj, ok := w.(http.Hijacker)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "cannot hijack connection")
		return
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		slog.Error("WebSocket hijack error", "error", err)
		jsonError(w, http.StatusInternalServerError, "hijack failed")
		return
	}
	defer conn.Close()

	// Send 101 Switching Protocols
	response := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		acceptKey,
	)

	if _, err := bufrw.WriteString(response); err != nil {
		slog.Error("WebSocket handshake write error", "error", err)
		return
	}
	bufrw.Flush()

	// Create WebSocket connection wrapper
	wsConn := &WSConn{
		conn:       conn,
		closeChan:  make(chan struct{}),
		remoteAddr: r.RemoteAddr,
		lastPing:   time.Now(),
	}

	// Track connection
	connID := generateConnID()
	wsManager.connections.Store(connID, wsConn)
	defer wsManager.connections.Delete(connID)

	slog.Info("New WebSocket connection", "remote_addr", r.RemoteAddr, "conn_id", connID)

	// Start ping/pong goroutine
	go s.wsPingLoop(conn, bufrw, wsConn)

	// Handle messages
	s.wsHandleMessages(conn, bufrw, wsConn)

	slog.Info("WebSocket connection closed", "remote_addr", r.RemoteAddr)
	}

// wsHandleMessages processes incoming WebSocket messages
func (s *ServerV2) wsHandleMessages(conn net.Conn, bufrw *bufio.ReadWriter, wsConn *WSConn) {
	ctx := context.Background()

	for {
		select {
		case <-wsConn.closeChan:
			return
		default:
			// Set read timeout
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			// Read frame
			frame, err := readWSFrame(conn)
			if err != nil {
				if !isConnectionClosed(err) {
					slog.Error("WebSocket read frame error", "error", err)
				}
				return
			}

			// Handle control frames
			if frame.opcode == opcodePing {
				// Respond with pong
				if err := writeWSFrame(conn, opcodePong, frame.payload); err != nil {
					slog.Error("WebSocket write pong error", "error", err)
					return
				}
				wsConn.updatePing()
				continue
			}

			if frame.opcode == opcodeClose {
				// Respond with close frame
				_ = writeWSFrame(conn, opcodeClose, []byte{})
				return
			}

			// Handle text frames (application data)
			if frame.opcode == opcodeText {
				var msg WSMessage
				if err := json.Unmarshal(frame.payload, &msg); err != nil {
					s.wsSendError(wsConn, fmt.Sprintf("invalid JSON: %v", err))
					continue
				}

				switch msg.Type {
				case WSMessageTypeChat:
					s.wsHandleChat(ctx, bufrw, wsConn, msg.Payload)
				case WSMessageTypeTool:
					s.wsHandleTool(ctx, wsConn, msg.Payload)
				case WSMessageTypeStatus:
					s.wsHandleStatus(wsConn)
				default:
					s.wsSendError(wsConn, fmt.Sprintf("unknown message type: %s", msg.Type))
				}
			}
		}
	}
}

// wsHandleChat handles WebSocket chat messages
func (s *ServerV2) wsHandleChat(ctx context.Context, bufrw *bufio.ReadWriter, wsConn *WSConn, payload map[string]interface{}) {
	message, _ := payload["message"].(string)
	mode, _ := payload["mode"].(string)
	sessionID, _ := payload["session_id"].(string)

	if message == "" {
		s.wsSendError(wsConn, "message is required")
		return
	}

	// Get or create session
	sess := s.sessions.GetOrCreate(sessionID)
	wsConn.sessionID = sess.ID

	// Set mode if specified
	if mode != "" {
		s.agent.SetMode(mode)
	}

	// Stream response
	ch, err := s.agent.Stream(ctx, message)
	if err != nil {
		s.wsSendError(wsConn, fmt.Sprintf("agent error: %v", err))
		return
	}

	for chunk := range ch {
		msg := WSMessage{
			Type: WSMessageTypeChat,
			Payload: map[string]interface{}{
				"content":    chunk,
				"session_id": sess.ID,
			},
		}
		if err := s.wsSendMessage(wsConn, msg); err != nil {
			slog.Error("WebSocket send error", "error", err)
			return
		}
	}

	// Send completion message
	memory := s.agent.GetMemory()
	tokens := memory.TokenCount()

	doneMsg := WSMessage{
		Type: WSMessageTypeChat,
		Payload: map[string]interface{}{
			"done":       true,
			"tokens":     tokens,
			"session_id": sess.ID,
		},
	}
	_ = s.wsSendMessage(wsConn, doneMsg)
}

// wsHandleTool handles WebSocket tool execution messages
func (s *ServerV2) wsHandleTool(ctx context.Context, wsConn *WSConn, payload map[string]interface{}) {
	toolName, _ := payload["tool_name"].(string)
	parameters, _ := payload["parameters"].(map[string]interface{})

	if toolName == "" {
		s.wsSendError(wsConn, "tool_name is required")
		return
	}

	// Convert parameters to JSON
	paramsBytes, err := json.Marshal(parameters)
	if err != nil {
		s.wsSendError(wsConn, "invalid parameters")
		return
	}

	result, err := s.toolReg.ExecuteTool(ctx, toolName, paramsBytes)
	response := WSMessage{
		Type: WSMessageTypeTool,
		Payload: map[string]interface{}{
			"tool_name": toolName,
			"result":    result,
		},
	}

	if err != nil {
		response.Payload["error"] = err.Error()
	}

	_ = s.wsSendMessage(wsConn, response)
}

// wsHandleStatus handles WebSocket status requests
func (s *ServerV2) wsHandleStatus(wsConn *WSConn) {
	memory := s.agent.GetMemory()
	longTermMem := s.agent.GetLongTermMemory()

	providers := s.router.ListProviders()
	tools := s.toolReg.List()

	response := WSMessage{
		Type: WSMessageTypeStatus,
		Payload: map[string]interface{}{
			"status": "ok",
			"version": "2.0.0",
			"providers": map[string]interface{}{
				"default": s.config.LLM.DefaultProvider,
				"available": providers,
				"count": len(providers),
			},
			"tools": map[string]interface{}{
				"count": len(tools),
			},
			"memory": map[string]interface{}{
				"tokens_used": memory.TokenCount(),
				"max_tokens": memory.GetMaxTokens(),
			},
			"sessions": s.sessions.Count(),
			"long_term_memory": map[string]interface{}{
				"enabled": longTermMem != nil,
			},
			"connection_id": wsConn.remoteAddr,
		},
	}

	_ = s.wsSendMessage(wsConn, response)
}

// wsSendMessage sends a WebSocket message
func (s *ServerV2) wsSendMessage(wsConn *WSConn, msg WSMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Lock write mutex
	wsConn.writeMu.Lock()
	defer wsConn.writeMu.Unlock()

	return writeWSFrame(wsConn.conn, opcodeText, data)
}

// wsSendError sends an error message over WebSocket
func (s *ServerV2) wsSendError(wsConn *WSConn, errorMsg string) {
	msg := WSMessage{
		Type: WSMessageTypeError,
		Payload: map[string]interface{}{
			"error": errorMsg,
		},
	}
	_ = s.wsSendMessage(wsConn, msg)
}

// wsPingLoop sends periodic ping frames
func (s *ServerV2) wsPingLoop(conn net.Conn, bufrw *bufio.ReadWriter, wsConn *WSConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wsConn.closeChan:
			return
		case <-ticker.C:
			// Check if connection is stale (no pong in 90 seconds)
			wsConn.pingMu.RLock()
			stale := time.Since(wsConn.lastPing) > 90*time.Second
			wsConn.pingMu.RUnlock()

			if stale {
				slog.Warn("Closing stale WebSocket connection", "remote_addr", wsConn.remoteAddr)
				close(wsConn.closeChan)
				conn.Close()
				return
			}

			// Send ping
			if err := writeWSFrame(conn, opcodePing, []byte("ping")); err != nil {
				slog.Error("WebSocket ping error", "error", err)
				close(wsConn.closeChan)
				return
			}
		}
	}
}

// updatePing updates the last ping time
func (c *WSConn) updatePing() {
	c.pingMu.Lock()
	c.lastPing = time.Now()
	c.pingMu.Unlock()
}

// ── WebSocket Protocol Helpers ──

const (
	opcodeContinuation = 0x0
	opcodeText         = 0x1
	opcodeBinary       = 0x2
	opcodeClose        = 0x8
	opcodePing         = 0x9
	opcodePong         = 0xA
)

type wsFrame struct {
	fin    bool
	rsv1   bool
	rsv2   bool
	rsv3   bool
	opcode byte
	masked bool
	mask   [4]byte
	payload []byte
}

func computeAcceptKey(wsKey string) string {
	const wsMagic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(wsKey))
	h.Write([]byte(wsMagic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func readWSFrame(conn net.Conn) (*wsFrame, error) {
	// Read first 2 bytes
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	frame := &wsFrame{
		fin:    (header[0] & 0x80) != 0,
		rsv1:   (header[0] & 0x40) != 0,
		rsv2:   (header[0] & 0x20) != 0,
		rsv3:   (header[0] & 0x10) != 0,
		opcode: header[0] & 0x0F,
		masked: (header[1] & 0x80) != 0,
	}

	payloadLen := int(header[1] & 0x7F)

	// Read extended payload length
	if payloadLen == 126 {
		ext := make([]byte, 2)
		if _, err := io.ReadFull(conn, ext); err != nil {
			return nil, err
		}
		payloadLen = int(ext[0])<<8 | int(ext[1])
	} else if payloadLen == 127 {
		ext := make([]byte, 8)
		if _, err := io.ReadFull(conn, ext); err != nil {
			return nil, err
		}
		// Ignore high 32 bits for now (frames > 4GB not supported)
		payloadLen = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
	}

	// Read masking key if present
	if frame.masked {
		if _, err := io.ReadFull(conn, frame.mask[:]); err != nil {
			return nil, err
		}
	}

	// Read payload
	frame.payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(conn, frame.payload); err != nil {
		return nil, err
	}

	// Unmask if needed
	if frame.masked {
		for i := range frame.payload {
			frame.payload[i] ^= frame.mask[i%4]
		}
	}

	return frame, nil
}

func writeWSFrame(conn net.Conn, opcode byte, payload []byte) error {
	var buf bytes.Buffer

	// First byte
	header := byte(0x80 | opcode)
	buf.WriteByte(header)

	// Payload length
	payloadLen := len(payload)
	if payloadLen < 126 {
		buf.WriteByte(byte(payloadLen))
	} else if payloadLen < 65536 {
		buf.WriteByte(126)
		buf.WriteByte(byte(payloadLen >> 8))
		buf.WriteByte(byte(payloadLen))
	} else {
		buf.WriteByte(127)
		buf.WriteByte(byte(payloadLen >> 56))
		buf.WriteByte(byte(payloadLen >> 48))
		buf.WriteByte(byte(payloadLen >> 40))
		buf.WriteByte(byte(payloadLen >> 32))
		buf.WriteByte(byte(payloadLen >> 24))
		buf.WriteByte(byte(payloadLen >> 16))
		buf.WriteByte(byte(payloadLen >> 8))
		buf.WriteByte(byte(payloadLen))
	}

	// Payload (no masking for server-to-client)
	buf.Write(payload)

	_, err := conn.Write(buf.Bytes())
	return err
}

func isConnectionClosed(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "connection reset by peer")
}

func generateConnID() string {
	return fmt.Sprintf("conn_%d", time.Now().UnixNano())
}