import { test, describe } from "node:test";
import assert from "node:assert/strict";

const storage = new Map();
globalThis.localStorage = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
};
globalThis.fetch = async () => ({ ok: true });

const { setAuth } = await import("../src/lib/auth.js");
const {
  getStoredFriends,
  addFriend,
  sendChatMessage,
  receiveChatMessage,
  reconcileFriendsWithServer,
  getChatMessages,
} = await import("../src/lib/friendsChat.js");

function makeUser(id, email, handle, name) {
  return { id, email, handle, name };
}

describe("Chat Deduplication, Self-Chat Prevention & Multi-Account Isolation", () => {
  test("never adds current authenticated user as a friend or creates a self-chat on echo", () => {
    storage.clear();
    const me = makeUser("uid-priyanshi", "priyanshi@gmail.com", "p_p", "Priyanshi");
    setAuth("hdr.eyJzdWIiOiJ1aWQtcHJpeWFuc2hpIiwiZXhwIjo5OTk5OTk5OTk5fQ.sig", me);

    // Attempt to add self directly
    addFriend({ id: "uid-priyanshi", name: "Priyanshi", handle: "@p_p", email: "priyanshi@gmail.com" });
    assert.equal(getStoredFriends().length, 0);

    // Attempt receiveChatMessage from self echo
    const res = receiveChatMessage({
      messageId: "msg-self-1",
      friendId: "uid-priyanshi",
      friendEmail: "priyanshi@gmail.com",
      friendHandle: "p_p",
      friendName: "Priyanshi",
      text: "joo",
      sender: "friend",
    });
    assert.equal(res.friend, null);
    assert.equal(getStoredFriends().length, 0);
  });

  test("sendChatMessage deduplicates by message ID (no duplicate bubbles)", () => {
    storage.clear();
    const me = makeUser("uid-p", "p@gmail.com", "p_p", "P");
    setAuth("hdr.eyJzdWIiOiJ1aWQtcCIsImV4cCI6OTk5OTk5OTk5OX0.sig", me);

    addFriend({ id: "uid-devansh", name: "devansh_baghel", handle: "devansh_baghel", email: "devansh@gmail.com" });

    sendChatMessage("uid-devansh", { id: "msg-101", text: "Hi" }, "me");
    sendChatMessage("uid-devansh", { id: "msg-101", text: "Hi" }, "me");

    const msgs = getChatMessages("uid-devansh");
    assert.equal(msgs.length, 1);
    assert.equal(msgs[0].id, "msg-101");
    assert.equal(msgs[0].text, "Hi");
  });

  test("reconcileFriendsWithServer merges duplicate friend entries and removes self entries", () => {
    storage.clear();
    const me = makeUser("uid-p", "p@gmail.com", "p_p", "Priyanshi");
    setAuth("hdr.eyJzdWIiOiJ1aWQtcCIsImV4cCI6OTk5OTk5OTk5OX0.sig", me);

    // Simulate broken legacy state: duplicate himanshu_04 (one synthetic, one canonical) + accidental self entry
    localStorage.setItem(
      "onusly_friends_list",
      JSON.stringify([
        { id: "friend-111", name: "himanshu_04", handle: "himanshu_04", email: "h@gmail.com", lastMessage: "nice to meet you to" },
        { id: "uid-himanshu", name: "himanshu_04", handle: "himanshu_04", email: "h@gmail.com", lastMessage: "No messages yet" },
        { id: "friend-999", name: "Priyanshi_praja", handle: "p_p", email: "p@gmail.com", lastMessage: "joo" },
      ])
    );
    localStorage.setItem(
      "onusly_friends_chats",
      JSON.stringify({
        "friend-111": [{ id: "m1", text: "nice to meet you to", sender: "friend", timestamp: 1000 }],
        "friend-999": [{ id: "m2", text: "joo", sender: "friend", timestamp: 2000 }],
      })
    );

    const { friends, remappedIds } = reconcileFriendsWithServer(
      [{ friendId: "uid-himanshu", friendName: "himanshu_04", friendHandle: "himanshu_04", friendEmail: "h@gmail.com" }],
      me
    );

    assert.equal(friends.length, 1);
    assert.equal(friends[0].id, "uid-himanshu");
    assert.equal(remappedIds["friend-111"], "uid-himanshu");
    const migratedMsgs = getChatMessages("uid-himanshu");
    assert.equal(migratedMsgs.length, 1);
    assert.equal(migratedMsgs[0].text, "nice to meet you to");
  });

  test("switching accounts via setAuth clears previous user localStorage chat cache", () => {
    storage.clear();
    const userA = makeUser("uid-a", "a@gmail.com", "user_a", "User A");
    setAuth("hdr.eyJzdWIiOiJ1aWQtYSIsImV4cCI6OTk5OTk5OTk5OX0.sig", userA);
    addFriend({ id: "uid-friend", name: "Friend", handle: "friend_1", email: "f@gmail.com" });
    assert.equal(getStoredFriends().length, 1);

    const userB = makeUser("uid-b", "b@gmail.com", "user_b", "User B");
    setAuth("hdr.eyJzdWIiOiJ1aWQtYiIsImV4cCI6OTk5OTk5OTk5OX0.sig", userB);
    assert.equal(getStoredFriends().length, 0);
  });
});
