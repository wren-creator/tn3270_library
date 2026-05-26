package tn3270e_test

// tn3270e_test.go
// Tests for constants, EBCDIC conversion, and session construction.
// Run with: go test ./...

import (
	"strings"
	"testing"

	"github.com/wren-creator/tn3270_library/go"
)

// ── Constants ──────────────────────────────────────────────────────────────

func TestAIDValues(t *testing.T) {
	tests := []struct {
		name string
		got  byte
		want byte
	}{
		{"ENTER", tn3270e.AIDEnter, 0x7D},
		{"CLEAR", tn3270e.AIDClear, 0x6D},
		{"PA1", tn3270e.AIDPA1, 0x6C},
		{"PA2", tn3270e.AIDPA2, 0x6E},
		{"PA3", tn3270e.AIDPA3, 0x6B},
		{"PF1", tn3270e.AIDPF1, 0xF1},
		{"PF12", tn3270e.AIDPF12, 0x7C},
		{"PF24", tn3270e.AIDPF24, 0x4C},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("AID%s = 0x%02X, want 0x%02X", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestModelDims(t *testing.T) {
	tests := []struct {
		model string
		rows  int
		cols  int
	}{
		{"3278-2", 24, 80},
		{"3278-5", 27, 132},
		{"IBM-3278-2", 24, 80},
		{"unknown-model", 24, 80}, // should default to 3278-2
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			d := tn3270e.ModelDims(tt.model)
			if d.Rows != tt.rows || d.Cols != tt.cols {
				t.Errorf("ModelDims(%q) = %dx%d, want %dx%d",
					tt.model, d.Rows, d.Cols, tt.rows, tt.cols)
			}
		})
	}
}

// ── EBCDIC ─────────────────────────────────────────────────────────────────

func TestEBCDICToASCII_BasicCP037(t *testing.T) {
	// EBCDIC bytes for "HELLO" in CP037
	ebcdic := []byte{0xC8, 0xC5, 0xD3, 0xD3, 0xD6}
	got := tn3270e.EBCDICToASCII(ebcdic, 37)
	if got != "HELLO" {
		t.Errorf("EBCDICToASCII = %q, want %q", got, "HELLO")
	}
}

func TestEBCDICToASCII_Digits(t *testing.T) {
	// EBCDIC digits 0–9 are 0xF0–0xF9 in CP037
	ebcdic := []byte{0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9}
	got := tn3270e.EBCDICToASCII(ebcdic, 37)
	if got != "0123456789" {
		t.Errorf("EBCDICToASCII digits = %q, want %q", got, "0123456789")
	}
}

func TestEBCDICToASCII_FallbackCodePage(t *testing.T) {
	// Unknown code page should fall back to CP037
	ebcdic := []byte{0xC8, 0xC5, 0xD3, 0xD3, 0xD6}
	got := tn3270e.EBCDICToASCII(ebcdic, 9999)
	if got != "HELLO" {
		t.Errorf("fallback codepage: got %q, want %q", got, "HELLO")
	}
}

func TestASCIIToEBCDIC_RoundTrip(t *testing.T) {
	original := "LOGON TSO"
	ebcdic := tn3270e.ASCIIToEBCDIC(original, 37)
	roundtrip := tn3270e.EBCDICToASCII(ebcdic, 37)
	if roundtrip != original {
		t.Errorf("round-trip: got %q, want %q", roundtrip, original)
	}
}

func TestASCIIToEBCDICFixed_Padding(t *testing.T) {
	buf := tn3270e.ASCIIToEBCDICFixed("HI", 5, 37)
	if len(buf) != 5 {
		t.Errorf("fixed length: got %d bytes, want 5", len(buf))
	}
	back := tn3270e.EBCDICToASCII(buf, 37)
	if back != "HI   " {
		t.Errorf("padded value: got %q, want %q", back, "HI   ")
	}
}

func TestASCIIToEBCDICFixed_Truncation(t *testing.T) {
	buf := tn3270e.ASCIIToEBCDICFixed("HELLO WORLD", 5, 37)
	if len(buf) != 5 {
		t.Errorf("truncated length: got %d bytes, want 5", len(buf))
	}
	back := tn3270e.EBCDICToASCII(buf, 37)
	if back != "HELLO" {
		t.Errorf("truncated value: got %q, want %q", back, "HELLO")
	}
}

func TestRegisterCodePage(t *testing.T) {
	// Identity table: EBCDIC byte n → ASCII byte n
	var identity [256]byte
	for i := range identity {
		identity[i] = byte(i)
	}
	if err := tn3270e.RegisterCodePage(9998, identity, "Test Identity"); err != nil {
		t.Fatal("RegisterCodePage:", err)
	}
	buf := []byte{65, 66, 67} // ASCII A, B, C
	got := tn3270e.EBCDICToASCII(buf, 9998)
	if got != "ABC" {
		t.Errorf("custom codepage: got %q, want %q", got, "ABC")
	}
}

func TestListCodePages(t *testing.T) {
	pages := tn3270e.ListCodePages()
	found := false
	for _, p := range pages {
		if p.Number == 37 {
			found = true
			if !strings.Contains(p.Name, "CP037") {
				t.Errorf("CP037 name = %q, expected to contain 'CP037'", p.Name)
			}
		}
	}
	if !found {
		t.Error("CP037 not found in ListCodePages")
	}
}

// ── Session Construction ───────────────────────────────────────────────────

func TestNewSession_Defaults(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host: "localhost",
		Port: 23,
	})

	if sess.Model() != "3278-2" {
		t.Errorf("default model = %q, want 3278-2", sess.Model())
	}
	if sess.Rows() != 24 {
		t.Errorf("default rows = %d, want 24", sess.Rows())
	}
	if sess.Cols() != 80 {
		t.Errorf("default cols = %d, want 80", sess.Cols())
	}
}

func TestNewSession_WideScreen(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host:  "localhost",
		Port:  23,
		Model: "3278-5",
	})
	if sess.Rows() != 27 {
		t.Errorf("3278-5 rows = %d, want 27", sess.Rows())
	}
	if sess.Cols() != 132 {
		t.Errorf("3278-5 cols = %d, want 132", sess.Cols())
	}
}

func TestNewSession_InitialState(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host: "localhost",
		Port: 23,
	})
	if sess.State() != tn3270e.StateDisconnected {
		t.Errorf("initial state = %v, want Disconnected", sess.State())
	}
}

func TestNewSession_Channels(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host: "localhost",
		Port: 23,
	})
	// All channels should be non-nil and readable without blocking
	if sess.Screen() == nil {
		t.Error("Screen() channel is nil")
	}
	if sess.Errors() == nil {
		t.Error("Errors() channel is nil")
	}
	if sess.Connected() == nil {
		t.Error("Connected() channel is nil")
	}
	if sess.Ready() == nil {
		t.Error("Ready() channel is nil")
	}
	if sess.Disconnected() == nil {
		t.Error("Disconnected() channel is nil")
	}
}

func TestSetCursorRC(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host: "localhost",
		Port: 23,
	})
	// Row 2, Col 5 on 80-col screen → (2-1)*80 + (5-1) = 84
	sess.SetCursorRC(2, 5)
	if sess.CursorAddr() != 84 {
		t.Errorf("SetCursorRC(2,5) = %d, want 84", sess.CursorAddr())
	}
}

func TestSetCursor_Clamps(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host: "localhost",
		Port: 23,
	})
	sess.SetCursor(999999)
	max := sess.Rows()*sess.Cols() - 1
	if sess.CursorAddr() != max {
		t.Errorf("SetCursor clamped to %d, want %d", sess.CursorAddr(), max)
	}
}

func TestGetScreenText_EmptyScreen(t *testing.T) {
	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host: "localhost",
		Port: 23,
	})
	text := sess.GetScreenText()
	lines := strings.Split(text, "\n")
	if len(lines) != sess.Rows() {
		t.Errorf("screen text has %d lines, want %d", len(lines), sess.Rows())
	}
	if len(lines[0]) != sess.Cols() {
		t.Errorf("line length = %d, want %d", len(lines[0]), sess.Cols())
	}
}
