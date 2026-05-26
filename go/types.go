package tn3270e

// types.go
// ─────────────────────────────────────────────────────────────────────────────
// All public types exposed by the library.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"crypto/tls"
	"io"
	"log"
)

// ── Session Options ────────────────────────────────────────────────────────

// SessionOptions configures a new TN3270E session.
type SessionOptions struct {
	// Host is the mainframe hostname or IP address.
	Host string

	// Port is the TCP port. Typical values: 23 (TN3270), 992 (TLS), 339 (alternate).
	Port int

	// LUName is the specific LU to request from VTAM.
	// Leave empty to accept any available LU from the pool.
	LUName string

	// Model is the terminal model string sent during negotiation.
	// Determines screen dimensions. Common values:
	//   "3278-2"  — 24×80  (most common, default)
	//   "3278-5"  — 27×132 (wide screen, SDSF/ISPF split)
	//   "IBM-DYNAMIC" — negotiate dimensions via Query
	Model string

	// Codepage is the EBCDIC code page number.
	// Common values: 37 (US/Canada), 500 (International).
	// Defaults to 37 if not set.
	Codepage int

	// UseTN3270E controls whether to attempt TN3270E enhanced negotiation.
	// Set to false for z/VM hosts or hosts that only support classic TN3270.
	// Defaults to true.
	UseTN3270E bool

	// UseTLS wraps the connection in TLS.
	UseTLS bool

	// TLSConfig is the TLS configuration when UseTLS is true.
	// If nil, uses default TLS settings.
	TLSConfig *tls.Config

	// SocketTimeoutSecs is the idle socket timeout in seconds.
	// Defaults to 120 seconds if not set.
	SocketTimeoutSecs int

	// Logger is used for debug/info/error output.
	// If nil, logging is suppressed.
	// Pass log.Default() or a custom *log.Logger for output.
	Logger *log.Logger

	// LogWriter is an alternative to Logger — if set, a logger will be
	// created writing to this writer. Ignored if Logger is set.
	LogWriter io.Writer

	// ID is an optional identifier included in log messages.
	// Useful when managing multiple sessions.
	ID string
}

// ── Screen Data ────────────────────────────────────────────────────────────

// ScreenData is delivered on the Screen channel every time the host
// sends a new screen (Write or Erase/Write command).
type ScreenData struct {
	// Rows and Cols are the screen dimensions.
	Rows int
	Cols int

	// Cursor is the linear buffer address of the cursor.
	Cursor int

	// LU is the negotiated LU name (empty if classic TN3270).
	LU string

	// Model is the terminal model in use.
	Model string

	// Buffer is the full screen buffer as a flat slice, row-major order.
	// Length = Rows × Cols. Access cell at row r, col c with Buffer[r*Cols+c].
	Buffer []Cell

	// Fields is the parsed list of 3270 fields on this screen.
	Fields []Field
}

// ── Screen Cell ────────────────────────────────────────────────────────────

// Cell represents a single position in the 3270 screen buffer.
type Cell struct {
	// Char is the ASCII character at this position.
	// Space for field attribute positions and unwritten cells.
	Char byte

	// IsFA is true if this position holds a field attribute byte.
	// FA positions display as blanks but control the following field.
	IsFA bool

	// FA is the field attribute byte if IsFA is true.
	FA byte

	// Protected is true if this cell is in a protected (read-only) field.
	Protected bool

	// Modified is true if the MDT (Modified Data Tag) bit is set.
	Modified bool

	// Color is the extended foreground color value (0 = default).
	Color byte

	// Highlight is the extended highlight value (0 = default).
	Highlight byte
}

// ── Field ──────────────────────────────────────────────────────────────────

// Field represents a 3270 screen field — a contiguous region of the buffer
// beginning at a field attribute (SF/SFE) position.
type Field struct {
	// StartAddr is the linear buffer address of the FA byte.
	StartAddr int

	// FA is the field attribute byte.
	FA byte

	// Protected indicates a read-only field.
	Protected bool

	// Numeric indicates a numeric-only input field.
	Numeric bool

	// Modified is true if the MDT bit is set (field has been changed).
	Modified bool

	// Value is the current ASCII content of the field.
	Value string

	// Length is the number of character positions in the field
	// (not counting the FA position itself).
	Length int
}

// ── Field Data (for SendAid) ───────────────────────────────────────────────

// FieldData carries field content to transmit with an AID key.
// Addr should be the linear buffer address of the first character
// position of the field (FA position + 1).
type FieldData struct {
	Addr  int
	Value string
}

// ── Session State ──────────────────────────────────────────────────────────

// SessionState represents the current connection lifecycle state.
type SessionState int

const (
	StateDisconnected  SessionState = iota // Not connected
	StateConnecting                        // TCP connection in progress
	StateNegotiating                       // Telnet/TN3270E negotiation in progress
	StateReady                             // Session live, screens arriving
	StateError                             // Unrecoverable error
)

// String returns a human-readable name for the state.
func (s SessionState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateNegotiating:
		return "Negotiating"
	case StateReady:
		return "Ready"
	case StateError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ── Events ─────────────────────────────────────────────────────────────────

// ConnectedEvent is sent on the Connected channel when the TCP socket opens.
type ConnectedEvent struct {
	Host string
	Port int
}

// ReadyEvent is sent on the Ready channel when TN3270E negotiation completes.
type ReadyEvent struct {
	LU    string // negotiated LU name
	Model string // negotiated terminal model
}

// DisconnectedEvent is sent on the Disconnected channel when the session ends.
type DisconnectedEvent struct {
	Reason string
}

// ErrorEvent is sent on the Errors channel when a protocol or socket error occurs.
type ErrorEvent struct {
	Err error
}
