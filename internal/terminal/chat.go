package terminal

import (
	"strings"
	"time"

	"gestalt/internal/event"
)

// ChatSessionID is the special session id for chat-only messages.
const ChatSessionID = "chat"

// PublishChatMessage emits a chat message event for UI consumption.
func (m *Manager) PublishChatMessage(message, source, role string) bool {
	if m == nil {
		return false
	}
	text := strings.TrimSpace(message)
	if text == "" {
		return false
	}
	bus := m.chatBus
	if bus == nil {
		return false
	}
	chatRole := strings.TrimSpace(role)
	if chatRole == "" {
		chatRole = "user"
	}
	eventSource := strings.TrimSpace(source)
	bus.Publish(event.NewChatEvent(ChatSessionID, chatRole, text, eventSource, time.Now().UTC()))
	return true
}

// ChatBus returns the chat event bus for UI subscriptions.
func (m *Manager) ChatBus() *event.Bus[event.ChatEvent] {
	if m == nil {
		return nil
	}
	return m.chatBus
}
