const PINNED_GOALS_KEY = "onusly_pinned_goals";
export const MAX_PINNED_GOALS = 6;

/**
 * Retrieve the array of pinned goal IDs from localStorage.
 * @returns {string[]}
 */
export function getPinnedGoalIds() {
  try {
    const raw = localStorage.getItem(PINNED_GOALS_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/**
 * Persist the array of pinned goal IDs to localStorage.
 * @param {string[]} ids
 */
export function savePinnedGoalIds(ids) {
  try {
    localStorage.setItem(PINNED_GOALS_KEY, JSON.stringify(ids));
  } catch (err) {
    console.error("Failed to save pinned goals:", err);
  }
}

/**
 * Check if a goal is currently pinned.
 * @param {string} goalId
 * @returns {boolean}
 */
export function isGoalPinned(goalId) {
  if (!goalId) return false;
  return getPinnedGoalIds().includes(goalId);
}

/**
 * Toggle pin status for a goal. Maximum 6 goals can be pinned.
 * @param {string} goalId
 * @returns {{ pinned: boolean, pinnedIds: string[], error: string | null }}
 */
export function togglePinGoal(goalId) {
  if (!goalId) {
    return { pinned: false, pinnedIds: getPinnedGoalIds(), error: "Invalid goal ID" };
  }

  const current = getPinnedGoalIds();
  const exists = current.includes(goalId);

  if (exists) {
    const updated = current.filter((id) => id !== goalId);
    savePinnedGoalIds(updated);
    return { pinned: false, pinnedIds: updated, error: null };
  }

  if (current.length >= MAX_PINNED_GOALS) {
    return {
      pinned: false,
      pinnedIds: current,
      error: `Dashboard limit reached: You can pin up to ${MAX_PINNED_GOALS} goals. Unpin a goal to pin this one.`,
    };
  }

  const updated = [...current, goalId];
  savePinnedGoalIds(updated);
  return { pinned: true, pinnedIds: updated, error: null };
}

/**
 * Remove a goal from the pinned list (e.g. on goal deletion).
 * @param {string} goalId
 */
export function removePinnedGoal(goalId) {
  if (!goalId) return;
  const current = getPinnedGoalIds();
  if (current.includes(goalId)) {
    savePinnedGoalIds(current.filter((id) => id !== goalId));
  }
}

/**
 * Transfer pin from an old goal to a newly repeated goal instance.
 * @param {string} oldGoalId
 * @param {string} newGoalId
 * @returns {string[]}
 */
export function transferPinGoal(oldGoalId, newGoalId) {
  if (!oldGoalId || !newGoalId) return getPinnedGoalIds();
  const current = getPinnedGoalIds();
  const index = current.indexOf(oldGoalId);
  let updated;
  if (index !== -1) {
    updated = [...current];
    updated[index] = newGoalId;
  } else {
    if (current.includes(newGoalId)) {
      updated = current;
    } else if (current.length < MAX_PINNED_GOALS) {
      updated = [...current, newGoalId];
    } else {
      updated = current;
    }
  }
  savePinnedGoalIds(updated);
  return updated;
}
