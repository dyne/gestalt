package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/api"
	"gestalt/internal/runner/launchspec"
	"gestalt/internal/runner/tmux"
	"gestalt/internal/runner/tmuxsession"
	"gestalt/internal/terminal"
)

const mockCodexEchoScript = `#!/usr/bin/env bash
set -euo pipefail
while IFS= read -r line; do
  printf 'ECHO:%s\n' "$line"
done
`

type capturePty struct {
	mu     sync.Mutex
	writes [][]byte
	closed chan struct{}
}

func newCapturePty() *capturePty {
	return &capturePty{closed: make(chan struct{})}
}

func (p *capturePty) Read(data []byte) (int, error) {
	<-p.closed
	return 0, io.EOF
}

func (p *capturePty) Write(data []byte) (int, error) {
	p.mu.Lock()
	p.writes = append(p.writes, append([]byte(nil), data...))
	p.mu.Unlock()
	return len(data), nil
}

func (p *capturePty) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func (p *capturePty) Resize(cols, rows uint16) error {
	return nil
}

type captureFactory struct {
	pty *capturePty
}

func (f *captureFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	if f.pty == nil {
		f.pty = newCapturePty()
	}
	return f.pty, nil, nil
}

func TestGestaltSendEndToEnd(t *testing.T) {
	t.Skip("obsolete: legacy PTY path replaced by tmux-backed agent sessions")
	factory := &captureFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:  "Codex",
				Shell: "/bin/sh",
			},
		},
	})

	session, err := manager.Create("codex", "shell", "Codex")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, manager, "secret", api.StatusConfig{}, "", nil, nil, nil, nil)

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})

	withMockClient(t, transport, func() {
		t.Setenv("GESTALT_TOKEN", "secret")
		var stderr bytes.Buffer

		code := runWithSender([]string{session.ID}, strings.NewReader("ping"), &stderr, sendInput)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d: %s", code, stderr.String())
		}

		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			factory.pty.mu.Lock()
			if len(factory.pty.writes) > 0 {
				combined := bytes.Join(factory.pty.writes, nil)
				factory.pty.mu.Unlock()
				if string(combined) != "ping" {
					t.Fatalf("expected payload %q, got %q", "ping", string(combined))
				}
				return
			}
			factory.pty.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for PTY write")
	})
}

func TestTmuxMockCodexFixtureEchoesInput(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := installMockCodexBinary(t, tmpDir)

	cmd := exec.Command(binPath, "-c", "model=o3")
	cmd.Stdin = strings.NewReader("ping\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run fixture: %v (%s)", err, string(out))
	}
	if !strings.Contains(string(out), "ECHO:ping") {
		t.Fatalf("expected echoed output, got %q", string(out))
	}
}

func TestGestaltSendTmuxIntegration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	binDir := t.TempDir()
	codexPath := installMockCodexBinary(t, binDir)

	workdir := t.TempDir()
	workdir = filepath.Join(workdir, "test")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalCWD)
	})

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)
	agentsDir := filepath.Join(workdir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	agentConfig := "name = \"Codex\"\nshell = \"codex\"\ncli_type = \"codex\"\ninterface = \"cli\"\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), []byte(agentConfig), 0o644); err != nil {
		t.Fatalf("write agent config: %v", err)
	}
	var startWindowErr error

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &captureFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex",
				CLIType:   "codex",
				Interface: "cli",
			},
		},
		StartExternalTmuxWindow: func(launch *launchspec.LaunchSpec) error {
			sessionName, err := tmuxsession.WorkdirSessionName()
			if err != nil {
				return err
			}
			client := tmux.NewClient()
			exists, err := client.HasSession(sessionName)
			if err != nil {
				startWindowErr = err
				return err
			}
			if !exists {
				if err := client.CreateSession(sessionName, nil); err != nil {
					startWindowErr = err
					return err
				}
			}
			err = client.CreateWindow(sessionName, launch.SessionID, []string{codexPath})
			if err != nil {
				startWindowErr = err
			}
			return err
		},
		TmuxClientFactory: func() terminal.TmuxClient { return tmux.NewClient() },
		AgentsDir:         agentsDir,
	})

	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		t.Fatalf("resolve tmux session name: %v", err)
	}
	if tmuxSessionName != "Gestalt test" {
		t.Fatalf("expected dedicated tmux session %q, got %q", "Gestalt test", tmuxSessionName)
	}
	tmuxClient := tmux.NewClient()
	exists, err := tmuxClient.HasSession(tmuxSessionName)
	if err != nil {
		t.Fatalf("check tmux session: %v", err)
	}
	if exists {
		t.Skipf("tmux session %q already exists; refusing to reuse existing session", tmuxSessionName)
	}
	if err := tmuxClient.CreateSession(tmuxSessionName, nil); err != nil {
		t.Fatalf("create tmux session: %v", err)
	}
	t.Cleanup(func() {
		_ = tmuxClient.KillSession(tmuxSessionName)
	})

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, manager, "", api.StatusConfig{}, "", nil, nil, nil, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listener unavailable: %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: mux},
	}
	server.Start()
	defer server.Close()

	createReqBody := strings.NewReader(`{"agent":"codex","runner":"server"}`)
	createResp, err := http.Post(server.URL+"/api/sessions", "application/json", createReqBody)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201, got %d (%s) startWindowErr=%v", createResp.StatusCode, string(body), startWindowErr)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatalf("missing session id")
	}
	t.Cleanup(func() {
		_ = manager.Delete(created.ID)
	})

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host := parsedURL.Hostname()
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	var stderr bytes.Buffer
	exitCode := runWithSender([]string{"--host", host, "--port", strconv.Itoa(port), "Codex"}, strings.NewReader("ping\n"), &stderr, sendInput)
	if exitCode != 0 {
		t.Fatalf("expected send exit code 0, got %d (%s)", exitCode, stderr.String())
	}

	target := tmuxSessionName + ":" + created.ID
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		captured, err := tmux.NewClient().CapturePane(target)
		if err == nil && strings.Contains(string(captured), "ECHO:ping") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	captured, _ := tmux.NewClient().CapturePane(target)
	t.Fatalf("expected tmux pane output to contain %q, got %q", "ECHO:ping", string(captured))
}

func installMockCodexBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte(mockCodexEchoScript), 0o755); err != nil {
		t.Fatalf("write mock codex: %v", err)
	}
	return path
}
