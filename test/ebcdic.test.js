'use strict';

/**
 * test/ebcdic.test.js
 * Basic tests for EBCDIC conversion — runnable with Node.js built-in test runner.
 *   node --test test/
 */

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const Ebcdic = require('../src/ebcdic');

describe('Ebcdic.toAscii', () => {
  it('converts a basic CP037 string', () => {
    // EBCDIC bytes for 'HELLO' in CP037
    const ebcdic = Buffer.from([0xC8, 0xC5, 0xD3, 0xD3, 0xD6]);
    assert.equal(Ebcdic.toAscii(ebcdic, 37), 'HELLO');
  });

  it('converts EBCDIC digits', () => {
    // '0'–'9' in EBCDIC CP037 are 0xF0–0xF9
    const digits = Buffer.from([0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9]);
    assert.equal(Ebcdic.toAscii(digits, 37), '0123456789');
  });

  it('replaces non-printable bytes with spaces', () => {
    const buf = Buffer.from([0x00, 0x01]); // control chars
    assert.equal(Ebcdic.toAscii(buf, 37), '  ');
  });

  it('falls back to CP037 for unknown code page', () => {
    const ebcdic = Buffer.from([0xC8, 0xC5, 0xD3, 0xD3, 0xD6]);
    assert.equal(Ebcdic.toAscii(ebcdic, 999), 'HELLO'); // 999 → fallback to 37
  });
});

describe('Ebcdic.fromAscii', () => {
  it('round-trips ASCII → EBCDIC → ASCII', () => {
    const original = 'LOGON TSO';
    const ebcdic   = Ebcdic.fromAscii(original, 37);
    const roundtrip = Ebcdic.toAscii(ebcdic, 37);
    assert.equal(roundtrip, original);
  });

  it('maps chars with no EBCDIC equivalent to 0x3F', () => {
    // ASCII 0x7F (DEL) has no standard CP037 mapping → should become 0x3F ('?')
    const buf = Ebcdic.fromAscii('\x7F', 37);
    assert.equal(buf[0], 0x3F);
  });
});

describe('Ebcdic.fromAsciiFixed', () => {
  it('pads short strings with spaces', () => {
    const buf = Ebcdic.fromAsciiFixed('HI', 5, 37);
    assert.equal(buf.length, 5);
    const back = Ebcdic.toAscii(buf, 37);
    assert.equal(back, 'HI   ');
  });

  it('truncates long strings', () => {
    const buf = Ebcdic.fromAsciiFixed('HELLO WORLD', 5, 37);
    assert.equal(buf.length, 5);
    const back = Ebcdic.toAscii(buf, 37);
    assert.equal(back, 'HELLO');
  });
});

describe('Ebcdic.registerCodepage', () => {
  it('registers and uses a custom code page', () => {
    // Create a trivial identity table (EBCDIC byte n → ASCII byte n)
    const identityTable = Buffer.from(Array.from({ length: 256 }, (_, i) => i));
    Ebcdic.registerCodepage(9999, identityTable, 'Test Identity');
    const buf = Buffer.from([65, 66, 67]); // ASCII A, B, C
    assert.equal(Ebcdic.toAscii(buf, 9999), 'ABC');
  });

  it('throws if table is not 256 bytes', () => {
    assert.throws(() => Ebcdic.registerCodepage(1234, Buffer.alloc(10)), /256 bytes/);
  });
});

describe('Ebcdic.listCodepages', () => {
  it('returns an array including CP037', () => {
    const pages = Ebcdic.listCodepages();
    const cp37  = pages.find(p => p.number === 37);
    assert.ok(cp37, 'CP037 should be listed');
    assert.ok(cp37.name.includes('CP037'));
  });
});
