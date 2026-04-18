# TN3270E & VTAM Glossary

A reference for developers working with IBM mainframe terminal protocols.
This knowledge is increasingly rare — the goal here is to document not just
*what* these terms mean, but *why* they exist and how they fit together.

> **Reading order:** If you're new to this space, start with the
> [SNA & VTAM Background](#sna--vtam-background) section before diving into
> the alphabetical entries. Understanding the network model makes everything else click.

---

## SNA & VTAM Background

### Why does any of this exist?

In the 1970s, IBM needed a way to connect thousands of "dumb" terminals
(IBM 3270 display stations) to mainframes running MVS (now z/OS). The answer
was **SNA** — Systems Network Architecture — a complete, layered networking
model that IBM designed before TCP/IP existed.

SNA is hierarchical. The mainframe is always at the top. Every device in
the network has a defined role, a defined address, and a defined set of
protocols it speaks. Nothing is peer-to-peer; everything flows through VTAM
(Virtual Telecommunications Access Method) on the mainframe.

When TCP/IP won the networking wars, IBM didn't abandon SNA — they built a
bridge. **TN3270** (later **TN3270E**) lets TCP/IP clients speak the 3270
terminal protocol over a standard Telnet connection. Under the covers, VTAM
still manages the sessions as if they were SNA LU Type 2 sessions. The
TCP/IP network is invisible to VTAM.

### The 3270 data stream model

Unlike VT100 or xterm, which stream characters, 3270 terminals are
**screen-oriented** and **record-oriented**:

- The host sends a complete screen description (a "write command") all at once
- The terminal renders it and waits — the keyboard is **locked**
- When the operator presses a key (Enter, PF1, etc.), the terminal sends
  the **entire modified screen contents** back to the host in one record
- The host processes it, decides what to send next, and the cycle repeats

This is called the **bracket protocol** in SNA terminology. There is no
character-by-character echo. The terminal is a display device with a local
buffer, not a character terminal.

This model was designed for 2400 baud leased lines in the 1970s — sending
only changed data in each direction was essential for performance.

---

## Alphabetical Reference

---

### ACB — Access Control Block

An in-storage control block used by VTAM to represent an open connection
between an application program and the VTAM subsystem. When a CICS or TSO
region calls `VTAM OPEN ACB`, VTAM creates the ACB and from that point the
application can issue VTAM RPL (Request Parameter List) calls to send and
receive data.

Not directly visible in TN3270E, but the sessions you establish through
TN3270E ultimately terminate at an application that has an open ACB.

---

### ACBNAME — ACB Name

The name of the VTAM ACB, used as the application's network name. In CICS,
the ACBNAME is typically the same as the CICS region's APPLID. When you
connect a TN3270E session to `CICSPROD`, VTAM routes your session to the
application whose ACB is named `CICSPROD`.

---

### AID — Attention Identifier

A single byte that begins every inbound (terminal → host) 3270 data record.
It tells the host what action the operator took:

| AID | Hex | Meaning |
|-----|-----|---------|
| ENTER | `0x7D` | Enter key — most common |
| CLEAR | `0x6D` | Clear key — clears screen, sends short record |
| PA1 | `0x6C` | Program Attention 1 — no field data sent |
| PA2 | `0x6E` | Program Attention 2 |
| PA3 | `0x6B` | Program Attention 3 |
| SYSREQ | `0xF0` | System Request — switches to SSCP-LU session |
| PF1–PF9 | `0xF1`–`0xF9` | Program Function keys 1–9 |
| PF10–PF12 | `0x7A`–`0x7C` | Program Function keys 10–12 |
| PF13–PF24 | `0xC1`–`0xC9`, `0x4A`–`0x4C` | Program Function keys 13–24 |

**PA keys** are special: they send only the AID byte and cursor address, no
field data. The host receives the key press but not the screen contents.
They are used for functions like "scroll up" (PF7/PF8 in ISPF) where the
host doesn't need the input fields.

**PF keys** send the AID byte, cursor address, and all modified fields (those
with the MDT bit set).

---

### APPLID — Application Identifier

The name by which a VTAM application (CICS, TSO, IMS, JES2, etc.) is known
in the SNA network. When users select an application from the VTAM logon
screen (USSTAB), they type its APPLID.

In network definitions, the APPLID is defined in the `APPL` statement in
the VTAM application major node.

---

### ATTN — Attention

In SNA flow control, an ATTN signal is sent by the terminal to interrupt
the current operation. In 3270 terminals, the AID key mechanism serves
this purpose. SYSREQ is the closest equivalent to a true attention signal.

---

### BIND — Session Initiation

An SNA command that establishes an LU-LU session. The BIND image contains
all the session parameters: protocol mode, buffer sizes, pacing windows,
and more. In TN3270E, the host may deliver a BIND image to the client as
a `BIND_IMAGE` data type record — this tells you the real SNA parameters
of the session even though you're on TCP/IP.

The BIND response (`BIND RSP`) from the terminal confirms the session is
established. After BIND, the application sends the first screen.

---

### Buffer Address Encoding

3270 uses a non-linear encoding for screen buffer addresses. Addresses are
stored as two bytes, but the encoding is *not* simple binary. Instead, each
address byte uses a 6-bit code table (sometimes called the "12-bit encoding"):

```
Bits 7–6 of byte 1: encoding type flag (00 or 01 for code-table, 11 for binary)
Bits 5–0 of byte 1: upper 6 bits of address
Bits 5–0 of byte 2: lower 6 bits of address
```

The code table maps 0–63 to EBCDIC values in the range `0x40`–`0xFF`. This
is because early 3270 hardware transmitted addresses as EBCDIC characters,
and the encoding had to avoid values that EBCDIC treated specially.

When decoding a buffer address from the wire: check bits 7–6 of the first
byte. If `00` or `11`, treat it as 14-bit binary. Otherwise use the 6-bit
code table for each byte.

---

### CINIT — Control Initiate

An SNA request unit sent by VTAM to an application (like CICS) to initiate
a session with a terminal LU. The CINIT contains the BIND parameters and
the name of the terminal LU. The application responds by sending a BIND to
the terminal LU.

Not directly visible in TN3270E, but CINIT is what happens behind the scenes
when you connect a TN3270E session and VTAM routes it to CICS.

---

### CLSDSTx — Close Destination

VTAM verbs used by applications to end sessions with terminal LUs. CLSDST
PASS is a particularly important variant — it passes the terminal session to
another application (used in CICS multi-region operations and VTAM logon
screen processing).

---

### CMPSC — Compression

SNA supports data stream compression. Not commonly used in TN3270E but
relevant in native SNA environments. The compression algorithm is defined
in the BIND parameters.

---

### CP037 — Code Page 037

The standard IBM EBCDIC code page for US English. Maps 256 byte values to
characters. The mapping is completely different from ASCII:

- EBCDIC `A` = `0xC1` (ASCII `A` = `0x41`)
- EBCDIC `0` = `0xF0` (ASCII `0` = `0x30`)
- EBCDIC space = `0x40` (ASCII space = `0x20`)

There is no contiguous run of alphabetic characters in EBCDIC. `A`–`I` are
`0xC1`–`0xC9`, then there's a gap, `J`–`R` are `0xD1`–`0xD9`, gap, `S`–`Z`
are `0xE2`–`0xE9`. This matters for range checks on EBCDIC data.

Common code pages: 37 (US), 273 (Germany), 277 (Denmark/Norway),
278 (Finland/Sweden), 280 (Italy), 284 (Spain), 285 (UK), 297 (France),
500 (International/Belgium/Switzerland).

---

### Cross-Domain Session

An SNA session between LUs in different VTAM domains (different mainframes
or different VTAM instances). Requires a Cross-Domain Resource Manager
(CDRM) and is defined in cross-domain resource (CDRSC) major nodes.

In large enterprises, users at one LPAR may be routed to applications on
another LPAR through cross-domain session setup. From the TN3270E
perspective, the session looks the same — the routing is invisible.

---

### CTERM — Control Terminate

An SNA request that terminates an LU-LU session. CTERM is sent by VTAM to
inform an application that a terminal has disconnected or a session has ended.

---

### Data Type (TN3270E)

The first byte of every TN3270E data record header (the 5-byte TN3270E
header prepended to each record). Identifies what kind of data follows:

| Value | Name | Description |
|-------|------|-------------|
| `0x00` | DATA-3270 | Standard 3270 datastream |
| `0x01` | SCS-DATA | SNA Character String (printer) |
| `0x02` | RESPONSE | Positive/negative response |
| `0x03` | BIND-IMAGE | SNA BIND image |
| `0x04` | UNBIND | Session ended |
| `0x05` | NVT-DATA | Network Virtual Terminal (not 3270) |
| `0x06` | REQUEST | Control request |
| `0x07` | SSCP-LU | SSCP-LU session data (pre-BIND logon) |

Most TN3270 clients only need to handle `0x00` (standard screen data) and
`0x07` (the VTAM logon screen before the application BIND).

---

### DLOGMOD — Default Logon Mode

The name of the VTAM logon mode table entry used when no specific mode is
requested during session setup. Logon modes define session parameters like
buffer size, pacing counts, and cryptography settings.

Common default logon mode names: `SNX32702` (3270 model 2), `SNX32705`
(3270 model 5, 132-column), `DSILGMOD` (IBM default).

---

### DSI — Data Streams Interface

The component of VTAM that handles the 3270 data stream — managing the
write commands, orders, and field attributes that make up screen content.

---

### EAU — Erase All Unprotected

A 3270 write command (`0x6F`) that clears all unprotected (input) fields on
the screen without sending any new data. Used when an application wants to
reset the form without redrawing the whole screen.

---

### EBCDIC — Extended Binary Coded Decimal Interchange Code

IBM's character encoding standard, used by all mainframe systems. See
**CP037** for details. EBCDIC predates ASCII as an IBM standard, originating
with the IBM 1401 in the early 1960s and the System/360 in 1964.

The practical consequence for TN3270E: every character that passes between
the terminal and the mainframe is in EBCDIC. Your client must convert to/from
the local character set (usually ASCII or UTF-8) at both ends.

---

### EOR — End of Record

Telnet option 25 (`0x19`) and the `IAC EOR` (`0xFF 0xEF`) sequence. Required
for TN3270. Since 3270 data records are variable-length and the transport is
a byte stream (TCP), there must be a way to delimit records. TN3270 uses
`IAC EOR` as the record terminator, which is why both BINARY and EOR Telnet
options must be negotiated before 3270 data can flow.

---

### FA — Field Attribute

A special byte in the 3270 screen buffer that marks the beginning of a field
and defines its properties. The FA byte occupies one screen position (it
displays as a blank) and encodes:

| Bits | Meaning |
|------|---------|
| Bit 5 | Protected (1 = read-only, 0 = input) |
| Bit 4 | Numeric (1 = numeric-only input) |
| Bits 3–2 | Intensity / pen-selectable |
| Bit 0 | MDT — Modified Data Tag |

The FA byte itself is encoded in EBCDIC (values `0x40`–`0x7F` and
`0xC0`–`0xFF`). To test the attribute bits, you must work with the raw
byte value, not an ASCII translation.

**Intensity values (bits 3–2):**
- `00` = Normal display
- `01` = Normal display, pen-selectable (selector pen era)
- `10` = High intensity
- `11` = Non-display (password fields)

---

### FID — Format Indicator

Part of the SNA Transmission Header (TH). Defines the format of the header
itself. FID0 through FID4 are the standard formats:
- **FID2** is used for SNA sessions involving subarea nodes (most LAN/WAN SNA)
- **FID4** is used for subarea-to-subarea flow (inter-domain)

Mostly invisible in TN3270E, but you'll see references in IBM documentation.

---

### FMH — Function Management Header

An optional header prepended to SNA request units that provides control
information above the basic data. FMH-5 is the one most relevant to 3270 —
it carries the ATTACH function that initiates APPC conversations.

---

### FMI — Function Management Interface

The SNA layer that manages the presentation and formatting of data. For
LU Type 2 (3270), FMI handles the 3270 data stream protocol.

---

### Functions (TN3270E)

During TN3270E sub-negotiation, after the device type and LU are agreed,
the host and client negotiate which TN3270E "functions" to use. Functions
are extensions to the basic TN3270E protocol:

| Function | Description |
|----------|-------------|
| BIND-IMAGE | Receive the SNA BIND image |
| DATA-STREAM-CTL | Data stream control |
| RESPONSES | Send/receive SNA response protocol |
| SYSREQ | Support the SYSREQ key for SSCP-LU switching |

The negotiation is: host sends `FUNCTIONS REQUEST <list>`, client responds
`FUNCTIONS IS <list>`. The client echoes back what it supports from the
requested list. A minimal client can respond with an empty function list and
still get basic 3270 functionality.

---

### GDS — General Data Stream

A structured data format used in APPC (LU 6.2) conversations. Not relevant
for LU Type 2 (3270) terminal sessions, but you'll encounter it if working
with CICS APPC bridges.

---

### IBM-DYNAMIC (Model String)

A special terminal model string used in TN3270E negotiation to request a
dynamically-sized screen. After connecting with this model, the host sends
a Query Structured Field reply that includes the terminal's actual dimensions.
The client then sends a Query Reply with its actual size, and the host adjusts.

Not all hosts support IBM-DYNAMIC; many require an explicit model like
`IBM-3278-2` or `IBM-3278-5`.

---

### IC — Insert Cursor (Order `0x13`)

A 3270 datastream order. When the host renders a Write command, an IC order
at a given buffer position means "place the cursor here after rendering."
A screen can only have one IC; if multiple IC orders appear, the last one wins.

---

### IAC — Interpret As Command (`0xFF`)

The Telnet escape byte. When the receiver sees `0xFF` in the byte stream, the
next byte is a Telnet command, not data. To send a literal `0xFF` data byte,
send `0xFF 0xFF` (IAC IAC). TN3270 parsers must handle this IAC-escaping
correctly or they will misinterpret `0xFF` bytes in EBCDIC data.

---

### ISPF — Interactive System Productivity Facility

IBM's primary menu-driven interface for TSO. Users navigate panels, edit
datasets, submit JCL, browse job output, and manage files through ISPF. It
makes heavy use of 3270 features: protected/unprotected fields, PF key
assignments, split-screen (requiring 3278-5 or 3279-5 model), and color.

ISPF panels are defined using Panel Definition Language (PDL) which maps
directly onto 3270 datastream concepts.

---

### JES — Job Entry Subsystem

The mainframe component that manages batch job execution. JES2 and JES3
are the two variants (JES2 is more common on modern systems). Operators
and users interact with JES through SDSF (System Display and Search Facility),
which is a full-screen 3270 application. JES also provides SYSIN/SYSOUT
spool management.

---

### LU — Logical Unit

The SNA endpoint that applications and terminals present to the network. An
LU is identified by a name (up to 8 characters) and a type:

| LU Type | Protocol | Usage |
|---------|----------|-------|
| 0 | Custom | APPC before LU 6.2 existed |
| 1 | SCS | Printer (SNA Character String) |
| **2** | **3270 display** | **What TN3270 emulates** |
| 3 | 3270 printer | 3270-style printer sessions |
| 6.2 | APPC | Peer-to-peer, CICS ISC, MQ, FTP |

When you establish a TN3270E session, the TN3270E server creates a virtual
LU Type 2 in VTAM on your behalf. The LU name (e.g., `LU3A0042`) is what
VTAM and the application see as the terminal address.

**LU pools:** Installations typically define a pool of available LU names in
the VTAM configuration. The TN3270E server assigns one from the pool to each
new connection. This is why you see names like `LU3A0001` through `LU3A0099`.

---

### LU Name

The network name of an LU, up to 8 EBCDIC characters. LU names must be
unique within a VTAM domain. They appear in:
- VTAM major node definitions (`.vtamlst` dataset)
- The TN3270E sub-negotiation (CONNECT parameter in DEVICE-TYPE REQUEST)
- The OIA (Operator Information Area) of 3270 emulators
- NETSTAT output on z/OS

Naming conventions vary by installation. Common patterns:
- `LU3Annnn` — 3270 display LU pool
- `LU3Pnnnn` — 3270 printer LU pool
- `TSU00001` — TSO user LU (temporary, per-session)

---

### LUNAME — See LU Name

---

### LPAR — Logical Partition

A hardware-defined partition of a mainframe system, each running its own
operating system instance. A physical IBM Z mainframe may have dozens of
LPARs. Each LPAR with z/OS will typically run its own VTAM instance, though
LPARs can be interconnected via XCF (Cross-System Coupling Facility) and
share VTAM resources through sysplex configurations.

From a TN3270E perspective, you connect to a specific LPAR's IP address and
port. The TN3270E server on that LPAR handles your session.

---

### MDT — Modified Data Tag

Bit 0 of the field attribute byte. When set, it indicates the field has been
modified by the operator (or programmatically). During a Read Modified
operation, only fields with MDT=1 are transmitted to the host. This is a
key performance optimization — on a screen with 50 fields, if the operator
only changed 2, only those 2 are sent.

The host can reset all MDT bits by including a WCC with the RESET bit set
in a Write command.

Applications can also set MDT programmatically (using the SFE order with
the attribute's MDT bit set) to force a field to be included in the Read
Modified output even if the operator didn't change it.

---

### Mode Table — See MODEENT / DLOGMOD

---

### MODEENT

A VTAM macro that defines a logon mode table entry. Logon modes specify
session parameters negotiated during BIND:

- `RUSIZES` — maximum RU (Request Unit) sizes for each direction
- `PACING` — pacing window sizes (flow control)
- `ENCR` — encryption requirements
- `SRCVPAC` — secondary (terminal) receive pacing

The mode table entry name is agreed upon at session initiation. Typical
IBM-supplied mode names: `SNX32702`, `SNX32705`, `#INTERSC`, `DSILGMOD`.

---

### MRO — Multi-Region Operation

A CICS feature allowing multiple CICS regions to share terminal sessions
and communicate with each other. In MRO, a terminal-owning region (TOR)
handles all terminal I/O while an application-owning region (AOR) runs the
application logic. VTAM CLSDST PASS is used to transfer the session
between regions.

---

### NCP — Network Control Program

IBM software that ran on the 3745/3725/3720 Communications Controllers (Front
End Processors, or FEPs). The NCP managed the physical SNA network — polling
cluster controllers, managing transmission groups, and buffering data. NCP
essentially implemented the lower layers of SNA in dedicated hardware.

NCP is largely obsolete today (the FEPs have been decommissioned at most
sites), but its concepts live on in the IP-to-SNA gateway terminology.

---

### NVT — Network Virtual Terminal

The Telnet standard for character terminal sessions (before 3270 extensions).
In TN3270E, data type `0x05` (NVT-DATA) carries NVT-style data. Some hosts
send NVT data before the TN3270E negotiation is complete or when the session
is in an SSCP-LU state.

---

### OIA — Operator Information Area

The bottom line (row 25 on a 24-line screen) of a 3270 display, used to
show session status information. Not part of the 3270 data stream from the
host — the terminal (or emulator) renders it locally from session state.

Typical OIA content:
- Session name / LU name
- Keyboard lock indicator (X SYSTEM, X CLOCK, etc.)
- Insert mode indicator
- Connection status

In TN3270E emulators, the OIA is synthesized from negotiated LU name,
keyboard state, and connection status.

---

### Order (3270 Datastream)

Control bytes embedded within 3270 Write command data that direct the
terminal's buffer pointer or set field attributes. Orders have values in
the range `0x01`–`0x3F` (which are non-displayable in EBCDIC). Key orders:

| Order | Hex | Name | Parameters |
|-------|-----|------|------------|
| SF | `0x1D` | Start Field | 1 byte (FA) |
| SFE | `0x29` | Start Field Extended | count, then pairs |
| SBA | `0x11` | Set Buffer Address | 2 bytes (address) |
| IC | `0x13` | Insert Cursor | none |
| PT | `0x05` | Program Tab | none |
| RA | `0x3C` | Repeat to Address | 2 bytes (address) + 1 byte (char) |
| EUA | `0x12` | Erase Unprotected to Address | 2 bytes (address) |
| SA | `0x28` | Set Attribute | 2 bytes (type, value) |
| MF | `0x2C` | Modify Field | count, then pairs |
| GE | `0x08` | Graphic Escape | 1 byte (char) |

---

### PACING — Flow Control

SNA's mechanism for preventing a fast sender from overwhelming a slow
receiver. The receiver grants "pacing credits" to the sender. The sender
can transmit that many RUs before waiting for more credits.

For TN3270, pacing is mostly transparent — the TN3270E server handles it
between VTAM and the application. But understanding it explains why a host
sometimes pauses before sending the next screen.

---

### PLU / SLU — Primary / Secondary Logical Unit

In an SNA LU-LU session, one LU is designated Primary (PLU) and the other
Secondary (SLU). The PLU initiates the BIND and controls the session.

For LU Type 2 (3270) sessions:
- **PLU** = the application (CICS, TSO, etc.)
- **SLU** = the terminal (your TN3270E session)

This distinction matters for understanding who sends what kind of SNA
requests. The PLU sends `WRITE` commands (the application sends screens).
The SLU sends inbound data (the terminal sends keystrokes).

---

### PU — Physical Unit

The SNA node that represents the physical (or virtual) hardware. PU types:

| Type | Description |
|------|-------------|
| PU T1 | Terminal node (e.g., a 3270 cluster controller's downstream device) |
| **PU T2** | **Cluster controller** (e.g., IBM 3174, 3274) |
| PU T4 | Communications controller running NCP |
| **PU T5** | **Host node — VTAM itself** |

In TN3270E, the TN3270E server acts as a virtual PU T2 on behalf of your
TCP/IP connection. VTAM sees a cluster controller with attached LUs.

---

### RA — Repeat to Address (Order `0x3C`)

A 3270 datastream order that fills a range of buffer positions with a
single character. Takes a 2-byte destination address and a 1-byte character.
Efficient for drawing separator lines, clearing regions, or filling fields
with a pad character.

Example: `0x3C 0x7D 0x40 0x40` — fill from current position to address
`0x1D40` (decoded) with EBCDIC space (`0x40`).

---

### RF — Read Fields

See **Read Modified**.

---

### Read Buffer

A 3270 command (`0xF2`) that causes the terminal to send the entire screen
buffer to the host, regardless of MDT bits. Used by applications that need
to read every field, including unmodified ones. Less common than Read Modified.

---

### Read Modified

A 3270 command (`0xF6`) that causes the terminal to send only the fields
with the MDT bit set (modified fields) plus the cursor position. This is
the most common read operation — it minimizes the data sent for each
keyboard operation.

**Read Modified All** (`0x6E`) includes fields marked with MDT *and* also
any fields that the program set to "transmit on enter" using the attribute.

---

### RPL — Request Parameter List

A VTAM control block used by application programs to issue VTAM requests
(GET, PUT, SEND, RECEIVE, etc.). The application fills in an RPL and calls
VTAM with it. Not visible in TN3270E, but essential for VTAM application
programming.

---

### RU — Request Unit

The basic unit of data in SNA. An RU is a variable-length data block
transmitted between LUs. The maximum RU size is negotiated in the BIND.
For 3270 sessions, the typical maximum RU size is 1536 bytes (though
modern installations often use larger values).

Multiple RUs may be required to send a full screen if the datastream
exceeds the maximum RU size. Chaining rules govern how multiple RUs are
assembled into a complete message.

---

### SBA — Set Buffer Address (Order `0x11`)

A 3270 datastream order that moves the buffer address pointer to a
specified location. Takes a 2-byte encoded address. All subsequent data
bytes or SF orders write at that address (incrementing forward).

The buffer is linear — address 0 is row 1, column 1. Address (rows×cols)
wraps back to 0.

---

### SCS — SNA Character String

The data stream used by LU Type 1 (SCS printer) sessions. SCS uses control
characters for carriage return, line feed, form feed, and horizontal tab.
It's simpler than the 3270 data stream but has its own control character set.

In TN3270E, SCS data arrives with data type `0x01` in the TN3270E header.

---

### SDSF — System Display and Search Facility

IBM's job and system monitor, displaying job queues, job output, system logs,
and resource usage. SDSF is a full-screen 3270 application that uses PF keys
extensively. Users who know "SDSF commands" are usually pressing PF keys or
typing commands in the command field.

---

### SF — Start Field (Order `0x1D`)

The fundamental 3270 datastream order. Places a field attribute byte at the
current buffer address. The FA byte encodes whether the field is protected,
numeric, its display intensity, and its MDT status.

The position of the SF order is the "field attribute" position — it displays
as a blank. The field content begins at the next position and extends until
(but not including) the next SF order.

---

### SFE — Start Field Extended (Order `0x29`)

Extended version of SF. Instead of a single FA byte, SFE takes a count
followed by attribute type/value pairs. Supports extended attributes like
foreground color, background color, highlighting (blink, reverse, underline),
and character set.

The first attribute pair is typically type `0xC0` (3270 field attribute),
equivalent to the FA byte in a standard SF.

---

### SNA — Systems Network Architecture

IBM's layered networking model, defined in 1974. SNA has seven layers
(predating but paralleling the OSI model):

1. Physical
2. Data Link (SDLC — Synchronous Data Link Control)
3. Path Control (routing)
4. Transmission Control (sessions, sequencing)
5. Data Flow Control (brackets, chaining)
6. Presentation Services (data formatting)
7. Transaction Services (application services, APPC)

VTAM implements layers 4–7 on the mainframe. NCP (now replaced by IP
networks) implemented layers 2–3.

---

### SSCP — System Services Control Point

The component of VTAM that manages the SNA domain — handles logon/logoff,
session initiation (CINIT), and operator commands. SSCP-LU sessions (type
`0x07` in TN3270E) are the pre-application sessions you see as the VTAM
logon screen.

When you first connect TN3270E to a mainframe, before you've selected an
application, you're in an SSCP-LU session. The VTAM logon screen (USSTAB)
is rendered through this session. After you type an application name and
press Enter, VTAM establishes an LU-LU session between your terminal LU
and the application, transitioning to DATA-3270 (`0x00`) data type.

The SYSREQ key switches between the SSCP-LU session and the active LU-LU
session — equivalent to "disconnect from application, return to VTAM menu."

---

### Subnegotiation (TN3270E)

Telnet subnegotiations (SB...SE sequences) carry structured data beyond
simple option negotiation. In TN3270E, subnegotiation performs the full
device-type and LU binding handshake. See the [Protocol Reference](PROTOCOL.md)
for the complete negotiation sequence.

---

### TCAM — Telecommunications Access Method

IBM's predecessor to VTAM, used on early System/360 and System/370 systems.
Mostly obsolete, replaced by VTAM in the late 1970s. Still referenced in
older IBM documentation.

---

### TN3270 — Telnet 3270

The protocol defined in RFC 1576 (1994). Allows a 3270 terminal emulator to
connect to a mainframe over TCP/IP using Telnet as the transport. TN3270
negotiates:
1. Binary transmission (Telnet option 0)
2. End-of-Record (Telnet option 25)
3. Terminal type (Telnet option 24) — the terminal model string

TN3270 has no concept of LU binding; the host assigns any available LU.

---

### TN3270E — TN3270 Enhanced

The protocol defined in RFC 2355 (1998). Adds to TN3270:
- **LU binding** — client can request a specific LU name
- **Device type negotiation** — structured handshake replacing raw TTYPE
- **Data type header** — 5-byte header on each record identifying data type
- **BIND image delivery** — host can deliver SNA BIND parameters
- **Response protocol** — client can send SNA positive/negative responses
- **SYSREQ support** — proper handling of the SYSREQ key for SSCP-LU switching

TN3270E is the current standard and is supported by IBM Communications Server,
Cisco CIPs, and all modern 3270 gateways.

---

### TSO — Time Sharing Option

IBM's interactive user environment on z/OS. Users log on to TSO to run ISPF,
write CLISTs/REXX, submit JCL, and manage datasets. A TSO logon creates a
TSO address space running under z/OS.

The TSO VTAM logon sequence: VTAM logon screen → type TSOPROC → VTAM sends
CINIT to TSO → TSO sends BIND → TSO sends logon panel → user enters credentials
→ TSO initializes ISPF or native TSO.

---

### USSTAB — Unformatted System Services Table

The VTAM configuration that defines the logon screen. The USSTAB is an
assembler macro that VTAM uses to generate the formatted logon screen
displayed when a user first connects (the SSCP-LU screen). It defines what
the screen looks like and how to parse the LOGON command.

The USSTAB is what you see as the "VTAM logon screen" — typically showing
the system name and a field to type the application name.

---

### VTAM — Virtual Telecommunications Access Method

IBM's networking subsystem for z/OS (and earlier MVS/ESA). VTAM:
- Manages all SNA network resources (LUs, PUs, lines)
- Handles session setup (BIND), teardown (UNBIND), and routing
- Provides the VTAM programming interface (ACBs, RPLs, exits) to applications
- Runs the SSCP function (logon screens, session management)
- Bridges TCP/IP and SNA through Communications Server (TN3270E server, FTP)

On modern z/OS, VTAM is branded as **ACF/VTAM** (Advanced Communications
Function for VTAM) and is delivered as part of **IBM Communications Server
for z/OS**.

VTAM starts and stops as a z/OS started task. Its configuration lives in
the `SYS1.VTAMLST` dataset (or equivalent). Operators interact with VTAM
through the z/OS console using MODIFY VTAM commands.

---

### VTAM Major Node

A dataset member in `SYS1.VTAMLST` (or equivalent) that defines a set of
VTAM network resources. Types of major nodes:
- **Application major node** — defines VTAM applications (APPL statements)
- **Switched major node** — defines dial-up SNA connections
- **Local SNA major node** — defines locally-attached cluster controllers
- **NCP major node** — defines the NCP (now rare)
- **Model major node** — templates for dynamic definitions

The TN3270E server creates **dynamic definitions** — it activates virtual
PUs and LUs in VTAM on-the-fly as TCP/IP connections arrive, without
pre-defined major node entries.

---

### WCC — Write Control Character

The byte immediately following a 3270 Write or Erase/Write command. Controls
what the terminal does before rendering the new data:

| Bit | Hex Mask | Meaning |
|-----|----------|---------|
| 0 (MSB) | `0x80` | (reserved) |
| 1 | `0x40` | Reset — clear MDT bits on all unprotected fields |
| 2 | `0x20` | (reserved) |
| 3 | `0x10` | (reserved) |
| 4 | `0x08` | Printer — start print operation |
| 5 | `0x04` | Alarm — sound the terminal bell |
| 6 | `0x02` | Unlock keyboard — allow operator input after write |
| 7 (LSB) | `0x01` | Restore — restore format (reset to base character set) |

A typical WCC value of `0x42` means: Reset MDT bits + Unlock keyboard.
The Reset bit (0x40) is set almost always to avoid accumulating MDT bits
across screens. Unlock (0x02) must be set or the keyboard stays locked
after the write — the operator can't type.

---

### X SYSTEM — Keyboard Inhibit

The state where the terminal keyboard is locked and operator input is
rejected. Displayed as `X SYSTEM` or `[X]` in the OIA. The keyboard
is locked between the time the host sends a screen and when it unlocks
it via the WCC unlock bit, or between a Read operation and when the host
responds.

Applications that fail to unlock the keyboard (missing WCC unlock bit)
leave users unable to type — a common bug in 3270 applications.

---

### XID — Exchange Identification

An SNA control message used during session establishment to exchange
identification information between nodes. Relevant in native SNA networking;
not directly visible in TN3270E.

---

## See Also

- [`PROTOCOL.md`](PROTOCOL.md) — TN3270E negotiation sequence step by step
- [`DATASTREAM.md`](DATASTREAM.md) — 3270 datastream structure and all orders
- [RFC 2355](https://www.rfc-editor.org/rfc/rfc2355) — TN3270E specification
- [RFC 1576](https://www.rfc-editor.org/rfc/rfc1576) — Original TN3270
- IBM GA23-0059 — 3270 Data Stream Programmer's Reference (search IBM Knowledge Center)
- IBM SC31-6082 — SNA Formats
