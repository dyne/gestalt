package terminal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const mcpProtocolVersion = "2024-11-05"

type mcpPty struct {
	base     Pty
	outR     *io.PipeReader
	outW     *io.PipeWriter
	commands chan string
	incoming chan mcpMessage
	closed   chan struct{}

	lineMu  sync.Mutex
	lineBuf []byte
	lastCR  bool

	sendMu    sync.Mutex
	idCounter uint64

	initOnce sync.Once
	initErr  error

	threadID string
	debug    bool

	turnMu      sync.RWMutex
	turnHandler func(mcpTurnInfo)
	turnCount   uint64

	closeOnce sync.Once
}

type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpTurnInfo struct {
	Turn     uint64
	ThreadID string
	Tool     string
}

func newMCPPty(base Pty, debug bool) *mcpPty {
	outR, outW := io.Pipe()
	pty := &mcpPty{
		base:     base,
		outR:     outR,
		outW:     outW,
		commands: make(chan string, 16),
		incoming: make(chan mcpMessage, 32),
		closed:   make(chan struct{}),
		debug:    debug,
	}
	go pty.readLoop()
	go pty.commandLoop()
	return pty
}

func (p *mcpPty) SetTurnHandler(handler func(mcpTurnInfo)) {
	p.turnMu.Lock()
	p.turnHandler = handler
	p.turnMu.Unlock()
}

func (p *mcpPty) WaitReady(timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- p.ensureInitialized()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("mcp initialize timeout")
	}
}

func (p *mcpPty) Read(data []byte) (int, error) {
	return p.outR.Read(data)
}

func (p *mcpPty) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if p.isClosed() {
		return 0, io.ErrClosedPipe
	}

	p.lineMu.Lock()
	defer p.lineMu.Unlock()

	for _, b := range data {
		if b == '\r' {
			p.lastCR = true
			if len(p.lineBuf) > 0 {
				line := string(p.lineBuf)
				p.lineBuf = p.lineBuf[:0]
				if strings.TrimSpace(line) != "" {
					select {
					case p.commands <- line:
					case <-p.closed:
						return 0, io.ErrClosedPipe
					}
				}
			} else {
				p.lineBuf = p.lineBuf[:0]
			}
			continue
		}
		if b == '\n' {
			if p.lastCR {
				p.lastCR = false
				continue
			}
			p.lineBuf = append(p.lineBuf, b)
			continue
		}
		if p.lastCR {
			p.lastCR = false
		}
		p.lineBuf = append(p.lineBuf, b)
	}
	return len(data), nil
}

func (p *mcpPty) Close() error {
	var err error
	p.closeOnce.Do(func() {
		close(p.closed)
		close(p.commands)
		_ = p.outW.Close()
		err = p.base.Close()
	})
	return err
}

func (p *mcpPty) Resize(cols, rows uint16) error {
	return nil
}

func (p *mcpPty) isClosed() bool {
	select {
	case <-p.closed:
		return true
	default:
		return false
	}
}

func (p *mcpPty) readLoop() {
	reader := bufio.NewReader(p.base)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			if line != "" {
				var msg mcpMessage
				if err := json.Unmarshal([]byte(line), &msg); err != nil {
					p.writeError(fmt.Errorf("mcp parse error: %w", err))
				} else {
					if msg.Method != "" {
						p.handleNotification(msg)
						continue
					}
					select {
					case p.incoming <- msg:
					case <-p.closed:
						return
					}
				}
			}
		}
		if err != nil {
			close(p.incoming)
			_ = p.Close()
			return
		}
	}
}

func (p *mcpPty) commandLoop() {
	for command := range p.commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if err := p.ensureInitialized(); err != nil {
			p.writeError(err)
			continue
		}
		p.writeUserEcho(command)
		result, err := p.callTool(command)
		if err != nil {
			p.writeError(err)
			continue
		}
		if result.isError {
			p.writeError(errors.New(result.content))
			continue
		}
		p.writeAssistant(result.content)
		p.emitTurnComplete(result)
	}
}

func (p *mcpPty) ensureInitialized() error {
	p.initOnce.Do(func() {
		p.initErr = p.initialize()
	})
	return p.initErr
}

func (p *mcpPty) initialize() error {
	id := p.nextID()
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "gestalt",
				"version": "mcp-proxy",
			},
		},
	}
	if err := p.send(req); err != nil {
		return err
	}
	resp, err := p.awaitResponse(id)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize: %s", resp.Error.Message)
	}
	notify := mcpNotification{
		JSONRPC: "2.0",
		Method:  "initialized",
		Params:  map[string]interface{}{},
	}
	if err := p.send(notify); err != nil {
		return err
	}
	return nil
}

func (p *mcpPty) callTool(prompt string) (mcpResult, error) {
	args := map[string]interface{}{
		"prompt": prompt,
	}
	tool := "codex"
	if strings.TrimSpace(p.threadID) != "" {
		tool = "codex-reply"
		args["threadId"] = p.threadID
	}
	id := p.nextID()
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      tool,
			"arguments": args,
		},
	}
	if err := p.send(req); err != nil {
		return mcpResult{}, err
	}
	resp, err := p.awaitResponse(id)
	if err != nil {
		return mcpResult{}, err
	}
	if resp.Error != nil {
		return mcpResult{}, errors.New(resp.Error.Message)
	}
	result, err := parseMCPResult(resp.Result)
	if err != nil {
		return mcpResult{}, err
	}
	result.tool = tool
	if result.threadID != "" {
		p.threadID = result.threadID
	}
	return result, nil
}

func (p *mcpPty) send(payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	_, err = p.base.Write(data)
	return err
}

func (p *mcpPty) awaitResponse(id int64) (mcpMessage, error) {
	for {
		select {
		case msg, ok := <-p.incoming:
			if !ok {
				return mcpMessage{}, io.EOF
			}
			if msgID, ok := msg.idInt64(); ok && msgID == id {
				return msg, nil
			}
		case <-p.closed:
			return mcpMessage{}, io.ErrClosedPipe
		}
	}
}

func (p *mcpPty) handleNotification(msg mcpMessage) {
	if msg.Method == "" {
		return
	}
	eventLine := formatMCPNotification(msg)
	if eventLine == "" {
		return
	}
	p.writeOutput(eventLine + "\n")
}

const mcpNotificationMaxLen = 512

func formatMCPNotification(msg mcpMessage) string {
	if msg.Method == "" {
		return ""
	}
	prefix := fmt.Sprintf("[mcp %s]", msg.Method)
	suffix := ""
	if msg.Method == "codex/event" {
		// Best-effort envelope: { _meta: { requestId, threadId }, id, msg: { type, message } }.
		// Meta fields are ignored in the console output by default.
		type eventPayload struct {
			Msg struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"msg"`
		}
		var payload eventPayload
		if err := json.Unmarshal(msg.Params, &payload); err == nil {
			msgType := strings.TrimSpace(payload.Msg.Type)
			msgText := strings.TrimSpace(payload.Msg.Message)
			if msgType != "" && msgText != "" {
				suffix = fmt.Sprintf("%s: %s", msgType, msgText)
			} else if msgType != "" {
				suffix = msgType
			} else if msgText != "" {
				suffix = msgText
			}
		}
	}
	if suffix == "" {
		suffix = compactParams(msg.Params)
		if suffix == "" {
			suffix = "{}"
		}
	}
	suffix = truncateRunes(suffix, mcpNotificationMaxLen)
	return fmt.Sprintf("%s %s", prefix, suffix)
}

func compactParams(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err == nil {
		if buf.Len() > 0 {
			return buf.String()
		}
	}
	return string(trimmed)
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max == 1 {
		return "\u2026"
	}
	return string(runes[:max-1]) + "\u2026"
}

func (p *mcpPty) writeUserEcho(command string) {
	p.writeOutput(fmt.Sprintf("> %s\n", command))
}

func (p *mcpPty) writeAssistant(content string) {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return
	}
	p.writeOutput(content + "\n")
}

func (p *mcpPty) writeError(err error) {
	if err == nil {
		return
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "unknown error"
	}
	p.writeOutput(fmt.Sprintf("! error: %s\n", message))
}

func (p *mcpPty) writeOutput(message string) {
	if message == "" || p.isClosed() {
		return
	}
	message = normalizeMCPOutput(message)
	_, _ = p.outW.Write([]byte(message))
}

func normalizeMCPOutput(message string) string {
	if !strings.Contains(message, "\n") {
		return message
	}
	var builder strings.Builder
	builder.Grow(len(message) + 8)
	var prev byte
	for i := 0; i < len(message); i++ {
		b := message[i]
		if b == '\n' && prev != '\r' {
			builder.WriteByte('\r')
		}
		builder.WriteByte(b)
		prev = b
	}
	return builder.String()
}

func (p *mcpPty) emitTurnComplete(result mcpResult) {
	p.turnMu.RLock()
	handler := p.turnHandler
	p.turnMu.RUnlock()
	if handler == nil {
		return
	}
	turn := atomic.AddUint64(&p.turnCount, 1)
	handler(mcpTurnInfo{
		Turn:     turn,
		ThreadID: result.threadID,
		Tool:     result.tool,
	})
}

func (p *mcpPty) nextID() int64 {
	return int64(atomic.AddUint64(&p.idCounter, 1))
}

func (m mcpMessage) idInt64() (int64, bool) {
	if len(m.ID) == 0 {
		return 0, false
	}
	var numeric int64
	if err := json.Unmarshal(m.ID, &numeric); err == nil {
		return numeric, true
	}
	var text string
	if err := json.Unmarshal(m.ID, &text); err == nil {
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

type mcpResult struct {
	content  string
	threadID string
	isError  bool
	tool     string
}

func parseMCPResult(raw json.RawMessage) (mcpResult, error) {
	if len(raw) == 0 {
		return mcpResult{}, errors.New("empty MCP result")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return mcpResult{}, err
	}
	result := mcpResult{}
	obj, ok := value.(map[string]interface{})
	if !ok {
		if text, ok := value.(string); ok {
			result.content = text
			return result, nil
		}
		return mcpResult{}, errors.New("unsupported MCP result shape")
	}
	result.content = extractMCPContent(obj)
	result.threadID = extractMCPThreadID(obj)
	if isError, ok := obj["isError"].(bool); ok {
		result.isError = isError
	}
	if result.content == "" {
		if structured, ok := obj["structuredContent"].(map[string]interface{}); ok {
			if text, ok := structured["content"].(string); ok {
				result.content = text
			}
		}
	}
	return result, nil
}

func extractMCPContent(obj map[string]interface{}) string {
	value, ok := obj["content"]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, entry := range typed {
			item, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := item["text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			if text, ok := item["content"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func extractMCPThreadID(obj map[string]interface{}) string {
	if threadID, ok := obj["threadId"].(string); ok {
		return threadID
	}
	if structured, ok := obj["structuredContent"].(map[string]interface{}); ok {
		if threadID, ok := structured["threadId"].(string); ok {
			return threadID
		}
	}
	return ""
}
