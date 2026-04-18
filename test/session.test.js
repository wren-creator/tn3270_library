'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { Tn3270Session, AID, CMD, ORDER, FA, MODEL_DIMENSIONS, constants } = require('../src/index');

describe('constants', () => {
  it('AID.ENTER is 0x7D', () => assert.equal(AID.ENTER, 0x7D));
  it('AID.CLEAR is 0x6D', () => assert.equal(AID.CLEAR, 0x6D));
  it('AID.PF1 is 0xF1',  () => assert.equal(AID.PF1,   0xF1));
  it('AID.PF24 is 0x4C', () => assert.equal(AID.PF24,  0x4C));

  it('CMD.ERASE_WRITE is 0xF5',  () => assert.equal(CMD.ERASE_WRITE, 0xF5));
  it('CMD.WRITE is 0xF1',        () => assert.equal(CMD.WRITE,       0xF1));

  it('ORDER.SF is 0x1D',  () => assert.equal(ORDER.SF,  0x1D));
  it('ORDER.SBA is 0x11', () => assert.equal(ORDER.SBA, 0x11));
  it('ORDER.IC is 0x13',  () => assert.equal(ORDER.IC,  0x13));

  it('FA.PROTECTED is 0x20', () => assert.equal(FA.PROTECTED, 0x20));
  it('FA.MDT is 0x01',       () => assert.equal(FA.MDT,       0x01));

  it('3278-2 is 24×80',  () => {
    assert.deepEqual(MODEL_DIMENSIONS['3278-2'], { rows: 24, cols: 80 });
  });
  it('3278-5 is 27×132', () => {
    assert.deepEqual(MODEL_DIMENSIONS['3278-5'], { rows: 27, cols: 132 });
  });
});

describe('Tn3270Session constructor', () => {
  it('defaults to 3278-2 model', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    assert.equal(s.model, '3278-2');
    assert.equal(s.rows,  24);
    assert.equal(s.cols,  80);
  });

  it('accepts 3278-5 model', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23, model: '3278-5' });
    assert.equal(s.rows,  27);
    assert.equal(s.cols, 132);
  });

  it('defaults codepage to 37', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    assert.equal(s.codepage, 37);
  });

  it('defaults useTn3270e to true', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    assert.equal(s.useTn3270e, true);
  });

  it('sets useTn3270e to false when specified', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23, useTn3270e: false });
    assert.equal(s.useTn3270e, false);
  });

  it('initialises empty screen buffer of correct size', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23, model: '3278-2' });
    assert.equal(s.buffer.length, 24 * 80);
  });

  it('throws if connect() called after disconnect()', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    s._destroyed = true;
    assert.throws(() => s.connect(), /destroyed/);
  });
});

describe('Tn3270Session.getScreen', () => {
  it('returns 24 rows of 80 cells for a default 3278-2 session', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    const screen = s.getScreen();
    assert.equal(screen.length, 24);
    assert.equal(screen[0].length, 80);
  });

  it('each cell has char, protected, modified properties', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    const cell = s.getScreen()[0][0];
    assert.ok('char'      in cell);
    assert.ok('protected' in cell);
    assert.ok('modified'  in cell);
  });
});

describe('Tn3270Session.setCursor', () => {
  it('sets cursor address', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    s.setCursor(100);
    assert.equal(s.cursorAddr, 100);
  });

  it('clamps to buffer size', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    s.setCursor(999999);
    assert.equal(s.cursorAddr, 24 * 80 - 1);
  });
});

describe('Tn3270Session.setCursorRC', () => {
  it('converts row/col to linear address', () => {
    const s = new Tn3270Session({ host: 'localhost', port: 23 });
    s.setCursorRC(2, 5); // row 2, col 5 → (2-1)*80 + (5-1) = 84
    assert.equal(s.cursorAddr, 84);
  });
});
