'use strict';

/**
 * examples/basic-connect.js
 * ─────────────────────────────────────────────────────────────────────────────
 * Minimal example: connect to a TN3270E host, wait for the first screen,
 * print it as plain text, then disconnect.
 *
 * Usage:
 *   node examples/basic-connect.js <host> [port]
 *
 * Example:
 *   node examples/basic-connect.js 10.1.1.1 23
 * ─────────────────────────────────────────────────────────────────────────────
 */

const { Tn3270Session } = require('../src/index');

const host = process.argv[2] || 'localhost';
const port = parseInt(process.argv[3] || '23', 10);

const session = new Tn3270Session({
  host,
  port,
  model:      '3278-2',
  codepage:   37,
  useTn3270e: true,
  logger:     console,      // use console.debug/info/error
});

let screenCount = 0;

session.on('connected', ({ host, port }) => {
  console.log(`\n✓ TCP connected to ${host}:${port}`);
  console.log('  Negotiating TN3270E…\n');
});

session.on('ready', ({ lu, model }) => {
  console.log(`✓ TN3270E session ready`);
  if (lu) console.log(`  LU name: ${lu}`);
  console.log(`  Model:   ${model}\n`);
});

session.on('screen', data => {
  screenCount++;
  console.log(`\n${'─'.repeat(data.cols)}`);
  console.log(`SCREEN ${screenCount}  cursor=${data.cursor}  ${data.rows}×${data.cols}`);
  console.log('─'.repeat(data.cols));

  // Print each row as a plain string
  for (const row of data.screen) {
    console.log(row.map(c => c.char).join(''));
  }

  console.log('─'.repeat(data.cols));
  console.log(`Fields: ${data.fields.length} total, ` +
    `${data.fields.filter(f => !f.protected).length} unprotected`);

  // After seeing the first screen, disconnect (demo purposes)
  if (screenCount === 1) {
    console.log('\nFirst screen received — disconnecting.\n');
    session.disconnect('demo-complete');
  }
});

session.on('error', err => {
  console.error('Session error:', err.message);
  process.exit(1);
});

session.on('disconnected', reason => {
  console.log(`Disconnected: ${reason}`);
  process.exit(0);
});

console.log(`Connecting to ${host}:${port}…`);
session.connect();
