package api

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/terminal"
)

type mcpPipePty struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newMCPPipePty() (*mcpPipePty, *io.PipeReader, *io.PipeWriter) {
	serverIn, clientIn := io.Pipe()
	clientOut, serverOut := io.Pipe()
	return &mcpPipePty{
		reader: clientOut,
		writer: clientIn,
	}, serverIn, serverOut
}

func (p *mcpPipePty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *mcpPipePty) Write(data []byte) (int, error) {
	return p.writer.Write(data)
}

func (p *mcpPipePty) Close() error {
	_ = p.reader.Close()
	return p.writer.Close()
}

func (p *mcpPipePty) Resize(cols, rows uint16) error {
	return nil
}

type mcpTestFactory struct {
	promptCh chan string
}

func newMCPTestFactory() *mcpTestFactory {
	return &mcpTestFactory{promptCh: make(chan string, 4)}
}

func (f *mcpTestFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty, serverIn, serverOut := newMCPPipePty()
	go runFakeMCPServer(serverIn, serverOut, f.promptCh)
	return pty, &exec.Cmd{}, nil
}

func (f *mcpTestFactory) waitForPrompt(timeout time.Duration) (string, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case prompt := <-f.promptCh:
		return prompt, true
	case <-timer.C:
		return "", false
	}
}

func runFakeMCPServer(in io.Reader, out io.Writer, promptCh chan<- string) {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		method, _ := payload["method"].(string)
		switch method {
		case "initialize":
			id := decodeJSONID(payload["id"])
			writeMCP(out, map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
				},
			})
		case "initialized":
			continue
		case "tools/call":
			id := decodeJSONID(payload["id"])
			params, _ := payload["params"].(map[string]interface{})
			args, _ := params["arguments"].(map[string]interface{})
			if prompt, ok := args["prompt"].(string); ok {
				select {
				case promptCh <- prompt:
				default:
				}
			}
			writeMCP(out, map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"threadId": "thread-1",
					"content":  "ok",
				},
			})
		}
	}
}

func writeMCP(out io.Writer, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = out.Write(append(data, '\n'))
}

func decodeJSONID(value interface{}) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	default:
		return 0
	}
}
