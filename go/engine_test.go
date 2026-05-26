package tn3270e

// engine_test.go
// ─────────────────────────────────────────────────────────────────────────────
// White-box tests for the engine internals — Telnet parser, order processor,
// field parser, AID encoder, address codec. These use the internal package
// (no _test suffix) so we can reach unexported types directly.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"net"
	"testing"
	"time"
)

// makeEngine creates a minimal engine wired to a session for testing.
func makeEngine(model string) *engine {
	sess := NewSession(SessionOptions{Host: "localhost", Port: 23, Model: model})
	return sess.engine
}

// ── Address codec ──────────────────────────────────────────────────────────

func TestDecodeAddr_12bit(t *testing.T) {
	// 12-bit code table: top 2 bits of b1 are 01 or 10
	// BufAddrCode[1] = 0xC1, BufAddrCode[0] = 0x40
	// encodeAddr(64) → top 6 bits = 1, bottom 6 = 0
	// → BufAddrCode[1]=0xC1, BufAddrCode[0]=0x40
	b1, b2 := encodeAddr(64)
	got := decodeAddr(b1, b2)
	if got != 64 {
		t.Errorf("encode/decode 64: got %d", got)
	}
}

func TestDecodeAddr_RoundTrip(t *testing.T) {
	for _, addr := range []int{0, 1, 79, 80, 319, 1919} {
		b1, b2 := encodeAddr(addr)
		got := decodeAddr(b1, b2)
		if got != addr {
			t.Errorf("round-trip addr=%d: got %d (b1=0x%02X b2=0x%02X)",
				addr, got, b1, b2)
		}
	}
}

func TestDecodeAddr_RowCol(t *testing.T) {
	// Row 2, col 1 on 80-col screen = linear address 80
	b1, b2 := encodeAddr(80)
	got := decodeAddr(b1, b2)
	if got != 80 {
		t.Errorf("row2/col1: got %d, want 80", got)
	}
}

// ── Telnet parser ──────────────────────────────────────────────────────────

func TestParseTelnet_PlainData(t *testing.T) {
	e := makeEngine("3278-2")
	// Plain EBCDIC data with no IAC bytes — should accumulate in currentRecord
	e.recvBuf = []byte{0xC8, 0xC5, 0xD3, 0xD3, 0xD6} // HELLO in EBCDIC
	e.parseTelnet()
	if len(e.currentRecord) != 5 {
		t.Errorf("plain data: currentRecord len=%d, want 5", len(e.currentRecord))
	}
	if len(e.recvBuf) != 0 {
		t.Errorf("plain data: recvBuf should be empty, got %d bytes", len(e.recvBuf))
	}
}

func TestParseTelnet_IACEscape(t *testing.T) {
	e := makeEngine("3278-2")
	// IAC IAC → single 0xFF data byte
	e.recvBuf = []byte{0xFF, 0xFF}
	e.parseTelnet()
	if len(e.currentRecord) != 1 || e.currentRecord[0] != 0xFF {
		t.Errorf("IAC escape: got %v, want [0xFF]", e.currentRecord)
	}
}

func TestParseTelnet_EORDispatch(t *testing.T) {
	e := makeEngine("3278-2")
	// A simple 3270 Erase/Write record followed by IAC EOR
	// F5 = EraseWrite, C2 = WCC (reset+unlock), then IAC EOR
	// The record is too short to render a real screen but onRecord must be called.
	e.recvBuf = []byte{0xF5, 0xC2, 0xFF, 0xEF}
	e.parseTelnet()
	// After parsing, recvBuf should be empty and currentRecord reset
	if len(e.recvBuf) != 0 {
		t.Errorf("after EOR: recvBuf not empty: %v", e.recvBuf)
	}
	if len(e.currentRecord) != 0 {
		t.Errorf("after EOR: currentRecord not reset: %v", e.currentRecord)
	}
}

func TestParseTelnet_IncompleteWaits(t *testing.T) {
	e := makeEngine("3278-2")
	// IAC with no following byte — should leave recvBuf intact
	e.recvBuf = []byte{0xFF}
	e.parseTelnet()
	if len(e.recvBuf) != 1 {
		t.Errorf("incomplete IAC: recvBuf should still have 1 byte, got %d", len(e.recvBuf))
	}
}

func TestParseTelnet_MultipleRecords(t *testing.T) {
	e := makeEngine("3278-2")
	// Two minimal records back-to-back
	// Record 1: EraseWrite + WCC + IAC EOR
	// Record 2: Write + WCC + IAC EOR
	e.recvBuf = []byte{
		0xF5, 0xC2, 0xFF, 0xEF, // record 1
		0xF1, 0xC2, 0xFF, 0xEF, // record 2
	}
	e.parseTelnet()
	if len(e.recvBuf) != 0 {
		t.Errorf("two records: recvBuf not empty: %v", e.recvBuf)
	}
}

// ── Order processor ────────────────────────────────────────────────────────

func TestProcessOrders_SBA_SF(t *testing.T) {
	e := makeEngine("3278-2")
	// SBA to address 0 + SF with protected FA
	b1, b2 := encodeAddr(0)
	orders := []byte{
		OrderSBA, b1, b2,   // SBA → address 0
		OrderSF, 0x60,      // SF: protected, high-intensity
	}
	e.processOrders(orders, 0)

	e.mu.Lock()
	cell := e.buffer[0]
	e.mu.Unlock()

	if !cell.isFA {
		t.Error("buffer[0] should be an FA cell")
	}
	if cell.fa != 0x60 {
		t.Errorf("FA = 0x%02X, want 0x60", cell.fa)
	}
}

func TestProcessOrders_SBA_Data(t *testing.T) {
	e := makeEngine("3278-2")
	b1, b2 := encodeAddr(5)
	// Write EBCDIC 'H' (0xC8) at address 5
	orders := []byte{OrderSBA, b1, b2, 0xC8}
	e.processOrders(orders, 0)

	e.mu.Lock()
	ch := e.buffer[5].char
	e.mu.Unlock()

	if ch != 0xC8 {
		t.Errorf("buffer[5].char = 0x%02X, want 0xC8", ch)
	}
}

func TestProcessOrders_IC(t *testing.T) {
	e := makeEngine("3278-2")
	b1, b2 := encodeAddr(42)
	orders := []byte{OrderSBA, b1, b2, OrderIC}
	e.processOrders(orders, 0)

	e.mu.Lock()
	cur := e.cursorAddr
	e.mu.Unlock()

	if cur != 42 {
		t.Errorf("cursor after IC = %d, want 42", cur)
	}
}

func TestProcessOrders_RA(t *testing.T) {
	e := makeEngine("3278-2")
	// RA from address 0 to address 5 with EBCDIC underscore
	b1, b2 := encodeAddr(5)
	// Start at 0, RA to 5, fill with 0x6D (EBCDIC '_')
	orders := []byte{OrderRA, b1, b2, 0x6D}
	e.processOrders(orders, 0)

	e.mu.Lock()
	defer e.mu.Unlock()
	for i := 0; i < 5; i++ {
		if e.buffer[i].char != 0x6D {
			t.Errorf("buffer[%d].char = 0x%02X, want 0x6D", i, e.buffer[i].char)
		}
	}
}

func TestProcessOrders_SFE_Color(t *testing.T) {
	e := makeEngine("3278-2")
	// SFE at address 0: count=2, basic FA attr + color
	orders := []byte{
		OrderSFE,
		0x02,        // 2 pairs
		0xC0, 0x40,  // basic field attr = 0x40 (unprotected, normal)
		ExtAttrForeground, ColorGreen, // foreground = green
	}
	e.processOrders(orders, 0)

	e.mu.Lock()
	cell := e.buffer[0]
	e.mu.Unlock()

	if !cell.isFA {
		t.Error("SFE cell should be isFA")
	}
	if cell.color != ColorGreen {
		t.Errorf("color = 0x%02X, want 0x%02X (green)", cell.color, ColorGreen)
	}
}

// ── Field parser ───────────────────────────────────────────────────────────

func TestGetFields_Basic(t *testing.T) {
	e := makeEngine("3278-2")

	// Build a minimal screen: [FA protected] [FA unprotected] [data]
	e.mu.Lock()
	e.buffer[0].isFA = true
	e.buffer[0].fa = FAProtected // protected label

	e.buffer[10].isFA = true
	e.buffer[10].fa = 0x40 // unprotected input

	// Write "HELLO" starting at position 11
	hello := ASCIIToEBCDIC("HELLO", 37)
	for i, b := range hello {
		e.buffer[11+i].char = b
	}
	e.mu.Unlock()

	fields := e.getFields()
	if len(fields) < 2 {
		t.Fatalf("expected at least 2 fields, got %d", len(fields))
	}

	// Second field should be unprotected and contain "HELLO"
	f := fields[1]
	if f.Protected {
		t.Error("second field should be unprotected")
	}
	if len(f.Value) < 5 || f.Value[:5] != "HELLO" {
		t.Errorf("second field value = %q, want starts with 'HELLO'", f.Value)
	}
}

func TestGetFields_EmptyScreen(t *testing.T) {
	e := makeEngine("3278-2")
	fields := e.getFields()
	// Empty screen has no FA positions — no fields
	if len(fields) != 0 {
		t.Errorf("empty screen: got %d fields, want 0", len(fields))
	}
}

// ── Screen text ────────────────────────────────────────────────────────────

func TestGetScreenText_Dimensions(t *testing.T) {
	e := makeEngine("3278-2")
	text := e.getScreenText()
	lines := splitLines(text)
	if len(lines) != 24 {
		t.Errorf("3278-2 screen text: %d lines, want 24", len(lines))
	}
	if len(lines[0]) != 80 {
		t.Errorf("3278-2 screen text: line width=%d, want 80", len(lines[0]))
	}
}

func TestGetScreenText_WideScreen(t *testing.T) {
	e := makeEngine("3278-5")
	text := e.getScreenText()
	lines := splitLines(text)
	if len(lines) != 27 {
		t.Errorf("3278-5 screen text: %d lines, want 27", len(lines))
	}
	if len(lines[0]) != 132 {
		t.Errorf("3278-5 line width=%d, want 132", len(lines[0]))
	}
}

// ── resetMDT / eraseUnprotected ───────────────────────────────────────────

func TestResetMDT(t *testing.T) {
	e := makeEngine("3278-2")
	e.mu.Lock()
	e.buffer[5].modified = true
	e.buffer[10].modified = true
	e.mu.Unlock()

	e.resetMDT()

	e.mu.Lock()
	defer e.mu.Unlock()
	for i, c := range e.buffer {
		if c.modified {
			t.Errorf("buffer[%d].modified still true after resetMDT", i)
		}
	}
}

func TestEraseUnprotected(t *testing.T) {
	e := makeEngine("3278-2")
	e.mu.Lock()
	// FA: unprotected at position 0
	e.buffer[0].isFA = true
	e.buffer[0].fa = 0x40 // unprotected
	// Write some data after the FA
	e.buffer[1].char = 0xC8 // 'H' in EBCDIC
	e.buffer[2].char = 0xC9 // 'I'
	e.mu.Unlock()

	e.eraseUnprotected()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.buffer[1].char != 0x40 {
		t.Errorf("eraseUnprotected: buffer[1].char = 0x%02X, want 0x40 (space)", e.buffer[1].char)
	}
}

// ── AID encoding ───────────────────────────────────────────────────────────

func TestSendAid_NotConnected(t *testing.T) {
	e := makeEngine("3278-2")
	// Not connected — should return an error
	err := e.sendAid(AIDEnter, 0, nil)
	if err == nil {
		t.Error("sendAid on disconnected session should return error")
	}
}

func TestSendAid_ClearNoFields(t *testing.T) {
	// Verify CLEAR builds the shortest possible record (AID + IAC EOR only)
	// We test the encoding logic without a real connection by inspecting
	// what bytes would be written.
	e := makeEngine("3278-2")

	// Intercept the send — replace conn with a mock
	var captured []byte
	e.conn = &mockConn{writeFn: func(b []byte) (int, error) {
		captured = append(captured, b...)
		return len(b), nil
	}}

	err := e.sendAid(AIDClear, 0, nil)
	if err != nil {
		t.Fatal("sendAid CLEAR:", err)
	}

	// Expected: [AIDClear, IAC, EOR]
	want := []byte{AIDClear, TelnetIAC, TelnetEOR}
	if len(captured) != len(want) {
		t.Fatalf("CLEAR record: got %v, want %v", captured, want)
	}
	for i, b := range want {
		if captured[i] != b {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, captured[i], b)
		}
	}
}

func TestSendAid_EnterWithField(t *testing.T) {
	e := makeEngine("3278-2")
	var captured []byte
	e.conn = &mockConn{writeFn: func(b []byte) (int, error) {
		captured = append(captured, b...)
		return len(b), nil
	}}

	err := e.sendAid(AIDEnter, 80, []FieldData{{Addr: 81, Value: "A"}})
	if err != nil {
		t.Fatal("sendAid ENTER:", err)
	}

	// Record must start with AIDEnter
	if len(captured) == 0 || captured[0] != AIDEnter {
		t.Errorf("ENTER record should start with 0x7D, got 0x%02X", captured[0])
	}
	// Record must end with IAC EOR
	n := len(captured)
	if n < 2 || captured[n-2] != TelnetIAC || captured[n-1] != TelnetEOR {
		t.Errorf("ENTER record should end with IAC EOR, got %02X %02X",
			captured[n-2], captured[n-1])
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// mockConn is a minimal net.Conn for testing sendAid without a real socket.
type mockConn struct {
	writeFn func([]byte) (int, error)
}

func (m *mockConn) Write(b []byte) (int, error)         { return m.writeFn(b) }
func (m *mockConn) Read(b []byte) (int, error)          { return 0, nil }
func (m *mockConn) Close() error                        { return nil }
func (m *mockConn) LocalAddr() net.Addr                 { return nil }
func (m *mockConn) RemoteAddr() net.Addr                { return nil }
func (m *mockConn) SetDeadline(t time.Time) error       { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error   { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error  { return nil }
