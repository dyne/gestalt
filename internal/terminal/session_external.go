package terminal

import (
	"time"

	"gestalt/internal/agent"
)

// NewExternalSession constructs a session backed by an external runner.
func NewExternalSession(id, title, role string, createdAt time.Time, bufferLines int, historyScanMax int64, outputPolicy OutputBackpressurePolicy, outputSampleEvery uint64, profile *agent.Agent, sessionLogger *SessionLogger, inputLogger *InputLogger) *Session {
	return newSession(id, nil, newExternalRunner(), nil, title, role, createdAt, bufferLines, historyScanMax, outputPolicy, outputSampleEvery, profile, sessionLogger, inputLogger)
}
