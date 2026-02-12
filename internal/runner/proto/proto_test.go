package proto

import "testing"

func TestDecodeControlMessageHello(t *testing.T) {
	msg, err := DecodeControlMessage([]byte(`{"type":"hello","protocol_version":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hello, ok := msg.(HelloMessage)
	if !ok {
		t.Fatalf("expected hello message, got %T", msg)
	}
	if hello.ProtocolVersion != 1 {
		t.Fatalf("expected protocol version 1, got %d", hello.ProtocolVersion)
	}
}

func TestDecodeControlMessageRejectsUnknownType(t *testing.T) {
	if _, err := DecodeControlMessage([]byte(`{"type":"nope"}`)); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestDecodeControlMessageRejectsMissingFields(t *testing.T) {
	if _, err := DecodeControlMessage([]byte(`{"type":"hello"}`)); err == nil {
		t.Fatal("expected error for missing protocol_version")
	}
	if _, err := DecodeControlMessage([]byte(`{"type":"resize","cols":80}`)); err == nil {
		t.Fatal("expected error for missing rows")
	}
}

func TestDecodeControlMessageRejectsUnknownFields(t *testing.T) {
	if _, err := DecodeControlMessage([]byte(`{"type":"ping","extra":true}`)); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestEncodeControlMessageValidates(t *testing.T) {
	if _, err := EncodeControlMessage(HelloMessage{Type: ControlTypeHello}); err == nil {
		t.Fatal("expected error for missing protocol version")
	}
	if _, err := EncodeControlMessage(ResizeMessage{Type: ControlTypeResize, Cols: 0, Rows: 10}); err == nil {
		t.Fatal("expected error for invalid resize message")
	}
	if _, err := EncodeControlMessage(PingMessage{Type: ControlTypePing}); err != nil {
		t.Fatalf("unexpected error encoding ping: %v", err)
	}
}
