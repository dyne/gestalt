package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gestalt/internal/agent/shellgen"
	"gestalt/internal/runner/launchspec"
	"gestalt/internal/runner/proto"
	"gestalt/internal/runner/tmux"

	"github.com/gorilla/websocket"
)

const wsWriteTimeout = 10 * time.Second

type wsDialer interface {
	Dial(urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

type tmuxBridgeClient interface {
	PipePane(target, command string) error
	CapturePane(target string) ([]byte, error)
	ResizePane(target string, cols, rows uint16) error
	LoadBuffer(data []byte) error
	PasteBuffer(target string) error
	KillSession(name string) error
}

var runnerDialer wsDialer = websocket.DefaultDialer
var tmuxBridgeFactory = func() tmuxBridgeClient { return tmux.NewClient() }
var tailFileFunc = tailFile

func runRunnerBridge(ctx context.Context, launch *launchspec.LaunchSpec, baseURL, token string) error {
	if launch == nil {
		return errors.New("launch spec is required")
	}
	sessionID := strings.TrimSpace(launch.SessionID)
	if sessionID == "" {
		return errors.New("launch spec session id is required")
	}
	wsURL, err := runnerWebSocketURL(baseURL, sessionID)
	if err != nil {
		return err
	}
	conn, err := dialRunnerWebSocket(wsURL, token)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := tmuxBridgeFactory()
	if client == nil {
		return errors.New("tmux client unavailable")
	}

	sessionName := tmuxSessionName(sessionID)
	paneTarget := sessionName

	logPath, err := createLogFile(sessionID)
	if err != nil {
		return err
	}

	pipeCmd := fmt.Sprintf("cat >> %s", shellgen.EscapeShellArg(logPath))
	if err := client.PipePane(paneTarget, pipeCmd); err != nil {
		return err
	}

	writeMu := &sync.Mutex{}
	sendBinary := func(payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteMessage(websocket.BinaryMessage, payload)
	}
	sendControl := func(message interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteJSON(message)
	}

	hello := proto.HelloMessage{Type: proto.ControlTypeHello, ProtocolVersion: proto.ProtocolVersion}
	if err := sendControl(hello); err != nil {
		return err
	}

	if snapshot, err := client.CapturePane(paneTarget); err == nil && len(snapshot) > 0 {
		if err := sendBinary(snapshot); err != nil {
			return err
		}
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	streamErr := make(chan error, 1)
	go func() {
		streamErr <- tailFileFunc(streamCtx, logPath, sendBinary)
	}()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			cancel()
			<-streamErr
			_ = sendControl(proto.ExitMessage{Type: proto.ControlTypeExit})
			return err
		}
		switch msgType {
		case websocket.BinaryMessage:
			if err := injectInput(client, paneTarget, msg); err != nil {
				cancel()
				<-streamErr
				return err
			}
		case websocket.TextMessage:
			if err := handleRunnerMessage(client, paneTarget, sendControl, msg); err != nil {
				cancel()
				<-streamErr
				return err
			}
		}
	}
}

func runnerWebSocketURL(baseURL, sessionID string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("server URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse server URL: %w", err)
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", errors.New("unsupported server URL scheme")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	escapedID := url.PathEscape(sessionID)
	parsed.Path = basePath + "/ws/runner/session/" + sessionID
	parsed.RawPath = basePath + "/ws/runner/session/" + escapedID
	return parsed.String(), nil
}

func dialRunnerWebSocket(wsURL, token string) (*websocket.Conn, error) {
	header := http.Header{}
	if token = strings.TrimSpace(token); token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	conn, _, err := runnerDialer.Dial(wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("dial runner websocket: %w", err)
	}
	return conn, nil
}

func injectInput(client tmuxBridgeClient, target string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if err := client.LoadBuffer(payload); err != nil {
		return err
	}
	return client.PasteBuffer(target)
}

func handleRunnerMessage(client tmuxBridgeClient, target string, sendControl func(any) error, payload []byte) error {
	msg, err := proto.DecodeControlMessage(payload)
	if err != nil {
		return err
	}
	switch typed := msg.(type) {
	case proto.PingMessage:
		return sendControl(proto.PongMessage{Type: proto.ControlTypePong})
	case proto.ResizeMessage:
		return client.ResizePane(target, typed.Cols, typed.Rows)
	case proto.ExitMessage:
		return client.KillSession(strings.Split(target, ":")[0])
	default:
		return nil
	}
}

func createLogFile(sessionID string) (string, error) {
	name := fmt.Sprintf("gestalt-agent-%s.log", tmuxSessionName(sessionID))
	path := filepath.Join(os.TempDir(), name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("create log file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close log file: %w", err)
	}
	return path, nil
}

func tailFile(ctx context.Context, path string, onChunk func([]byte) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	var offset int64
	buf := make([]byte, 4096)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for {
				n, err := file.ReadAt(buf, offset)
				if n > 0 {
					chunk := append([]byte(nil), buf[:n]...)
					if sendErr := onChunk(chunk); sendErr != nil {
						return sendErr
					}
					offset += int64(n)
				}
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return err
				}
			}
		}
	}
}
