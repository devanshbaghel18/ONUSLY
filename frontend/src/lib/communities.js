const STORAGE_KEY = "onusly_user_communities";

// No pre-formed fake communities. Only communities created by the user will exist.
export const DEFAULT_COMMUNITIES = [];

export function getStoredCommunities() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function saveCommunities(list) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
  } catch (err) {
    console.error("Failed to save communities:", err);
  }
}

export function joinCommunity(communityId) {
  const list = getStoredCommunities();
  const updated = list.map((c) =>
    c.id === communityId
      ? { ...c, joined: true, membersCount: (c.membersCount || 0) + 1 }
      : c
  );
  saveCommunities(updated);
  return updated;
}

export function leaveCommunity(communityId) {
  const list = getStoredCommunities();
  const updated = list.map((c) =>
    c.id === communityId
      ? { ...c, joined: false, membersCount: Math.max(1, (c.membersCount || 1) - 1) }
      : c
  );
  saveCommunities(updated);
  return updated;
}

export function joinByCode(code) {
  const list = getStoredCommunities();
  const trimmed = code.trim().toUpperCase();
  const target = list.find((c) => c.code?.toUpperCase() === trimmed);
  if (!target) {
    throw new Error(`No community found with invite code "${trimmed}".`);
  }
  return joinCommunity(target.id);
}

export function createCommunity({
  name,
  description,
  category = "General Focus",
  isPrivate = true,
  requiredApprovals = 2,
}) {
  const list = getStoredCommunities();
  const randomSuffix = Math.floor(100 + Math.random() * 900);
  const codePrefix = name.split(" ")[0].toUpperCase().slice(0, 7) || "COMM";
  const code = `${codePrefix}-${randomSuffix}`;

  const newCommunity = {
    id: `comm-${Date.now()}`,
    name: name.trim(),
    code,
    category,
    isPrivate,
    membersCount: 1,
    avgStreak: "1 Day",
    established: "Just now",
    description: description.trim(),
    proofCriteria: "Peer screenshot or verification URL.",
    requiredApprovals: Number(requiredApprovals) || 2,
    joined: true,
    members: [
      {
        rank: 1,
        name: "You (Founder)",
        streak: "1 Day",
        completed: 0,
        isCurrentUser: true,
      },
    ],
  };

  const updated = [newCommunity, ...list];
  saveCommunities(updated);
  return updated;
}
