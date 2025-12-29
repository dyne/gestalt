package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultBufferSize = 1000

type Logger struct {
	buffer      *LogBuffer
	output      *log.Logger
	minLevel    Level
	baseContext map[string]string
	hub         *LogHub
}

func NewLogger(buffer *LogBuffer, minLevel Level) *Logger {
	return NewLoggerWithOutput(buffer, minLevel, os.Stdout)
}

func NewLoggerWithOutput(buffer *LogBuffer, minLevel Level, output io.Writer) *Logger {
	if buffer == nil {
		buffer = NewLogBuffer(DefaultBufferSize)
	}
	if output == nil {
		output = io.Discard
	}
	return &Logger{
		buffer:   buffer,
		output:   log.New(output, "", log.LstdFlags),
		minLevel: normalizeLevel(minLevel),
		hub:      NewLogHub(),
	}
}

func (l *Logger) Buffer() *LogBuffer {
	if l == nil {
		return nil
	}
	return l.buffer
}

func (l *Logger) Subscribe() (<-chan LogEntry, func()) {
	if l == nil || l.hub == nil {
		return nil, func() {}
	}
	return l.hub.Subscribe(0)
}

func (l *Logger) With(fields map[string]string) *Logger {
	if l == nil {
		return l
	}
	return &Logger{
		buffer:      l.buffer,
		output:      l.output,
		minLevel:    l.minLevel,
		baseContext: cloneFields(l.baseContext, fields),
		hub:         l.hub,
	}
}

func (l *Logger) Debug(message string, fields map[string]string) {
	l.log(LevelDebug, message, fields)
}

func (l *Logger) Info(message string, fields map[string]string) {
	l.log(LevelInfo, message, fields)
}

func (l *Logger) Warn(message string, fields map[string]string) {
	l.log(LevelWarning, message, fields)
}

func (l *Logger) Error(message string, fields map[string]string) {
	l.log(LevelError, message, fields)
}

func (l *Logger) Enabled(level Level) bool {
	if l == nil {
		return false
	}
	return levelRank(level) >= levelRank(l.minLevel)
}

func (l *Logger) log(level Level, message string, fields map[string]string) {
	if l == nil || !l.Enabled(level) {
		return
	}

	context := cloneFields(l.baseContext, fields)
	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   message,
		Context:   context,
	}
	if len(entry.Context) == 0 {
		entry.Context = nil
	}
	if l.buffer != nil {
		l.buffer.Add(entry)
	}
	if l.hub != nil {
		l.hub.Broadcast(entry)
	}
	if l.output != nil {
		l.output.Print(formatEntry(entry))
	}
}

func normalizeLevel(level Level) Level {
	switch level {
	case LevelDebug, LevelInfo, LevelWarning, LevelError:
		return level
	default:
		return LevelInfo
	}
}

func levelRank(level Level) int {
	switch level {
	case LevelDebug:
		return 0
	case LevelInfo:
		return 1
	case LevelWarning:
		return 2
	case LevelError:
		return 3
	default:
		return 1
	}
}

func ParseLevel(value string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warning", "warn":
		return LevelWarning, true
	case "error":
		return LevelError, true
	default:
		return "", false
	}
}

func LevelAtLeast(level, minLevel Level) bool {
	if minLevel == "" {
		return true
	}
	return levelRank(level) >= levelRank(minLevel)
}

func cloneFields(base, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	combined := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		combined[key] = value
	}
	for key, value := range extra {
		combined[key] = value
	}
	return combined
}

func formatEntry(entry LogEntry) string {
	builder := strings.Builder{}
	builder.WriteString("level=")
	builder.WriteString(string(entry.Level))
	builder.WriteString(" msg=")
	builder.WriteString(strconv.Quote(entry.Message))

	if len(entry.Context) == 0 {
		return builder.String()
	}

	keys := make([]string, 0, len(entry.Context))
	for key := range entry.Context {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%s=%s", key, strconv.Quote(entry.Context[key])))
	}
	return builder.String()
}
