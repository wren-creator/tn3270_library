package tn3270e

// engine.go
// ─────────────────────────────────────────────────────────────────────────────
// engine is the internal protocol implementation.
// The public Session type delegates all protocol work here.
//
// Protocol flow:
//   connect()
//     └── dial TCP/TLS
//         └── go readLoop()
//             └── parseTelnet()
//                 ├── handleOption()     — DO/DONT/WILL/WONT
//                 ├── handleSubneg()     — SB...SE sequences
//                 └── onRecord()         — complete 3270 data records
//                     └── parse3270()
//                         └── processOrders()
//                             └── emitScreen() → screenCh
// ─────────────────────────────────────────────────────────────────────────────

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// engine holds all internal protocol state.
type engine struct {
	sess      *Session   // back-reference for channel access and options
	conn      net.Conn
	mu        sync.Mutex // protects conn, currentState, buffer, cursorAddr
	destroyed bool

	// Session state
	currentState   SessionState
	negotiatedLU   string
	tn3270eEnabled bool

	// Screen buffer
	rows       int
	cols       int
	buffer     []cell
	cursorAddr int

	// Receive accumulation
	recvBuf       []byte
	currentRecord []byte
}

// cell is the internal screen buffer cell (unexported).
type cell struct {
	char      byte // EBCDIC character (0x40 = space)
	isFA      bool
	fa        byte
	color     byte
	highlight byte
	modified  bool
}

// newEngine creates a new engine for the given session.
func newEngine(s *Session) *engine {
	dims := ModelDims(s.opts.Model)
	e := &engine{
		sess:         s,
		currentState: StateDisconnected,
		rows:         dims.Rows,
		cols:         dims.Cols,
	}
	e.buffer = e.newBuffer()
	return e
}

func (e *engine) newBuffer() []cell {
	buf := make([]cell, e.rows*e.cols)
	for i := range buf {
		buf[i].char = 0x40 // EBCDIC space
	}
	return buf
}

// ── Lifecycle ──────────────────────────────────────────────────────────────

func (e *engine) connect() error {
	e.mu.Lock()
	if e.destroyed {
		e.mu.Unlock()
		return fmt.Errorf("session has been destroyed")
	}
	e.currentState = StateConnecting
	e.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", e.sess.opts.Host, e.sess.opts.Port)
	timeout := time.Duration(e.sess.opts.SocketTimeoutSecs) * time.Second

	var conn net.Conn
	var err error

	if e.sess.opts.UseTLS {
		tlsCfg := e.sess.opts.TLSConfig
		if tlsCfg == nil {
			tlsCfg = &tls.Config{}
		}
		conn, err = tls.DialWithDialer(
			&net.Dialer{Timeout: timeout},
			"tcp", addr, tlsCfg,
		)
	} else {
		conn, err = net.DialTimeout("tcp", addr, timeout)
	}

	if err != nil {
		e.setState(StateError)
		return fmt.Errorf("connect %s: %w", addr, err)
	}

	e.mu.Lock()
	e.conn = conn
	e.currentState = StateNegotiating
	e.mu.Unlock()

	select {
	case e.sess.connectedCh <- ConnectedEvent{Host: e.sess.opts.Host, Port: e.sess.opts.Port}:
	default:
	}

	e.sess.log.Printf("TCP connected to %s", addr)
	go e.readLoop()
	return nil
}

func (e *engine) disconnect(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.destroyed {
		return
	}
	e.destroyed = true

	if e.conn != nil {
		e.conn.Close()
		e.conn = nil
	}

	e.currentState = StateDisconnected
	e.sess.log.Printf("Disconnected: %s", reason)

	select {
	case e.sess.disconnectedCh <- DisconnectedEvent{Reason: reason}:
	default:
	}
}

func (e *engine) state() SessionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.currentState
}

func (e *engine) setState(s SessionState) {
	e.mu.Lock()
	e.currentState = s
	e.mu.Unlock()
}

func (e *engine) remoteAddr() net.Addr {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.conn != nil {
		return e.conn.RemoteAddr()
	}
	return nil
}

// ── Read loop ──────────────────────────────────────────────────────────────

func (e *engine) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := e.conn.Read(buf)
		if err != nil {
			if !e.destroyed {
				e.sess.errorCh <- ErrorEvent{Err: fmt.Errorf("read: %w", err)}
				e.disconnect("read-error")
			}
			return
		}
		e.recvBuf = append(e.recvBuf, buf[:n]...)
		e.parseTelnet()
	}
}

// ── Telnet stream parser ───────────────────────────────────────────────────
//
// Walks e.recvBuf byte by byte. The Telnet protocol embeds commands inside
// the data stream using IAC (0xFF) as an escape byte:
//
//   IAC IAC          → literal 0xFF data byte (escaped)
//   IAC EOR          → end of a 3270 data record
//   IAC SB ... IAC SE → subnegotiation payload
//   IAC DO/DONT/WILL/WONT <opt> → 3-byte option command
//
// Everything that is not a Telnet command is accumulated into currentRecord.
// When IAC EOR is seen, currentRecord is dispatched to onRecord().

func (e *engine) parseTelnet() {
	i := 0
	for i < len(e.recvBuf) {
		b := e.recvBuf[i]

		// ── Regular data byte ────────────────────────────────────────────
		if b != TelnetIAC {
			e.currentRecord = append(e.currentRecord, b)
			i++
			continue
		}

		// ── IAC — need at least one more byte ────────────────────────────
		if i+1 >= len(e.recvBuf) {
			break // wait for more data
		}
		cmd := e.recvBuf[i+1]

		// IAC IAC — escaped 0xFF data byte
		if cmd == TelnetIAC {
			e.currentRecord = append(e.currentRecord, TelnetIAC)
			i += 2
			continue
		}

		// IAC EOR — end of a complete 3270 record
		if cmd == TelnetEOR {
			if len(e.currentRecord) > 0 {
				record := make([]byte, len(e.currentRecord))
				copy(record, e.currentRecord)
				e.currentRecord = e.currentRecord[:0]
				e.onRecord(record)
			}
			i += 2
			continue
		}

		// IAC SB — subnegotiation: scan forward for IAC SE
		if cmd == TelnetSB {
			sePos := e.findSE(i + 2)
			if sePos < 0 {
				break // incomplete subneg — wait for more data
			}
			payload := make([]byte, sePos-(i+2))
			copy(payload, e.recvBuf[i+2:sePos])
			e.handleSubneg(payload)
			i = sePos + 2 // skip past IAC SE
			continue
		}

		// IAC DO/DONT/WILL/WONT <opt> — 3-byte sequence
		if cmd == TelnetDO || cmd == TelnetDONT ||
			cmd == TelnetWILL || cmd == TelnetWONT {
			if i+2 >= len(e.recvBuf) {
				break // wait for option byte
			}
			e.handleOption(cmd, e.recvBuf[i+2])
			i += 3
			continue
		}

		// Any other 2-byte IAC command (NOP, etc.) — skip
		i += 2
	}

	// Trim consumed bytes from recvBuf
	if i > 0 {
		e.recvBuf = e.recvBuf[i:]
	}
}

// findSE scans forward from start looking for the IAC SE sequence.
// Returns the index of the IAC byte, or -1 if not found yet.
func (e *engine) findSE(start int) int {
	for i := start; i < len(e.recvBuf)-1; i++ {
		if e.recvBuf[i] == TelnetIAC && e.recvBuf[i+1] == TelnetSE {
			return i
		}
	}
	return -1
}

// ── Telnet option negotiation ──────────────────────────────────────────────
//
// The host initiates negotiation by sending DO/WILL for each option it wants.
// We respond symmetrically. The critical options for TN3270:
//
//   BINARY (0x00) — suppresses Telnet CR/LF processing; required for raw 3270
//   EOR    (0x19) — enables IAC EOR as record delimiter; required for 3270
//   TTYPE  (0x18) — terminal type; host asks our model string
//   TN3270E(0x28) — enhanced protocol with LU binding (RFC 2355)

func (e *engine) handleOption(cmd, opt byte) {
	e.sess.log.Printf("Telnet %s %s", telnetCmdName(cmd), telnetOptName(opt))

	switch opt {

	case OptTN3270E:
		if cmd == TelnetDO {
			if !e.sess.opts.UseTN3270E {
				// TN3270E disabled — refuse and fall back to classic
				e.sess.log.Printf("TN3270E disabled — sending WONT")
				e.sendRaw([]byte{TelnetIAC, TelnetWONT, OptTN3270E})
				e.initClassicTN3270()
			} else {
				e.tn3270eEnabled = true
				e.sendRaw([]byte{TelnetIAC, TelnetWILL, OptTN3270E})
				// Host will now send SB TN3270E SEND DEVICE-TYPE
			}
		} else if cmd == TelnetDONT {
			// Host doesn't support TN3270E — fall back
			e.sendRaw([]byte{TelnetIAC, TelnetWONT, OptTN3270E})
			e.initClassicTN3270()
		}

	case OptTTYPE:
		// Used in classic TN3270 mode. Host sends SB TTYPE SEND;
		// we respond in handleSubneg with our model string.
		if cmd == TelnetDO {
			e.sendRaw([]byte{TelnetIAC, TelnetWILL, OptTTYPE})
		}

	case OptBinary:
		// Required: suppresses Telnet special character handling
		if cmd == TelnetDO {
			e.sendRaw([]byte{TelnetIAC, TelnetWILL, OptBinary})
		} else if cmd == TelnetWILL {
			e.sendRaw([]byte{TelnetIAC, TelnetDO, OptBinary})
		}

	case OptEOR:
		// Required: enables IAC EOR as record delimiter
		if cmd == TelnetDO {
			e.sendRaw([]byte{TelnetIAC, TelnetWILL, OptEOR})
		} else if cmd == TelnetWILL {
			e.sendRaw([]byte{TelnetIAC, TelnetDO, OptEOR})
		}

	default:
		// Unknown option — refuse politely
		if cmd == TelnetDO {
			e.sendRaw([]byte{TelnetIAC, TelnetWONT, opt})
		} else if cmd == TelnetWILL {
			e.sendRaw([]byte{TelnetIAC, TelnetDONT, opt})
		}
	}
}

// initClassicTN3270 sends the BINARY+EOR requests needed for classic TN3270
// (no TN3270E). Called when TN3270E is disabled or the host refuses it.
func (e *engine) initClassicTN3270() {
	e.sess.log.Printf("Classic TN3270 mode")
	e.sendRaw([]byte{TelnetIAC, TelnetDO, OptBinary})
	e.sendRaw([]byte{TelnetIAC, TelnetDO, OptEOR})
}

// ── TN3270E sub-negotiation ────────────────────────────────────────────────
//
// After WILL TN3270E is exchanged, the host drives a structured handshake
// via Telnet subnegotiations (SB...SE):
//
//   Host → Client: SB TN3270E SEND DEVICE-TYPE SE
//   Client → Host: SB TN3270E DEVICE-TYPE REQUEST "IBM-3278-2" [CONNECT "LU"] SE
//   Host → Client: SB TN3270E DEVICE-TYPE IS "IBM-3278-2" CONNECT "LU3A0042" SE
//   Host → Client: SB TN3270E FUNCTIONS REQUEST [func-list] SE
//   Client → Host: SB TN3270E FUNCTIONS IS [func-list] SE
//   ← session is now live; 3270 screens start arriving

func (e *engine) handleSubneg(payload []byte) {
	if len(payload) < 2 {
		return
	}
	opt := payload[0]
	fn := payload[1]

	// ── TTYPE subneg (classic TN3270 fallback) ────────────────────────────
	// Host sends SB TTYPE SEND SE; we reply with our model string.
	if opt == OptTTYPE {
		model := fmt.Sprintf("IBM-%s", e.sess.opts.Model)
		var buf bytes.Buffer
		buf.Write([]byte{TelnetIAC, TelnetSB, OptTTYPE, 0x00}) // 0x00 = IS
		buf.WriteString(model)
		buf.Write([]byte{TelnetIAC, TelnetSE})
		e.sendRaw(buf.Bytes())
		e.sess.log.Printf("TTYPE IS %s", model)
		return
	}

	if opt != OptTN3270E {
		return
	}

	// ── TN3270E SEND DEVICE-TYPE ──────────────────────────────────────────
	if fn == TN3ESend && len(payload) > 2 && payload[2] == TN3EDeviceType {
		model := fmt.Sprintf("IBM-%s", e.sess.opts.Model)
		var buf bytes.Buffer
		buf.Write([]byte{TelnetIAC, TelnetSB, OptTN3270E, TN3EDeviceType, TN3ERequest})
		buf.WriteString(model)
		if e.sess.opts.LUName != "" {
			buf.WriteByte(TN3EConnect)
			buf.WriteString(e.sess.opts.LUName)
		}
		buf.Write([]byte{TelnetIAC, TelnetSE})
		e.sendRaw(buf.Bytes())
		e.sess.log.Printf("TN3270E DEVICE-TYPE REQUEST %s LU=%q",
			model, e.sess.opts.LUName)
		return
	}

	// ── TN3270E DEVICE-TYPE IS ────────────────────────────────────────────
	// Host confirms device type and tells us our assigned LU name.
	if fn == TN3EDeviceType && len(payload) > 2 && payload[2] == TN3EIs {
		rest := payload[3:]
		// Find CONNECT marker to extract LU name
		if idx := bytes.IndexByte(rest, TN3EConnect); idx >= 0 {
			e.negotiatedLU = string(rest[idx+1:])
			e.sess.log.Printf("TN3270E: LU bound = %s", e.negotiatedLU)
		}
		return
	}

	// ── TN3270E DEVICE-TYPE REJECT ────────────────────────────────────────
	if fn == TN3EDeviceType && len(payload) > 2 && payload[2] == TN3EReject {
		reasonCode := byte(TN3EReason)
		if idx := bytes.IndexByte(payload, TN3EReason); idx >= 0 && idx+1 < len(payload) {
			reasonCode = payload[idx+1]
		}
		err := fmt.Errorf("TN3270E device-type rejected (reason 0x%02X)", reasonCode)
		e.sess.log.Printf("ERROR: %v", err)
		select {
		case e.sess.errorCh <- ErrorEvent{Err: err}:
		default:
		}
		return
	}

	// ── TN3270E FUNCTIONS REQUEST ─────────────────────────────────────────
	// Host tells us which TN3270E functions it wants. We echo back what we
	// support from that list. After this exchange the session is live.
	if fn == TN3EFunctions && len(payload) > 2 && payload[2] == TN3ERequest {
		funcList := payload[3:]
		var buf bytes.Buffer
		buf.Write([]byte{TelnetIAC, TelnetSB, OptTN3270E, TN3EFunctions, TN3EIs})
		buf.Write(funcList) // echo back the requested functions
		buf.Write([]byte{TelnetIAC, TelnetSE})
		e.sendRaw(buf.Bytes())

		e.setState(StateReady)
		e.sess.log.Printf("TN3270E FUNCTIONS IS — session live, LU=%s", e.negotiatedLU)

		select {
		case e.sess.readyCh <- ReadyEvent{LU: e.negotiatedLU, Model: e.sess.opts.Model}:
		default:
		}
		return
	}
}

// ── 3270 Record handler ────────────────────────────────────────────────────

// onRecord is called with a complete, IAC-unescaped 3270 data record.
// In TN3270E mode it strips the 5-byte header first.
func (e *engine) onRecord(record []byte) {
	data := record

	if e.tn3270eEnabled {
		if len(record) < 5 {
			return
		}
		dataType := record[0]
		// We handle DATA-3270 (0x00) and SSCP-LU (0x07) — both are
		// rendered as 3270 datastreams. Everything else is skipped.
		if dataType != DataType3270 && dataType != DataTypeSSCPLU {
			e.sess.log.Printf("TN3270E: skipping record type 0x%02X", dataType)
			return
		}
		data = record[5:]
	}

	if len(data) == 0 {
		return
	}

	e.parse3270(data)
}

// ── 3270 Datastream parser ─────────────────────────────────────────────────

// parse3270 dispatches on the first byte (the 3270 command byte).
func (e *engine) parse3270(data []byte) {
	if len(data) < 1 {
		return
	}
	cmd := data[0]
	e.sess.log.Printf("3270 cmd=0x%02X len=%d", cmd, len(data))

	switch cmd {

	case CmdEraseWrite, CmdEraseWriteAlt:
		// Erase/Write — clear the screen buffer first, then render
		e.mu.Lock()
		e.buffer = e.newBuffer()
		e.cursorAddr = 0
		e.mu.Unlock()
		fallthrough

	case CmdWrite:
		// Write — render data into the buffer
		if len(data) < 2 {
			return
		}
		wcc := data[1]
		if wcc&WCCReset != 0 {
			e.resetMDT()
		}
		e.processOrders(data, 2)
		e.emitScreen()

	case CmdEraseAllUnprotect:
		e.eraseUnprotected()
		e.emitScreen()

	case CmdWriteStructured:
		// Emit raw for consumers that need to handle Query/Reply etc.
		// No screen update.
		e.sess.log.Printf("WriteStructuredField — %d bytes", len(data)-1)

	default:
		e.sess.log.Printf("Unknown 3270 command 0x%02X", cmd)
	}
}

// ── Order processor ────────────────────────────────────────────────────────
//
// Orders are control codes embedded in the Write data (values 0x01–0x3F,
// which are non-printable in EBCDIC so they can't be confused with text).
// They direct the buffer address pointer and set field attributes.

func (e *engine) processOrders(data []byte, start int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	bufAddr := 0
	bufSize := len(e.buffer)
	i := start

	for i < len(data) {
		b := data[i]

		switch b {

		// ── Set Buffer Address (SBA) ─────────────────────────────────────
		// Moves the buffer pointer. Takes 2 address bytes.
		case OrderSBA:
			if i+2 >= len(data) {
				return
			}
			bufAddr = decodeAddr(data[i+1], data[i+2]) % bufSize
			i += 3

		// ── Start Field (SF) ─────────────────────────────────────────────
		// Places a field attribute at the current position.
		// The FA byte encodes protected/numeric/intensity/MDT.
		case OrderSF:
			if i+1 >= len(data) {
				return
			}
			if bufAddr < bufSize {
				e.buffer[bufAddr].isFA = true
				e.buffer[bufAddr].fa = data[i+1]
				e.buffer[bufAddr].char = 0x40
				e.buffer[bufAddr].color = 0
				e.buffer[bufAddr].highlight = 0
			}
			bufAddr = (bufAddr + 1) % bufSize
			i += 2

		// ── Start Field Extended (SFE) ───────────────────────────────────
		// Like SF but with additional attribute pairs for color, highlight, etc.
		case OrderSFE:
			if i+1 >= len(data) {
				return
			}
			pairCount := int(data[i+1])
			var fa, color, highlight byte
			for p := 0; p < pairCount; p++ {
				base := i + 2 + p*2
				if base+1 >= len(data) {
					break
				}
				attrType := data[base]
				attrVal := data[base+1]
				switch attrType {
				case ExtAttrForeground:
					color = attrVal
				case ExtAttrHighlight:
					highlight = attrVal
				case 0xC0: // basic 3270 field attribute
					fa = attrVal
				}
			}
			if bufAddr < bufSize {
				e.buffer[bufAddr].isFA = true
				e.buffer[bufAddr].fa = fa
				e.buffer[bufAddr].char = 0x40
				e.buffer[bufAddr].color = color
				e.buffer[bufAddr].highlight = highlight
			}
			bufAddr = (bufAddr + 1) % bufSize
			i += 2 + pairCount*2

		// ── Insert Cursor (IC) ───────────────────────────────────────────
		// Marks cursor position at current buffer address.
		// If multiple IC orders appear, the last one wins.
		case OrderIC:
			e.cursorAddr = bufAddr
			i++

		// ── Repeat to Address (RA) ───────────────────────────────────────
		// Fills buffer positions from current address to destination with
		// a single repeated character. Efficient for separator lines.
		case OrderRA:
			if i+3 >= len(data) {
				return
			}
			toAddr := decodeAddr(data[i+1], data[i+2]) % bufSize
			fillChar := data[i+3]
			for bufAddr != toAddr {
				if bufAddr < bufSize {
					e.buffer[bufAddr].char = fillChar
					e.buffer[bufAddr].isFA = false
				}
				bufAddr = (bufAddr + 1) % bufSize
			}
			i += 4

		// ── Erase Unprotected to Address (EUA) ──────────────────────────
		// Clears unprotected cells from current address to destination.
		case OrderEUA:
			if i+2 >= len(data) {
				return
			}
			toAddr := decodeAddr(data[i+1], data[i+2]) % bufSize
			for bufAddr != toAddr {
				c := &e.buffer[bufAddr%bufSize]
				if !c.isFA && (c.fa&FAProtected == 0) {
					c.char = 0x40
				}
				bufAddr = (bufAddr + 1) % bufSize
			}
			i += 3

		// ── Set Attribute (SA) / Modify Field (MF) ──────────────────────
		// Extended attribute changes — skip the type+value pair(s).
		case OrderSA:
			i += 3 // [SA, type, value]

		case OrderMF:
			if i+1 >= len(data) {
				return
			}
			pairCount := int(data[i+1])
			i += 2 + pairCount*2

		// ── Program Tab (PT) ─────────────────────────────────────────────
		// Advance to first character of next unprotected field.
		case OrderPT:
			for bufAddr < bufSize {
				c := &e.buffer[bufAddr]
				if c.isFA && (c.fa&FAProtected == 0) {
					bufAddr = (bufAddr + 1) % bufSize
					break
				}
				bufAddr++
			}
			i++

		// ── Graphic Escape (GE) ──────────────────────────────────────────
		// Next byte is from the alternate graphics character set — skip it.
		case OrderGE:
			i += 2

		default:
			// Printable EBCDIC character — write to buffer
			if bufAddr < bufSize {
				e.buffer[bufAddr].char = b
				e.buffer[bufAddr].isFA = false
			}
			bufAddr = (bufAddr + 1) % bufSize
			i++
		}
	}
}

// ── Screen buffer helpers ──────────────────────────────────────────────────

// resetMDT clears the Modified Data Tag bit on every cell.
// Called when the WCC has the Reset bit set.
func (e *engine) resetMDT() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.buffer {
		e.buffer[i].modified = false
	}
}

// eraseUnprotected clears all unprotected (input) field cells to EBCDIC space.
func (e *engine) eraseUnprotected() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Walk the buffer tracking the current field's protection status.
	// When we hit an FA, note its protected bit; apply to following cells.
	protected := true // before first field, treat as protected
	for i := range e.buffer {
		c := &e.buffer[i]
		if c.isFA {
			protected = c.fa&FAProtected != 0
		} else if !protected {
			c.char = 0x40
			c.modified = false
		}
	}
}

func (e *engine) setCursor(addr int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	max := e.rows*e.cols - 1
	if addr < 0 {
		addr = 0
	}
	if addr > max {
		addr = max
	}
	e.cursorAddr = addr
}

// emitScreen builds ScreenData from the current buffer and sends it
// on the screen channel. If the channel is full, the oldest entry is dropped.
func (e *engine) emitScreen() {
	screen := e.getScreen()
	fields := e.getFields()

	data := ScreenData{
		Rows:   e.rows,
		Cols:   e.cols,
		Cursor: e.cursorAddr,
		LU:     e.negotiatedLU,
		Model:  e.sess.opts.Model,
		Buffer: screen,
		Fields: fields,
	}

	select {
	case e.sess.screenCh <- data:
	default:
		select {
		case <-e.sess.screenCh:
		default:
		}
		e.sess.screenCh <- data
	}
}

// ── Screen reading ─────────────────────────────────────────────────────────

// getScreen converts the internal EBCDIC buffer to the public Cell slice.
func (e *engine) getScreen() []Cell {
	e.mu.Lock()
	defer e.mu.Unlock()

	cp := e.sess.opts.Codepage
	out := make([]Cell, len(e.buffer))

	for i, c := range e.buffer {
		var ch byte = ' '
		if !c.isFA {
			s := EBCDICToASCII([]byte{c.char}, cp)
			if len(s) > 0 {
				ch = s[0]
			}
		}
		out[i] = Cell{
			Char:      ch,
			IsFA:      c.isFA,
			FA:        c.fa,
			Protected: c.isFA && (c.fa&FAProtected != 0),
			Modified:  c.modified,
			Color:     c.color,
			Highlight: c.highlight,
		}
	}
	return out
}

// getFields walks the buffer and returns all 3270 fields.
// A field begins at each FA position and ends just before the next FA.
// The buffer is circular — a field can wrap from the last position back to 0.
func (e *engine) getFields() []Field {
	e.mu.Lock()
	defer e.mu.Unlock()

	cp := e.sess.opts.Codepage
	var fields []Field
	var current *Field

	for i, c := range e.buffer {
		if c.isFA {
			// Finalise previous field
			if current != nil {
				current.Length = i - current.StartAddr - 1
				fields = append(fields, *current)
			}
			// Start new field
			current = &Field{
				StartAddr: i,
				FA:        c.fa,
				Protected: c.fa&FAProtected != 0,
				Numeric:   c.fa&FANumeric != 0,
				Modified:  c.fa&FAMDT != 0,
				Value:     "",
			}
		} else if current != nil {
			s := EBCDICToASCII([]byte{c.char}, cp)
			if len(s) > 0 {
				current.Value += string(s[0])
			}
		}
	}

	// Close the last field (wraps to end of buffer)
	if current != nil {
		current.Length = len(e.buffer) - current.StartAddr - 1
		fields = append(fields, *current)
	}

	return fields
}

// getScreenText returns the screen as a plain ASCII string, rows separated by newlines.
func (e *engine) getScreenText() string {
	screen := e.getScreen()
	var sb strings.Builder
	for r := 0; r < e.rows; r++ {
		for c := 0; c < e.cols; c++ {
			sb.WriteByte(screen[r*e.cols+c].Char)
		}
		if r < e.rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ── AID encoding ───────────────────────────────────────────────────────────
//
// Inbound (terminal → host) record format:
//
//   [AID] [cursor-hi] [cursor-lo] [SBA field-hi field-lo field-data...] IAC EOR
//
// PA keys send only AID + cursor (no field data).
// CLEAR sends only AID (no cursor, no data).
// All other keys send AID + cursor + SBA blocks for each modified field.

func (e *engine) sendAid(aid byte, cursorAddr int, fields []FieldData) error {
	cp := e.sess.opts.Codepage
	var buf bytes.Buffer

	// TN3270E 5-byte header: [DATA-TYPE=0x00, REQUEST=0x00, RESPONSE=0x00, SEQ-HI=0x00, SEQ-LO=0x00]
	if e.tn3270eEnabled {
		buf.Write([]byte{DataType3270, 0x00, 0x00, 0x00, 0x00})
	}

	// AID byte
	buf.WriteByte(aid)

	// CLEAR sends no cursor or field data
	if aid == AIDClear {
		buf.Write([]byte{TelnetIAC, TelnetEOR})
		return e.send(buf.Bytes())
	}

	// Cursor address (2 bytes, 3270 encoded)
	curHi, curLo := encodeAddr(cursorAddr)
	buf.Write([]byte{curHi, curLo})

	// PA keys send only AID + cursor, no field data
	if aid == AIDPA1 || aid == AIDPA2 || aid == AIDPA3 {
		buf.Write([]byte{TelnetIAC, TelnetEOR})
		return e.send(buf.Bytes())
	}

	// Field data: SBA + EBCDIC content for each modified field
	for _, fd := range fields {
		fHi, fLo := encodeAddr(fd.Addr)
		buf.Write([]byte{OrderSBA, fHi, fLo})
		ebcdic := ASCIIToEBCDIC(fd.Value, cp)
		// IAC-escape any 0xFF bytes in the EBCDIC data
		for _, b := range ebcdic {
			buf.WriteByte(b)
			if b == TelnetIAC {
				buf.WriteByte(TelnetIAC) // escape
			}
		}
	}

	buf.Write([]byte{TelnetIAC, TelnetEOR})
	return e.send(buf.Bytes())
}

// ── Wire I/O ───────────────────────────────────────────────────────────────

// sendRaw writes bytes directly to the TCP connection.
func (e *engine) sendRaw(data []byte) {
	if err := e.send(data); err != nil {
		e.sess.log.Printf("sendRaw error: %v", err)
	}
}

// send writes bytes to the TCP connection (thread-safe).
func (e *engine) send(data []byte) error {
	e.mu.Lock()
	conn := e.conn
	e.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}
	_, err := conn.Write(data)
	return err
}

// ── Address encoding ───────────────────────────────────────────────────────

// decodeAddr decodes a 2-byte 3270 buffer address into a linear offset.
// 3270 uses a non-linear 6-bit encoding; the top 2 bits signal the format.
func decodeAddr(b1, b2 byte) int {
	typ := (b1 & 0xC0) >> 6
	if typ == 0x00 {
		// 14-bit binary
		return (int(b1&0x3F) << 8) | int(b2)
	}
	// 12-bit code table
	return (int(b1&0x3F) << 6) | int(b2&0x3F)
}

// encodeAddr encodes a linear buffer address into two 3270 address bytes.
func encodeAddr(addr int) (byte, byte) {
	return BufAddrCode[(addr>>6)&0x3F], BufAddrCode[addr&0x3F]
}

// ── Debug helpers ──────────────────────────────────────────────────────────

func telnetCmdName(b byte) string {
	switch b {
	case TelnetDO:
		return "DO"
	case TelnetDONT:
		return "DONT"
	case TelnetWILL:
		return "WILL"
	case TelnetWONT:
		return "WONT"
	}
	return fmt.Sprintf("0x%02X", b)
}

func telnetOptName(b byte) string {
	switch b {
	case OptBinary:
		return "BINARY"
	case OptEOR:
		return "EOR"
	case OptTTYPE:
		return "TTYPE"
	case OptTN3270E:
		return "TN3270E"
	}
	return fmt.Sprintf("0x%02X", b)
}
