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

export function addFriend({ name, email }) {
  const list = getStoredFriends();
  const trimmedName = name.trim();
  const trimmedEmail = email ? email.trim().toLowerCase() : "";

  // Check if friend with same email already exists
  const existing = list.find((f) => f.email && f.email === trimmedEmail);
  if (existing) {
    return list;
  }

  const newFriend = {
    id: `friend-${Date.now()}`,
    name: trimmedName,
    email: trimmedEmail,
    status: "Active Partner",
    lastMessage: "Connected as accountability friend",
    lastMessageTime: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
  };

  const updated = [newFriend, ...list];
  saveFriends(updated);
  return updated;
}

export function getChatMessages(friendId) {
  try {
    const raw = localStorage.getItem(CHATS_KEY);
    if (!raw) return [];
    const allChats = JSON.parse(raw);
    return allChats[friendId] || [];
  } catch {
    return [];
  }
}

export function sendChatMessage(friendId, text, sender = "me") {
  try {
    const raw = localStorage.getItem(CHATS_KEY);
    const allChats = raw ? JSON.parse(raw) : {};
    const friendMessages = allChats[friendId] || [];

    const newMsg = {
      id: `msg-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
      text: text.trim(),
      sender,
      time: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    };

    allChats[friendId] = [...friendMessages, newMsg];
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    // Also update friend's last message preview
    const friends = getStoredFriends();
    const updatedFriends = friends.map((f) =>
      f.id === friendId
        ? {
            ...f,
            lastMessage: text.trim(),
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

export function receiveChatMessage({ friendEmail, text, time, sender = "friend" }) {
  try {
    const normalizedEmail = friendEmail ? friendEmail.trim().toLowerCase() : "";
    let friends = getStoredFriends();
    let friend = friends.find((f) => f.email && f.email.toLowerCase() === normalizedEmail);

    // If the friend is not yet in our friend list, automatically add them!
    if (!friend) {
      const nameGuess = normalizedEmail.split("@")[0] || "Friend";
      const capitalName = nameGuess.charAt(0).toUpperCase() + nameGuess.slice(1);
      friend = {
        id: `friend-${Date.now()}`,
        name: capitalName,
        email: normalizedEmail,
        status: "Active Partner",
        lastMessage: text,
        lastMessageTime: time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      };
      friends = [friend, ...friends];
      saveFriends(friends);
    }

    // Append to friend's chat history
    const raw = localStorage.getItem(CHATS_KEY);
    const allChats = raw ? JSON.parse(raw) : {};
    const friendMessages = allChats[friend.id] || [];

    const newMsg = {
      id: `msg-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
      text: text.trim(),
      sender,
      time: time || new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    };

    allChats[friend.id] = [...friendMessages, newMsg];
    localStorage.setItem(CHATS_KEY, JSON.stringify(allChats));

    // Update friend's last message preview
    const updatedFriends = friends.map((f) =>
      f.id === friend.id
        ? {
            ...f,
            lastMessage: text.trim(),
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
