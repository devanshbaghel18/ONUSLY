import { describe, it, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import {
  syncFromDesktopAgent,
  pairDesktopAgent,
  processDesktopStatus,
  processDesktopPairRequest,
  processDesktopPairPoll,
} from '../background.js';
import * as storage from '../src/storage.js';

describe('Sub-part 6.1 — Extension Native Messaging Bridge Suite', () => {
  beforeEach(() => {
    // Reset browser globals
    globalThis.chrome = undefined;
    globalThis.browser = undefined;
  });

  it('1. Returns graceful unsupported error when runtime has no native messaging API', async () => {
    const resSync = await syncFromDesktopAgent();
    assert.equal(resSync.success, false);
    assert.equal(resSync.error, 'Native messaging unsupported');

    const resPair = await pairDesktopAgent('token-123', 'user-456');
    assert.equal(resPair.success, false);
    assert.equal(resPair.error, 'Native messaging unsupported');
  });

  it('2. Successfully pairs with desktop agent via Chrome sendNativeMessage', async () => {
    let sentHostName = '';
    let sentPayload = null;

    globalThis.chrome = {
      runtime: {
        id: 'jifigaejbinmdhnljobkodcgeaplojmg',
        lastError: null,
        sendNativeMessage: (hostName, payload, callback) => {
          sentHostName = hostName;
          sentPayload = payload;
          callback({ success: true, paired: true });
        },
      },
    };

    const res = await pairDesktopAgent('jwt-token-abc', 'user-himanshu');
    assert.equal(res.success, true);
    assert.equal(res.paired, true);
    assert.equal(sentHostName, 'com.onusly.desktop_agent');
    assert.equal(sentPayload.action, 'pair-request');
    assert.equal(sentPayload.userId, 'user-himanshu');
    assert.equal(sentPayload.token, 'jwt-token-abc');
  });

  it('3. Handles desktop agent connection failure cleanly without crashing', async () => {
    globalThis.chrome = {
      runtime: {
        id: 'jifigaejbinmdhnljobkodcgeaplojmg',
        lastError: { message: 'Specified native messaging host not found.' },
        sendNativeMessage: (hostName, payload, callback) => {
          callback(null);
        },
      },
    };

    const res = await pairDesktopAgent('jwt-token-abc', 'user-himanshu');
    assert.equal(res.success, false);
    assert.equal(res.error, 'Specified native messaging host not found.');
  });

  it('4. Bridge handlers query status, pair-request, and pair-poll via sendNativeMessage', async () => {
    const calls = [];
    globalThis.chrome = {
      runtime: {
        id: 'jifigaejbinmdhnljobkodcgeaplojmg',
        lastError: null,
        sendNativeMessage: (hostName, payload, callback) => {
          calls.push({ hostName, payload });
          if (payload.action === 'get-status') {
            callback({ success: true, status: { connected: true, enforcing: true } });
            return;
          }
          if (payload.action === 'pair-request') {
            callback({ success: true, pairCode: 'AB12CD', expiresAt: 1999999999 });
            return;
          }
          if (payload.action === 'pair-poll') {
            callback({ success: true, status: { connected: true, paired: true } });
            return;
          }
          callback({ success: false, error: 'unexpected action' });
        },
      },
    };

    const st = await processDesktopStatus();
    assert.equal(st.available, true);
    assert.equal(st.status.connected, true);

    const pr = await processDesktopPairRequest();
    assert.equal(pr.success, true);
    assert.equal(pr.pairCode, 'AB12CD');

    const pp = await processDesktopPairPoll('ab12cd');
    assert.equal(pp.success, true);
    assert.equal(pp.paired, true);
    assert.deepEqual(
      calls.map((c) => c.payload),
      [
        { action: 'get-status' },
        { action: 'pair-request' },
        { action: 'pair-poll', code: 'AB12CD' },
      ]
    );
  });
});
