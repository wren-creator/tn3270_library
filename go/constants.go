package tn3270e

// constants.go
// ─────────────────────────────────────────────────────────────────────────────
// All TN3270 / TN3270E / VTAM protocol constants.
//
// References:
//   RFC 854   — Telnet Protocol
//   RFC 856   — Telnet Binary Transmission
//   RFC 885   — Telnet End of Record
//   RFC 1091  — Telnet Terminal Type
//   RFC 1576  — TN3270 Current Practices
//   RFC 2355  — TN3270E Enhancements
//   IBM GA23-0059 — 3270 Data Stream Programmer's Reference
// ─────────────────────────────────────────────────────────────────────────────

// ── Telnet Command Bytes ───────────────────────────────────────────────────
const (
	TelnetIAC  = 0xFF // Interpret As Command — begins a Telnet command sequence
	TelnetDONT = 0xFE // DONT <option> — refuse/stop an option
	TelnetDO   = 0xFD // DO <option>   — request/agree to an option
	TelnetWONT = 0xFC // WONT <option> — refuse to enable an option
	TelnetWILL = 0xFB // WILL <option> — offer/agree to enable an option
	TelnetSB   = 0xFA // SB <option>   — begin subnegotiation
	TelnetSE   = 0xF0 // SE            — end subnegotiation
	TelnetEOR  = 0xEF // EOR           — End Of Record (delimits 3270 records)
	TelnetNOP  = 0xF1 // NOP           — no-operation keepalive
)

// ── Telnet Option Codes ────────────────────────────────────────────────────
const (
	OptBinary  = 0x00 // RFC 856  — binary transmission (required for 3270)
	OptEcho    = 0x01 // RFC 857  — echo (DONT for 3270)
	OptEOR     = 0x19 // RFC 885  — end-of-record (required for 3270)
	OptTTYPE   = 0x18 // RFC 1091 — terminal type (model string)
	OptTN3270E = 0x28 // RFC 2355 — TN3270E enhanced protocol
)

// ── TN3270E Sub-negotiation Function Codes ────────────────────────────────
const (
	TN3EAssociate   = 0x00 // Associate printer LU to display LU
	TN3EConnect     = 0x01 // LU name to connect to follows
	TN3EDeviceType  = 0x02 // Device-type subnegotiation
	TN3EFunctions   = 0x03 // Functions negotiation
	TN3EIs          = 0x04 // "IS" — answering half of REQUEST/IS
	TN3EReason      = 0x05 // Error reason code follows
	TN3EReject      = 0x06 // Host rejects device-type or LU request
	TN3ERequest     = 0x07 // "REQUEST" — asking half of REQUEST/IS
	TN3ESend        = 0x08 // Host asks us to send something
)

// ── TN3270E Data Type Codes ────────────────────────────────────────────────
// First byte of every TN3270E 5-byte record header.
const (
	DataType3270     = 0x00 // Standard 3270 datastream
	DataTypeSCS      = 0x01 // SNA Character String (printer)
	DataTypeResponse = 0x02 // Positive/negative response
	DataTypeBindImg  = 0x03 // SNA BIND image
	DataTypeUnbind   = 0x04 // LU-LU session ended
	DataTypeNVT      = 0x05 // Network Virtual Terminal data
	DataTypeRequest  = 0x06 // Control request
	DataTypeSSCPLU   = 0x07 // SSCP-LU session (pre-BIND logon screen)
)

// ── TN3270E Reason Codes ──────────────────────────────────────────────────
const (
	ReasonConnPartner    = 0x00 // Connected to partner LU
	ReasonDeviceInUse    = 0x01 // LU already has a session
	ReasonInvAssociate   = 0x02 // Invalid ASSOCIATE request
	ReasonInvName        = 0x03 // Invalid LU or device-type name
	ReasonInvDeviceType  = 0x04 // Device type not supported
	ReasonTypeNameError  = 0x05 // Device type + name combination invalid
	ReasonUnknownError   = 0x06 // Unspecified error
	ReasonUnsupportedReq = 0x07 // Function not supported
)

// ── 3270 Write Commands ────────────────────────────────────────────────────
const (
	CmdWrite              = 0xF1 // Write — update screen, reset MDT bits
	CmdEraseWrite         = 0xF5 // Erase/Write — clear screen first
	CmdEraseWriteAlt      = 0x7E // Erase/Write Alternate — alternate screen size
	CmdReadBuffer         = 0xF2 // Read Buffer — send full screen buffer
	CmdReadModified       = 0xF6 // Read Modified — send only modified fields
	CmdReadModifiedAll    = 0x6E // Read Modified All — modified + ENTER fields
	CmdEraseAllUnprotect  = 0x6F // Erase All Unprotected — clear input fields
	CmdWriteStructured    = 0xF3 // Write Structured Field
)

// ── 3270 Datastream Orders ─────────────────────────────────────────────────
const (
	OrderSF  = 0x1D // Start Field — begins a field, sets field attributes
	OrderSFE = 0x29 // Start Field Extended — SF with extended attribute pairs
	OrderSBA = 0x11 // Set Buffer Address — moves the buffer address pointer
	OrderSA  = 0x28 // Set Attribute — changes attribute without starting field
	OrderMF  = 0x2C // Modify Field — modifies attributes of current field
	OrderIC  = 0x13 // Insert Cursor — places cursor at current buffer position
	OrderPT  = 0x05 // Program Tab — advance to next unprotected field
	OrderRA  = 0x3C // Repeat to Address — fill range with one character
	OrderEUA = 0x12 // Erase Unprotected to Address
	OrderGE  = 0x08 // Graphic Escape — next byte is graphic character set char
)

// ── AID (Attention Identifier) Bytes ──────────────────────────────────────
// First byte of every inbound (terminal→host) datastream record.
const (
	AIDNone   = 0x60 // No AID — used in Read Buffer responses
	AIDEnter  = 0x7D // Enter key
	AIDClear  = 0x6D // Clear key — sends only AID, no field data
	AIDPA1    = 0x6C // Program Attention 1 — no field data sent
	AIDPA2    = 0x6E // Program Attention 2
	AIDPA3    = 0x6B // Program Attention 3
	AIDSysReq = 0xF0 // System Request — SSCP-LU session attention
	AIDPF1    = 0xF1
	AIDPF2    = 0xF2
	AIDPF3    = 0xF3
	AIDPF4    = 0xF4
	AIDPF5    = 0xF5
	AIDPF6    = 0xF6
	AIDPF7    = 0xF7
	AIDPF8    = 0xF8
	AIDPF9    = 0xF9
	AIDPF10   = 0x7A
	AIDPF11   = 0x7B
	AIDPF12   = 0x7C
	AIDPF13   = 0xC1
	AIDPF14   = 0xC2
	AIDPF15   = 0xC3
	AIDPF16   = 0xC4
	AIDPF17   = 0xC5
	AIDPF18   = 0xC6
	AIDPF19   = 0xC7
	AIDPF20   = 0xC8
	AIDPF21   = 0xC9
	AIDPF22   = 0x4A
	AIDPF23   = 0x4B
	AIDPF24   = 0x4C
)

// ── Field Attribute Bits ───────────────────────────────────────────────────
const (
	FAProtected        = 0x20 // Bit 5 — field is protected (read-only)
	FANumeric          = 0x10 // Bit 4 — numeric input only
	FAMDT              = 0x01 // Bit 0 — Modified Data Tag
	FAIntensityNormal  = 0x00 // Normal display
	FAIntensityHigh    = 0x08 // High intensity
	FAIntensityHidden  = 0x0C // Non-display (password fields)
	FAIntensityMask    = 0x0C // Mask to extract intensity bits
)

// ── Extended Attribute Types ───────────────────────────────────────────────
const (
	ExtAttrAllChars       = 0x00 // Reset all extended attributes
	ExtAttrHighlight      = 0x41 // Highlight type
	ExtAttrForeground     = 0x42 // Foreground color
	ExtAttrCharSet        = 0x43 // Character set
	ExtAttrBackground     = 0x45 // Background color
	ExtAttrTransparency   = 0x46 // Transparency
	ExtAttrFieldOutline   = 0x60 // Field outline
)

// ── Color Values ───────────────────────────────────────────────────────────
const (
	ColorDefault      = 0x00
	ColorBlue         = 0xF1
	ColorRed          = 0xF2
	ColorPink         = 0xF3
	ColorGreen        = 0xF4
	ColorTurquoise    = 0xF5
	ColorYellow       = 0xF6
	ColorWhite        = 0xF7
	ColorBlack        = 0xF8
	ColorDeepBlue     = 0xF9
	ColorOrange       = 0xFA
	ColorPurple       = 0xFB
	ColorPaleGreen    = 0xFC
	ColorPaleTurquoise = 0xFD
	ColorGrey         = 0xFE
)

// ── Highlight Values ───────────────────────────────────────────────────────
const (
	HighlightDefault    = 0x00
	HighlightBlink      = 0xF1
	HighlightReverse    = 0xF2
	HighlightUnderscore = 0xF4
	HighlightIntensify  = 0xF8
)

// ── WCC (Write Control Character) Bits ────────────────────────────────────
const (
	WCCReset          = 0x40 // Reset MDT bits on all fields
	WCCSoundAlarm     = 0x04 // Sound the terminal alarm
	WCCUnlockKeyboard = 0x02 // Unlock the keyboard
	WCCRestoreFormat  = 0x01 // Restore format
)

// ── Buffer Address Code Table ──────────────────────────────────────────────
// 3270 uses a non-linear 6-bit encoding for buffer addresses.
// Index = 6-bit address value, value = byte transmitted on the wire.
var BufAddrCode = [64]byte{
	0x40, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7,
	0xC8, 0xC9, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F,
	0x50, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7,
	0xD8, 0xD9, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F,
	0x60, 0x61, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7,
	0xE8, 0xE9, 0x6A, 0x6B, 0x6C, 0x6D, 0x6E, 0x6F,
	0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7,
	0xF8, 0xF9, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F,
}

// ── Terminal Model Dimensions ──────────────────────────────────────────────

// ModelDimensions holds the row and column count for a terminal model.
type ModelDimensions struct {
	Rows int
	Cols int
}

// KnownModels maps terminal model strings to their screen dimensions.
var KnownModels = map[string]ModelDimensions{
	"3278-2":     {Rows: 24, Cols: 80},
	"3278-3":     {Rows: 32, Cols: 80},
	"3278-4":     {Rows: 43, Cols: 80},
	"3278-5":     {Rows: 27, Cols: 132},
	"IBM-3278-2": {Rows: 24, Cols: 80},
	"IBM-3278-5": {Rows: 27, Cols: 132},
	"IBM-DYNAMIC": {Rows: 24, Cols: 80}, // negotiated dynamically
}

// ModelDims returns the dimensions for a given model string,
// defaulting to 3278-2 (24x80) if the model is not recognised.
func ModelDims(model string) ModelDimensions {
	if d, ok := KnownModels[model]; ok {
		return d
	}
	return ModelDimensions{Rows: 24, Cols: 80}
}

// ── SNA Structural Constants ───────────────────────────────────────────────
const (
	LUType0 = 0 // LU 0 — custom/negotiated
	LUType1 = 1 // LU 1 — SCS printer
	LUType2 = 2 // LU 2 — 3270 display terminal (what TN3270 emulates)
	LUType3 = 3 // LU 3 — 3270 printer
	LUType4 = 4 // LU 4 — word processing / SCS printer
	LUType6 = 6 // LU 6.2 — APPC peer-to-peer

	PUType1 = 1 // PU T1 — terminal node
	PUType2 = 2 // PU T2 — cluster controller
	PUType4 = 4 // PU T4 — communications controller
	PUType5 = 5 // PU T5 — host (VTAM itself)
)
