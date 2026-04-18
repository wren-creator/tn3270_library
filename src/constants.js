'use strict';

/**
 * src/constants.js
 * ─────────────────────────────────────────────────────────────────────────────
 * All TN3270 / TN3270E / VTAM protocol constants in one place.
 *
 * Protocol references:
 *   RFC 1576  — TN3270 (original)
 *   RFC 2355  — TN3270E (enhanced, LU binding, data types)
 *   IBM GA23-0059 — 3270 Data Stream Programmer's Reference
 *   IBM SC31-6082  — SNA Formats (LU/PU types, RU categories)
 * ─────────────────────────────────────────────────────────────────────────────
 */

// ── Telnet Command Bytes ───────────────────────────────────────────────────
// These are the standard Telnet control bytes defined in RFC 854.
// In TN3270 they appear in the byte stream prefixed by IAC (0xFF).
const TELNET = Object.freeze({
  IAC:  0xFF,   // Interpret As Command  — begins a Telnet command sequence
  DONT: 0xFE,   // DONT <option>         — refuse/stop an option
  DO:   0xFD,   // DO <option>           — request/agree to an option
  WONT: 0xFC,   // WONT <option>         — refuse to enable an option
  WILL: 0xFB,   // WILL <option>         — offer/agree to enable an option
  SB:   0xFA,   // SB <option> ...       — begin subnegotiation
  SE:   0xF0,   // SE                    — end subnegotiation
  EOR:  0xEF,   // EOR                   — End Of Record (marks end of 3270 datastream)
  NOP:  0xF1,   // NOP                   — no-operation (keepalive)
});

// ── Telnet Option Codes ────────────────────────────────────────────────────
// These follow a DO/DONT/WILL/WONT command byte to identify which option.
const OPT = Object.freeze({
  BINARY:   0x00,  // RFC 856  — binary transmission (required for 3270)
  ECHO:     0x01,  // RFC 857  — echo (usually DONT for 3270)
  EOR:      0x19,  // RFC 885  — end-of-record transmission (required for 3270)
  TTYPE:    0x18,  // RFC 1091 — terminal type (host asks for model string)
  TN3270E:  0x28,  // RFC 2355 — TN3270E extended protocol
});

// ── TN3270E Sub-negotiation Function Codes ────────────────────────────────
// These appear inside SB TN3270E ... SE sequences.
// They drive the TN3270E connection handshake:
//   Host sends SEND DEVICE-TYPE
//   Client responds with DEVICE-TYPE REQUEST <type> CONNECT <lu>
//   Host responds with DEVICE-TYPE IS <type> CONNECT <lu>
//   Host sends FUNCTIONS REQUEST <list>
//   Client responds with FUNCTIONS IS <list>  ← session is now live
const TN3E = Object.freeze({
  ASSOCIATE:    0x00,  // (rarely used) bind a printer LU to a display LU
  CONNECT:      0x01,  // subcommand: name of LU to connect to
  DEVICE_TYPE:  0x02,  // DEVICE-TYPE subnegotiation
  FUNCTIONS:    0x03,  // FUNCTIONS negotiation (what TN3270E features to use)
  IS:           0x04,  // "IS" — the answering half of REQUEST/IS pairs
  REASON:       0x05,  // error reason code (follows REJECT)
  REJECT:       0x06,  // host rejects our device-type or LU request
  REQUEST:      0x07,  // "REQUEST" — the asking half of REQUEST/IS pairs
  SEND:         0x08,  // host asks us to send something (e.g. SEND DEVICE-TYPE)
});

// ── TN3270E Reason Codes ──────────────────────────────────────────────────
// These follow a REJECT REASON byte to explain why the host refused.
const TN3E_REASON = Object.freeze({
  CONN_PARTNER:      0x00,  // connected to partner LU
  DEVICE_IN_USE:     0x01,  // LU already has a session
  INV_ASSOCIATE:     0x02,  // invalid ASSOCIATE request
  INV_NAME:          0x03,  // invalid LU or device-type name
  INV_DEVICE_TYPE:   0x04,  // device type not supported
  TYPE_NAME_ERROR:   0x05,  // device type + name combination invalid
  UNKNOWN_ERROR:     0x06,  // unspecified error
  UNSUPPORTED_REQ:   0x07,  // function requested is not supported
});

// ── TN3270E Data Type Codes ────────────────────────────────────────────────
// The TN3270E header prepends a 5-byte header to every data record:
//   [DATA-TYPE] [REQUEST] [RESPONSE] [SEQ-NUM-HIGH] [SEQ-NUM-LOW]
const TN3E_DATATYPE = Object.freeze({
  DATA_3270:  0x00,  // standard 3270 datastream
  BIND_IMAGE: 0x03,  // SNA BIND image (LU-LU session parameters)
  NVT_DATA:   0x05,  // NVT (Network Virtual Terminal) data — not 3270
  REQUEST:    0x06,  // control request
  RESPONSE:   0x02,  // positive/negative response
  SCS_DATA:   0x01,  // SNA Character String (printer data)
  SSCP_LU:    0x07,  // SSCP-LU session data (logon screen before BIND)
  UNBIND:     0x04,  // LU-LU session ended
});

// ── 3270 Write Commands ────────────────────────────────────────────────────
// These are the first byte of a 3270 outbound (host→terminal) datastream.
const CMD = Object.freeze({
  WRITE:           0xF1,  // Write — update screen, reset MDT bits
  ERASE_WRITE:     0xF5,  // Erase/Write — clear screen first, then write
  ERASE_WRITE_ALT: 0x7E,  // Erase/Write Alternate — use alternate screen size
  READ_BUFFER:     0xF2,  // Read Buffer — send entire screen buffer to host
  READ_MODIFIED:   0xF6,  // Read Modified — send only modified fields
  READ_MODIFIED_ALL: 0x6E, // Read Modified All — send modified + ENTER fields
  ERASE_ALL_UNPROTECTED: 0x6F, // Erase All Unprotected — clear all input fields
  WRITE_STRUCTURED_FIELD: 0xF3, // Write Structured Field — extended features
});

// ── 3270 Datastream Orders ─────────────────────────────────────────────────
// Orders appear within the data portion of a Write command to control
// how the host positions and formats data in the screen buffer.
const ORDER = Object.freeze({
  SF:  0x1D,  // Start Field — begins a field, sets field attributes
  SFE: 0x29,  // Start Field Extended — SF with extended attribute pairs
  SBA: 0x11,  // Set Buffer Address — moves the buffer address pointer
  SA:  0x28,  // Set Attribute — changes an attribute without starting a field
  MF:  0x2C,  // Modify Field — modifies attributes of the current field
  IC:  0x13,  // Insert Cursor — places cursor at the current buffer position
  PT:  0x05,  // Program Tab — advance to next unprotected field
  RA:  0x3C,  // Repeat to Address — fill a range of buffer positions with one character
  EUA: 0x12,  // Erase Unprotected to Address — clear unprotected cells to address
  GE:  0x08,  // Graphic Escape — next byte is a PS/2 graphic character set char
});

// ── 3270 AID (Attention Identifier) Bytes ────────────────────────────────
// These are the first byte of an inbound (terminal→host) datastream.
// They tell the host what key the operator pressed.
const AID = Object.freeze({
  NONE:   0x60,  // No AID — used in Read Buffer responses with no key press
  ENTER:  0x7D,  // Enter key
  CLEAR:  0x6D,  // Clear key — erases screen, sends short AID record
  PA1:    0x6C,  // Program Attention 1 — sends only AID, no field data
  PA2:    0x6E,  // Program Attention 2
  PA3:    0x6B,  // Program Attention 3
  SYSREQ: 0xF0,  // System Request — SSCP-LU session attention
  // PF1–PF24 — Program Function keys
  PF1:  0xF1, PF2:  0xF2, PF3:  0xF3, PF4:  0xF4,
  PF5:  0xF5, PF6:  0xF6, PF7:  0xF7, PF8:  0xF8,
  PF9:  0xF9, PF10: 0x7A, PF11: 0x7B, PF12: 0x7C,
  PF13: 0xC1, PF14: 0xC2, PF15: 0xC3, PF16: 0xC4,
  PF17: 0xC5, PF18: 0xC6, PF19: 0xC7, PF20: 0xC8,
  PF21: 0xC9, PF22: 0x4A, PF23: 0x4B, PF24: 0x4C,
});

// ── Field Attribute Bits ───────────────────────────────────────────────────
// The FA byte following an SF order encodes field properties.
// The FA byte is itself EBCDIC-encoded (in the range 0x40–0x7F and 0xC0–0xFF).
// Bit positions refer to the decoded attribute byte before EBCDIC encoding.
const FA = Object.freeze({
  PROTECTED:   0x20,  // Bit 5 — field is protected (read-only)
  NUMERIC:     0x10,  // Bit 4 — field accepts numeric input only
  MDT:         0x01,  // Bit 0 — Modified Data Tag — set when field is changed

  // Bits 3–2: display intensity / selector pen
  INTENSITY_NORMAL:     0x00,  // normal display
  INTENSITY_HIGH:       0x08,  // high intensity
  INTENSITY_NONDISPLAY: 0x0C,  // non-display (password fields)
  INTENSITY_MASK:       0x0C,  // mask to extract intensity bits
});

// ── Extended Attribute Types ───────────────────────────────────────────────
// Used in SFE and SA orders as [type, value] pairs.
const EXT_ATTR = Object.freeze({
  ALL_CHARS:        0x00,  // reset all extended attributes
  FIELD_OUTLINE:    0x60,  // field outline (box drawing)
  FOREGROUND_COLOR: 0x42,  // foreground color (see COLOR below)
  BACKGROUND_COLOR: 0x45,  // background color
  HIGHLIGHTING:     0x41,  // highlight type (see HIGHLIGHT below)
  CHAR_SET:         0x43,  // character set (alternate character sets)
  TRANSPARENCY:     0x46,  // transparency
  VALIDATION:       0xC0,  // field validation (fill, mandatory fill, trigger)
});

// ── 3270 Color Values ──────────────────────────────────────────────────────
// Used with EXT_ATTR.FOREGROUND_COLOR and EXT_ATTR.BACKGROUND_COLOR
const COLOR = Object.freeze({
  DEFAULT:      0x00,  // use terminal default
  BLUE:         0xF1,
  RED:          0xF2,
  PINK:         0xF3,
  GREEN:        0xF4,
  TURQUOISE:    0xF5,
  YELLOW:       0xF6,
  WHITE:        0xF7,
  BLACK:        0xF8,
  DEEP_BLUE:    0xF9,
  ORANGE:       0xFA,
  PURPLE:       0xFB,
  PALE_GREEN:   0xFC,
  PALE_TURQUOISE: 0xFD,
  GREY:         0xFE,
  WHITE2:       0xFF,
});

// ── 3270 Highlight Values ──────────────────────────────────────────────────
// Used with EXT_ATTR.HIGHLIGHTING
const HIGHLIGHT = Object.freeze({
  DEFAULT:   0x00,  // no highlight
  BLINK:     0xF1,  // blinking
  REVERSE:   0xF2,  // reverse video
  UNDERSCORE: 0xF4, // underline
  INTENSITY: 0xF8,  // intensify
});

// ── 3270 Screen Model Dimensions ──────────────────────────────────────────
// The model string (sent during TTYPE negotiation) determines screen size.
// These correspond to the physical 3270 terminal models.
const MODEL_DIMENSIONS = Object.freeze({
  '3278-2':     { rows: 24,  cols: 80  },  // Most common — standard 80x24
  '3278-3':     { rows: 32,  cols: 80  },  // 32x80
  '3278-4':     { rows: 43,  cols: 80  },  // 43x80
  '3278-5':     { rows: 27,  cols: 132 },  // 132-column wide screen
  'IBM-3278-2': { rows: 24,  cols: 80  },  // Alternate naming format
  'IBM-3278-5': { rows: 27,  cols: 132 },
  'IBM-DYNAMIC': { rows: 24, cols: 80  },  // Negotiated dynamically via QUERY
});

// ── Write Control Character (WCC) Bits ────────────────────────────────────
// The WCC byte follows the Write/Erase Write command byte and controls
// what the terminal does before rendering the datastream.
const WCC = Object.freeze({
  RESET:           0x40,  // Reset MDT bits on all fields
  SOUND_ALARM:     0x04,  // Sound the terminal alarm
  UNLOCK_KEYBOARD: 0x02,  // Unlock the keyboard (allow operator input)
  RESTORE_FORMAT:  0x01,  // Restore format (reset alternate character sets)
});

// ── Buffer Address Encoding ────────────────────────────────────────────────
// 3270 uses a non-linear 6-bit encoding for buffer addresses (SBA, RA, EUA).
// The address is packed into two bytes using this code table.
// Address values 0x00–0x3F map to EBCDIC codes; the encoding is NOT binary.
const BUF_ADDR_CODE = Object.freeze([
  0x40, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7,  // 0x00–0x07
  0xC8, 0xC9, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F,  // 0x08–0x0F
  0x50, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7,  // 0x10–0x17
  0xD8, 0xD9, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F,  // 0x18–0x1F
  0x60, 0x61, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7,  // 0x20–0x27
  0xE8, 0xE9, 0x6A, 0x6B, 0x6C, 0x6D, 0x6E, 0x6F,  // 0x28–0x2F
  0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7,  // 0x30–0x37
  0xF8, 0xF9, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F,  // 0x38–0x3F
]);

// ── VTAM / SNA Structural Constants ───────────────────────────────────────
// For reference when working with SNA flows below the TN3270E layer.
const SNA = Object.freeze({
  // Logical Unit types — defines what protocol a node speaks
  LU_TYPE_0: 0,  // LU 0 — custom/negotiated (e.g. APPC sessions before LU 6.2)
  LU_TYPE_1: 1,  // LU 1 — SCS printer (SNA Character String)
  LU_TYPE_2: 2,  // LU 2 — 3270 display terminal ← what TN3270 emulates
  LU_TYPE_3: 3,  // LU 3 — 3270 printer
  LU_TYPE_4: 4,  // LU 4 — word processing / SCS printer
  LU_TYPE_6: 6,  // LU 6.2 — APPC (peer-to-peer, MQ, CICS ISC, FTP)

  // Physical Unit types — defines the role in the SNA network hierarchy
  PU_TYPE_1: 1,  // PU T1 — terminal node (e.g. a cluster controller)
  PU_TYPE_2: 2,  // PU T2 — cluster controller (e.g. IBM 3174, 3274)
  PU_TYPE_4: 4,  // PU T4 — communications controller (FEP, e.g. IBM 3745)
  PU_TYPE_5: 5,  // PU T5 — host (VTAM itself is a PU T5)
});

module.exports = {
  TELNET,
  OPT,
  TN3E,
  TN3E_REASON,
  TN3E_DATATYPE,
  CMD,
  ORDER,
  AID,
  FA,
  EXT_ATTR,
  COLOR,
  HIGHLIGHT,
  MODEL_DIMENSIONS,
  WCC,
  BUF_ADDR_CODE,
  SNA,
};
