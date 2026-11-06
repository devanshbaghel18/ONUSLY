#!/usr/bin/env python3
import json
import time
import urllib.request
import subprocess

API_URL = "http://localhost:8080"
TOKEN_OWNER = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3QtdXNlckBvbnVzbHkuY29tIiwiZXhwIjoxNzk2NTAyODY0LCJoYW5kbGUiOiJ0ZXN0dXNlciIsIm5hbWUiOiJUZXN0IFVzZXIiLCJzdWIiOiJ0ZXN0LXVzZXItZTJlIn0.wXSB3g7ITCyQ_ZTsbulWweNXrBRfo8EKl1Mu6wRxQ0Q"
TOKEN_APPROVER = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImRldmFuc2hiYWdoZWwwNTlAZ21haWwuY29tIiwiZXhwIjoxNzk2NTAzMTMwLCJoYW5kbGUiOiJkZXZhbnNoX2JhZ2hlbCIsIm5hbWUiOiJEZXZhbnNoIEJhZ2hlbCIsInN1YiI6IjEwNTAwMTE0MTcwNjc3NTQxNDk5MSJ9.zkBtZTOvs9F2q2JPcIeA07ekDsggx-XpLQ2hiT8BTiw"

def api_call(path, method="GET", payload=None, token=TOKEN_OWNER):
    url = f"{API_URL}{path}"
    data = json.dumps(payload).encode('utf-8') if payload else None
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}"
        },
        method=method
    )
    with urllib.request.urlopen(req) as resp:
        body = resp.read().decode('utf-8')
        return json.loads(body) if body else {}

print("=================================================================")
print("      APPROVAL-DRIVEN UNLOCKING LATENCY & INVARIANT TEST         ")
print("=================================================================")

# 1. Create an active goal
print("\n[Step 1] Creating active goal with restricted domain 'latency-test.local'...")
goal = api_call("/goals", method="POST", payload={
    "title": "Study Calculus",
    "description": "Block latency-test.local",
    "approvalType": "friend",
    "approverEmail": "devanshbaghel059@gmail.com",
    "blockedDomains": ["latency-test.local"]
}, token=TOKEN_OWNER)

goal_id = goal["id"]
print(f"   Created Goal ID: {goal_id}, Status: {goal['status']}")
assert goal["status"] == "active", f"Expected active status, got {goal['status']}"

# 2. Confirm it is blocked
blocklist = api_call("/enforcement/blocklist", token=TOKEN_OWNER)
targets = blocklist.get("targets") or []
found = any(goal_id == t["goalId"] for t in targets)
print(f"[Step 2] Confirmed goal is present in blocklist: {found}")
assert found, "Goal target missing from blocklist"

# 3. Submit proof
print("\n[Step 3] Submitting proof for goal...")
proof = api_call(f"/goals/{goal_id}/proofs", method="POST", payload={
    "proofType": "text",
    "textExplanation": "Calculus problem set completed"
}, token=TOKEN_OWNER)
proof_id = proof["id"]
print(f"   Submitted Proof ID: {proof_id}")

# 4. Reject it and confirm blocking remains
print("\n[Step 4] Accountability partner rejects proof...")
rejection = api_call(f"/goals/{goal_id}/proofs/{proof_id}/approval", method="POST", payload={
    "status": "rejected",
    "comment": "Incomplete work, problem 5 is missing"
}, token=TOKEN_APPROVER)
print(f"   Decision status: {rejection['status']}")

# Verify goal reverted to active and blocking remains
goal_after_reject = api_call(f"/goals/{goal_id}", token=TOKEN_OWNER)
print(f"   Goal status after rejection: {goal_after_reject['status']}")
assert goal_after_reject["status"] == "active", "Goal did not revert to active on rejection"

blocklist_after_reject = api_call("/enforcement/blocklist", token=TOKEN_OWNER)
targets_after_reject = blocklist_after_reject.get("targets") or []
still_blocked = any(goal_id == t["goalId"] for t in targets_after_reject)
print(f"   CRITICAL INVARIANT: Is restriction still locked after rejection? {'YES! ✅' if still_blocked else 'NO ❌'}")
assert still_blocked, "Security violation: restriction dropped on rejection!"

# 5. Resubmit proof
print("\n[Step 5] Resubmitting proof with full work...")
proof2 = api_call(f"/goals/{goal_id}/proofs", method="POST", payload={
    "proofType": "text",
    "textExplanation": "Completed problem 5 as requested"
}, token=TOKEN_OWNER)
proof2_id = proof2["id"]
print(f"   Resubmitted Proof ID: {proof2_id}")

# 6. Approve proof and measure latency to unblock
print("\n[Step 6 & 7] Partner approves proof; measuring unlock latency...")
t0 = time.time()
approval = api_call(f"/goals/{goal_id}/proofs/{proof2_id}/approval", method="POST", payload={
    "status": "approved",
    "comment": "Great job, all verified!"
}, token=TOKEN_APPROVER)
t_approval = time.time()
print(f"   Approval recorded in {round((t_approval - t0)*1000, 1)}ms: {approval['status']}")

# Now query the agent via native host/IPC for on-demand sync (as triggered by blocked.html every 5s or button)
import struct

def query_agent_sync():
    proc = subprocess.Popen(
        ["./desktop/onusly-native-host"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE
    )
    req_data = json.dumps({"action": "sync"}).encode('utf-8')
    proc.stdin.write(struct.pack('<I', len(req_data)) + req_data)
    proc.stdin.flush()
    raw_len = proc.stdout.read(4)
    if not raw_len:
        return None
    msg_len = struct.unpack('<I', raw_len)[0]
    raw = proc.stdout.read(msg_len)
    proc.stdin.close()
    proc.wait()
    return json.loads(raw.decode('utf-8'))

sync_resp = query_agent_sync()
t1 = time.time()
latency_ms = round((t1 - t0) * 1000, 2)
latency_sec = round(t1 - t0, 3)

blocklist_data = sync_resp.get("blocklist") or {}
active_targets = blocklist_data.get("targets") or []
is_unblocked = not any(goal_id == t.get("goalId") for t in active_targets)

print(f"\n[Step 8] Agent and Extension Unlocked Status: {'UNLOCKED! ✅' if is_unblocked else 'STILL BLOCKED ❌'}")
print(f"   Measured Total Approval-to-Unlock Latency: {latency_sec}s ({latency_ms}ms)")
print(f"   PRD Requirement (< 10 seconds): {'SATISFIED! ✅' if latency_sec < 10.0 else 'FAILED ❌'}")

assert is_unblocked, "Goal was not unlocked after approval"
assert latency_sec < 10.0, f"Latency {latency_sec}s exceeded 10-second requirement"

print("\n=================================================================")
print("      APPROVAL-TO-UNLOCK LATENCY BENCHMARK PASSED SUCCESSFULLY   ")
print("=================================================================")
