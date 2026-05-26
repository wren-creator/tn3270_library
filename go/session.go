package tn3270e

// session.go
// ─────────────────────────────────────────────────────────────────────────────
// Session — the main public type.
//
// Usage:
//
//	sess := tn3270e.NewSession(tn3270e.SessionOptions{
//	    Host:       "10.1.1.1",
//	    Port:       23,
//	    Model:      "3278-2",
//	    UseTN3270E: true,
//	})
//
//	if err := sess.Connect(); err != nil {
//	    log.Fatal(err)
//	}
//	defer sess.Disconnect("done")
//
//	for {
//	    select {
//	    case screen := <-sess.Screen():
//	        if strings.Contains(sess.GetScreenText(), "READY") {
//	            sess.SendAid(tn3270e.AIDEnter)
//	        }
//	    case err := <-sess.Errors():
//	        log.Fatal(err)
//	    case ev := <-sess.Disconnected():
//	        fmt.Println("gone:", ev.Reason)
//	        return
//	    }
//	}
// ─────────────────────────────────────────────────────────────────────────────

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
)

// Session is a TN3270/TN3270E client session.
// Create one with NewSession; call Connect to open the connection.
// All screen updates arrive on the channel returned by Screen().
type Session struct {
	opts SessionOptions

	// internal protocol engine (unexported)
	engine *engine

	// channels — consumers read from these
	screenCh       chan ScreenData
	errorCh        chan ErrorEvent
	connectedCh    chan ConnectedEvent
	readyCh        chan ReadyEvent
	disconnectedCh chan DisconnectedEvent

	log *log.Logger
}

// NewSession creates a new session with the given options.
// Does not connect until Connect() is called.
func NewSession(opts SessionOptions) *Session {
	// Apply defaults
	if opts.Model == "" {
		opts.Model = "3278-2"
	}
	if opts.Codepage == 0 {
		opts.Codepage = 37
	}
	if opts.SocketTimeoutSecs == 0 {
		opts.SocketTimeoutSecs = 120
	}
	if opts.ID == "" {
		opts.ID = fmt.Sprintf("sess-%05x", rand.Intn(0xFFFFF))
	}

	// Set up logger
	var logger *log.Logger
	switch {
	case opts.Logger != nil:
		logger = opts.Logger
	case opts.LogWriter != nil:
		logger = log.New(opts.LogWriter, fmt.Sprintf("[%s] ", opts.ID), log.LstdFlags)
	default:
		logger = log.New(io.Discard, "", 0) // silent
	}

	s := &Session{
		opts:           opts,
		screenCh:       make(chan ScreenData, 8),
		errorCh:        make(chan ErrorEvent, 4),
		connectedCh:    make(chan ConnectedEvent, 1),
		readyCh:        make(chan ReadyEvent, 1),
		disconnectedCh: make(chan DisconnectedEvent, 1),
		log:            logger,
	}

	s.engine = newEngine(s)
	return s
}

// ── Channels (read-only for consumers) ────────────────────────────────────

// Screen returns a channel that delivers ScreenData every time the host
// sends a new screen. The channel is buffered (capacity 8).
func (s *Session) Screen() <-chan ScreenData {
	return s.screenCh
}

// Errors returns a channel that delivers ErrorEvents on protocol or
// socket errors. Always check this channel in your select loop.
func (s *Session) Errors() <-chan ErrorEvent {
	return s.errorCh
}

// Connected returns a channel that fires once when the TCP socket opens.
func (s *Session) Connected() <-chan ConnectedEvent {
	return s.connectedCh
}

// Ready returns a channel that fires once when TN3270E negotiation
// completes and the session is live.
func (s *Session) Ready() <-chan ReadyEvent {
	return s.readyCh
}

// Disconnected returns a channel that fires when the session ends.
func (s *Session) Disconnected() <-chan DisconnectedEvent {
	return s.disconnectedCh
}

// ── Lifecycle ──────────────────────────────────────────────────────────────

// Connect opens the TCP (or TLS) connection and begins Telnet negotiation.
// Returns immediately; negotiation and screen delivery happen asynchronously
// via the channels. The session is ready for input after the Ready() channel fires.
func (s *Session) Connect() error {
	return s.engine.connect()
}

// Disconnect closes the session with the given reason string.
// The reason is delivered on the Disconnected() channel.
func (s *Session) Disconnect(reason string) {
	s.engine.disconnect(reason)
}

// State returns the current session lifecycle state.
func (s *Session) State() SessionState {
	return s.engine.state()
}

// ── Sending Data ───────────────────────────────────────────────────────────

// SendAid transmits an AID key to the host with no field data.
// Use for keys like PF3, CLEAR, PA1 where no input is needed.
//
//	sess.SendAid(tn3270e.AIDPF3)
//	sess.SendAid(tn3270e.AIDClear)
func (s *Session) SendAid(aid byte) error {
	return s.engine.sendAid(aid, s.engine.cursorAddr, nil)
}

// SendAidWithFields transmits an AID key with modified field data.
// fields contains the field addresses and ASCII values to send.
//
//	sess.SendAidWithFields(tn3270e.AIDEnter, []tn3270e.FieldData{
//	    {Addr: 80, Value: "IBMUSER"},
//	})
func (s *Session) SendAidWithFields(aid byte, fields []FieldData) error {
	return s.engine.sendAid(aid, s.engine.cursorAddr, fields)
}

// SendAidAt transmits an AID key with a specific cursor address and field data.
func (s *Session) SendAidAt(aid byte, cursorAddr int, fields []FieldData) error {
	return s.engine.sendAid(aid, cursorAddr, fields)
}

// ── Reading the Screen ─────────────────────────────────────────────────────

// GetScreen returns the current screen as a flat buffer of Cells.
// Length = Rows × Cols. Access row r, col c with buffer[r*cols+c].
func (s *Session) GetScreen() []Cell {
	return s.engine.getScreen()
}

// GetScreenText returns the current screen as a plain string,
// rows separated by newlines. Useful for simple screen scraping.
//
//	if strings.Contains(sess.GetScreenText(), "ENTER USERID") {
//	    // on logon screen
//	}
func (s *Session) GetScreenText() string {
	return s.engine.getScreenText()
}

// GetFields returns all fields on the current screen.
func (s *Session) GetFields() []Field {
	return s.engine.getFields()
}

// GetFieldByIndex returns the field at the given index, or an error
// if the index is out of range.
func (s *Session) GetFieldByIndex(i int) (Field, error) {
	fields := s.engine.getFields()
	if i < 0 || i >= len(fields) {
		return Field{}, fmt.Errorf("field index %d out of range (0–%d)", i, len(fields)-1)
	}
	return fields[i], nil
}

// FindField returns the first field whose value contains the given substring,
// or an error if not found.
func (s *Session) FindField(substr string) (Field, error) {
	for _, f := range s.engine.getFields() {
		if strings.Contains(f.Value, substr) {
			return f, nil
		}
	}
	return Field{}, fmt.Errorf("no field containing %q found", substr)
}

// ── Cursor ─────────────────────────────────────────────────────────────────

// SetCursor sets the cursor to a linear buffer address.
func (s *Session) SetCursor(addr int) {
	s.engine.setCursor(addr)
}

// SetCursorRC sets the cursor by row and column (1-based).
func (s *Session) SetCursorRC(row, col int) {
	dims := ModelDims(s.opts.Model)
	addr := (row-1)*dims.Cols + (col - 1)
	s.engine.setCursor(addr)
}

// CursorAddr returns the current cursor buffer address.
func (s *Session) CursorAddr() int {
	return s.engine.cursorAddr
}

// ── Session Info ───────────────────────────────────────────────────────────

// LUName returns the negotiated LU name, or empty string for classic TN3270.
func (s *Session) LUName() string {
	return s.engine.negotiatedLU
}

// Model returns the terminal model in use.
func (s *Session) Model() string {
	return s.opts.Model
}

// Rows returns the screen row count.
func (s *Session) Rows() int {
	return ModelDims(s.opts.Model).Rows
}

// Cols returns the screen column count.
func (s *Session) Cols() int {
	return ModelDims(s.opts.Model).Cols
}

// RemoteAddr returns the remote address of the TCP connection,
// or nil if not connected.
func (s *Session) RemoteAddr() net.Addr {
	return s.engine.remoteAddr()
}
