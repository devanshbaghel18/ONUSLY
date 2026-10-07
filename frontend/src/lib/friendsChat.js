const FRIENDS_KEY = "onusly_friends_list";
const CHATS_KEY = "onusly_friends_chats";

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

export function addFriend({ name, handle, email, picture }) {
  const list = getStoredFriends();
  const cleanHandle = handle ? handle.replace(/[@\s]/g, "").toLowerCase() : "";
  const trimmedEmail = email ? email.trim().toLowerCase() : "";
  const trimmedName = (name || "").trim() || (cleanHandle ? `@${cleanHandle}` : "Friend");

  // Check if friend with same handle or email already exists
  const existing = list.find((f) => 
    (cleanHandle && f.handle && f.handle.toLowerCase() === cleanHandle) ||
    (trimmedEmail && f.email && f.email === trimmedEmail)
  );
  if (existing) {
    return list;
  }

  const newFriend = {
    id: `friend-${Date.now()}`,
    name: trimmedName,
    handle: cleanHandle,
    email: trimmedEmail,
    picture: picture || "",
    status: "Active Partner",
    lastMessage: "Connected as accountability friend",
    lastMessageTime: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
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
    return msgs.map((m) => {
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
    };

    allChats[friendId] = [...friendMessages, newMsg];
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    // Also update friend's last message preview
    const previewText = newMsg.isProof ? `📸 Proof: ${newMsg.goalTitle || "Goal"}` : newMsg.text;
    const friends = getStoredFriends();
    const updatedFriends = friends.map((f) =>
      f.id === friendId
        ? {
            ...f,
            lastMessage: previewText,
            lastMessageTime: newMsg.time,
          }
        : f
    );
    saveFriends(updatedFriends);

    return allChats[friendId];
  } catch (err) {
    console.error("Failed to send message:", err);
    return [];
  }
}

export function receiveChatMessage({ friendEmail, friendHandle, friendName, text, time, sender = "friend", messageId, isProof, goalId, goalTitle, ownerId, approved, images, files, externalLink }) {
  try {
    const normalizedEmail = friendEmail ? friendEmail.trim().toLowerCase() : "";
    const cleanHandle = friendHandle ? friendHandle.replace(/[@\s]/g, "").toLowerCase() : "";
    let friends = getStoredFriends();
    let friend = friends.find((f) => 
      (cleanHandle && f.handle && f.handle.toLowerCase() === cleanHandle) ||
      (normalizedEmail && f.email && f.email.toLowerCase() === normalizedEmail)
    );

    // If the friend is not yet in our friend list, automatically add them!
    if (!friend) {
      const fallback = cleanHandle ? `@${cleanHandle}` : (normalizedEmail ? normalizedEmail.split("@")[0] : "Friend");
      const displayName = friendName ? friendName.trim() : fallback;
      const capitalName = displayName.startsWith("@") ? displayName : displayName.charAt(0).toUpperCase() + displayName.slice(1);
      friend = {
        id: `friend-${Date.now()}`,
        name: capitalName,
        handle: cleanHandle,
        email: normalizedEmail,
        status: "Active Partner",
        lastMessage: isProof ? `📸 Proof: ${goalTitle || "Goal"}` : (text || "Connected"),
        lastMessageTime: time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
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
        (!existing.approved && approved);

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
        };
        friendMessages[existingIndex] = updatedMsg;
        allChats[friend.id] = friendMessages;
        localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));
      }
      return { friend, messages: allChats[friend.id] };
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
    };

    allChats[friend.id] = [...friendMessages, newMsg];
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    // Update friend's last message preview
    const previewText = newMsg.isProof ? `📸 Proof: ${newMsg.goalTitle || "Goal"}` : newMsg.text;
    const updatedFriends = friends.map((f) =>
      f.id === friend.id
        ? {
            ...f,
            lastMessage: previewText,
            lastMessageTime: newMsg.time,
          }
        : f
    );
    saveFriends(updatedFriends);

    return { friend, messages: allChats[friend.id] };
  } catch (err) {
    console.error("Failed to receive message:", err);
    return { friend: null, messages: [] };
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
          const preview = lastMsg.isProof ? `📸 Proof: ${lastMsg.goalTitle || "Goal"}` : lastMsg.text;
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

    const normCurrentEmail = (currentUserEmail || "").toLowerCase();
    const normCurrentHandle = (currentUserHandle || "").toLowerCase().replace(/^@/, "");
    const map = new Map();

    local.forEach((m) => {
      if (m.id) {
        const textMatch = (m.text || "").match(/verify my goal:\s*["“]?([^"”\n\r]+)["”]?/i);
        const enriched = (textMatch && !m.isProof)
          ? { ...m, isProof: true, goalTitle: m.goalTitle || textMatch[1].trim() }
          : m;
        map.set(m.id, enriched);
      }
    });

    serverMessages.forEach((m) => {
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
