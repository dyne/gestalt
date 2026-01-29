package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

type testPty struct {
	reader   *io.PipeReader
	writer   *io.PipeWriter
	writeCh  chan []byte
	resizeCh chan [2]uint16
}

func newTestPty() *testPty {
	reader, writer := io.Pipe()
	return &testPty{
		reader:   reader,
		writer:   writer,
		writeCh:  make(chan []byte, 4),
		resizeCh: make(chan [2]uint16, 4),
	}
}

func (p *testPty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *testPty) Write(data []byte) (int, error) {
	copied := append([]byte(nil), data...)
	select {
	case p.writeCh <- copied:
	default:
	}
	return len(data), nil
}

func (p *testPty) Close() error {
	_ = p.reader.Close()
	return p.writer.Close()
}

func (p *testPty) Resize(cols, rows uint16) error {
	select {
	case p.resizeCh <- [2]uint16{cols, rows}:
	default:
	}
	return nil
}

func (p *testPty) emitOutput(data []byte) error {
	_, err := p.writer.Write(data)
	return err
}

func (p *testPty) waitForWrite(expected []byte, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case got := <-p.writeCh:
		return bytes.Equal(got, expected)
	case <-timer.C:
		return false
	}
}

const terminalTestAgentID = "codex"

func newTerminalTestManager(options terminal.ManagerOptions) *terminal.Manager {
	if options.Agents == nil {
		options.Agents = map[string]agent.Agent{
			terminalTestAgentID: {Name: "Codex"},
		}
	}
	return terminal.NewManager(options)
}

func escapeTerminalID(id string) string {
	return url.PathEscape(id)
}

func terminalAPIPath(id string) string {
	return "/api/sessions/" + escapeTerminalID(id)
}

func (p *testPty) waitForResize(cols, rows uint16, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case got := <-p.resizeCh:
		return got[0] == cols && got[1] == rows
	case <-timer.C:
		return false
	}
}

type testFactory struct {
	mu   sync.Mutex
	ptys []*testPty
}

func (f *testFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty := newTestPty()
	f.mu.Lock()
	f.ptys = append(f.ptys, pty)
	f.mu.Unlock()
	return pty, nil, nil
}

func TestTerminalWebSocketBridge(t *testing.T) {
	factory := &testFactory{}
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := pty.emitOutput([]byte("hello\n")); err != nil {
		t.Fatalf("emit output: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if !bytes.Contains(msg, []byte("hello")) {
		t.Fatalf("expected output to contain hello, got %q", string(msg))
	}

	payload := []byte("ls\n")
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("write websocket: %v", err)
	}

	if !pty.waitForWrite(payload, 500*time.Millisecond) {
		t.Fatalf("expected PTY to receive %q", string(payload))
	}
}

func TestTerminalWebSocketResize(t *testing.T) {
	factory := &testFactory{}
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	resize := []byte(`{"type":"resize","cols":80,"rows":24}`)
	if err := conn.WriteMessage(websocket.TextMessage, resize); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	if !pty.waitForResize(80, 24, 500*time.Millisecond) {
		t.Fatalf("expected resize to reach PTY")
	}
}

func TestTerminalWebSocketAuth(t *testing.T) {
	factory := &testFactory{}
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: &TerminalHandler{
			Manager:   manager,
			AuthToken: "secret",
		}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatalf("expected unauthorized websocket dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", resp)
	}

	wsURL = wsURL + "?token=secret"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket with token: %v", err)
	}
	conn.Close()
}

func TestTerminalWebSocketConcurrentConnections(t *testing.T) {
	factory := &testFactory{}
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	connA, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket A: %v", err)
	}
	defer connA.Close()

	connB, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket B: %v", err)
	}
	defer connB.Close()

	if err := pty.emitOutput([]byte("ping\n")); err != nil {
		t.Fatalf("emit output: %v", err)
	}

	if !readWebSocketContains(t, connA, "ping") {
		t.Fatalf("expected connection A to receive output")
	}
	if !readWebSocketContains(t, connB, "ping") {
		t.Fatalf("expected connection B to receive output")
	}
}

func TestTerminalWebSocketReconnect(t *testing.T) {
	factory := &testFactory{}
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	conn.Close()

	conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("redial websocket: %v", err)
	}
	defer conn.Close()

	if err := pty.emitOutput([]byte("reconnected\n")); err != nil {
		t.Fatalf("emit output: %v", err)
	}
	if !readWebSocketContains(t, conn, "reconnected") {
		t.Fatalf("expected output after reconnect")
	}
}

func TestTerminalWebSocketCatchupFromCursor(t *testing.T) {
	factory := &testFactory{}
	logDir := t.TempDir()
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:         "/bin/sh",
		PtyFactory:    factory,
		SessionLogDir: logDir,
	})

	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	beforePayload := []byte("before-" + strings.Repeat("a", 5000) + "\n")
	afterPayload := []byte("after-" + strings.Repeat("b", 5000) + "\n")

	if err := pty.emitOutput(beforePayload); err != nil {
		t.Fatalf("emit before payload: %v", err)
	}
	cursor := waitForHistoryCursorAtLeast(t, manager, session.ID, int64(len(beforePayload)), 2*time.Second)

	if err := pty.emitOutput(afterPayload); err != nil {
		t.Fatalf("emit after payload: %v", err)
	}
	_ = waitForHistoryCursorAtLeast(t, manager, session.ID, cursor+int64(len(afterPayload)), 2*time.Second)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID) + "?cursor=" + strconv.FormatInt(cursor, 10)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read catch-up: %v", err)
	}
	if !bytes.Contains(msg, []byte("after-")) {
		t.Fatalf("expected catch-up to include after payload")
	}
	if bytes.Contains(msg, []byte("before-")) {
		t.Fatalf("did not expect catch-up to include before payload")
	}
}

func TestTerminalHistoryCatchupHasNoGaps(t *testing.T) {
	const totalLines = 1200
	const firstBatch = 400
	const gapBatch = 400
	const liveBatch = totalLines - firstBatch - gapBatch

	factory := &testFactory{}
	logDir := t.TempDir()
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:         "/bin/sh",
		PtyFactory:    factory,
		SessionLogDir: logDir,
	})

	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	lineBytes := int64(len(formatLine(1)))

	emitLines := func(start, end int) {
		for i := start; i <= end; i++ {
			line := formatLine(i)
			if err := pty.emitOutput([]byte(line)); err != nil {
				t.Fatalf("emit output: %v", err)
			}
		}
	}

	emitLines(1, firstBatch)
	cursorAfterFirst := waitForHistoryCursorAtLeast(t, manager, session.ID, lineBytes*int64(firstBatch), 2*time.Second)

	handler := &RestHandler{Manager: manager}
	historyPayload := fetchHistoryWithCursor(t, handler, session.ID, 1200, cursorAfterFirst, 2*time.Second)

	historyNumbers := parseLineNumbers(t, historyPayload.Lines)
	if len(historyNumbers) != firstBatch {
		preview := historyNumbers
		if len(preview) > 8 {
			preview = preview[:8]
		}
		t.Fatalf("expected %d history lines, got %d (preview %v)", firstBatch, len(historyNumbers), preview)
	}
	for i := 1; i < len(historyNumbers); i++ {
		if historyNumbers[i] != historyNumbers[i-1]+1 {
			t.Fatalf("history not contiguous around %d: %v", i, historyNumbers[maxInt(0, i-3):minInt(len(historyNumbers), i+3)])
		}
	}

	emitLines(firstBatch+1, firstBatch+gapBatch)
	_ = waitForHistoryCursorAtLeast(t, manager, session.ID, lineBytes*int64(firstBatch+gapBatch), 2*time.Second)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID) + "?cursor=" + strconv.FormatInt(*historyPayload.Cursor, 10)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	wsCatchup := readWebSocketLines(t, conn, gapBatch, 2*time.Second)

	emitLines(firstBatch+gapBatch+1, totalLines)

	wsLive := readWebSocketLines(t, conn, liveBatch, 2*time.Second)
	wsLines := append(wsCatchup, wsLive...)
	wsNumbers := parseLineNumbers(t, wsLines)
	for i := 1; i < len(wsNumbers); i++ {
		if wsNumbers[i] != wsNumbers[i-1]+1 {
			t.Fatalf("ws output not contiguous around %d: %v", i, wsNumbers[maxInt(0, i-3):minInt(len(wsNumbers), i+3)])
		}
	}

	combined := append(historyNumbers, wsNumbers...)
	if len(combined) == 0 {
		t.Fatalf("expected combined output")
	}
	historyTail := 0
	if len(historyNumbers) > 0 {
		historyTail = historyNumbers[len(historyNumbers)-1]
	}
	wsHead := 0
	if len(wsNumbers) > 0 {
		wsHead = wsNumbers[0]
	}
	for i := 1; i < len(combined); i++ {
		if combined[i] != combined[i-1]+1 {
			start := maxInt(0, i-3)
			end := minInt(len(combined), i+3)
			t.Fatalf("expected contiguous sequence (history tail %d, ws head %d), got %v", historyTail, wsHead, combined[start:end])
		}
	}
	if combined[len(combined)-1] != totalLines {
		t.Fatalf("expected last line %d, got %d", totalLines, combined[len(combined)-1])
	}
	if combined[0] != 1 {
		t.Fatalf("expected first line 1, got %d", combined[0])
	}

	if cursorAfterFirst <= 0 {
		t.Fatalf("expected cursor after first batch to be positive")
	}
	if liveBatch == 0 {
		t.Fatalf("expected live batch to be non-zero")
	}
}

func TestTerminalWebSocketCloseEndsHandler(t *testing.T) {
	factory := &testFactory{}
	manager := newTerminalTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	session, err := manager.Create(terminalTestAgentID, "test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	handlerDone := make(chan struct{})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			(&TerminalHandler{Manager: manager}).ServeHTTP(w, r)
			close(handlerDone)
		})},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	_ = conn.Close()

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func readWebSocketContains(t *testing.T, conn *websocket.Conn, text string) bool {
	t.Helper()
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return false
		}
		if bytes.Contains(msg, []byte(text)) {
			return true
		}
	}
	return false
}

func readWebSocketLines(t *testing.T, conn *websocket.Conn, expected int, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	lines := make([]string, 0, expected)
	buffer := []byte{}

	for len(lines) < expected && time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			t.Fatalf("read websocket: %v", err)
		}
		buffer = append(buffer, msg...)
		for {
			index := bytes.IndexByte(buffer, '\n')
			if index == -1 {
				break
			}
			line := string(buffer[:index])
			buffer = buffer[index+1:]
			if line == "" {
				continue
			}
			lines = append(lines, line)
		}
	}

	if len(lines) < expected {
		t.Fatalf("expected %d lines, got %d", expected, len(lines))
	}
	return lines
}

func parseLineNumbers(t *testing.T, lines []string) []int {
	t.Helper()
	numbers := make([]int, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "line ") {
			t.Fatalf("unexpected line format: %q", line)
		}
		value, err := strconv.Atoi(strings.TrimPrefix(line, "line "))
		if err != nil {
			t.Fatalf("parse line number: %v", err)
		}
		numbers = append(numbers, value)
	}
	return numbers
}

func formatLine(n int) string {
	return fmt.Sprintf("line %05d\n", n)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func waitForHistoryCursorAtLeast(t *testing.T, manager *terminal.Manager, id string, min int64, timeout time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cursor, err := manager.HistoryCursor(id)
		if err != nil {
			t.Fatalf("history cursor: %v", err)
		}
		if cursor != nil && *cursor >= min {
			return *cursor
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for cursor to reach %d", min)
	return 0
}

func fetchHistoryWithCursor(t *testing.T, handler *RestHandler, id string, lines int, minCursor int64, timeout time.Duration) terminalOutputResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, terminalAPIPath(id)+"/history?lines="+strconv.Itoa(lines), nil)
		req.Header.Set("Authorization", "Bearer secret")
		res := httptest.NewRecorder()

		restHandler("secret", nil, handler.handleTerminal)(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.Code)
		}

		var payload terminalOutputResponse
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode history response: %v", err)
		}
		if payload.Cursor != nil && *payload.Cursor >= minCursor {
			return payload
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for history cursor >= %d", minCursor)
	return terminalOutputResponse{}
}
