package main

import (
	"bytes"
	"errors"
	"testing"
)

// Replies captured from a real monitor (input report, including the leading
// report-ID byte that hidapi returns).
var (
	replyBrightness = []byte{0x00, 0x6e, 0x88, 0x02, 0x00, 0x10, 0x00, 0x00, 0x64, 0x00, 0x1c, 0xdc}
	replyKVM        = []byte{0x00, 0x6e, 0x88, 0x02, 0x00, 0xe0, 0x69, 0x00, 0x01, 0x00, 0x00, 0x3c}
	replyColourMode = []byte{0x00, 0x6e, 0x88, 0x02, 0x00, 0xe0, 0x03, 0x00, 0x04, 0x00, 0x01, 0x52}
)

func TestGetVCPPayload(t *testing.T) {
	tests := []struct {
		code uint16
		want []byte
	}{
		{0x10, []byte{0x51, 0x82, 0x01, 0x10, 0xc2}},
		{0xe069, []byte{0x51, 0x83, 0x01, 0xe0, 0x69, 0x5a}},
	}
	for _, tt := range tests {
		if got := getVCPPayload(tt.code); !bytes.Equal(got, tt.want) {
			t.Errorf("getVCPPayload(%#x) = % x, want % x", tt.code, got, tt.want)
		}
	}
}

func TestBridgeReport(t *testing.T) {
	payload := getVCPPayload(0x10)
	wr := bridgeReport(false, len(payload), payload)
	if len(wr) != 193 {
		t.Fatalf("len = %d, want 193", len(wr))
	}
	if want := []byte{0x00, 0x40, 0xc6, 0, 0, 0, 0, 0x05, 0, 0x6e, 0x00, 0x80}; !bytes.Equal(wr[:12], want) {
		t.Errorf("write header = % x, want % x", wr[:12], want)
	}
	if !bytes.Equal(wr[1+0x40:1+0x40+len(payload)], payload) {
		t.Errorf("payload not at offset 0x40")
	}

	rd := bridgeReport(true, ddcReplyLen, nil)
	if want := []byte{0x00, 0x40, 0xd6, 0x51, 0, 0, 0, 0x24, 0, 0x6e, 0x01, 0x80}; !bytes.Equal(rd[:12], want) {
		t.Errorf("read header = % x, want % x", rd[:12], want)
	}
}

func TestParseGetReply(t *testing.T) {
	tests := []struct {
		name     string
		reply    []byte
		code     uint16
		cur, max uint16
	}{
		{"one-byte code", replyBrightness, 0x10, 28, 100},
		{"two-byte code", replyKVM, 0xe069, 0, 1},
		{"two-byte code, nonzero", replyColourMode, 0xe003, 1, 4},
	}
	for _, tt := range tests {
		cur, max, err := parseGetReply(tt.reply, tt.code)
		if err != nil || cur != tt.cur || max != tt.max {
			t.Errorf("%s: got (%d, %d, %v), want (%d, %d, nil)", tt.name, cur, max, err, tt.cur, tt.max)
		}
	}
}

func TestParseGetReplyStale(t *testing.T) {
	// Reading "count" (0xe02a) returned the previous colour-mode reply.
	if _, _, err := parseGetReply(replyColourMode, 0xe02a); !errors.Is(err, errStale) {
		t.Errorf("err = %v, want errStale", err)
	}
}

func TestParseGetReplyGarbage(t *testing.T) {
	if _, _, err := parseGetReply(make([]byte, 193), 0x10); err == nil {
		t.Error("expected an error for an empty report")
	}
}

func TestParseCode(t *testing.T) {
	props := map[string]Property{"brightness": {Name: "brightness", Value: 0x10}}
	for in, want := range map[string]uint16{"brightness": 0x10, "0x60": 0x60, "96": 96, "0xe069": 0xe069} {
		if got, err := parseCode(in, props); err != nil || got != want {
			t.Errorf("parseCode(%q) = %#x, %v; want %#x", in, got, err, want)
		}
	}
	for _, in := range []string{"nope", "0", "0x10000"} {
		if _, err := parseCode(in, props); err == nil {
			t.Errorf("parseCode(%q) should fail", in)
		}
	}
}
