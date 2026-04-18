# node-tn3270e

A complete, zero-dependency TN3270/TN3270E protocol library for Node.js.

Handles the full mainframe terminal session lifecycle — from raw TCP connection through Telnet negotiation, LU binding, 3270 datastream parsing, and EBCDIC conversion — so you can focus on building applications, not protocol archaeology.

```
npm install node-tn3270e
```

---

## Why this library exists

TN3270E and VTAM are mature, stable protocols powering thousands of mainframe installations worldwide. The practitioners who built and maintain these systems are retiring, and the knowledge lives mostly in IBM manuals from the 1980s–2000s and the heads of engineers who have been doing it for 30 years.

This library aims to be a living, documented reference implementation — not just working code, but code that explains *why* at every step, so the knowledge survives.

See [`docs/`](./docs/) for the companion glossary and protocol reference.

---

## Quick Start

```javascript
const { Tn3270Session } = require('node-tn3270e');

const session = new Tn3270Session({
  host:    '10.1.1.1',
  port:    23,
  model:   '3278-2',
  logger:  console,
});

session.on('ready', () => {
  console.log('Session live — waiting for first screen');
});

session.on('screen', data => {
  // Print the screen as plain text
  const text = data.screen.map(row => row.map(c => c.char).join('')).join('\n');
  console.log(text);
});

session.on('error',        err    => console.error(err));
session.on('disconnected', reason => console.log('Gone:', reason));

session.connect();
```

---

## API Reference

### `new Tn3270Session(options)`

Creates a new session instance. Does not connect until you call `.connect()`.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `host` | string | *(required)* | Mainframe hostname or IP |
| `port` | number | *(required)* | TCP port — typically 23, 992 (TLS), or 339 |
| `useTls` | boolean | `false` | Wrap the connection in TLS/SSL |
| `tlsOptions` | object | `{}` | Node.js `tls.connect()` options (certs, rejectUnauthorized, etc.) |
| `luName` | string | `null` | Specific LU name to request from VTAM (omit for any available LU) |
| `model` | string | `'3278-2'` | Terminal model string — determines screen dimensions |
| `codepage` | number | `37` | EBCDIC code page (37=US, 500=International, etc.) |
| `useTn3270e` | boolean | `true` | Attempt TN3270E negotiation; set `false` for z/VM or classic hosts |
| `socketTimeoutMs` | number | `120000` | Idle socket timeout in milliseconds |
| `logger` | object | no-op | Logger with `.debug()`, `.info()`, `.error()` — pass `console` for output |
| `id` | string | auto | Session identifier used in log messages |

---

### Events

| Event | Payload | Description |
|-------|---------|-------------|
| `connected` | `{ host, port }` | TCP socket is open; negotiation beginning |
| `ready` | `{ lu, model }` | TN3270E negotiation complete; session is live |
| `screen` | *(see below)* | New screen arrived from the host |
| `error` | `Error` | Protocol or socket error |
| `disconnected` | `reason: string` | Session ended (TCP close, timeout, or explicit disconnect) |
| `structuredField` | `Buffer` | Raw Write Structured Field payload (advanced use) |

#### `screen` event payload

```javascript
{
  rows:   24,          // screen height
  cols:   80,          // screen width
  cursor: 160,         // linear buffer address of cursor
  lu:     'LU3A0042', // negotiated LU name (or null)
  model:  '3278-2',
  screen: [            // 2D array — [row][col]
    [
      { char: 'L', protected: true, modified: false, fa: undefined, color: 0, highlight: 0 },
      ...
    ],
    ...
  ],
  fields: [            // parsed field list
    { startAddr: 0, fa: 0x60, protected: true,  numeric: false, modified: false, value: 'LOGON', length: 5 },
    { startAddr: 80, fa: 0x40, protected: false, numeric: false, modified: false, value: '        ', length: 8 },
    ...
  ]
}
```

---

### Methods

#### `session.connect()`

Opens the TCP (or TLS) connection and begins Telnet negotiation.

---

#### `session.disconnect([reason])`

Closes the session. Emits `disconnected` with the given reason string.

---

#### `session.sendAid(aidKey, [fields], [cursor])`

Send an AID (attention) key to the host, optionally including modified field data.

```javascript
// Press Enter with no field changes
session.sendAid('ENTER');

// Type into a field and press Enter
session.sendAid('ENTER', [{ addr: 80, value: 'LOGON IBMUSER' }]);

// Press PF3
session.sendAid('PF3');

// Press Clear
session.sendAid('CLEAR');
```

Valid `aidKey` values: `ENTER`, `CLEAR`, `PA1`, `PA2`, `PA3`, `SYSREQ`, `PF1`–`PF24`

The `fields` array contains objects: `{ addr: bufferAddress, value: asciiString }`.
The `addr` should be the address of the first character cell of the field
(one position after the field attribute byte).

---

#### `session.getScreen()`

Returns the current screen as a 2D array of cell objects.

```javascript
const rows = session.getScreen();
// rows[0][0] → { char: ' ', protected: false, modified: false, fa: 0x60, color: 0, highlight: 0 }
```

---

#### `session.getFields()`

Returns all fields on the current screen as a flat array.

```javascript
const fields = session.getFields();
const unprotected = fields.filter(f => !f.protected);
```

---

#### `session.getScreenText()`

Returns the entire screen as a plain string with rows separated by newlines.
Useful for quick screen scraping.

```javascript
const text = session.getScreenText();
if (text.includes('ENTER USERID')) { /* on logon screen */ }
```

---

#### `session.setCursor(addr)`

Set cursor to a linear buffer address.

#### `session.setCursorRC(row, col)`

Set cursor by row and column (1-based).

---

### EBCDIC Utilities

```javascript
const { Ebcdic } = require('node-tn3270e');

// EBCDIC buffer → ASCII string
const str = Ebcdic.toAscii(buffer, 37);

// ASCII string → EBCDIC Buffer
const buf = Ebcdic.fromAscii('LOGON TSO', 37);

// Fixed-length field: pad/truncate to exactly N bytes
const fixed = Ebcdic.fromAsciiFixed('HELLO', 10, 37); // 'HELLO     '

// Register a custom code page
Ebcdic.registerCodepage(273, myTableBuffer, 'CP273 Germany');

// List available code pages
const pages = Ebcdic.listCodepages();
```

---

### Constants

All protocol constants are exported for use in your own parsing or encoding:

```javascript
const { AID, CMD, ORDER, FA, COLOR, HIGHLIGHT, TELNET, OPT, TN3E, SNA } = require('node-tn3270e');

// AID bytes
AID.ENTER   // 0x7D
AID.PF3     // 0xF3
AID.CLEAR   // 0x6D

// Field attribute bits
FA.PROTECTED   // 0x20
FA.NUMERIC     // 0x10
FA.MDT         // 0x01

// 3270 Write commands
CMD.WRITE          // 0xF1
CMD.ERASE_WRITE    // 0xF5

// 3270 datastream orders
ORDER.SF   // 0x1D  Start Field
ORDER.SBA  // 0x11  Set Buffer Address
ORDER.IC   // 0x13  Insert Cursor
```

---

## Terminal Models

| Model string | Rows | Cols | Notes |
|---|---|---|---|
| `3278-2` | 24 | 80 | Most common — virtually every TSO/CICS installation |
| `3278-3` | 32 | 80 | Less common |
| `3278-4` | 43 | 80 | Less common |
| `3278-5` | 27 | 132 | 132-column wide screen — used for SDSF, ISPF split-screen |
| `IBM-DYNAMIC` | 24 | 80 | Negotiate dimensions via Query Structured Field |

---

## Common Port Guide

| Port | Protocol | Notes |
|---|---|---|
| 23 | TN3270 / TN3270E | Standard Telnet — unencrypted |
| 992 | TN3270E over TLS | IBM recommended for production |
| 339 | TN3270 / TN3270E | Alternate — common on some installations |

---

## Publishing

See [`docs/PUBLISHING.md`](./docs/PUBLISHING.md) for step-by-step instructions on releasing a new version to npm and GitHub Packages.

---

## Documentation

- [`docs/GLOSSARY.md`](./docs/GLOSSARY.md) — TN3270E and VTAM terminology reference
- [`docs/PROTOCOL.md`](./docs/PROTOCOL.md) — Protocol negotiation deep-dive
- [`docs/DATASTREAM.md`](./docs/DATASTREAM.md) — 3270 datastream structure and orders
- [`docs/PUBLISHING.md`](./docs/PUBLISHING.md) — How to publish new releases

---

## License

MIT
