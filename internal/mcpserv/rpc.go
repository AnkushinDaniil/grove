package mcpserv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// mcpProtocolVersion is the MCP revision grove's server advertises. Clients
// negotiate their own; grove echoes a supported version in the initialize
// result. https://modelcontextprotocol.io/specification
const mcpProtocolVersion = "2025-06-18"

// JSON-RPC 2.0 error codes used by the server.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// maxLineBytes bounds a single framed JSON-RPC message. MCP payloads are small;
// a generous cap stops a hostile or wedged peer from exhausting memory.
const maxLineBytes = 4 * 1024 * 1024

// rpcRequest is an incoming JSON-RPC 2.0 request or notification. ID is carried
// verbatim (number or string) and echoed on the response; its absence marks a
// notification, which gets no reply.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// isNotification reports whether the message expects no response.
func (r rpcRequest) isNotification() bool { return len(r.ID) == 0 }

// rpcResponse is an outgoing JSON-RPC 2.0 response. Exactly one of Result or
// Error is set.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

// newRPCError builds an *rpcError; handlers return it to control the wire code.
func newRPCError(code int, msg string) *rpcError { return &rpcError{Code: code, Message: msg} }

// lineConn frames newline-delimited JSON-RPC over a byte stream — the transport
// MCP's stdio servers use, here carried over the daemon's Unix socket. It is
// used by a single connection goroutine, so it needs no internal locking.
type lineConn struct {
	r *bufio.Reader
	w io.Writer
}

func newLineConn(rw io.ReadWriter) *lineConn {
	return &lineConn{r: bufio.NewReaderSize(rw, 64*1024), w: rw}
}

// readLine returns the next non-empty framed message, or io.EOF at end of
// stream. Blank lines are skipped; over-long lines are rejected.
func (c *lineConn) readLine() ([]byte, error) {
	for {
		line, err := c.r.ReadBytes('\n')
		if len(line) > maxLineBytes {
			return nil, fmt.Errorf("framed message exceeds %d bytes", maxLineBytes)
		}
		trimmed := trimLine(line)
		if len(trimmed) > 0 {
			return trimmed, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// writeMessage frames one JSON value as a line.
func (c *lineConn) writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal rpc message: %w", err)
	}
	if _, err := c.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write rpc message: %w", err)
	}
	return nil
}

// trimLine strips trailing CR/LF and surrounding spaces without allocating.
func trimLine(b []byte) []byte {
	for len(b) > 0 {
		switch b[len(b)-1] {
		case '\n', '\r', ' ', '\t':
			b = b[:len(b)-1]
		default:
			return b
		}
	}
	return b
}
