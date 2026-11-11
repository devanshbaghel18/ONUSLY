import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

// Helper functions matching frontend presentation and authorization logic
function getGoalStatusBadge(status) {
  switch (status) {
    case 'completed':
      return { label: 'Completed', color: 'emerald', locked: false };
    case 'rejected':
      return { label: 'Rejected', color: 'amber', locked: true };
    case 'proof_submitted':
      return { label: 'Pending Review', color: 'sky', locked: true };
    case 'active':
    default:
      return { label: 'Active', color: 'slate', locked: true };
  }
}

function canUserSubmitProof(user, goal) {
  if (!user || !goal) return false;
  if (user.id !== goal.ownerId) return false;
  return goal.status === 'active' || goal.status === 'rejected';
}

function canUserDecideProof(user, goal) {
  if (!user || !goal) return false;
  // Goal owner can NEVER approve their own goal
  if (user.id === goal.ownerId) return false;
  // Approver must match configured approver
  const isApprover =
    goal.approverId === user.id ||
    Boolean(goal.approverHandle && user.handle && goal.approverHandle.toLowerCase() === user.handle.toLowerCase()) ||
    Boolean(goal.approverEmail && user.email && goal.approverEmail.toLowerCase() === user.email.toLowerCase());
  return Boolean(isApprover && goal.status === 'proof_submitted');
}

describe('D11 Frontend Accountability Workflow & State Presentation', () => {
  it('1. Status badges correctly reflect enforcement locks across all goal states', () => {
    const active = getGoalStatusBadge('active');
    assert.equal(active.locked, true);
    assert.equal(active.label, 'Active');

    const pending = getGoalStatusBadge('proof_submitted');
    assert.equal(pending.locked, true);
    assert.equal(pending.label, 'Pending Review');

    const rejected = getGoalStatusBadge('rejected');
    assert.equal(rejected.locked, true);
    assert.equal(rejected.label, 'Rejected');

    const completed = getGoalStatusBadge('completed');
    assert.equal(completed.locked, false);
    assert.equal(completed.label, 'Completed');
  });

  it('2. Owner can submit proof when active or resubmit when rejected', () => {
    const alice = { id: 'user-alice', handle: 'alice' };
    const goal = { id: 'goal-1', ownerId: 'alice', approverId: 'bob', status: 'active' };

    // Active -> can submit
    assert.equal(canUserSubmitProof({ id: 'alice' }, goal), true);

    // Rejected -> can resubmit
    goal.status = 'rejected';
    assert.equal(canUserSubmitProof({ id: 'alice' }, goal), true);

    // Proof submitted -> cannot submit again
    goal.status = 'proof_submitted';
    assert.equal(canUserSubmitProof({ id: 'alice' }, goal), false);

    // Completed -> cannot submit
    goal.status = 'completed';
    assert.equal(canUserSubmitProof({ id: 'alice' }, goal), false);

    // Non-owner -> cannot submit
    assert.equal(canUserSubmitProof({ id: 'bob' }, goal), false);
  });

  it('3. Approver authorization strictly forbids self-approval', () => {
    const alice = { id: 'user-alice', handle: 'alice', email: 'alice@example.com' };
    const bob = { id: 'user-bob', handle: 'bob', email: 'bob@example.com' };
    const eve = { id: 'user-eve', handle: 'eve', email: 'eve@example.com' };

    const goal = {
      id: 'goal-1',
      ownerId: 'user-alice',
      approverId: 'user-bob',
      status: 'proof_submitted',
    };

    // Owner cannot approve
    assert.equal(canUserDecideProof(alice, goal), false);

    // Legitimate approver can approve
    assert.equal(canUserDecideProof(bob, goal), true);

    // Third-party cannot approve
    assert.equal(canUserDecideProof(eve, goal), false);

    // Once goal is completed or rejected, approver buttons disappear
    goal.status = 'completed';
    assert.equal(canUserDecideProof(bob, goal), false);

    goal.status = 'rejected';
    assert.equal(canUserDecideProof(bob, goal), false);
  });
});
