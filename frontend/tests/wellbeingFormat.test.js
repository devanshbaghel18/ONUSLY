import test from "node:test";
import assert from "node:assert/strict";
import { formatDuration, formatDelta, formatShortDay } from "../src/lib/wellbeingFormat.js";

test("formatDuration formats seconds, minutes, and hours cleanly", () => {
  assert.equal(formatDuration(0), "0m");
  assert.equal(formatDuration(45), "45s");
  assert.equal(formatDuration(300), "5m");
  assert.equal(formatDuration(3600), "1h");
  assert.equal(formatDuration(5460), "1h 31m");
});

test("formatDelta prints human comparison deltas", () => {
  assert.equal(formatDelta(0), "Even with yesterday");
  assert.equal(formatDelta(600), "+10m vs prior");
  assert.equal(formatDelta(-3720), "-1h 2m vs prior");
});

test("formatShortDay handles valid and empty dates safely", () => {
  assert.ok(formatShortDay("2026-11-26").length > 0);
  assert.equal(formatShortDay(""), "");
});
