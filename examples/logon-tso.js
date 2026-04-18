'use strict';

/**
 * examples/logon-tso.js
 * ─────────────────────────────────────────────────────────────────────────────
 * Example: automate a TSO logon by detecting the logon screen,
 * filling in the userid field, and pressing Enter.
 *
 * Usage:
 *   node examples/logon-tso.js <host> <userid> [password]
 *
 * NOTE: This example demonstrates the API surface only — real credentials
 * are never included in the library. The screen text patterns used here
 * ('ENTER USERID') are typical IBM TSO VTAM logon screen strings.
 * ─────────────────────────────────────────────────────────────────────────────
 */

const { Tn3270Session } = require('../src/index');

const host     = process.argv[2] || 'localhost';
const userid   = process.argv[3] || 'IBMUSER';
const password = process.argv[4] || '';       // omit to enter manually

const session = new Tn3270Session({
  host,
  port:     23,
  model:    '3278-2',
  codepage: 37,
  logger: {
    debug: () => {},          // suppress debug noise for this demo
    info:  msg => console.log('[INFO]', msg),
    error: msg => console.error('[ERROR]', msg),
  },
});

let state = 'await-logon';

session.on('ready', () => {
  console.log('TN3270E session ready, waiting for VTAM logon screen…');
});

session.on('screen', data => {
  const text = session.getScreenText();

  // ── State: waiting for the VTAM/TSO logon screen ────────────────────────
  if (state === 'await-logon') {
    // TSO VTAM logon screens typically contain 'ENTER USERID' or 'TSO/E LOGON'
    if (text.includes('ENTER USERID') || text.includes('TSO/E LOGON')) {
      console.log('✓ Logon screen detected');
      state = 'logging-on';

      // Find the first unprotected field — that's the userid field
      const useridField = data.fields.find(f => !f.protected);
      if (!useridField) {
        console.error('Could not find userid field');
        session.disconnect('no-userid-field');
        return;
      }

      console.log(`  Typing userid into field at addr=${useridField.startAddr}`);

      // Clear the field first, then type our userid
      const inputAddr = useridField.startAddr + 1; // +1: skip the FA byte itself
      session.sendAid('ENTER', [{ addr: inputAddr, value: userid }]);
      return;
    }
  }

  // ── State: typed userid, now check for password prompt ──────────────────
  if (state === 'logging-on') {
    if (text.includes('ENTER PASSWORD') || text.includes('PASSWORD')) {
      console.log('✓ Password prompt detected');
      if (!password) {
        console.log('  (No password provided — leaving for operator)');
        state = 'done';
        return;
      }
      const pwdField = data.fields.find(f => !f.protected);
      if (pwdField) {
        session.sendAid('ENTER', [{ addr: pwdField.startAddr + 1, value: password }]);
        state = 'done';
      }
      return;
    }

    // Reached READY prompt — logon succeeded
    if (text.includes('READY') || text.includes('***')) {
      console.log('✓ TSO READY — logon successful');
      state = 'done';
      // In a real application you would continue from here
      // For demo: print the screen and disconnect
      console.log('\n' + text);
      setTimeout(() => session.disconnect('demo'), 500);
      return;
    }
  }
});

session.on('error',       err    => { console.error('Error:', err.message); process.exit(1); });
session.on('disconnected', reason => { console.log('Disconnected:', reason); process.exit(0); });

session.connect();
