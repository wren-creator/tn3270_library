# 3270 Datastream Reference

The 3270 data stream is the binary protocol that carries screen content from
host to terminal and keyboard input from terminal to host. This document
covers every command, order, and field attribute, with hex values and explanations.

---

## Outbound Datastream (Host → Terminal)

Every outbound record begins with a **command byte**, then a **WCC byte**
(for write commands), then a sequence of **data bytes** and **orders**.

### Write Commands

| Command | Hex | Name |
|---------|-----|------|
| Write | `0xF1` | Write — update screen, leave unmodified areas alone |
| Erase/Write | `0xF5` | Erase/Write — clear entire screen buffer, then write |
| Erase/Write Alt | `0x7E` | Same but uses alternate screen dimensions |
| Read Buffer | `0xF2` | Read Buffer — return full screen buffer |
| Read Modified | `0xF6` | Read Modified — return only modified fields |
| Read Modified All | `0x6E` | Return modified + trigger fields |
| Erase All Unprotected | `0x6F` | Clear all unprotected fields |
| Write Structured Field | `0xF3` | Extended commands (Query, etc.) |

> **Write vs. Erase/Write:** `Write` (`0xF1`) updates the buffer starting
> at whatever position the SBA orders direct it to. Areas not written remain
> unchanged — this allows partial screen updates. `Erase/Write` (`0xF5`)
> clears the entire buffer to EBCDIC spaces first, then applies the new data.
> Most full-screen updates use Erase/Write; partial updates (e.g., updating a
> status line) use Write.

---

### Write Control Character (WCC)

The byte immediately after a Write or Erase/Write command. Controls
terminal behavior before rendering:

```
Bit:  7  6  5  4  3  2  1  0
      │  │  │  │  │  │  │  └─ Restore (reset to base character set)
      │  │  │  │  │  │  └──── Unlock keyboard
      │  │  │  │  │  └─────── Sound alarm
      │  │  │  │  └────────── Printer start
      │  │  │  └───────────── (reserved)
      │  │  └──────────────── (reserved)
      │  └─────────────────── Reset MDT bits
      └────────────────────── (reserved)
```

Common WCC values:

| WCC | Hex | Meaning |
|-----|-----|---------|
| Reset + Unlock | `0x42` | Most common — clear MDT bits, allow typing |
| Reset + Unlock + Alarm | `0x46` | Same, but also beep |
| Unlock only | `0x02` | Update screen without resetting MDT |
| Reset only | `0x40` | Reset MDT bits, keyboard stays locked |

> **The unlock bit is critical.** If the WCC doesn't include the unlock bit
> (`0x02`), the keyboard remains locked after the write and the operator
> cannot type. This is a common bug in hand-crafted 3270 applications.

---

### Datastream Orders

Orders appear embedded in the data portion of a Write command. Values
`0x01`–`0x3F` are reserved for orders (they are non-printable in EBCDIC
so they cannot be confused with character data).

#### SF — Start Field (`0x1D`)

Places a field attribute byte at the current buffer address.

```
1D [FA]
```

The FA byte encodes field properties (see Field Attributes section below).
The FA byte occupies one buffer position (displayed as blank).
The field begins at the next position and extends until the next SF/SFE.

#### SFE — Start Field Extended (`0x29`)

Extended version of SF with multiple attribute pairs.

```
29 [count] [type1] [value1] [type2] [value2] ...
```

Attribute types:
- `0xC0` — Basic 3270 field attribute (same as SF FA byte)
- `0x41` — Extended highlighting
- `0x42` — Foreground color
- `0x43` — Character set
- `0x45` — Background color
- `0x46` — Transparency
- `0x60` — Field outlining

#### SBA — Set Buffer Address (`0x11`)

Moves the buffer address pointer to the specified position. All subsequent
data or order bytes are written starting at this address.

```
11 [addr-byte-1] [addr-byte-2]
```

Address encoding: see Buffer Address Encoding in [GLOSSARY.md](GLOSSARY.md).

Example: Move to row 5, col 10 on a 80-column screen:
- Linear address = (5-1) × 80 + (10-1) = 329 = 0x0149
- Encoded: `0x50 0xC9` (using 12-bit code table)

#### IC — Insert Cursor (`0x13`)

Places the cursor at the current buffer address after rendering.
No parameters. If multiple IC orders appear in one datastream, the
last one determines the final cursor position.

```
13
```

#### PT — Program Tab (`0x05`)

Advances the buffer address to the first character position of the next
unprotected field. Used to skip from one field to the next efficiently.

```
05
```

#### RA — Repeat to Address (`0x3C`)

Fills buffer positions from the current address up to (but not including)
the specified destination address with a single character. Efficient for
drawing separator lines, filling fields with a specific character, or
clearing a region.

```
3C [to-addr-byte-1] [to-addr-byte-2] [fill-char]
```

Example: Fill 20 positions with EBCDIC underscores:
```
11 [start-addr]          SBA to start
3C [end-addr] 6D         RA to end, fill with EBCDIC '_' (0x6D)
```

#### EUA — Erase Unprotected to Address (`0x12`)

Clears all unprotected field positions from the current address to (but
not including) the specified address. Protected fields are left unchanged.

```
12 [to-addr-byte-1] [to-addr-byte-2]
```

#### SA — Set Attribute (`0x28`)

Changes an extended attribute at the current buffer position without
defining a new field (no SF). Takes an attribute type/value pair.

```
28 [attr-type] [attr-value]
```

#### MF — Modify Field (`0x2C`)

Modifies attribute(s) of the existing field at the current buffer address.
Takes a count followed by type/value pairs.

```
2C [count] [type1] [value1] ...
```

#### GE — Graphic Escape (`0x08`)

The next byte is interpreted as a character from an alternate (graphic)
character set rather than EBCDIC. Used for line-drawing characters and
other special graphics.

```
08 [graphic-char]
```

---

## Field Attributes (FA Byte)

The FA byte following an SF order encodes all basic field properties.
The FA byte is EBCDIC-encoded (values range from `0x40` to `0xFF`).

```
Bit:  7  6  5  4  3  2  1  0
      │  │  │  │  │  │  │  └─ MDT (Modified Data Tag)
      │  │  │  │  │  └──┘
      │  │  │  │  └──────────  Intensity (2 bits, see below)
      │  │  │  └───────────── Numeric
      │  │  └──────────────── Protected
      │  └─────────────────── (always 1 in valid FA bytes)
      └────────────────────── (always 0 in valid FA bytes)
```

> The top two bits of the FA byte are always `01` in the EBCDIC encoding
> scheme used for FA bytes. This is why FA bytes always appear in the range
> `0x40`–`0x7F` and `0xC0`–`0xFF` — these are the EBCDIC "zone" ranges.

### Bit 5 — Protected

- `1` = Protected: operator cannot modify this field. The cursor skips
  over protected fields when tabbing. Data in protected fields is NOT
  sent to the host on Read Modified (unless it also has MDT set).
- `0` = Unprotected: operator can type in this field.

### Bit 4 — Numeric

- `1` = Numeric: numeric-only input (digits, period, minus, dup, field mark).
  The hardware 3270 enforced this; software emulators may not.
- `0` = Alphanumeric: any character allowed.

### Bits 3–2 — Intensity / Pen-Detectable

| Bits | Value | Display | Pen |
|------|-------|---------|-----|
| `00` | Normal | Normal intensity | No |
| `01` | Normal | Normal intensity | Yes (selector pen era) |
| `10` | High | High intensity | Yes |
| `11` | Non-display | Hidden | No |

The non-display setting (`11`) is used for password fields — the characters
are in the buffer but not visible.

### Bit 0 — MDT (Modified Data Tag)

- `1` = Modified: field will be included in Read Modified output
- `0` = Unmodified: field skipped on Read Modified

The terminal sets MDT=1 when the operator types in a field. The host can
also set MDT=1 programmatically (to force a field into the Read Modified
output even if the operator didn't change it) or reset it with WCC Reset.

### Common FA values

| FA | Hex | Description |
|----|-----|-------------|
| Protected, normal, no MDT | `0x60` | Label fields |
| Protected, high intensity | `0xE0` | Headers, titles |
| Protected, non-display | `0x6C` | Hidden data |
| Unprotected, normal | `0x40` | Input fields |
| Unprotected, high | `0xC0` | Highlighted input |
| Unprotected, numeric | `0x50` | Numeric input |
| Unprotected, MDT set | `0x41` | Pre-filled input (force transmit) |
| Protected, MDT set | `0x61` | Protected but will transmit |

---

## Extended Attributes

Extended attributes are set via SFE or SA orders using type/value pairs.

### Foreground Color (`0x42`)

| Value | Color |
|-------|-------|
| `0x00` | Default (terminal default) |
| `0xF1` | Blue |
| `0xF2` | Red |
| `0xF3` | Pink / Magenta |
| `0xF4` | Green |
| `0xF5` | Turquoise / Cyan |
| `0xF6` | Yellow |
| `0xF7` | White |
| `0xF8` | Black |
| `0xF9` | Deep Blue |
| `0xFA` | Orange |
| `0xFB` | Purple |
| `0xFC` | Pale Green |
| `0xFD` | Pale Turquoise |
| `0xFE` | Grey |

### Highlighting (`0x41`)

| Value | Effect |
|-------|--------|
| `0x00` | Default (no highlighting) |
| `0xF1` | Blink |
| `0xF2` | Reverse video |
| `0xF4` | Underscore / Underline |
| `0xF8` | Intensify |

### Field Outlining (`0x60`)

Draws visible outlines around field boundaries (box drawing). Values
are bitmasks:
- `0x01` = underline
- `0x02` = right vertical
- `0x04` = overline
- `0x08` = left vertical

---

## Inbound Datastream (Terminal → Host)

Every inbound record begins with an **AID byte**, then a **2-byte cursor
address**, then optionally **field data** for all modified fields.

### Format

```
[AID] [cursor-addr-byte-1] [cursor-addr-byte-2] [field-data...] IAC EOR
```

### Field Data Format

For each modified field:

```
11 [field-addr-byte-1] [field-addr-byte-2] [EBCDIC field content]
│
└── SBA order (0x11)
```

The field data is a series of SBA+data blocks, one per modified field,
in buffer-address order (low address first).

### AID-Only Records

PA keys (PA1, PA2, PA3) send only the AID and cursor address, with no
field data:

```
[AID-PA] [cursor-addr-byte-1] [cursor-addr-byte-2] IAC EOR
```

### CLEAR Key

The CLEAR key sends only the AID byte, no cursor address, no field data:

```
6D IAC EOR
```

After CLEAR, the host typically sends an Erase/Write to render a new screen.

---

## Buffer Address Encoding

The 3270 buffer address encoding uses a 6-bit code table. This encoding
dates to the 1960s when 3270 hardware transmitted addresses as EBCDIC
characters over synchronous (SDLC) lines.

### Encoding table

Index (6-bit value) → Code byte transmitted:

```
 0 → 0x40    16 → 0x50    32 → 0x60    48 → 0xF0
 1 → 0xC1    17 → 0xD1    33 → 0x61    49 → 0xF1
 2 → 0xC2    18 → 0xD2    34 → 0xE2    50 → 0xF2
 3 → 0xC3    19 → 0xD3    35 → 0xE3    51 → 0xF3
 4 → 0xC4    20 → 0xD4    36 → 0xE4    52 → 0xF4
 5 → 0xC5    21 → 0xD5    37 → 0xE5    53 → 0xF5
 6 → 0xC6    22 → 0xD6    38 → 0xE6    54 → 0xF6
 7 → 0xC7    23 → 0xD7    39 → 0xE7    55 → 0xF7
 8 → 0xC8    24 → 0xD8    40 → 0xE8    56 → 0xF8
 9 → 0xC9    25 → 0xD9    41 → 0xE9    57 → 0xF9
10 → 0x4A    26 → 0x5A    42 → 0x6A    58 → 0x7A
11 → 0x4B    27 → 0x5B    43 → 0x6B    59 → 0x7B
12 → 0x4C    28 → 0x5C    44 → 0x6C    60 → 0x7C
13 → 0x4D    29 → 0x5D    45 → 0x6D    61 → 0x7D
14 → 0x4E    30 → 0x5E    46 → 0x6E    62 → 0x7E
15 → 0x4F    31 → 0x5F    47 → 0x6F    63 → 0x7F
```

### Decoding algorithm

```javascript
function decodeAddr(b1, b2) {
  const type = (b1 & 0xC0) >> 6;
  if (type === 0x00 || type === 0x03) {
    // 14-bit binary mode
    return ((b1 & 0x3F) << 8) | b2;
  }
  // 12-bit code table mode
  return ((b1 & 0x3F) << 6) | (b2 & 0x3F);
}
```

### Common addresses (80-column, 24-row screen)

| Row | Col | Linear | Encoded (12-bit) |
|-----|-----|--------|-------------------|
| 1 | 1 | 0 | `0x40 0x40` |
| 1 | 2 | 1 | `0x40 0xC1` |
| 2 | 1 | 80 | `0x41 0x50` |
| 12 | 1 | 880 | `0x4D 0x70` |
| 24 | 80 | 1919 | `0x5D 0x7F` |

---

## Screen Model Dimensions

| Model | Rows | Cols | Total Cells | Notes |
|-------|------|------|-------------|-------|
| 3278-2 | 24 | 80 | 1,920 | Standard — almost universal |
| 3278-3 | 32 | 80 | 2,560 | Uncommon |
| 3278-4 | 43 | 80 | 3,440 | Uncommon |
| 3278-5 | 27 | 132 | 3,564 | Wide screen — SDSF, ISPF split |
| IBM-DYNAMIC | negotiated | negotiated | — | Dynamic resize via Query |

### Model string format

During TTYPE or TN3270E device-type negotiation, the model string sent is:

```
IBM-3278-2     (standard 80×24)
IBM-3278-5     (wide 132×27)
IBM-3279-2     (color — same dimensions as 3278-2)
IBM-3279-5     (color wide)
```

The `3278` series is monochrome; `3279` is color. Modern installations
accept either name; the color capability is negotiated separately via
Query Structured Field.

---

## Write Structured Field (`0xF3`)

The Write Structured Field command carries extended protocol messages
that don't fit in the basic 3270 datastream. Each structured field has:

```
[WSF command 0xF3]
[length-high] [length-low]   ← 2-byte length (includes these 2 bytes)
[SF-ID]                      ← structured field type identifier
[SF-data...]
```

### Common structured field IDs

| ID | Hex | Name |
|----|-----|------|
| Query | `0x01` | Query terminal capabilities |
| Query Reply | `0x81` | Terminal answers Query |
| Read Partition | `0x01` | (same space, context-dependent) |
| Outbound 3270DS | `0x40` | 3270 data stream in partitioned mode |
| Set Reply Mode | `0x09` | Configure what attributes are returned |
| Erase/Reset | `0x03` | Extended erase |

### Query / Query Reply sequence

If the host supports it, it sends a WSF Query to discover terminal
capabilities. The terminal responds with Query Reply structured fields
describing its features:

- **Summary** (`0x80`) — list of supported query types
- **Usable Area** (`0x81`) — screen dimensions in pixels and characters
- **Color** (`0x86`) — supported color pairs
- **Highlight** (`0x87`) — supported highlight types
- **Implicit Partition** (`0xA6`) — default partition size

This is how IBM-DYNAMIC model sessions negotiate actual screen dimensions.

---

## Practical Parsing Notes

### Parsing order is critical

When parsing an inbound record, the AID byte determines what follows:
- `0x6D` (CLEAR): nothing follows, record ends
- `0x60` (No AID, Read Buffer response): SBA + full buffer data
- Any other AID: 2-byte cursor address, then SBA+data blocks for each modified field

### EBCDIC space vs. null

Unmodified field positions contain `0x40` (EBCDIC space), not `0x00`.
Comparing to `0x00` to detect "empty" will fail — always compare to `0x40`.

### Field boundary wrapping

The screen buffer is logically circular. A field can wrap from the last
position on the screen (row 24, col 80 on a 3278-2) back to position 0
(row 1, col 1). Parsers must handle modular arithmetic on buffer addresses.

### The FA position

The FA byte position is NOT part of the field content. When building
inbound data, the field address in the SBA before the field data should
be the position of the **first content character**, not the FA byte.
The FA byte is at (fieldStart), the first content byte is at (fieldStart + 1).

---

## See Also

- [`GLOSSARY.md`](GLOSSARY.md) — Term definitions including AID, WCC, MDT, LU
- [`PROTOCOL.md`](PROTOCOL.md) — TN3270E negotiation sequence
- RFC 2355 — TN3270E specification
- IBM GA23-0059 — 3270 Data Stream Programmer's Reference
