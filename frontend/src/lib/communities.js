const STORAGE_KEY = "onusly_user_communities";
const COMMUNITY_CHATS_KEY = "onusly_community_chats";
const COMMUNITY_CLEARED_AT_KEY = "onusly_community_chats_cleared_at";

export function getCommunityClearedAt(communityId) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CLEARED_AT_KEY);
    if (!raw) return 0;
    const map = JSON.parse(raw);
    return Number(map[communityId]) || 0;
  } catch {
    return 0;
  }
}

export function setCommunityClearedAt(communityId, timestamp = Date.now()) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CLEARED_AT_KEY);
    const map = raw ? JSON.parse(raw) : {};
    map[communityId] = timestamp;
    localStorage.setItem(COMMUNITY_CLEARED_AT_KEY, JSON.stringify(map));
  } catch (err) {
    console.error("Failed to set community cleared at:", err);
  }
}

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
      ? { ...c, joined: true, membersCount: (c.membersCount || 1) }
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

export function deleteCommunity(communityId) {
  try {
    const list = getStoredCommunities();
    const updated = list.filter((c) => c.id !== communityId);
    saveCommunities(updated);

    // Clean up community chats
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    if (raw) {
      const chats = JSON.parse(raw);
      delete chats[communityId];
      localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(chats));
    }
    return updated;
  } catch (err) {
    console.error("Failed to delete community:", err);
    return getStoredCommunities();
  }
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

export function addOrUpdateCommunity(community) {
  if (!community || !community.id) return getStoredCommunities();
  const list = getStoredCommunities();
  const existingIdx = list.findIndex(
    (c) =>
      c.id === community.id ||
      (c.code && community.code && c.code.toUpperCase() === community.code.toUpperCase())
  );
  let updated;
  if (existingIdx >= 0) {
    updated = [...list];
    updated[existingIdx] = { ...updated[existingIdx], ...community, joined: true };
  } else {
    updated = [{ ...community, joined: true }, ...list];
  }
  saveCommunities(updated);
  return updated;
}

export function createCommunity({
  name,
  description,
  category = "General Focus",
  isPrivate = true,
  requiredApprovals = 2,
  selectedFriends = [],
}) {
  const list = getStoredCommunities();
  const randomSuffix = Math.floor(100 + Math.random() * 900);
  const codePrefix = name.split(" ")[0].toUpperCase().slice(0, 7) || "COMM";
  const code = `${codePrefix}-${randomSuffix}`;

  const initialMembers = [
    {
      rank: 1,
      name: "You (Founder)",
      streak: "1 Day",
      completed: 0,
      isCurrentUser: true,
      isFounder: true,
    },
    ...selectedFriends.map((f, idx) => ({
      rank: idx + 2,
      name: f.name || `@${f.handle || "friend"}`,
      handle: f.handle || "",
      email: f.email || "",
      picture: f.picture || "",
      streak: "Active",
      completed: 0,
      isCurrentUser: false,
    })),
  ];

  const newCommunity = {
    id: `comm-${Date.now()}`,
    name: name.trim(),
    code,
    category,
    isPrivate,
    membersCount: initialMembers.length,
    avgStreak: "1 Day",
    established: "Just now",
    description: description.trim(),
    proofCriteria: "Screenshot or verification URL.",
    requiredApprovals: Number(requiredApprovals) || 2,
    joined: true,
    isFounder: true,
    members: initialMembers,
    lastMessage: selectedFriends.length > 0 
      ? `Community founded with ${selectedFriends.length} friend${selectedFriends.length > 1 ? "s" : ""}`
      : "Community founded. Ready for accountability!",
    lastMessageTime: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
  };

  const updated = [newCommunity, ...list];
  saveCommunities(updated);
  return updated;
}

// ==========================================
// COMMUNITY GROUP CHATS PERSISTENCE
// ==========================================
export function getCommunityMessages(communityId) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    if (!raw) return [];
    const all = JSON.parse(raw);
    const list = all[communityId] || [];
    const clearedAt = getCommunityClearedAt(communityId);
    if (clearedAt > 0) {
      return list.filter((m) => {
        const msgTime = m.timestamp || (m.time && m.time.includes("T") ? new Date(m.time).getTime() : 0);
        return !msgTime || msgTime > clearedAt;
      });
    }
    return list;
  } catch {
    return [];
  }
}


export function sendCommunityMessage(communityId, msg) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    const all = raw ? JSON.parse(raw) : {};
    const existing = all[communityId] || [];

    const msgId = msg.id || `comm-msg-${Date.now()}-${Math.random().toString(36).substr(2, 6)}`;

    // If message with this ID already exists, do not duplicate
    if (existing.some((m) => m.id === msgId)) {
      return existing;
    }

    const newMsg = {
      id: msgId,
      sender: "me",
      senderName: msg.senderName || "You",
      senderHandle: msg.senderHandle || "",
      text: msg.text || "",
      isProof: Boolean(msg.isProof),
      goalId: msg.goalId || "",
      goalTitle: msg.goalTitle || "",
      ownerId: msg.ownerId || "",
      approved: Boolean(msg.approved),
      images: Array.isArray(msg.images) ? msg.images : [],
      files: Array.isArray(msg.files) ? msg.files : [],
      externalLink: msg.externalLink || "",
      time: msg.time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      replyTo: msg.replyTo || null,
    };

    all[communityId] = [...existing, newMsg];
    localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(all));

    // Update community card last message preview
    const list = getStoredCommunities();
    const updated = list.map((c) =>
      c.id === communityId
        ? {
            ...c,
            lastMessage: newMsg.isProof ? `📸 Proof: ${newMsg.goalTitle || "Goal"}` : newMsg.text,
            lastMessageTime: newMsg.time,
          }
        : c
    );
    saveCommunities(updated);

    return all[communityId];
  } catch (err) {
    console.error("Failed to send community message:", err);
    return [];
  }
}

export function receiveCommunityMessage(communityId, msg) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    const all = raw ? JSON.parse(raw) : {};
    const existing = all[communityId] || [];

    // Avoid duplicate messages if already present by ID or identical self-sent content
    const isDuplicate = existing.some((m) => {
      if (msg.id && m.id === msg.id) return true;
      if (
        m.sender === "me" &&
        msg.sender === "me" &&
        m.text === msg.text &&
        Boolean(m.isProof) === Boolean(msg.isProof)
      ) {
        return true;
      }
      return false;
    });

    if (isDuplicate) {
      return existing;
    }

    const newMsg = {
      id: msg.id || `comm-msg-${Date.now()}-${Math.random().toString(36).substr(2, 6)}`,
      sender: msg.sender || "member",
      senderName: msg.senderName || msg.senderHandle || "Member",
      senderHandle: msg.senderHandle || "",
      text: msg.text || "",
      isProof: Boolean(msg.isProof),
      goalId: msg.goalId || "",
      goalTitle: msg.goalTitle || "",
      ownerId: msg.ownerId || "",
      approved: Boolean(msg.approved),
      images: Array.isArray(msg.images) ? msg.images : [],
      files: Array.isArray(msg.files) ? msg.files : [],
      externalLink: msg.externalLink || "",
      time: msg.time ? new Date(msg.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      replyTo: msg.replyTo || null,
    };

    all[communityId] = [...existing, newMsg];
    localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(all));

    // Update preview
    const list = getStoredCommunities();
    const updated = list.map((c) =>
      c.id === communityId
        ? {
            ...c,
            lastMessage: newMsg.isProof ? `📸 Proof: ${newMsg.goalTitle || "Goal"}` : newMsg.text,
            lastMessageTime: newMsg.time,
          }
        : c
    );
    saveCommunities(updated);

    return all[communityId];
  } catch (err) {
    console.error("Failed to receive community message:", err);
    return [];
  }
}

export function deleteCommunityMessage(communityId, messageId) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    if (!raw) return [];
    const all = JSON.parse(raw);
    const existing = all[communityId] || [];
    const filtered = existing.filter((m) => m.id !== messageId);
    all[communityId] = filtered;
    localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(all));
    return filtered;
  } catch (err) {
    console.error("Failed to delete community message:", err);
    return [];
  }
}

export function deleteCommunityMessageForMe(communityId, messageId) {
  return deleteCommunityMessage(communityId, messageId);
}

export function clearCommunityChat(communityId) {
  try {
    if (!communityId) return [];
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    const all = raw ? JSON.parse(raw) : {};
    all[communityId] = [];
    localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(all));

    setCommunityClearedAt(communityId, Date.now());

    const list = getStoredCommunities();
    const updated = list.map((c) =>
      c.id === communityId
        ? { ...c, lastMessage: "", lastMessageTime: "" }
        : c
    );
    saveCommunities(updated);
    return [];
  } catch (err) {
    console.error("Failed to clear community chat:", err);
    return [];
  }
}


export function toggleFavoriteCommunity(communityId) {
  try {
    if (!communityId) return getStoredCommunities();
    const list = getStoredCommunities();
    const updated = list.map((c) =>
      c.id === communityId ? { ...c, isFavorite: !c.isFavorite } : c
    );
    saveCommunities(updated);
    return updated;
  } catch (err) {
    console.error("Failed to toggle favorite community:", err);
    return getStoredCommunities();
  }
}

export function markCommunityMessageApproved(goalId, messageId) {
  try {
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    if (!raw) return;
    const all = JSON.parse(raw);
    let changed = false;
    for (const commId in all) {
      if (Array.isArray(all[commId])) {
        all[commId] = all[commId].map((m) => {
          if ((messageId && m.id === messageId) || (goalId && m.goalId === goalId)) {
            changed = true;
            return { ...m, approved: true };
          }
          return m;
        });
      }
    }
    if (changed) {
      localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(all));
    }
  } catch (err) {
    console.error("Failed to mark community message approved:", err);
  }
}

export function mergeCommunityHistory(communityId, serverMessages, currentUserEmail, currentUserHandle) {
  if (!serverMessages || !Array.isArray(serverMessages)) return getCommunityMessages(communityId);
  try {
    const raw = localStorage.getItem(COMMUNITY_CHATS_KEY);
    const all = raw ? JSON.parse(raw) : {};
    const local = all[communityId] || [];
    const clearedAt = getCommunityClearedAt(communityId);

    const normCurrentEmail = (currentUserEmail || "").toLowerCase();
    const normCurrentHandle = (currentUserHandle || "").toLowerCase().replace(/^@/, "");
    const map = new Map();

    local.forEach((m) => {
      if (m.id) {
        if (clearedAt > 0) {
          const msgTime = m.timestamp || (m.time && m.time.includes("T") ? new Date(m.time).getTime() : 0);
          if (msgTime > 0 && msgTime <= clearedAt) {
            return;
          }
        }
        map.set(m.id, m);
      }
    });

    serverMessages.forEach((m) => {
      if (clearedAt > 0) {
        const msgTime = m.time ? new Date(m.time).getTime() : 0;
        if (msgTime > 0 && msgTime <= clearedAt) {
          return;
        }
      }

      const senderE = (m.senderEmail || "").toLowerCase();
      const senderH = (m.senderHandle || "").toLowerCase().replace(/^@/, "");
      const isMe = (normCurrentEmail && senderE === normCurrentEmail) ||
                   (normCurrentHandle && senderH === normCurrentHandle);

      const existing = map.get(m.id) || {};
      const text = m.text || existing.text || "";
      const textMatch = text.match(/verify my goal:\s*["“]?([^"”\n\r]+)["”]?/i);
      const isProof = Boolean(m.isProof || existing.isProof || textMatch);
      const goalTitle = m.goalTitle || existing.goalTitle || (textMatch ? textMatch[1].trim() : "");
      const goalId = m.goalId || existing.goalId || "";
      const ownerId = m.ownerId || existing.ownerId || "";
      const approved = Boolean(m.approved || existing.approved);
      const images = (Array.isArray(m.images) && m.images.length > 0) ? m.images : (existing.images || []);
      const files = (Array.isArray(m.files) && m.files.length > 0) ? m.files : (existing.files || []);
      const externalLink = m.externalLink || existing.externalLink || "";

      const formatted = {
        ...existing,
        id: m.id,
        text,
        sender: isMe ? "me" : "member",
        senderName: isMe ? "You" : (m.senderName || (m.senderHandle ? `@${m.senderHandle.replace(/^@/, "")}` : "Member")),
        senderHandle: m.senderHandle || existing.senderHandle || "",
        isProof,
        goalId,
        goalTitle,
        ownerId,
        approved,
        images,
        files,
        externalLink,
        replyTo: m.replyTo || existing.replyTo || null,
        timestamp: m.time ? new Date(m.time).getTime() : (existing.timestamp || Date.now()),
        time: m.time
          ? (m.time.includes("T") ? new Date(m.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : m.time)
          : (existing.time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })),
      };

      map.set(m.id, formatted);
    });

    const merged = Array.from(map.values()).sort((a, b) => {
      const timeA = a.timestamp || 0;
      const timeB = b.timestamp || 0;
      return timeA - timeB;
    });

    all[communityId] = merged;
    localStorage.setItem(COMMUNITY_CHATS_KEY, JSON.stringify(all));

    // Update community card preview
    if (merged.length > 0) {
      const lastMsg = merged[merged.length - 1];
      const list = getStoredCommunities();
      const updated = list.map((c) =>
        c.id === communityId
          ? {
              ...c,
              lastMessage: lastMsg.isProof ? `📸 Proof: ${lastMsg.goalTitle || "Goal"}` : lastMsg.text,
              lastMessageTime: lastMsg.time,
            }
          : c
      );
      saveCommunities(updated);
    }

    return merged;
  } catch (err) {
    console.error("Failed to merge community history:", err);
    return getCommunityMessages(communityId);
  }
}
