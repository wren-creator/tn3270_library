# TN3270E Protocol Reference

A step-by-step reference for the TN3270E connection and session lifecycle.
Every byte exchange is documented with the *why*, not just the *what*.

---

## Overview: The Three Layers

A TN3270E session involves three distinct protocol layers stacked on top of each other:

```
┌─────────────────────────────────────────────┐
│  3270 Data Stream                           │  ← Application layer
│  Write commands, orders, field attributes   │
├─────────────────────────────────────────────┤
│  TN3270E                                    │  ← Session layer
│  Data type header, LU binding, functions    │
├─────────────────────────────────────────────┤
│  Telnet                                     │  ← Transport layer
│  Option negotiation, IAC escaping, EOR      │
├─────────────────────────────────────────────┤
│  TCP (port 23 / 992 / 339)                  │
└─────────────────────────────────────────────┘
```

The negotiation proceeds bottom-up: Telnet options are agreed first, then
TN3270E device type and LU binding, then 3270 data flows.

---

## Phase 1: TCP Connection

Standard TCP three-way handshake to port 23 (or 992 for TLS, 339 for alternate).

If TLS: the TLS handshake completes first, then Telnet begins.

After connection is established, **the host speaks first**. This is unlike
most client-server protocols. The host immediately sends a burst of Telnet
option negotiation bytes.

---

## Phase 2: Telnet Option Negotiation

### What the host sends (immediately on connect)

A typical host sends several option requests at once (they may arrive in one
TCP segment or several):

```
FF FD 28   IAC DO TN3270E        ← "I want TN3270E, will you support it?"
FF FD 18   IAC DO TTYPE          ← "Tell me your terminal type"
FF FD 00   IAC DO BINARY         ← "Use binary mode (no CR/LF munging)"
FF FB 00   IAC WILL BINARY       ← "I will also use binary mode"
FF FD 19   IAC DO EOR            ← "Use end-of-record markers"
FF FB 19   IAC WILL EOR          ← "I will also use EOR"
```

### What the client responds (TN3270E path)

```
FF FB 28   IAC WILL TN3270E      ← "Yes, I support TN3270E"
FF FB 18   IAC WILL TTYPE        ← "Yes, I'll tell you my type"
FF FB 00   IAC WILL BINARY       ← "Yes, binary mode"
FF FD 00   IAC DO BINARY         ← "You use binary mode too"
FF FB 19   IAC WILL EOR          ← "Yes, EOR"
FF FD 19   IAC DO EOR            ← "You use EOR too"
```

> **Why both sides agree on BINARY and EOR?**
> Telnet was designed for NVT (character terminal) sessions where the server
> echoes characters and CR/LF has special meaning. Binary mode suppresses
> all that special handling. EOR mode enables the `IAC EOR` sequence to
> delimit 3270 records, since 3270 datastreams are record-oriented, not
> character-oriented.

### Fallback: Classic TN3270 path

If the client responds `IAC WONT TN3270E`, the host falls back to classic
TN3270 (RFC 1576). The TTYPE negotiation then carries the terminal model:

```
Host: FF FA 18 01 FF F0          IAC SB TTYPE SEND IAC SE
Client: FF FA 18 00 49 42 4D 2D 33 32 37 38 2D 32 FF F0
                                  IAC SB TTYPE IS "IBM-3278-2" IAC SE
```

After TTYPE exchange and BINARY+EOR agreement, the host sends the first
screen directly as 3270 data (no 5-byte TN3270E header).

---

## Phase 3: TN3270E Sub-Negotiation

This is the distinctive part of TN3270E (vs. classic TN3270). After the
client sends `WILL TN3270E`, the host initiates a structured handshake
via Telnet subnegotiations.

### Step 1: Host requests device type

```
FF FA 28 08 02 FF F0
│         │  │
│         │  └── TN3E.DEVICE_TYPE (0x02)
│         └───── TN3E.SEND (0x08): "send me your device type"
└──────────────── IAC SB OPT_TN3270E
...FF F0 = IAC SE
```

Translation: *"Tell me what kind of terminal you are."*

### Step 2: Client requests a device type (and optionally an LU)

```
FF FA 28 02 07 49 42 4D 2D 33 32 37 38 2D 32 FF F0
│         │  │  └─────────────────────────┘
│         │  │         "IBM-3278-2"
│         │  └── TN3E.REQUEST (0x07)
│         └───── TN3E.DEVICE_TYPE (0x02)
└──────────────── IAC SB OPT_TN3270E
```

To request a specific LU name, add `TN3E.CONNECT (0x01)` + LU name bytes:

```
FF FA 28 02 07
   49 42 4D 2D 33 32 37 38 2D 32    "IBM-3278-2"
   01                                CONNECT
   4C 55 33 41 30 30 34 32           "LU3A0042"
FF F0
```

> **Why request a specific LU?**
> Some applications require a terminal to connect from a specific LU name
> (for security, routing, or printer pairing). In most cases, you omit the
> CONNECT and let VTAM assign any available LU from the pool.

### Step 3: Host confirms device type (and assigned LU)

```
FF FA 28 02 04 49 42 4D 2D 33 32 37 38 2D 32 01 4C 55 33 41 30 30 34 32 FF F0
│         │  │  └─────────────────────────┘ │  └─────────────────────────┘
│         │  │         "IBM-3278-2"         │         "LU3A0042"
│         │  └── TN3E.IS (0x04)             └── TN3E.CONNECT (0x01)
│         └───── TN3E.DEVICE_TYPE (0x02)
```

The client extracts the LU name from after the `CONNECT (0x01)` byte.
This is the LU name VTAM assigned — display it in your OIA.

### Step 4: Host requests functions

```
FF FA 28 03 07 [function bytes...] FF F0
│         │  │
│         │  └── TN3E.REQUEST (0x07)
│         └───── TN3E.FUNCTIONS (0x03)
```

Function bytes are a list of supported TN3270E extensions.
Common values: `0x00` (BIND-IMAGE), `0x02` (DATA-STREAM-CTL),
`0x04` (RESPONSES), `0x08` (SYSREQ).

### Step 5: Client echoes functions back

```
FF FA 28 03 04 [same function bytes] FF F0
│         │  │
│         │  └── TN3E.IS (0x04)
│         └───── TN3E.FUNCTIONS (0x03)
```

The client responds with the functions it supports from the requested list.
A minimal client can respond with an empty function list:
`FF FA 28 03 04 FF F0` — and still receive basic 3270 data.

> **Session is now live.** After FUNCTIONS IS, VTAM routes the terminal LU
> to either an SSCP-LU session (showing the VTAM logon screen) or directly
> to an application if an LU-specific route is configured.

---

## Phase 4: TN3270E Data Records

Every record (in both directions) now has a 5-byte TN3270E header:

```
┌──────────┬─────────┬──────────┬──────────┬──────────┐
│ DATA-TYPE│ REQUEST │ RESPONSE │ SEQ-HIGH │ SEQ-LOW  │
│  1 byte  │  1 byte │  1 byte  │  1 byte  │  1 byte  │
└──────────┴─────────┴──────────┴──────────┴──────────┘
```

Followed by the actual data, terminated by `IAC EOR`.

### Data type values

| Value | Name | Direction | Description |
|-------|------|-----------|-------------|
| `0x00` | DATA-3270 | Host→Terminal | Standard 3270 datastream (screens) |
| `0x01` | SCS-DATA | Host→Terminal | SNA Character String (printers) |
| `0x02` | RESPONSE | Terminal→Host | Positive/negative response |
| `0x03` | BIND-IMAGE | Host→Terminal | Raw SNA BIND data |
| `0x04` | UNBIND | Host→Terminal | Session terminated |
| `0x05` | NVT-DATA | Both | Raw NVT character data |
| `0x06` | REQUEST | Host→Terminal | Control request |
| `0x07` | SSCP-LU | Both | SSCP-LU session data (pre-BIND logon screen) |

### Example: Host sends a screen

```
00 00 00 00 00   ← TN3270E header (DATA-3270, no request, no response, seq 0)
F5 C2            ← Erase/Write command (0xF5) + WCC (0xC2 = reset+unlock)
11 40 40         ← SBA to address 0x0000 (row 1, col 1)
1D 60            ← SF with FA=0x60 (protected, high-intensity)
... screen data ...
FF EF            ← IAC EOR
```

### Example: Terminal sends Enter key

```
00 00 00 00 00   ← TN3270E header
7D              ← AID: ENTER
C5 D6            ← Cursor address (encoded)
11 C2 40         ← SBA: field address
C9 C2 D4 E4 E2 C5 D9  ← "IBMUSER" in EBCDIC
FF EF            ← IAC EOR
```

---

## Phase 5: SSCP-LU vs LU-LU Data

### SSCP-LU session (data type 0x07)

When you first connect, before selecting an application, you're in an SSCP-LU
session. The VTAM USSTAB generates the logon screen:

```
07 00 00 00 00   ← TN3270E header (SSCP-LU data)
[3270 datastream for the VTAM logon screen]
FF EF
```

The client renders this exactly like a DATA-3270 screen. When the user types
an application name and presses Enter:

```
07 00 00 00 00   ← TN3270E header (SSCP-LU data — terminal→host)
7D [cursor] [field data for "CICSPROD"]
FF EF
```

### Transition to LU-LU (data type 0x00)

VTAM processes the application name, sends CINIT to the application, the
application sends BIND, and from that point data type changes to `0x00`:

```
03 00 00 00 00   ← TN3270E header (BIND-IMAGE — if BIND-IMAGE function was negotiated)
[raw SNA BIND RU]
FF EF

00 00 00 00 00   ← DATA-3270 — first application screen
[3270 datastream]
FF EF
```

---

## Phase 6: Session Termination

### Normal disconnect (client-initiated)

Simply close the TCP socket. VTAM detects the TCP close, generates a CLSDST
to the application, and the LU is returned to the pool.

### UNBIND (host-initiated)

If the application ends the session (e.g., user logs off TSO), the host
sends:

```
04 00 00 00 00   ← UNBIND data type
[optional UNBIND reason code]
FF EF
```

The client should display a "session ended" message and either close or
return to a ready state.

---

## IAC Escaping

Any `0xFF` byte in the data stream must be escaped as `0xFF 0xFF`. This
applies to EBCDIC data — EBCDIC `0xFF` does appear in valid data (it's
the EO — Edit Mark character in EBCDIC).

A robust parser must handle:
- `FF FF` → single `0xFF` data byte
- `FF EF` → IAC EOR (end of record)
- `FF FA` → start of subnegotiation
- `FF FD` → IAC DO <option>
- All others → Telnet command (handle or ignore)

---

## Error Cases

### DEVICE-TYPE REJECT

If the requested LU is in use or the device type is unsupported:

```
FF FA 28 02 06 [reason-code-byte] FF F0
│         │  │  └── TN3E.REASON (0x05) + reason value
│         │  └── TN3E.REJECT (0x06)
│         └───── TN3E.DEVICE_TYPE (0x02)
```

Reason codes:
| Value | Meaning |
|-------|---------|
| `0x00` | Connected to partner |
| `0x01` | Device in use (LU already has a session) |
| `0x02` | Invalid ASSOCIATE |
| `0x03` | Invalid name (LU doesn't exist) |
| `0x04` | Invalid device type |
| `0x05` | Type/name error |
| `0x06` | Unknown error |
| `0x07` | Unsupported request |

### Host refuses TN3270E

If the host sends `IAC DONT TN3270E` after you sent `WILL TN3270E`, respond
with `IAC WONT TN3270E` and fall back to classic TN3270.

### Host refuses BINARY or EOR

If the host sends `IAC DONT BINARY` or `IAC DONT EOR`, the session cannot
proceed as a 3270 session. This indicates the host doesn't recognize this
connection as a 3270 port, or the port requires a different configuration.

---

## Byte-by-Byte Reference

### Telnet command bytes

| Byte | Hex | Name |
|------|-----|------|
| IAC | `0xFF` | Interpret As Command |
| DONT | `0xFE` | Don't use option |
| DO | `0xFD` | Do use option |
| WONT | `0xFC` | Won't use option |
| WILL | `0xFB` | Will use option |
| SB | `0xFA` | Start subnegotiation |
| SE | `0xF0` | End subnegotiation |
| EOR | `0xEF` | End of record |
| NOP | `0xF1` | No operation |

### Telnet option codes

| Option | Hex | Name | RFC |
|--------|-----|------|-----|
| BINARY | `0x00` | Binary transmission | 856 |
| ECHO | `0x01` | Echo | 857 |
| TTYPE | `0x18` | Terminal type | 1091 |
| EOR | `0x19` | End of record | 885 |
| TN3270E | `0x28` | TN3270E | 2355 |

### TN3270E sub-option function codes

| Code | Hex | Name |
|------|-----|------|
| ASSOCIATE | `0x00` | Associate (printer binding) |
| CONNECT | `0x01` | LU connect name follows |
| DEVICE-TYPE | `0x02` | Device type subnegotiation |
| FUNCTIONS | `0x03` | Function list negotiation |
| IS | `0x04` | "IS" response |
| REASON | `0x05` | Error reason code follows |
| REJECT | `0x06` | Rejection |
| REQUEST | `0x07` | "REQUEST" |
| SEND | `0x08` | "SEND" — request the other side send something |
