package proto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ProtocolVersion is the current runner protocol version.
const ProtocolVersion = 1

// ControlType describes control message types sent over the runner WS channel.
type ControlType string

const (
	ControlTypeHello  ControlType = "hello"
	ControlTypeResize ControlType = "resize"
	ControlTypeExit   ControlType = "exit"
	ControlTypePing   ControlType = "ping"
	ControlTypePong   ControlType = "pong"
)

// ControlMessage holds the common type field for control messages.
type ControlMessage struct {
	Type ControlType `json:"type"`
}

// HelloMessage announces a runner connection and protocol version.
type HelloMessage struct {
	Type            ControlType `json:"type"`
	ProtocolVersion int         `json:"protocol_version"`
}

// ResizeMessage requests a terminal resize.
type ResizeMessage struct {
	Type ControlType `json:"type"`
	Cols uint16      `json:"cols"`
	Rows uint16      `json:"rows"`
}

// ExitMessage reports runner shutdown.
type ExitMessage struct {
	Type ControlType `json:"type"`
	Code int         `json:"code,omitempty"`
}

// PingMessage is a keepalive probe.
type PingMessage struct {
	Type ControlType `json:"type"`
}

// PongMessage is a keepalive response.
type PongMessage struct {
	Type ControlType `json:"type"`
}

// EncodeControlMessage validates and encodes a control message as JSON.
func EncodeControlMessage(msg interface{}) ([]byte, error) {
	if err := ValidateControlMessage(msg); err != nil {
		return nil, err
	}
	return json.Marshal(msg)
}

// DecodeControlMessage decodes and validates a control message from JSON.
func DecodeControlMessage(payload []byte) (interface{}, error) {
	var envelope ControlMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	if envelope.Type == "" {
		return nil, errors.New("control message missing type")
	}

	switch envelope.Type {
	case ControlTypeHello:
		var msg HelloMessage
		if err := decodeStrict(payload, &msg); err != nil {
			return nil, err
		}
		return msg, ValidateControlMessage(msg)
	case ControlTypeResize:
		var msg ResizeMessage
		if err := decodeStrict(payload, &msg); err != nil {
			return nil, err
		}
		return msg, ValidateControlMessage(msg)
	case ControlTypeExit:
		var msg ExitMessage
		if err := decodeStrict(payload, &msg); err != nil {
			return nil, err
		}
		return msg, ValidateControlMessage(msg)
	case ControlTypePing:
		var msg PingMessage
		if err := decodeStrict(payload, &msg); err != nil {
			return nil, err
		}
		return msg, ValidateControlMessage(msg)
	case ControlTypePong:
		var msg PongMessage
		if err := decodeStrict(payload, &msg); err != nil {
			return nil, err
		}
		return msg, ValidateControlMessage(msg)
	default:
		return nil, fmt.Errorf("unknown control message type %q", envelope.Type)
	}
}

// ValidateControlMessage validates a control message payload.
func ValidateControlMessage(msg interface{}) error {
	switch typed := msg.(type) {
	case HelloMessage:
		if typed.Type != ControlTypeHello {
			return fmt.Errorf("hello message has invalid type %q", typed.Type)
		}
		if typed.ProtocolVersion <= 0 {
			return errors.New("hello message requires protocol_version")
		}
		return nil
	case ResizeMessage:
		if typed.Type != ControlTypeResize {
			return fmt.Errorf("resize message has invalid type %q", typed.Type)
		}
		if typed.Cols == 0 || typed.Rows == 0 {
			return errors.New("resize message requires non-zero cols and rows")
		}
		return nil
	case ExitMessage:
		if typed.Type != ControlTypeExit {
			return fmt.Errorf("exit message has invalid type %q", typed.Type)
		}
		return nil
	case PingMessage:
		if typed.Type != ControlTypePing {
			return fmt.Errorf("ping message has invalid type %q", typed.Type)
		}
		return nil
	case PongMessage:
		if typed.Type != ControlTypePong {
			return fmt.Errorf("pong message has invalid type %q", typed.Type)
		}
		return nil
	default:
		return errors.New("unsupported control message type")
	}
}

func decodeStrict(payload []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("control message has trailing data")
		}
		return err
	}
	return nil
}
