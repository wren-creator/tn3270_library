'use strict';

/**
 * node-tn3270e
 * ─────────────────────────────────────────────────────────────────────────────
 * A full TN3270/TN3270E protocol library for Node.js.
 *
 * Handles the complete mainframe terminal session lifecycle:
 *   · Telnet option negotiation (RFC 854, 856, 885, 1091)
 *   · TN3270E device-type and LU binding (RFC 2355)
 *   · 3270 datastream parsing: Write commands, orders, screen buffer
 *   · EBCDIC ↔ ASCII conversion for multiple IBM code pages
 *   · AID key encoding for inbound (terminal→host) datastreams
 *
 * Quick start:
 *
 *   const { Tn3270Session } = require('node-tn3270e');
 *
 *   const session = new Tn3270Session({
 *     host: '10.1.1.1',
 *     port: 23,
 *     model: '3278-2',
 *     logger: console,
 *   });
 *
 *   session.on('ready',  () => console.log('Session live, waiting for screen'));
 *   session.on('screen', data => {
 *     const text = data.screen.map(row => row.map(c => c.char).join('')).join('\n');
 *     console.log(text);
 *   });
 *   session.on('error',       err    => console.error(err));
 *   session.on('disconnected', reason => console.log('Disconnected:', reason));
 *
 *   session.connect();
 *
 *   // After first screen arrives:
 *   session.sendAid('PF3');
 * ─────────────────────────────────────────────────────────────────────────────
 */

const { Tn3270Session } = require('./session');
const Ebcdic            = require('./ebcdic');
const constants         = require('./constants');

module.exports = {
  /** The main session class. */
  Tn3270Session,

  /** EBCDIC conversion utilities. */
  Ebcdic,

  /** All TN3270/TN3270E/VTAM protocol constants. */
  constants,

  // Re-export the most-used constant groups at the top level for convenience
  AID:             constants.AID,
  CMD:             constants.CMD,
  ORDER:           constants.ORDER,
  FA:              constants.FA,
  COLOR:           constants.COLOR,
  HIGHLIGHT:       constants.HIGHLIGHT,
  MODEL_DIMENSIONS: constants.MODEL_DIMENSIONS,
  TELNET:          constants.TELNET,
  OPT:             constants.OPT,
  TN3E:            constants.TN3E,
  SNA:             constants.SNA,
};
