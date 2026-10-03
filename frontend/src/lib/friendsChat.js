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
      id: `msg-${Date.now()}`,
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
