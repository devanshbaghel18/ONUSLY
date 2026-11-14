import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

// Core logic for processing and formatting multi-attempt dossiers in the frontend
function processTimelineAttempts(rawAttempts = []) {
  if (!Array.isArray(rawAttempts) || rawAttempts.length === 0) {
    return [];
  }

  // Sort ascending by attemptNumber
  const sorted = [...rawAttempts].sort((a, b) => a.attemptNumber - b.attemptNumber);

  return sorted.map((att, idx) => ({
    ...att,
    isLatest: idx === sorted.length - 1,
    statusColor:
      att.status === 'approved'
        ? 'emerald'
        : att.status === 'rejected'
        ? 'amber'
        : 'sky',
  }));
}

function calculateStreakStats(goals = [], auditLog = []) {
  let streak = 0;
  let totalVerified = 0;

  for (const g of goals) {
    if (g.status === 'completed') {
      streak++;
      totalVerified++;
    } else if (g.status === 'active' || g.status === 'proof_submitted') {
      continue;
    } else {
      break;
    }
  }

  const completionRate = goals.length > 0 ? (totalVerified / goals.length) * 100 : 100;

  return {
    streak,
    totalVerified,
    completionRate: Math.round(completionRate),
    auditReceiptCount: auditLog.length,
  };
}

describe('Part 5 — Frontend Multi-Attempt Timeline & Accountability Streaks Suite', () => {
  it('1. Correctly formats and orders multi-attempt submissions with latest indicator', () => {
    const rawAttempts = [
      {
        attemptNumber: 1,
        proofId: 'proof-1',
        submittedAt: '2026-11-14T01:00:00Z',
        status: 'rejected',
        comment: 'Screenshot too blurry',
      },
      {
        attemptNumber: 2,
        proofId: 'proof-2',
        submittedAt: '2026-11-14T02:00:00Z',
        status: 'rejected',
        comment: 'Missing test logs',
      },
      {
        attemptNumber: 3,
        proofId: 'proof-3',
        submittedAt: '2026-11-14T03:00:00Z',
        status: 'pending',
      },
    ];

    const processed = processTimelineAttempts(rawAttempts);

    assert.equal(processed.length, 3);
    assert.equal(processed[0].attemptNumber, 1);
    assert.equal(processed[0].status, 'rejected');
    assert.equal(processed[0].isLatest, false);
    assert.equal(processed[0].statusColor, 'amber');

    assert.equal(processed[1].attemptNumber, 2);
    assert.equal(processed[1].status, 'rejected');
    assert.equal(processed[1].isLatest, false);

    assert.equal(processed[2].attemptNumber, 3);
    assert.equal(processed[2].status, 'pending');
    assert.equal(processed[2].isLatest, true);
    assert.equal(processed[2].statusColor, 'sky');
  });

  it('2. Evaluates accountability streak and verification accuracy correctly', () => {
    const userGoals = [
      { id: 'g1', status: 'completed' },
      { id: 'g2', status: 'completed' },
      { id: 'g3', status: 'completed' },
      { id: 'g4', status: 'proof_submitted' }, // In progress, shouldn't break streak
    ];

    const auditLog = [
      { receiptId: 'rcpt-1', decision: 'approved', attemptIndex: 1 },
      { receiptId: 'rcpt-2', decision: 'rejected', attemptIndex: 1 },
      { receiptId: 'rcpt-3', decision: 'approved', attemptIndex: 2 },
    ];

    const stats = calculateStreakStats(userGoals, auditLog);

    assert.equal(stats.streak, 3);
    assert.equal(stats.totalVerified, 3);
    assert.equal(stats.completionRate, 75); // 3 out of 4 completed
    assert.equal(stats.auditReceiptCount, 3);
  });

  it('3. Handles single-attempt edge cases safely with zero errors', () => {
    const emptyTimeline = processTimelineAttempts([]);
    assert.deepEqual(emptyTimeline, []);

    const singleAttempt = processTimelineAttempts([
      { attemptNumber: 1, proofId: 'p-single', status: 'pending' },
    ]);
    assert.equal(singleAttempt.length, 1);
    assert.equal(singleAttempt[0].isLatest, true);
    assert.equal(singleAttempt[0].statusColor, 'sky');
  });
});

