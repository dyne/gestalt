package api

import "testing"

func TestParseControlMessageResize(t *testing.T) {
	msg, ok := parseControlMessage([]byte(`{"type":"resize","cols":120,"rows":40}`))
	if !ok {
		t.Fatalf("expected resize control message")
	}
	if msg.Cols != 120 || msg.Rows != 40 {
		t.Fatalf("unexpected size: %d x %d", msg.Cols, msg.Rows)
	}
}

func TestParseControlMessageRejects(t *testing.T) {
	if _, ok := parseControlMessage([]byte(`not-json`)); ok {
		t.Fatalf("expected invalid JSON to be rejected")
	}
	if _, ok := parseControlMessage([]byte(`{"type":"input","data":"ls"}`)); ok {
		t.Fatalf("expected non-resize control to be rejected")
	}
	if _, ok := parseControlMessage([]byte(`{"type":"resize","cols":0,"rows":10}`)); ok {
		t.Fatalf("expected invalid resize values to be rejected")
	}
}
