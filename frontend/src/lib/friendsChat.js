import { getUser } from "./auth.js";

const FRIENDS_KEY = "onusly_friends_list";
const CHATS_KEY = "onusly_friends_chats";
const CLEARED_AT_KEY = "onusly_friends_chats_cleared_at";

function normHandle(h) {
  return String(h || "").replace(/[@\s]/g, "").toLowerCase();
}

function normEmail(e) {
  return String(e || "").trim().toLowerCase();
}

export function isSelfUser(candidate, user = getUser()) {
  if (!candidate || !user) return false;
  const myId = String(user.id || user.sub || "").trim();
  const myEmail = normEmail(user.email);
  const myHandle = normHandle(user.handle);

  const cId = String(candidate.id || candidate.friendId || "").trim();
  const cEmail = normEmail(candidate.email || candidate.friendEmail);
  const cHandle = normHandle(candidate.handle || candidate.friendHandle);

  if (myId && cId && !cId.startsWith("friend-") && cId === myId) return true;
  if (myEmail && cEmail && cEmail === myEmail) return true;
  if (myHandle && cHandle && cHandle === myHandle) return true;
  return false;
}

export function getFriendClearedAt(friendId) {
  try {
    const raw = localStorage.getItem(CLEARED_AT_KEY);
    if (!raw) return 0;
    const map = JSON.parse(raw);
    return Number(map[friendId]) || 0;
  } catch {
    return 0;
  }
}

export function setFriendClearedAt(friendId, timestamp = Date.now()) {
  try {
    const raw = localStorage.getItem(CLEARED_AT_KEY);
    const map = raw ? JSON.parse(raw) : {};
    map[friendId] = timestamp;
    localStorage.setItem(CLEARED_AT_KEY, JSON.stringify(map));
  } catch (err) {
    console.error("Failed to set friend cleared at:", err);
  }
}

export function getStoredFriends() {
  try {
    const raw = localStorage.getItem(FRIENDS_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function saveFriends(friends) {
  try {
    localStorage.setItem(FRIENDS_KEY, JSON.stringify(friends));
  } catch (err) {
    console.error("Failed to save friends:", err);
  }
}

export function addFriend({ id, name, handle, email, picture }) {
  const list = getStoredFriends();
  const cleanId = String(id || "").trim();
  const cleanHandle = normHandle(handle);
  const trimmedEmail = normEmail(email);
  const trimmedName = (name || "").trim() || (cleanHandle ? `@${cleanHandle}` : "Friend");

  if (isSelfUser({ id: cleanId, handle: cleanHandle, email: trimmedEmail })) {
    return list;
  }

  // Check if friend with same id, handle, or email already exists
  const existing = list.find((f) =>
    (cleanId && f.id === cleanId) ||
    (cleanHandle && f.handle && normHandle(f.handle) === cleanHandle) ||
    (trimmedEmail && f.email && normEmail(f.email) === trimmedEmail)
  );
  if (existing) {
    return list;
  }

  const newFriend = {
    id: cleanId || `friend-${Date.now()}`,
    name: trimmedName,
    handle: cleanHandle,
    email: trimmedEmail,
    picture: picture || "",
    status: "Active Partner",
    lastMessage: "Connected as accountability friend",
    lastMessageTime: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    unreadCount: 0,
  };

  const updated = [newFriend, ...list];
  saveFriends(updated);
  return updated;
}

export function deleteFriend(friendId) {
  try {
    const list = getStoredFriends();
    const updated = list.filter((f) => f.id !== friendId);
    saveFriends(updated);

    // Clean up chat history with this friend
    const raw = localStorage.getItem(CHATS_KEY);
    if (raw) {
      const allChats = JSON.parse(raw);
      delete allChats[friendId];
      localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));
    }
    return updated;
  } catch (err) {
    console.error("Failed to delete friend:", err);
    return getStoredFriends();
  }
}

export function getChatMessages(friendId) {
  try {
    const raw = localStorage.getItem(CHATS_KEY);
    if (!raw) return [];
    const allChats = JSON.parse(raw);
    const msgs = allChats[friendId] || [];
    const clearedAt = getFriendClearedAt(friendId);
    return msgs
      .filter((m) => {
        if (!clearedAt) return true;
        const msgTime = m.timestamp || (m.time && m.time.includes("T") ? new Date(m.time).getTime() : 0);
        return !msgTime || msgTime > clearedAt;
      })
      .map((m) => {
        const textMatch = (m.text || "").match(/verify my goal:\s*["“]?([^"”\n\r]+)["”]?/i);
        if (textMatch && !m.isProof) {
          return {
            ...m,
            isProof: true,
            goalTitle: m.goalTitle || textMatch[1].trim(),
          };
        }
        return m;
      });
  } catch {
    return [];
  }
}

export function sendChatMessage(friendId, messageData, sender = "me") {
  try {
    const raw = localStorage.getItem(CHATS_KEY);
    const allChats = raw ? JSON.parse(raw) : {};
    const friendMessages = allChats[friendId] || [];

    const isDataObj = typeof messageData === "object" && messageData !== null;
    const text = isDataObj ? (messageData.text || "") : String(messageData || "");

    const newMsg = {
      id: (isDataObj && messageData.id) || `msg-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
      text: text.trim(),
      sender,
      isProof: isDataObj ? Boolean(messageData.isProof) : false,
      goalId: isDataObj ? (messageData.goalId || "") : "",
      goalTitle: isDataObj ? (messageData.goalTitle || "") : "",
      ownerId: isDataObj ? (messageData.ownerId || "") : "",
      approved: isDataObj ? Boolean(messageData.approved) : false,
      images: isDataObj && Array.isArray(messageData.images) ? messageData.images : [],
      files: isDataObj && Array.isArray(messageData.files) ? messageData.files : [],
      externalLink: isDataObj ? (messageData.externalLink || "") : "",
      time: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      timestamp: (isDataObj && messageData.timestamp) || Date.now(),
      replyTo: isDataObj && messageData.replyTo ? messageData.replyTo : null,
    };


    const existingIdx = friendMessages.findIndex((m) => m.id === newMsg.id);
    if (existingIdx >= 0) {
      friendMessages[existingIdx] = { ...friendMessages[existingIdx], ...newMsg };
      allChats[friendId] = [...friendMessages];
    } else {
      allChats[friendId] = [...friendMessages, newMsg];
    }
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    // Also update friend's last message preview and reset unread count
    const previewText = newMsg.isProof ? `[Proof] ${newMsg.goalTitle || "Goal"}` : newMsg.text;
    const friends = getStoredFriends();
    const updatedFriends = friends.map((f) =>
      f.id === friendId
        ? {
            ...f,
            lastMessage: previewText,
            lastMessageTime: newMsg.time,
            unreadCount: 0,
          }
        : f
    );
    const targetFriend = updatedFriends.find((f) => f.id === friendId);
    const otherFriends = updatedFriends.filter((f) => f.id !== friendId);
    const sortedFriends = targetFriend ? [targetFriend, ...otherFriends] : updatedFriends;
    saveFriends(sortedFriends);

    return allChats[friendId];
  } catch (err) {
    console.error("Failed to send message:", err);
    return [];
  }
}

export function receiveChatMessage({ friendId, friendEmail, friendHandle, friendName, text, time, sender = "friend", messageId, isProof, goalId, goalTitle, ownerId, approved, images, files, externalLink, isCurrentlyActive = false, replyTo = null }) {
  try {
    const cleanFriendId = String(friendId || "").trim();
    const normalizedEmail = normEmail(friendEmail);
    const cleanHandle = normHandle(friendHandle);

    // Guard: never create or append a self-chat entry for the currently authenticated user
    if (isSelfUser({ id: cleanFriendId, handle: cleanHandle, email: normalizedEmail })) {
      return { friend: null, messages: [] };
    }

    let friends = getStoredFriends();
    let friend = friends.find((f) =>
      (cleanFriendId && f.id === cleanFriendId) ||
      (cleanHandle && f.handle && normHandle(f.handle) === cleanHandle) ||
      (normalizedEmail && f.email && normEmail(f.email) === normalizedEmail)
    );

    // If friend exists, unify email/handle/name
    if (friend) {
      let changed = false;
      if (normalizedEmail && (!friend.email || friend.email !== normalizedEmail)) {
        friend.email = normalizedEmail;
        changed = true;
      }
      if (cleanHandle && (!friend.handle || friend.handle !== cleanHandle)) {
        friend.handle = cleanHandle;
        changed = true;
      }
      if (friendName && (!friend.name || friend.name.startsWith("@"))) {
        friend.name = friendName.trim();
        changed = true;
      }
      if (changed) {
        saveFriends(friends);
      }
    } else {
      // If the friend is not yet in our friend list, automatically add them!
      const fallback = cleanHandle ? `@${cleanHandle}` : (normalizedEmail ? normalizedEmail.split("@")[0] : "Friend");
      const displayName = friendName ? friendName.trim() : fallback;
      const capitalName = displayName.startsWith("@") ? displayName : displayName.charAt(0).toUpperCase() + displayName.slice(1);
      friend = {
        id: cleanFriendId || `friend-${Date.now()}`,
        name: capitalName,
        handle: cleanHandle,
        email: normalizedEmail,
        status: "Active Partner",
        lastMessage: isProof ? `[Proof] ${goalTitle || "Goal"}` : (text || "Connected"),
        lastMessageTime: time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
        unreadCount: isCurrentlyActive || sender === "me" ? 0 : 1,
      };
      friends = [friend, ...friends];
      saveFriends(friends);
    }

    // Append to friend's chat history
    const raw = localStorage.getItem(CHATS_KEY);
    const allChats = raw ? JSON.parse(raw) : {};
    const friendMessages = allChats[friend.id] || [];

    const textStr = (text || "").trim();
    const textMatch = textStr.match(/verify my goal:\s*["“]?([^"”\n\r]+)["”]?/i);
    const finalIsProof = Boolean(isProof || textMatch);
    const finalGoalTitle = goalTitle || (textMatch ? textMatch[1].trim() : "");

    const existingIndex = messageId ? friendMessages.findIndex((m) => m.id === messageId) : -1;
    if (existingIndex >= 0) {
      const existing = friendMessages[existingIndex];
      const shouldUpdate =
        (!existing.isProof && finalIsProof) ||
        (!existing.goalId && goalId) ||
        (!existing.goalTitle && finalGoalTitle) ||
        (!existing.approved && approved) ||
        (!existing.replyTo && replyTo);

      if (shouldUpdate) {
        const updatedMsg = {
          ...existing,
          isProof: existing.isProof || finalIsProof,
          goalId: existing.goalId || goalId || "",
          goalTitle: existing.goalTitle || finalGoalTitle,
          ownerId: existing.ownerId || ownerId || "",
          approved: existing.approved || Boolean(approved),
          images: (Array.isArray(images) && images.length > 0) ? images : existing.images,
          files: (Array.isArray(files) && files.length > 0) ? files : existing.files,
          externalLink: externalLink || existing.externalLink,
          replyTo: replyTo || existing.replyTo || null,
        };
        friendMessages[existingIndex] = updatedMsg;
        allChats[friend.id] = friendMessages;
        localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));
      }
      return { friend, messages: allChats[friend.id] };
    }

    const clearedAt = getFriendClearedAt(friend.id);
    const msgTimestamp = (time && time.includes("T")) ? new Date(time).getTime() : Date.now();
    if (clearedAt > 0 && msgTimestamp <= clearedAt) {
      return { friend, messages: friendMessages };
    }

    const finalId = messageId || `msg-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`;
    const newMsg = {
      id: finalId,
      text: textStr,
      sender,
      isProof: finalIsProof,
      goalId: goalId || "",
      goalTitle: finalGoalTitle,
      ownerId: ownerId || "",
      approved: Boolean(approved),
      images: Array.isArray(images) ? images : [],
      files: Array.isArray(files) ? files : [],
      externalLink: externalLink || "",
      time: time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      timestamp: msgTimestamp,
      replyTo: replyTo || null,
    };

    allChats[friend.id] = [...friendMessages, newMsg];
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));


    // Update friend's last message preview and unread count
    const previewText = newMsg.isProof ? `[Proof] ${newMsg.goalTitle || "Goal"}` : newMsg.text;
    const updatedFriends = friends.map((f) =>
      f.id === friend.id
        ? {
            ...f,
            lastMessage: previewText,
            lastMessageTime: newMsg.time,
            unreadCount: isCurrentlyActive || sender === "me" ? 0 : ((Number(f.unreadCount) || 0) + 1),
          }
        : f
    );
    const targetFriend = updatedFriends.find((f) => f.id === friend.id);
    const otherFriends = updatedFriends.filter((f) => f.id !== friend.id);
    const sortedFriends = targetFriend ? [targetFriend, ...otherFriends] : updatedFriends;
    saveFriends(sortedFriends);

    return { friend, messages: allChats[friend.id] };
  } catch (err) {
    console.error("Failed to receive message:", err);
    return { friend: null, messages: [] };
  }
}

export function clearUnreadCount(friendId) {
  try {
    if (!friendId) return getStoredFriends();
    const friends = getStoredFriends();
    const updated = friends.map((f) =>
      f.id === friendId ? { ...f, unreadCount: 0 } : f
    );
    saveFriends(updated);
    return updated;
  } catch (err) {
    console.error("Failed to clear unread count:", err);
    return getStoredFriends();
  }
}

export function clearFriendChat(friendId) {
  try {
    if (!friendId) return [];
    const raw = localStorage.getItem(CHATS_KEY);
    const allChats = raw ? JSON.parse(raw) : {};
    allChats[friendId] = [];
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    setFriendClearedAt(friendId, Date.now());

    const friends = getStoredFriends();
    const updated = friends.map((f) =>
      f.id === friendId
        ? { ...f, lastMessage: "", lastMessageTime: "", unreadCount: 0 }
        : f
    );
    saveFriends(updated);
    return [];
  } catch (err) {
    console.error("Failed to clear friend chat:", err);
    return [];
  }
}


export function toggleFavoriteFriend(friendId) {
  try {
    if (!friendId) return getStoredFriends();
    const friends = getStoredFriends();
    const updated = friends.map((f) =>
      f.id === friendId ? { ...f, isFavorite: !f.isFavorite } : f
    );
    saveFriends(updated);
    return updated;
  } catch (err) {
    console.error("Failed to toggle favorite friend:", err);
    return getStoredFriends();
  }
}

export function deleteChatMessageForMe(friendId, messageId) {
  try {
    if (!friendId || !messageId) return [];
    const raw = localStorage.getItem(CHATS_KEY);
    if (!raw) return [];
    const allChats = JSON.parse(raw);
    const msgs = allChats[friendId] || [];
    const updatedMsgs = msgs.filter((m) => m.id !== messageId);
    allChats[friendId] = updatedMsgs;
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    // Update last message preview
    const lastMsg = updatedMsgs[updatedMsgs.length - 1];
    const friends = getStoredFriends();
    const updatedFriends = friends.map((f) =>
      f.id === friendId
        ? {
            ...f,
            lastMessage: lastMsg ? (lastMsg.isProof ? `[Proof] ${lastMsg.goalTitle || "Goal"}` : lastMsg.text) : "",
            lastMessageTime: lastMsg ? lastMsg.time : "",
          }
        : f
    );
    saveFriends(updatedFriends);

    return updatedMsgs;
  } catch (err) {
    console.error("Failed to delete chat message for me:", err);
    return [];
  }
}

/**
 * Deletes a chat message by ID across all chat threads and updates friend previews.
 * @param {string} messageId
 * @returns {void}
 */
export function deleteChatMessageGlobally(messageId) {
  try {
    if (!messageId) return;
    const raw = localStorage.getItem(CHATS_KEY);
    if (!raw) return;
    const allChats = JSON.parse(raw);
    let changed = false;

    // Remove this message from ALL friend chat threads
    for (const friendId of Object.keys(allChats)) {
      const messages = allChats[friendId];
      if (Array.isArray(messages)) {
        const filtered = messages.filter((m) => m.id !== messageId);
        if (filtered.length !== messages.length) {
          allChats[friendId] = filtered;
          changed = true;
        }
      }
    }

    if (changed) {
      localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

      // Update lastMessage preview for all friends
      const friends = getStoredFriends();
      const updatedFriends = friends.map((f) => {
        const remaining = allChats[f.id] || [];
        const lastMsg = remaining.length > 0 ? remaining[remaining.length - 1] : null;
        if (lastMsg) {
          const preview = lastMsg.isProof ? `[Proof] ${lastMsg.goalTitle || "Goal"}` : lastMsg.text;
          return {
            ...f,
            lastMessage: preview,
            lastMessageTime: lastMsg.time,
          };
        }
        return {
          ...f,
          lastMessage: "",
          lastMessageTime: "",
        };
      });
      saveFriends(updatedFriends);
    }
  } catch (err) {
    console.error("Failed to delete chat message globally:", err);
  }
}

export function deleteChatMessage(friendId, messageId) {
  deleteChatMessageGlobally(messageId);
  return getChatMessages(friendId);
}

export function mergeChatHistory(friendId, serverMessages, currentUserEmail, currentUserHandle) {
  if (!serverMessages || !Array.isArray(serverMessages)) return getChatMessages(friendId);
  try {
    const raw = localStorage.getItem(CHATS_KEY);
    const allChats = raw ? JSON.parse(raw) : {};
    const local = allChats[friendId] || [];
    const clearedAt = getFriendClearedAt(friendId);

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
        const textMatch = (m.text || "").match(/verify my goal:\s*["“]?([^"”\n\r]+)["”]?/i);
        const enriched = (textMatch && !m.isProof)
          ? { ...m, isProof: true, goalTitle: m.goalTitle || textMatch[1].trim() }
          : m;
        map.set(m.id, enriched);
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
        sender: isMe ? "me" : "friend",
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

    const merged = Array.from(map.values());
    allChats[friendId] = merged;
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));
    return merged;

  } catch (err) {
    console.error("Failed to merge chat history:", err);
    return getChatMessages(friendId);
  }
}

export function markChatMessageApproved(goalId, messageId) {
  try {
    const raw = localStorage.getItem(CHATS_KEY);
    if (!raw) return;
    const allChats = JSON.parse(raw);
    let changed = false;
    for (const friendId in allChats) {
      if (Array.isArray(allChats[friendId])) {
        allChats[friendId] = allChats[friendId].map((m) => {
          if ((messageId && m.id === messageId) || (goalId && m.goalId === goalId)) {
            changed = true;
            return { ...m, approved: true };
          }
          return m;
        });
      }
    }
    if (changed) {
      localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));
    }
  } catch (err) {
    console.error("Failed to mark chat message approved:", err);
  }
}

/**
 * Reconciles local friend list with canonical server friends:
 * - Removes any accidental self-entries (matching currentUser id/email/handle)
 * - Migrates temporary friend-<timestamp> chat histories into canonical server friendId keys
 * - Deduplicates duplicate entries sharing the same id, handle, or email
 */
export function reconcileFriendsWithServer(backendFriends = [], currentUser = getUser()) {
  try {
    const local = getStoredFriends();
    const rawChats = localStorage.getItem(CHATS_KEY);
    const allChats = rawChats ? JSON.parse(rawChats) : {};
    const rawCleared = localStorage.getItem(CLEARED_AT_KEY);
    const clearedMap = rawCleared ? JSON.parse(rawCleared) : {};
    const remappedIds = {};

    const migrateMessages = (fromId, toId) => {
      if (!fromId || !toId || fromId === toId) return;
      remappedIds[fromId] = toId;
      const fromMsgs = Array.isArray(allChats[fromId]) ? allChats[fromId] : [];
      const toMsgs = Array.isArray(allChats[toId]) ? allChats[toId] : [];
      if (fromMsgs.length > 0) {
        const seen = new Map();
        [...toMsgs, ...fromMsgs].forEach((m) => {
          if (m?.id) seen.set(m.id, m);
        });
        const merged = Array.from(seen.values()).sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
        allChats[toId] = merged;
      }
      if (fromId in allChats) {
        delete allChats[fromId];
      }
      if (clearedMap[fromId] && (!clearedMap[toId] || clearedMap[fromId] > clearedMap[toId])) {
        clearedMap[toId] = clearedMap[fromId];
      }
      delete clearedMap[fromId];
    };

    const deduped = [];
    const findMatchIndex = (cand) => {
      const cId = String(cand.id || cand.friendId || "").trim();
      const cH = normHandle(cand.handle || cand.friendHandle);
      const cE = normEmail(cand.email || cand.friendEmail);
      return deduped.findIndex((existing) =>
        (cId && existing.id === cId) ||
        (cH && existing.handle && normHandle(existing.handle) === cH) ||
        (cE && existing.email && normEmail(existing.email) === cE)
      );
    };

    // First clean and deduplicate local friends, stripping self-entries
    for (const lf of local) {
      if (!lf || isSelfUser(lf, currentUser)) {
        if (lf?.id && lf.id in allChats) delete allChats[lf.id];
        continue;
      }
      const idx = findMatchIndex(lf);
      if (idx === -1) {
        deduped.push({
          ...lf,
          handle: normHandle(lf.handle),
          email: normEmail(lf.email),
        });
      } else {
        const keep = deduped[idx];
        const keepIsCanonical = keep.id && !String(keep.id).startsWith("friend-");
        const lfIsCanonical = lf.id && !String(lf.id).startsWith("friend-");
        if (!keepIsCanonical && lfIsCanonical) {
          migrateMessages(keep.id, lf.id);
          deduped[idx] = {
            ...keep,
            ...lf,
            id: lf.id,
            handle: normHandle(lf.handle || keep.handle),
            email: normEmail(lf.email || keep.email),
            lastMessage: keep.lastMessage || lf.lastMessage,
            lastMessageTime: keep.lastMessageTime || lf.lastMessageTime,
          };
        } else {
          migrateMessages(lf.id, keep.id);
        }
      }
    }

    // Merge canonical server friends into deduped list
    if (Array.isArray(backendFriends)) {
      for (const bf of backendFriends) {
        if (!bf || !bf.friendId) continue;
        if (isSelfUser(bf, currentUser)) continue;

        const bfHandle = normHandle(bf.friendHandle);
        const bfEmail = normEmail(bf.friendEmail);
        const idx = findMatchIndex({ id: bf.friendId, handle: bfHandle, email: bfEmail });

        if (idx === -1) {
          const msgs = Array.isArray(allChats[bf.friendId]) ? allChats[bf.friendId] : [];
          const lastMsg = msgs.length > 0 ? msgs[msgs.length - 1] : null;
          deduped.push({
            id: bf.friendId,
            name: bf.friendName || (bfHandle ? `@${bfHandle}` : bfEmail) || "Friend",
            handle: bfHandle,
            email: bfEmail,
            picture: "",
            status: "Active Partner",
            lastMessage: lastMsg ? (lastMsg.isProof ? `[Proof] ${lastMsg.goalTitle || "Goal"}` : lastMsg.text) : "Connected as accountability friend",
            lastMessageTime: lastMsg?.time || "",
            unreadCount: 0,
          });
        } else {
          const existing = deduped[idx];
          if (existing.id !== bf.friendId) {
            migrateMessages(existing.id, bf.friendId);
          }
          deduped[idx] = {
            ...existing,
            id: bf.friendId,
            name: bf.friendName || existing.name || (bfHandle ? `@${bfHandle}` : "Friend"),
            handle: bfHandle || existing.handle,
            email: bfEmail || existing.email,
          };
        }
      }
    }

    saveFriends(deduped);
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));
    localStorage.setItem(CLEARED_AT_KEY, JSON.stringify(clearedMap));
    return { friends: deduped, remappedIds };
  } catch (err) {
    console.error("Failed to reconcile friends:", err);
    return { friends: getStoredFriends(), remappedIds: {} };
  }
}
