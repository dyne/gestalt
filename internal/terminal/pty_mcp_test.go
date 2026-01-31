package terminal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type pipePty struct {
	inWriter  *io.PipeWriter
	outReader *io.PipeReader
}

func newPipePty() (*pipePty, *io.PipeReader, *io.PipeWriter) {
	serverInReader, clientInWriter := io.Pipe()
	clientOutReader, serverOutWriter := io.Pipe()
	return &pipePty{
		inWriter:  clientInWriter,
		outReader: clientOutReader,
	}, serverInReader, serverOutWriter
}

func (p *pipePty) Read(data []byte) (int, error) {
	return p.outReader.Read(data)
}

func (p *pipePty) Write(data []byte) (int, error) {
	return p.inWriter.Write(data)
}

func (p *pipePty) Close() error {
	return errors.Join(p.inWriter.Close(), p.outReader.Close())
}

func (p *pipePty) Resize(cols, rows uint16) error {
	return nil
}

type callInfo struct {
	name string
	args map[string]interface{}
}

type fakeMCPServer struct {
	t      *testing.T
	in     *bufio.Scanner
	out    io.Writer
	onCall func(id int64, name string, args map[string]interface{})
}

func newFakeMCPServer(t *testing.T, in io.Reader, out io.Writer, onCall func(id int64, name string, args map[string]interface{})) *fakeMCPServer {
	return &fakeMCPServer{
		t:      t,
		in:     bufio.NewScanner(in),
		out:    out,
		onCall: onCall,
	}
}

func (s *fakeMCPServer) run() {
	for s.in.Scan() {
		line := strings.TrimSpace(s.in.Text())
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		method, _ := raw["method"].(string)
		switch method {
		case "initialize":
			id := decodeID(raw["id"])
			s.send(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"protocolVersion": mcpProtocolVersion,
					"capabilities":    map[string]interface{}{},
				},
			})
		case "initialized":
			continue
		case "tools/call":
			id := decodeID(raw["id"])
			params, _ := raw["params"].(map[string]interface{})
			name, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]interface{})
			if s.onCall != nil {
				s.onCall(id, name, args)
			}
		default:
			continue
		}
	}
}

func (s *fakeMCPServer) send(payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		s.t.Fatalf("marshal: %v", err)
	}
	_, _ = s.out.Write(append(data, '\n'))
}

func decodeID(value interface{}) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case string:
		if typed == "" {
			return 0
		}
		return parseInt64(typed)
	default:
		return 0
	}
}

func parseInt64(value string) int64 {
	var out int64
	_, _ = fmt.Sscan(value, &out)
	return out
}

func readOutputUntil(t *testing.T, p Pty, want []string) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		tmp := make([]byte, 256)
		for {
			n, err := p.Read(tmp)
			if n > 0 {
				buf.Write(tmp[:n])
				if containsAll(buf.String(), want) {
					done <- buf.String()
					return
				}
			}
			if err != nil {
				done <- buf.String()
				return
			}
		}
	}()

	select {
	case out := <-done:
		return out
	case <-time.After(2 * time.Second):
		_ = p.Close()
		t.Fatalf("timeout waiting for output")
		return ""
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func TestMCPPtyRoundTrip(t *testing.T) {
	pty, serverIn, serverOut := newPipePty()
	callCh := make(chan callInfo, 2)
	threadID := "thread-1"
	errCh := make(chan string, 1)

	server := newFakeMCPServer(t, serverIn, serverOut, nil)
	server.onCall = func(id int64, name string, args map[string]interface{}) {
		callCh <- callInfo{name: name, args: args}
		if name == "codex" {
			server.send(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"threadId": threadID,
					"content":  "hello",
				},
			})
			return
		}
		if name == "codex-reply" {
			server.send(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"threadId": threadID,
					"content":  "next",
				},
			})
			return
		}
		select {
		case errCh <- fmt.Sprintf("unexpected tool: %s", name):
		default:
		}
	}
	go server.run()

	mcp := newMCPPty(pty, false)
	_, _ = mcp.Write([]byte("hello\r"))
	_, _ = mcp.Write([]byte("next\r"))

	out := readOutputUntil(t, mcp, []string{"> hello", "hello", "> next", "next"})
	if !strings.Contains(out, "> hello") || !strings.Contains(out, "> next") {
		t.Fatalf("missing echo output: %q", out)
	}

	first := <-callCh
	if first.name != "codex" {
		t.Fatalf("expected codex call, got %q", first.name)
	}
	if _, ok := first.args["threadId"]; ok {
		t.Fatalf("did not expect threadId on first call")
	}
	second := <-callCh
	if second.name != "codex-reply" {
		t.Fatalf("expected codex-reply call, got %q", second.name)
	}
	if second.args["threadId"] != threadID {
		t.Fatalf("expected threadId %q, got %#v", threadID, second.args["threadId"])
	}
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}

	_ = mcp.Close()
}

func TestMCPPtyMultilinePrompt(t *testing.T) {
	pty, serverIn, serverOut := newPipePty()
	callCh := make(chan callInfo, 1)

	server := newFakeMCPServer(t, serverIn, serverOut, nil)
	server.onCall = func(id int64, name string, args map[string]interface{}) {
		callCh <- callInfo{name: name, args: args}
		server.send(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"threadId": "thread-1",
				"content":  "ok",
			},
		})
	}
	go server.run()

	mcp := newMCPPty(pty, false)
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := mcp.Read(buf); err != nil {
				return
			}
		}
	}()
	_, _ = mcp.Write([]byte("line1\nline2\r"))

	select {
	case call := <-callCh:
		if call.name != "codex" {
			t.Fatalf("expected codex call, got %q", call.name)
		}
		if call.args["prompt"] != "line1\nline2" {
			t.Fatalf("expected multiline prompt, got %#v", call.args["prompt"])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for prompt")
	}

	_ = mcp.Close()
}

func TestMCPPtyErrorResponse(t *testing.T) {
	pty, serverIn, serverOut := newPipePty()
	server := newFakeMCPServer(t, serverIn, serverOut, nil)
	server.onCall = func(id int64, name string, args map[string]interface{}) {
		server.send(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"threadId": "thread-1",
				"content":  "bad request",
				"isError":  true,
			},
		})
	}
	go server.run()

	mcp := newMCPPty(pty, false)
	_, _ = mcp.Write([]byte("oops\r"))

	out := readOutputUntil(t, mcp, []string{"! error: bad request"})
	if !strings.Contains(out, "! error: bad request") {
		t.Fatalf("expected error output, got %q", out)
	}
	_ = mcp.Close()
}
