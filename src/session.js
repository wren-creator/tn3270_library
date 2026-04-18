'use strict';

/**
 * src/session.js
 * ─────────────────────────────────────────────────────────────────────────────
 * Tn3270Session — a self-contained TN3270/TN3270E client session.
 *
 * Handles the full protocol lifecycle:
 *   1. TCP (or TLS) connection
 *   2. Telnet option negotiation (BINARY, EOR, TTYPE, TN3270E)
 *   3. TN3270E sub-negotiation and LU binding (RFC 2355)
 *   4. 3270 datastream parsing → screen buffer model
 *   5. Inbound (host→terminal) data: Write, Erase/Write, orders
 *   6. Outbound (terminal→host) data: AID keys, field data, cursor address
 *
 * Usage:
 *   const { Tn3270Session } = require('node-tn3270e');
 *
 *   const session = new Tn3270Session({
 *     host: '10.1.1.1',
 *     port: 23,
 *     model: '3278-2',
 *   });
 *
 *   session.on('screen', data => console.log(data.fields));
 *   session.on('connected', () => console.log('TCP open, negotiating...'));
 *   session.on('ready', () => console.log('Session live, screen incoming'));
 *   session.on('disconnected', reason => console.log('Gone:', reason));
 *   session.on('error', err => console.error(err));
 *
 *   session.connect();
 *
 * Protocol references:
 *   RFC 854   — Telnet Protocol Specification
 *   RFC 856   — Telnet Binary Transmission
 *   RFC 885   — Telnet End of Record Option
 *   RFC 1091  — Telnet Terminal Type Option
 *   RFC 1576  — TN3270 Current Practices
 *   RFC 2355  — TN3270 Enhancements (TN3270E)
 *   IBM GA23-0059 — 3270 Data Stream Programmer's Reference
 * ─────────────────────────────────────────────────────────────────────────────
 */

const net = require('net');
const tls = require('tls');
const { EventEmitter } = require('events');

const Ebcdic = require('./ebcdic');
const {
  TELNET, OPT, TN3E, CMD, ORDER, AID, FA, WCC, MODEL_DIMENSIONS, BUF_ADDR_CODE,
} = require('./constants');

const { IAC, DONT, DO, WONT, WILL, SB, SE, EOR } = TELNET;
const {
  SF, SFE, SBA, SA, MF, IC, PT, RA, EUA,
} = ORDER;

// ── Helpers ───────────────────────────────────────────────────────────────

function modelDimensions(model) {
  return MODEL_DIMENSIONS[model] || MODEL_DIMENSIONS['3278-2'];
}

function newBuffer(rows, cols) {
  return Array.from({ length: rows * cols }, () => ({
    char: 0x40,       // EBCDIC space
    fa: undefined,    // Field Attribute byte (undefined = not an FA position)
    color: 0,
    highlight: 0,
    modified: false,
  }));
}

/** Decode a 3270 buffer address from two bytes into a linear offset. */
function decodeAddr(b1, b2) {
  // 3270 uses a 6-bit encoding; the top 2 bits of each byte identify the
  // encoding type (00=14-bit, 01/11=12-bit code table).
  const type = (b1 & 0xC0) >> 6;
  if (type === 0x00 || type === 0x03) {
    // 14-bit binary: bottom 6 bits of b1 + all 8 bits of b2
    return ((b1 & 0x3F) << 8) | b2;
  }
  // 12-bit code table: bottom 6 bits of each byte
  return ((b1 & 0x3F) << 6) | (b2 & 0x3F);
}

/** Encode a linear buffer address into two 3270 address bytes. */
function encodeAddr(addr) {
  return [BUF_ADDR_CODE[(addr >> 6) & 0x3F], BUF_ADDR_CODE[addr & 0x3F]];
}

/** Debug name for a Telnet command byte. */
function cmdName(b) {
  return { [DO]: 'DO', [DONT]: 'DONT', [WILL]: 'WILL', [WONT]: 'WONT' }[b] || `0x${b.toString(16)}`;
}

/** Debug name for a Telnet option byte. */
function optName(b) {
  return {
    [OPT.BINARY]: 'BINARY', [OPT.EOR]: 'EOR',
    [OPT.TTYPE]: 'TTYPE',  [OPT.TN3270E]: 'TN3270E',
  }[b] || `0x${b.toString(16)}`;
}

// ── Tn3270Session ─────────────────────────────────────────────────────────

class Tn3270Session extends EventEmitter {

  /**
   * @param {object}  opts
   * @param {string}  opts.host           - Mainframe hostname or IP
   * @param {number}  opts.port           - TCP port (typically 23, 992, or 339)
   * @param {boolean} [opts.useTls=false] - Wrap connection in TLS
   * @param {object}  [opts.tlsOptions]   - Node.js tls.connect() options
   * @param {string}  [opts.luName]       - Specific LU name to request (or omit for any)
   * @param {string}  [opts.model='3278-2'] - Terminal model string
   * @param {number}  [opts.codepage=37]  - EBCDIC code page number
   * @param {boolean} [opts.useTn3270e=true] - Attempt TN3270E negotiation (set false for z/VM)
   * @param {number}  [opts.socketTimeoutMs=120000] - Idle socket timeout in ms
   * @param {object}  [opts.logger]       - Logger object with .debug/.info/.error methods
   * @param {string}  [opts.id]           - Optional session identifier for log messages
   */
  constructor(opts = {}) {
    super();

    this.id       = opts.id     || `sess-${Math.random().toString(36).slice(2, 7)}`;
    this.host     = opts.host;
    this.port     = opts.port;
    this.useTls   = opts.useTls   ?? false;
    this.tlsOpts  = opts.tlsOptions || {};
    this.luName   = opts.luName   || null;
    this.codepage = opts.codepage || 37;
    this.socketTimeoutMs = opts.socketTimeoutMs ?? 120_000;

    // TN3270E flag — set false for z/VM or hosts that don't support TN3270E
    this.useTn3270e = opts.useTn3270e ?? true;

    // Logger — defaults to a no-op; pass console or a winston/pino instance
    this._log = opts.logger || {
      debug: () => {},
      info:  () => {},
      error: () => {},
    };

    // Apply model — sets this.rows, this.cols
    this._applyModel(opts.model || '3278-2');

    // Screen buffer
    this.buffer     = newBuffer(this.rows, this.cols);
    this.cursorAddr = 0;

    // Telnet / TN3270E state
    this.tn3270eEnabled = false;
    this.negotiatedLu   = null;

    // Byte accumulation
    this.recvBuf       = Buffer.alloc(0);
    this._currentRecord = null;

    // Connection state
    this.socket     = null;
    this._destroyed = false;
  }

  // ── Public API ─────────────────────────────────────────────────────────

  /**
   * Open the TCP (or TLS) connection and begin Telnet negotiation.
   * Emits 'connected' when the socket is open, 'ready' when the 3270
   * session is fully negotiated and the first screen is expected.
   */
  connect() {
    if (this._destroyed) throw new Error('Session has been destroyed; create a new instance');

    const connectFn = this.useTls ? tls.connect : net.connect;
    const opts = {
      host: this.host,
      port: this.port,
      ...(this.useTls ? this.tlsOpts : {}),
    };

    this._log.info(`[${this.id}] Connecting → ${this.host}:${this.port} tls=${this.useTls} tn3270e=${this.useTn3270e}`);

    this.socket = connectFn(opts, () => {
      this._log.debug(`[${this.id}] TCP socket open`);
      this.emit('connected', { host: this.host, port: this.port });
      // Host initiates negotiation; we wait for DO TN3270E / DO TTYPE etc.
    });

    this.socket.on('data',    chunk => this._onData(chunk));
    this.socket.on('error',   err   => { this.emit('error', err); this._cleanup(); });
    this.socket.on('close',   ()    => { this.emit('disconnected', 'tcp-close'); this._cleanup(); });
    this.socket.setTimeout(this.socketTimeoutMs, () => {
      this.emit('error', new Error('Socket timeout'));
      this._cleanup();
    });
  }

  /**
   * Close the session gracefully.
   * @param {string} [reason='client'] - Reason string included in 'disconnected' event
   */
  disconnect(reason = 'client') {
    if (this._destroyed) return;
    this._destroyed = true;
    this._log.info(`[${this.id}] Disconnect: ${reason}`);
    this._cleanup();
    this.emit('disconnected', reason);
  }

  /**
   * Send an AID key with optional field data.
   *
   * @param {string}  aidKey       - Key name from the AID constants (e.g. 'ENTER', 'PF3', 'PA1')
   * @param {Array}   [fields=[]]  - Modified fields to include: [{addr, value}]
   * @param {number}  [cursor]     - Cursor address to report (defaults to this.cursorAddr)
   *
   * Example — press Enter:
   *   session.sendAid('ENTER');
   *
   * Example — type in a field and press Enter:
   *   session.sendAid('ENTER', [{ addr: 80, value: 'LOGON TSO' }]);
   *
   * Example — press PF3:
   *   session.sendAid('PF3');
   */
  sendAid(aidKey, fields = [], cursor) {
    const aidByte = AID[aidKey.toUpperCase()];
    if (aidByte === undefined) throw new Error(`Unknown AID key: ${aidKey}`);

    const curAddr = cursor ?? this.cursorAddr;
    const [ca1, ca2] = encodeAddr(curAddr);
    const bytes = [aidByte, ca1, ca2];

    for (const field of fields) {
      const [fa1, fa2] = encodeAddr(field.addr);
      bytes.push(0x11, fa1, fa2); // SBA order
      const ebcdicVal = Ebcdic.fromAscii(String(field.value), this.codepage);
      bytes.push(...ebcdicVal);
    }

    // In TN3270E mode, prefix with 5-byte header: [DATA_3270, 0, 0, seq-hi, seq-lo]
    const payload = this.tn3270eEnabled
      ? Buffer.from([0x00, 0x00, 0x00, 0x00, 0x00, ...bytes, IAC, EOR])
      : Buffer.from([...bytes, IAC, EOR]);

    this._send(payload);
    this._log.debug(`[${this.id}] sendAid ${aidKey} cursor=${curAddr} fields=${fields.length}`);
  }

  /**
   * Return the current screen as a 2D array of cell objects.
   * Each cell: { char: string, fa: number|undefined, protected: boolean,
   *              modified: boolean, color: number, highlight: number }
   *
   * @returns {Array<Array<object>>}
   */
  getScreen() {
    const rows = [];
    for (let r = 0; r < this.rows; r++) {
      const cells = [];
      for (let c = 0; c < this.cols; c++) {
        const cell = this.buffer[r * this.cols + c];
        cells.push({
          char: cell.fa !== undefined ? ' '
              : Ebcdic.toAscii(Buffer.from([cell.char]), this.codepage),
          fa:        cell.fa,
          protected: !!(cell.fa !== undefined && (cell.fa & FA.PROTECTED)),
          modified:  cell.modified,
          color:     cell.color,
          highlight: cell.highlight,
        });
      }
      rows.push(cells);
    }
    return rows;
  }

  /**
   * Return all fields on the current screen.
   * A field begins at each SF/SFE order position and ends just before the next.
   *
   * @returns {Array<object>} Each field: { startAddr, fa, protected, numeric,
   *                                        modified, value, length }
   */
  getFields() {
    const fields = [];
    let currentField = null;

    for (let a = 0; a < this.buffer.length; a++) {
      const cell = this.buffer[a];
      if (!cell) continue;

      if (cell.fa !== undefined) {
        if (currentField) {
          currentField.length = a - currentField.startAddr - 1;
          fields.push(currentField);
        }
        currentField = {
          startAddr: a,
          fa:        cell.fa,
          protected: !!(cell.fa & FA.PROTECTED),
          numeric:   !!(cell.fa & FA.NUMERIC),
          modified:  !!(cell.fa & FA.MDT),
          value:     '',
          length:    0,
        };
      } else if (currentField) {
        currentField.value += Ebcdic.toAscii(Buffer.from([cell.char]), this.codepage);
      }
    }

    if (currentField) {
      currentField.length = this.buffer.length - currentField.startAddr - 1;
      fields.push(currentField);
    }

    return fields;
  }

  /**
   * Return the text content of the screen as a plain string,
   * with rows separated by newlines.  Useful for simple screen scraping.
   *
   * @returns {string}
   */
  getScreenText() {
    const rows = this.getScreen();
    return rows.map(row => row.map(c => c.char).join('')).join('\n');
  }

  /**
   * Set cursor position.
   * @param {number} addr - Linear buffer address
   */
  setCursor(addr) {
    this.cursorAddr = Math.max(0, Math.min(addr, this.rows * this.cols - 1));
  }

  /**
   * Set cursor by row and column (1-based).
   * @param {number} row - Row number (1–this.rows)
   * @param {number} col - Column number (1–this.cols)
   */
  setCursorRC(row, col) {
    this.setCursor((row - 1) * this.cols + (col - 1));
  }

  // ── Internal: socket and data flow ────────────────────────────────────

  _cleanup() {
    if (this.socket) {
      try { this.socket.destroy(); } catch (_) {}
      this.socket = null;
    }
  }

  _send(buf) {
    if (this.socket && !this._destroyed) {
      this.socket.write(buf);
    }
  }

  _onData(chunk) {
    this.recvBuf = Buffer.concat([this.recvBuf, chunk]);
    this._parseTelnet();
  }

  // ── Telnet stream parser ───────────────────────────────────────────────
  // Walks the receive buffer byte by byte, extracting Telnet commands,
  // subnegotiations, and raw 3270 data records (terminated by IAC EOR).

  _parseTelnet() {
    let i = 0;
    while (i < this.recvBuf.length) {
      const b = this.recvBuf[i];

      // ── Regular data byte — accumulate into current 3270 record
      if (b !== IAC) {
        this._accumRecord(b);
        i++;
        continue;
      }

      // ── IAC — next byte is a command
      if (i + 1 >= this.recvBuf.length) break; // wait for more data

      const cmd = this.recvBuf[i + 1];

      // IAC IAC — escaped 0xFF data byte
      if (cmd === IAC) {
        this._accumRecord(IAC);
        i += 2;
        continue;
      }

      // IAC EOR — end of a 3270 data record
      if (cmd === EOR) {
        if (this._currentRecord && this._currentRecord.length > 0) {
          this._onRecord(Buffer.from(this._currentRecord));
          this._currentRecord = [];
        }
        i += 2;
        continue;
      }

      // IAC SB — subnegotiation
      if (cmd === SB) {
        const sePos = this._findSE(i + 2);
        if (sePos === -1) break; // wait for SE
        const payload = this.recvBuf.slice(i + 2, sePos);
        this._handleSubneg(payload);
        i = sePos + 2;
        continue;
      }

      // IAC DO/DONT/WILL/WONT <opt> — 3-byte option sequence
      if (cmd === DO || cmd === DONT || cmd === WILL || cmd === WONT) {
        if (i + 2 >= this.recvBuf.length) break;
        this._handleTelnetOption(cmd, this.recvBuf[i + 2]);
        i += 3;
        continue;
      }

      // Other 2-byte IAC commands (NOP, etc.)
      i += 2;
    }

    this.recvBuf = this.recvBuf.slice(i);
  }

  _accumRecord(byte) {
    if (!this._currentRecord) this._currentRecord = [];
    this._currentRecord.push(byte);
  }

  _findSE(start) {
    for (let i = start; i < this.recvBuf.length - 1; i++) {
      if (this.recvBuf[i] === IAC && this.recvBuf[i + 1] === SE) return i;
    }
    return -1;
  }

  // ── Telnet option negotiation ──────────────────────────────────────────

  _handleTelnetOption(cmd, opt) {
    this._log.debug(`[${this.id}] Telnet ${cmdName(cmd)} ${optName(opt)}`);

    // Terminal Type — host asks what model we are
    if (opt === OPT.TTYPE) {
      if (cmd === DO) this._send(Buffer.from([IAC, WILL, OPT.TTYPE]));
      return;
    }

    // TN3270E — attempt enhanced protocol or fall back to classic TN3270
    if (opt === OPT.TN3270E) {
      if (cmd === DO) {
        if (!this.useTn3270e) {
          this._log.info(`[${this.id}] TN3270E disabled — sending WONT`);
          this._send(Buffer.from([IAC, WONT, OPT.TN3270E]));
          this._initClassicTn3270();
        } else {
          this.tn3270eEnabled = true;
          this._send(Buffer.from([IAC, WILL, OPT.TN3270E]));
          this._sendDeviceType();
        }
      } else if (cmd === DONT) {
        this._send(Buffer.from([IAC, WONT, OPT.TN3270E]));
        this._initClassicTn3270();
      }
      return;
    }

    // BINARY mode — required for raw 3270 datastream
    if (opt === OPT.BINARY) {
      if (cmd === DO)   this._send(Buffer.from([IAC, WILL, OPT.BINARY]));
      if (cmd === WILL) this._send(Buffer.from([IAC, DO,   OPT.BINARY]));
      return;
    }

    // EOR — required to delimit 3270 records
    if (opt === OPT.EOR) {
      if (cmd === DO)   this._send(Buffer.from([IAC, WILL, OPT.EOR]));
      if (cmd === WILL) this._send(Buffer.from([IAC, DO,   OPT.EOR]));
      return;
    }

    // Unknown option — refuse
    if (cmd === DO)   this._send(Buffer.from([IAC, WONT, opt]));
    if (cmd === WILL) this._send(Buffer.from([IAC, DONT, opt]));
  }

  // ── Classic TN3270 fallback ────────────────────────────────────────────
  // When TN3270E is not available (z/VM, older hosts), we negotiate
  // BINARY + EOR + TTYPE and send our model string on TTYPE SB SEND.

  _initClassicTn3270() {
    this._log.info(`[${this.id}] Classic TN3270 mode`);
    this._send(Buffer.from([IAC, DO, OPT.BINARY]));
    this._send(Buffer.from([IAC, DO, OPT.EOR]));
  }

  // ── TN3270E subnegotiation ────────────────────────────────────────────

  _sendDeviceType() {
    // SB TN3270E SEND DEVICE-TYPE SE
    this._send(Buffer.from([IAC, SB, OPT.TN3270E, TN3E.SEND, TN3E.DEVICE_TYPE, IAC, SE]));
  }

  _handleSubneg(payload) {
    if (payload.length < 2) return;
    const opt  = payload[0];
    const func = payload[1];

    // ── TTYPE subneg: host sends SB TTYPE SEND SE; we reply with our model
    if (opt === OPT.TTYPE) {
      const modelStr = `IBM-${this.model}`;
      const reply = Buffer.concat([
        Buffer.from([IAC, SB, OPT.TTYPE, 0x00]), // 0x00 = IS
        Buffer.from(modelStr, 'ascii'),
        Buffer.from([IAC, SE]),
      ]);
      this._send(reply);
      this._log.debug(`[${this.id}] TTYPE IS ${modelStr}`);
      return;
    }

    if (opt !== OPT.TN3270E) return;

    // ── TN3270E SEND DEVICE-TYPE → we reply with DEVICE-TYPE REQUEST
    if (func === TN3E.SEND && payload[2] === TN3E.DEVICE_TYPE) {
      const deviceType = `IBM-${this.model}`;
      const parts = [IAC, SB, OPT.TN3270E, TN3E.DEVICE_TYPE, TN3E.REQUEST];
      parts.push(...Buffer.from(deviceType, 'ascii'));
      if (this.luName) {
        parts.push(TN3E.CONNECT);
        parts.push(...Buffer.from(this.luName, 'ascii'));
      }
      parts.push(IAC, SE);
      this._send(Buffer.from(parts));
      this._log.debug(`[${this.id}] TN3270E DEVICE-TYPE REQUEST ${deviceType}${this.luName ? ' CONNECT ' + this.luName : ''}`);
      return;
    }

    // ── TN3270E DEVICE-TYPE IS → extract negotiated LU name
    if (func === TN3E.DEVICE_TYPE && payload[2] === TN3E.IS) {
      const rest = payload.slice(3);
      const connIdx = rest.indexOf(TN3E.CONNECT);
      if (connIdx !== -1) {
        this.negotiatedLu = rest.slice(connIdx + 1).toString('ascii');
        this._log.info(`[${this.id}] TN3270E: LU bound = ${this.negotiatedLu}`);
      }
      return;
    }

    // ── TN3270E DEVICE-TYPE REJECT → fall back or emit error
    if (func === TN3E.DEVICE_TYPE && payload[2] === TN3E.REJECT) {
      const reasonCode = payload[payload.indexOf(TN3E.REASON) + 1];
      this._log.error(`[${this.id}] TN3270E REJECT reason=0x${(reasonCode || 0).toString(16)}`);
      this.emit('error', new Error(`TN3270E device-type rejected (reason 0x${(reasonCode || 0).toString(16)})`));
      return;
    }

    // ── TN3270E FUNCTIONS REQUEST → respond with FUNCTIONS IS (echo back)
    if (func === TN3E.FUNCTIONS && payload[2] === TN3E.REQUEST) {
      const funcList = payload.slice(3);
      const reply = Buffer.from([IAC, SB, OPT.TN3270E, TN3E.FUNCTIONS, TN3E.IS, ...funcList, IAC, SE]);
      this._send(reply);
      this._log.info(`[${this.id}] TN3270E FUNCTIONS IS — session live`);
      this.emit('ready', { lu: this.negotiatedLu, model: this.model });
      return;
    }
  }

  // ── 3270 Record handler ────────────────────────────────────────────────

  _onRecord(record) {
    let data = record;

    // Strip TN3270E 5-byte header if in enhanced mode
    if (this.tn3270eEnabled && record.length >= 5) {
      const dataType = record[0];
      // 0x00 = DATA-3270, 0x07 = SSCP-LU (logon screen before BIND)
      if (dataType !== 0x00 && dataType !== 0x07) {
        this._log.debug(`[${this.id}] TN3270E non-data record type=0x${dataType.toString(16)} — skipped`);
        return;
      }
      data = record.slice(5);
    }

    if (data.length === 0) return;

    this._parse3270(data);
  }

  // ── 3270 Datastream Parser ────────────────────────────────────────────

  _parse3270(data) {
    if (data.length < 1) return;
    const cmd = data[0];

    if (cmd === CMD.WRITE || cmd === CMD.ERASE_WRITE || cmd === CMD.ERASE_WRITE_ALT) {
      if (cmd === CMD.ERASE_WRITE || cmd === CMD.ERASE_WRITE_ALT) {
        this.buffer = newBuffer(this.rows, this.cols);
        this.cursorAddr = 0;
      }
      if (data.length < 2) return;
      const wcc = data[1];
      if (wcc & WCC.RESET) this._resetMdt();
      this._processOrders(data, 2);
      this._emitScreen();
      return;
    }

    if (cmd === CMD.ERASE_ALL_UNPROTECTED) {
      this._eraseUnprotected();
      this._emitScreen();
      return;
    }

    // Write Structured Field — extended feature; emit raw for consumers to handle
    if (cmd === CMD.WRITE_STRUCTURED_FIELD) {
      this.emit('structuredField', data.slice(1));
      return;
    }

    this._log.debug(`[${this.id}] Unknown 3270 command 0x${cmd.toString(16)}`);
  }

  _processOrders(data, start) {
    let bufAddr = 0;
    let i = start;

    while (i < data.length) {
      const b = data[i];

      // ── Set Buffer Address ───────────────────────────────────────
      if (b === SBA) {
        if (i + 2 >= data.length) break;
        bufAddr = decodeAddr(data[i + 1], data[i + 2]) % this.buffer.length;
        i += 3;
        continue;
      }

      // ── Start Field ──────────────────────────────────────────────
      if (b === SF) {
        if (i + 1 >= data.length) break;
        const faAttr = data[i + 1];
        if (this.buffer[bufAddr]) {
          this.buffer[bufAddr].fa   = faAttr;
          this.buffer[bufAddr].char = 0x40;
        }
        bufAddr = (bufAddr + 1) % this.buffer.length;
        i += 2;
        continue;
      }

      // ── Start Field Extended ─────────────────────────────────────
      if (b === SFE) {
        if (i + 1 >= data.length) break;
        const pairCount = data[i + 1];
        let faAttr = 0;
        let color = 0;
        let highlight = 0;
        for (let p = 0; p < pairCount; p++) {
          const attrType = data[i + 2 + p * 2];
          const attrVal  = data[i + 3 + p * 2];
          if (attrType === 0xC0) faAttr    = attrVal;
          if (attrType === 0x42) color     = attrVal;
          if (attrType === 0x41) highlight = attrVal;
        }
        if (this.buffer[bufAddr]) {
          this.buffer[bufAddr].fa        = faAttr;
          this.buffer[bufAddr].char      = 0x40;
          this.buffer[bufAddr].color     = color;
          this.buffer[bufAddr].highlight = highlight;
        }
        bufAddr = (bufAddr + 1) % this.buffer.length;
        i += 2 + pairCount * 2;
        continue;
      }

      // ── Insert Cursor ────────────────────────────────────────────
      if (b === IC) {
        this.cursorAddr = bufAddr;
        i++;
        continue;
      }

      // ── Repeat to Address ────────────────────────────────────────
      if (b === RA) {
        if (i + 3 >= data.length) break;
        const toAddr = decodeAddr(data[i + 1], data[i + 2]) % this.buffer.length;
        const fillChar = data[i + 3];
        while (bufAddr !== toAddr) {
          if (this.buffer[bufAddr]) this.buffer[bufAddr].char = fillChar;
          bufAddr = (bufAddr + 1) % this.buffer.length;
        }
        i += 4;
        continue;
      }

      // ── Erase Unprotected to Address ─────────────────────────────
      if (b === EUA) {
        if (i + 2 >= data.length) break;
        const toAddr = decodeAddr(data[i + 1], data[i + 2]) % this.buffer.length;
        let a = bufAddr;
        while (a !== toAddr) {
          const cell = this.buffer[a];
          if (cell && cell.fa === undefined && !(cell.fa & FA.PROTECTED)) {
            cell.char = 0x40;
          }
          a = (a + 1) % this.buffer.length;
        }
        i += 3;
        continue;
      }

      // ── Set Attribute / Modify Field ─────────────────────────────
      if (b === SA || b === MF) {
        i += 3; // [order, attrType, attrVal] — skip for now
        continue;
      }

      // ── Program Tab ──────────────────────────────────────────────
      if (b === PT) {
        // Advance to next unprotected field
        let a = bufAddr;
        while (a < this.buffer.length) {
          const cell = this.buffer[a];
          if (cell && cell.fa !== undefined && !(cell.fa & FA.PROTECTED)) {
            bufAddr = a + 1;
            break;
          }
          a++;
        }
        i++;
        continue;
      }

      // ── Graphic Escape ───────────────────────────────────────────
      if (b === ORDER.GE) {
        i += 2; // skip the graphics char
        continue;
      }

      // ── Printable character — write to buffer ────────────────────
      if (this.buffer[bufAddr]) {
        this.buffer[bufAddr].char = b;
      }
      bufAddr = (bufAddr + 1) % this.buffer.length;
      i++;
    }
  }

  _resetMdt() {
    for (const cell of this.buffer) {
      if (cell) cell.modified = false;
    }
  }

  _eraseUnprotected() {
    for (const cell of this.buffer) {
      if (cell && cell.fa === undefined) {
        // Find the preceding FA to check protection
        cell.char = 0x40;
      }
    }
  }

  _emitScreen() {
    this.emit('screen', {
      rows:       this.rows,
      cols:       this.cols,
      cursor:     this.cursorAddr,
      lu:         this.negotiatedLu,
      model:      this.model,
      screen:     this.getScreen(),
      fields:     this.getFields(),
    });
  }

  _applyModel(model) {
    this.model = model;
    const dims = modelDimensions(model);
    this.rows  = dims.rows;
    this.cols  = dims.cols;
    this.buffer = newBuffer(this.rows, this.cols);
    this.cursorAddr = 0;
  }
}

module.exports = { Tn3270Session };
