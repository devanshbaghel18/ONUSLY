import { useState, useMemo, useEffect, useRef } from "react";
import AppLayout from "../components/AppLayout";
import {
  Users,
  Search,
  Plus,
  ArrowLeft,
  Copy,
  Check,
  X,
  MessageSquare,
  Send,
  UserPlus,
  AtSign,
  ShieldCheck,
  Loader2,
} from "lucide-react";
import {
  getStoredCommunities,
  joinCommunity,
  leaveCommunity,
  joinByCode,
  createCommunity,
} from "../lib/communities";
import {
  getStoredFriends,
  addFriend,
  getChatMessages,
  sendChatMessage,
  receiveChatMessage,
  mergeChatHistory,
} from "../lib/friendsChat";
import { getChatHistory, lookupUser } from "../lib/api";
import { getUser } from "../lib/auth";
import { useWebSocket } from "../hooks/useWebSocket";

export default function Communities() {
  // Main mode: 'communities' | 'friends'
  const [mainView, setMainView] = useState("communities");

  // ==========================================
  // COMMUNITIES STATE
  // ==========================================
  const [communities, setCommunities] = useState(() => getStoredCommunities());
  const communityTab = "all"; // 'all' | 'my' | 'discover'
  const [searchQuery, setSearchQuery] = useState("");
  const [privateCode, setPrivateCode] = useState("");
  const [codeNotice, setCodeNotice] = useState({ error: "", success: "" });

  const [selectedCommunity, setSelectedCommunity] = useState(null);
  const [copiedCode, setCopiedCode] = useState(false);

  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newCommName, setNewCommName] = useState("");
  const [newCommDesc, setNewCommDesc] = useState("");
  const [newCommCategory, setNewCommCategory] = useState("General Focus");
  const [newCommApprovals, setNewCommApprovals] = useState(2);
  const [createError, setCreateError] = useState("");

  // ==========================================
  // FRIENDS & WHATSAPP CHAT STATE
  // ==========================================
  const [friends, setFriends] = useState(() => getStoredFriends());
  const [friendSearch, setFriendSearch] = useState("");
  const [activeFriendId, setActiveFriendId] = useState(() =>
    getStoredFriends().length > 0 ? getStoredFriends()[0].id : null
  );
  const activeFriend = friends.find((f) => f.id === activeFriendId);
  const activeFriendIdRef = useRef(activeFriendId);
  useEffect(() => {
    activeFriendIdRef.current = activeFriendId;
  }, [activeFriendId]);

  const currentUser = getUser();
  const [chatMessages, setChatMessages] = useState(() => {
    const stored = getStoredFriends();
    return stored.length > 0 ? getChatMessages(stored[0].id) : [];
  });
  const [messageInput, setMessageInput] = useState("");
  const [onlineEmails, setOnlineEmails] = useState(new Set());
  const [onlineHandles, setOnlineHandles] = useState(new Set());

  // Real-time WebSocket hook for instant live chat delivery and presence
  const { isConnected, send } = useWebSocket((event) => {
    if (event?.type === "presence.list") {
      const list = event.payload?.onlineEmails || [];
      setOnlineEmails(new Set(list.map((e) => String(e).toLowerCase())));
      const handles = event.payload?.onlineHandles || [];
      setOnlineHandles(new Set(handles.map((h) => String(h).toLowerCase().replace(/^@/, ""))));
    }

    if (event?.type === "chat.message") {
      const payload = event.payload || {};
      console.log("[Communities] Incoming real-time chat message:", payload);

      const timeFormatted = payload.time
        ? new Date(payload.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
        : new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

      const { friend, messages } = receiveChatMessage({
        messageId: payload.id,
        friendEmail: payload.senderEmail,
        friendHandle: payload.senderHandle,
        text: payload.text,
        time: timeFormatted,
        sender: "friend",
      });

      // Refresh friends list
      const updatedFriends = getStoredFriends();
      setFriends(updatedFriends);

      // If viewing this friend, update message view live
      const currentActiveId = activeFriendIdRef.current;
      const currentActive = updatedFriends.find((f) => f.id === currentActiveId);
      const isViewingThisFriend =
        currentActiveId === friend?.id ||
        (currentActive?.handle && friend?.handle && currentActive.handle.toLowerCase() === friend.handle.toLowerCase()) ||
        (currentActive?.email && friend?.email && currentActive.email.toLowerCase() === friend.email.toLowerCase()) ||
        !currentActiveId;

      if (friend && isViewingThisFriend) {
        if (!currentActiveId) setActiveFriendId(friend.id);
        setChatMessages(messages);
      }
    }
  });

  // Query online presence every 4 seconds while connected
  useEffect(() => {
    if (!isConnected) return;
    send({ type: "presence.query" });

    const interval = setInterval(() => {
      send({ type: "presence.query" });
    }, 4000);

    return () => clearInterval(interval);
  }, [isConnected, send]);

  const selectFriend = (id) => {
    setActiveFriendId(id);
    setChatMessages(getChatMessages(id));

    const f = friends.find((item) => item.id === id);
    const peerKey = f?.handle ? `@${f.handle}` : (f?.email || "");
    if (peerKey) {
      getChatHistory(peerKey)
        .then((serverMsgs) => {
          if (serverMsgs && serverMsgs.length > 0) {
            const merged = mergeChatHistory(id, serverMsgs, currentUser?.email, currentUser?.handle);
            setChatMessages(merged);
          }
        })
        .catch(() => {});
    }
  };

  // Sync latest chat messages from DynamoDB when activeFriend changes
  useEffect(() => {
    if (!activeFriend) return;
    const peerKey = activeFriend.handle ? `@${activeFriend.handle}` : (activeFriend.email || "");
    if (!peerKey) return;
    getChatHistory(peerKey)
      .then((serverMsgs) => {
        if (serverMsgs && serverMsgs.length > 0) {
          const merged = mergeChatHistory(activeFriend.id, serverMsgs, currentUser?.email, currentUser?.handle);
          setChatMessages(merged);
        }
      })
      .catch(() => {});
  }, [activeFriend?.id, activeFriend?.handle, activeFriend?.email, currentUser?.email, currentUser?.handle]);

  // Add Friend Modal State
  const [showAddFriendModal, setShowAddFriendModal] = useState(false);
  const [newFriendHandle, setNewFriendHandle] = useState("");
  const [newFriendName, setNewFriendName] = useState("");
  const [newFriendEmail, setNewFriendEmail] = useState("");
  const [verifiedPartner, setVerifiedPartner] = useState(null);
  const [verifyingHandle, setVerifyingHandle] = useState(false);
  const [friendError, setFriendError] = useState("");

  const chatEndRef = useRef(null);

  // Auto-scroll chat to bottom
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [chatMessages]);

  // Filtered communities
  const filteredCommunities = useMemo(() => {
    return communities.filter((c) => {
      const matchesSearch =
        c.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        c.description.toLowerCase().includes(searchQuery.toLowerCase());

      if (!matchesSearch) return false;
      if (communityTab === "my") return c.joined;
      if (communityTab === "discover") return !c.joined;
      return true;
    });
  }, [communities, searchQuery, communityTab]);

  // Filtered friends
  const filteredFriends = useMemo(() => {
    return friends.filter((f) =>
      f.name.toLowerCase().includes(friendSearch.toLowerCase())
    );
  }, [friends, friendSearch]);

  // Community handlers
  const handleJoin = (id, e) => {
    e?.stopPropagation();
    const updated = joinCommunity(id);
    setCommunities(updated);
    if (selectedCommunity?.id === id) {
      setSelectedCommunity(updated.find((c) => c.id === id));
    }
  };

  const handleLeave = (id, e) => {
    e?.stopPropagation();
    const updated = leaveCommunity(id);
    setCommunities(updated);
    if (selectedCommunity?.id === id) {
      setSelectedCommunity(updated.find((c) => c.id === id));
    }
  };

  const handleJoinByCode = (e) => {
    e.preventDefault();
    setCodeNotice({ error: "", success: "" });
    if (!privateCode.trim()) return;

    try {
      const updated = joinByCode(privateCode);
      setCommunities(updated);
      setPrivateCode("");
      setCodeNotice({ error: "", success: "Successfully joined quorum!" });
      setTimeout(() => setCodeNotice({ error: "", success: "" }), 3500);
    } catch (err) {
      setCodeNotice({ error: err.message, success: "" });
    }
  };

  const handleCopyCode = (code) => {
    navigator.clipboard.writeText(code);
    setCopiedCode(true);
    setTimeout(() => setCopiedCode(false), 2000);
  };

  const handleCreateCommunitySubmit = (e) => {
    e.preventDefault();
    if (!newCommName.trim()) {
      setCreateError("Name is required");
      return;
    }
    if (!newCommDesc.trim()) {
      setCreateError("Description is required");
      return;
    }

    const updated = createCommunity({
      name: newCommName,
      description: newCommDesc,
      category: newCommCategory,
      requiredApprovals: newCommApprovals,
      isPrivate: true,
    });

    setCommunities(updated);
    setShowCreateModal(false);
    setNewCommName("");
    setNewCommDesc("");
    setCreateError("");
  };

  // Friend handlers
  const handleVerifyTag = async () => {
    const clean = newFriendHandle.trim().replace(/^@/, "").toLowerCase();
    if (!clean) return;
    setVerifyingHandle(true);
    setFriendError("");
    try {
      const profile = await lookupUser(clean);
      if (profile) {
        setVerifiedPartner(profile);
        if (!newFriendName.trim() && profile.name) {
          setNewFriendName(profile.name);
        }
      }
    } catch (err) {
      setVerifiedPartner(null);
      setFriendError(err.message || "User not found with this handle");
    } finally {
      setVerifyingHandle(false);
    }
  };

  const handleAddFriendSubmit = async (e) => {
    e.preventDefault();
    const cleanHandle = newFriendHandle.trim().replace(/^@/, "").toLowerCase();
    if (!cleanHandle && !newFriendEmail.trim()) {
      setFriendError("Friend's ONUSLY handle is required (e.g. @himanshu)");
      return;
    }

    let friendData = {
      name: newFriendName.trim() || (verifiedPartner?.name || (cleanHandle ? `@${cleanHandle}` : "Friend")),
      handle: cleanHandle,
      email: newFriendEmail.trim(),
      picture: verifiedPartner?.picture || "",
    };

    if (cleanHandle && !verifiedPartner) {
      try {
        const profile = await lookupUser(cleanHandle);
        if (profile) {
          friendData.name = newFriendName.trim() || profile.name || `@${cleanHandle}`;
          friendData.picture = profile.picture;
        }
      } catch {
        // Fallback to manual add
      }
    }

    const updated = addFriend(friendData);

    setFriends(updated);
    setShowAddFriendModal(false);
    setNewFriendName("");
    setNewFriendHandle("");
    setNewFriendEmail("");
    setVerifiedPartner(null);
    setFriendError("");
    if (updated.length > 0) {
      selectFriend(updated[0].id);
    }
  };

  const handleSendMessage = (e) => {
    e.preventDefault();
    if (!messageInput.trim() || !activeFriendId) return;

    const text = messageInput.trim();
    const currentFriend = friends.find((f) => f.id === activeFriendId);
    const senderEmail = currentUser?.email || getUser()?.email || "";
    const senderHandle = currentUser?.handle || getUser()?.handle || "";

    // 1. Dispatch live over WebSocket to recipient
    console.log(`[Communities] Sending chat message to (${currentFriend?.email || ''}/@${currentFriend?.handle || ''}) from (${senderEmail}/@${senderHandle}): ${text}`);
    send({
      type: "chat.message",
      payload: {
        recipientEmail: currentFriend?.email || "",
        recipientHandle: currentFriend?.handle || "",
        senderEmail: senderEmail,
        senderHandle: senderHandle,
        text: text,
      },
    });

    // 2. Save locally for sender
    const updatedMessages = sendChatMessage(activeFriendId, text, "me");
    setChatMessages(updatedMessages);
    setMessageInput("");
    setFriends(getStoredFriends());
  };

  return (
    <AppLayout>
      <div className="mx-auto max-w-7xl space-y-6">
        {/* Top Header & Section Switcher */}
        <div className="flex flex-col justify-between gap-4 border-b border-[#2A2A2A] pb-6 sm:flex-row sm:items-center">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
              Community & Friends
            </h1>
            <p className="mt-1 text-sm text-[#A3A3A3]">
              Collaborate in private accountability quorums or chat 1-on-1 with dedicated partners.
            </p>
          </div>

          {/* Section Switcher Tabs */}
          <div className="flex items-center rounded-xl border border-[#333333] bg-[#1A1A1A] p-1 text-xs font-semibold">
            <button
              type="button"
              onClick={() => setMainView("communities")}
              className={`flex items-center gap-2 rounded-lg px-4 py-2 transition-all ${
                mainView === "communities"
                  ? "bg-white text-black font-bold shadow-sm"
                  : "text-[#A3A3A3] hover:text-white"
              }`}
            >
              <Users size={15} />
              Quorum Communities
            </button>
            <button
              type="button"
              onClick={() => setMainView("friends")}
              className={`flex items-center gap-2 rounded-lg px-4 py-2 transition-all ${
                mainView === "friends"
                  ? "bg-white text-black font-bold shadow-sm"
                  : "text-[#A3A3A3] hover:text-white"
              }`}
            >
              <MessageSquare size={15} />
              Friends Chat ({friends.length})
            </button>
          </div>
        </div>

        {/* ============================================================
            VIEW 1: COMMUNITIES SECTION
        ============================================================ */}
        {mainView === "communities" && (
          <div>
            {selectedCommunity ? (
              /* Selected Community Details */
              <div className="space-y-6">
                <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                  <div className="flex items-center gap-3">
                    <button
                      type="button"
                      onClick={() => setSelectedCommunity(null)}
                      className="rounded-xl border border-[#333333] bg-[#1A1A1A] p-2 text-white hover:border-white"
                    >
                      <ArrowLeft size={16} />
                    </button>
                    <div>
                      <h2 className="text-xl font-bold text-white">
                        {selectedCommunity.name}
                      </h2>
                      <p className="text-xs text-[#A3A3A3]">
                        {selectedCommunity.membersCount} Members • {selectedCommunity.category}
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    {selectedCommunity.joined ? (
                      <button
                        type="button"
                        onClick={(e) => handleLeave(selectedCommunity.id, e)}
                        className="rounded-xl border border-[#444444] bg-transparent px-4 py-2 text-xs font-semibold text-white hover:border-white"
                      >
                        Leave Quorum
                      </button>
                    ) : (
                      <button
                        type="button"
                        onClick={(e) => handleJoin(selectedCommunity.id, e)}
                        className="rounded-xl bg-white px-5 py-2 text-xs font-bold text-black hover:bg-neutral-200"
                      >
                        Join Quorum
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => handleCopyCode(selectedCommunity.code)}
                      className="flex items-center gap-1.5 rounded-xl border border-[#333333] bg-[#222222] px-3.5 py-2 text-xs font-semibold text-white hover:border-white"
                    >
                      <Copy size={14} />
                      {copiedCode ? "Copied" : selectedCommunity.code}
                    </button>
                  </div>
                </div>

                <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div className="lg:col-span-2 rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 space-y-4">
                    <h3 className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
                      Description & Rules
                    </h3>
                    <p className="text-sm leading-relaxed text-white">
                      {selectedCommunity.description}
                    </p>
                    <div className="border-t border-[#2B2B2B] pt-4 text-xs text-[#A3A3A3]">
                      <span>Required Approvals: </span>
                      <strong className="text-white">
                        {selectedCommunity.requiredApprovals} Peer Approvals
                      </strong>
                    </div>
                  </div>

                  <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 space-y-3">
                    <h3 className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
                      Quorum Code
                    </h3>
                    <div className="rounded-xl border border-[#333333] bg-[#121212] p-3 text-center font-mono text-base font-bold text-white">
                      {selectedCommunity.code}
                    </div>
                    <p className="text-[11px] text-[#A3A3A3]">
                      Share this private code to invite your trusted peers.
                    </p>
                  </div>
                </div>
              </div>
            ) : (
              /* Communities Explorer */
              <div className="space-y-6">
                <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex items-center gap-2">
                    <div className="relative w-72">
                      <Search
                        size={15}
                        className="absolute left-3.5 top-1/2 -translate-y-1/2 text-[#737373]"
                      />
                      <input
                        type="text"
                        placeholder="Search communities..."
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        className="w-full rounded-xl border border-[#333333] bg-[#1A1A1A] py-2 pl-9 pr-3 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                      />
                    </div>

                    <form onSubmit={handleJoinByCode} className="flex items-center gap-1.5">
                      <input
                        type="text"
                        placeholder="Private Code"
                        value={privateCode}
                        onChange={(e) => setPrivateCode(e.target.value)}
                        className="w-32 rounded-xl border border-[#333333] bg-[#1A1A1A] py-2 px-3 text-xs text-white uppercase placeholder-[#737373] focus:border-white focus:outline-none"
                      />
                      <button
                        type="submit"
                        className="rounded-xl border border-[#444444] bg-[#222222] px-3.5 py-2 text-xs font-semibold text-white hover:border-white"
                      >
                        Join
                      </button>
                    </form>
                  </div>

                  <button
                    type="button"
                    onClick={() => setShowCreateModal(true)}
                    className="flex items-center justify-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black hover:bg-neutral-200"
                  >
                    <Plus size={16} />
                    Create Community
                  </button>
                </div>

                {codeNotice.error && (
                  <div className="rounded-xl border border-[#444444] bg-[#222222] p-3 text-xs text-white">
                    {codeNotice.error}
                  </div>
                )}
                {codeNotice.success && (
                  <div className="rounded-xl border border-white bg-white/10 p-3 text-xs font-semibold text-white">
                    {codeNotice.success}
                  </div>
                )}

                {filteredCommunities.length === 0 ? (
                  <div className="rounded-3xl border border-dashed border-[#333333] bg-[#1A1A1A]/40 p-12 text-center">
                    <Users size={36} className="mx-auto text-[#737373] mb-3" />
                    <h3 className="text-base font-bold text-white">No communities yet</h3>
                    <p className="mt-1 text-xs text-[#A3A3A3]">
                      Click "+ Create Community" above to build your own quorum with customized rules.
                    </p>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                    {filteredCommunities.map((c) => (
                      <div
                        key={c.id}
                        onClick={() => setSelectedCommunity(c)}
                        className="cursor-pointer rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 transition-all hover:border-white"
                      >
                        <div className="flex items-center justify-between">
                          <h3 className="text-base font-bold text-white">{c.name}</h3>
                          <span className="rounded-full border border-[#444444] px-2.5 py-0.5 text-[10px] uppercase font-semibold text-[#A3A3A3]">
                            {c.category}
                          </span>
                        </div>
                        <p className="mt-3 text-xs leading-relaxed text-[#A3A3A3] line-clamp-2">
                          {c.description}
                        </p>
                        <div className="mt-5 flex items-center justify-between border-t border-[#2B2B2B] pt-4 text-xs">
                          <span className="text-[#737373]">{c.membersCount} Members</span>
                          <span className="font-semibold text-white">View Quorum →</span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {/* ============================================================
            VIEW 2: WHATSAPP-STYLE FRIENDS & CHAT SECTION
        ============================================================ */}
        {mainView === "friends" && (
          <div className="grid grid-cols-1 overflow-hidden rounded-2xl border border-[#2B2B2B] bg-[#181818] shadow-2xl md:grid-cols-12 min-h-[580px]">
            {/* Left Column: Friends List (WhatsApp style) */}
            <div className="border-b border-[#2A2A2A] bg-[#141414] p-4 md:col-span-4 md:border-b-0 md:border-r">
              {/* Header with Add Friend */}
              <div className="flex items-center justify-between pb-3">
                <span className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
                  Friends List ({friends.length})
                </span>
                <button
                  type="button"
                  onClick={() => setShowAddFriendModal(true)}
                  className="flex items-center gap-1.5 rounded-lg border border-[#444444] bg-[#222222] px-2.5 py-1.5 text-xs font-semibold text-white hover:border-white"
                >
                  <UserPlus size={14} />
                  Add Friend
                </button>
              </div>

              {/* Search Friends */}
              <div className="relative mt-2">
                <Search
                  size={14}
                  className="absolute left-3 top-1/2 -translate-y-1/2 text-[#737373]"
                />
                <input
                  type="text"
                  placeholder="Search friends..."
                  value={friendSearch}
                  onChange={(e) => setFriendSearch(e.target.value)}
                  className="w-full rounded-xl border border-[#2A2A2A] bg-[#1E1E1E] py-2 pl-8 pr-3 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                />
              </div>

              {/* Friends List items */}
              <div className="mt-4 space-y-1 max-h-[460px] overflow-y-auto">
                {filteredFriends.length === 0 ? (
                  <div className="py-12 text-center">
                    <p className="text-xs text-[#737373]">No friends added yet.</p>
                    <button
                      type="button"
                      onClick={() => setShowAddFriendModal(true)}
                      className="mt-3 text-xs font-semibold text-white underline"
                    >
                      + Add your first friend
                    </button>
                  </div>
                ) : (
                  filteredFriends.map((f) => {
                    const isSelected = f.id === activeFriendId;
                    return (
                      <div
                        key={f.id}
                        onClick={() => selectFriend(f.id)}
                        className={`flex cursor-pointer items-center justify-between rounded-xl p-3 transition-colors ${
                          isSelected
                            ? "bg-[#252525] border border-white/20"
                            : "hover:bg-[#1E1E1E]"
                        }`}
                      >
                        <div className="flex items-center gap-3 min-w-0">
                          <div className="relative flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full border border-[#444444] bg-[#2E2E2E] text-xs font-bold text-white overflow-hidden">
                            {f.picture ? (
                              <img src={f.picture} alt={f.name} className="h-full w-full object-cover" referrerPolicy="no-referrer" />
                            ) : (
                              f.name[0]?.toUpperCase() || "F"
                            )}
                            <span
                              className={`absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-[#1E1E1E] ${
                                (f.handle && onlineHandles.has(f.handle.toLowerCase().replace(/^@/, ""))) ||
                                (f.email && onlineEmails.has(f.email.trim().toLowerCase()))
                                  ? "bg-emerald-400"
                                  : "bg-neutral-600"
                              }`}
                              title={
                                (f.handle && onlineHandles.has(f.handle.toLowerCase().replace(/^@/, ""))) ||
                                (f.email && onlineEmails.has(f.email.trim().toLowerCase()))
                                  ? "Online"
                                  : "Offline"
                              }
                            />
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-1.5">
                              <p className="truncate text-xs font-semibold text-white">
                                {f.name}
                              </p>
                              {f.handle && (
                                <span className="font-mono text-[10px] text-[#A3A3A3]">
                                  @{f.handle.replace(/^@/, "")}
                                </span>
                              )}
                            </div>
                            <p className="truncate text-[11px] text-[#737373]">
                              {f.lastMessage || "Click to start chatting"}
                            </p>
                          </div>
                        </div>

                        {f.lastMessageTime && (
                          <span className="text-[10px] text-[#737373] flex-shrink-0 ml-2">
                            {f.lastMessageTime}
                          </span>
                        )}
                      </div>
                    );
                  })
                )}
              </div>
            </div>

            {/* Right Column: Chat Box (WhatsApp style) */}
            <div className="flex flex-col justify-between bg-[#121212] md:col-span-8">
              {activeFriend ? (
                <>
                  {/* Chat Header */}
                  <div className="flex items-center justify-between border-b border-[#2A2A2A] bg-[#181818] p-4">
                    <div className="flex items-center gap-3">
                      <div className="relative flex h-10 w-10 items-center justify-center rounded-full border border-[#444444] bg-[#282828] text-sm font-bold text-white overflow-hidden">
                        {activeFriend.picture ? (
                          <img src={activeFriend.picture} alt={activeFriend.name} className="h-full w-full object-cover" referrerPolicy="no-referrer" />
                        ) : (
                          activeFriend.name[0]?.toUpperCase()
                        )}
                        <span
                          className={`absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-[#181818] ${
                            (activeFriend.handle && onlineHandles.has(activeFriend.handle.toLowerCase().replace(/^@/, ""))) ||
                            (activeFriend.email && onlineEmails.has(activeFriend.email.trim().toLowerCase()))
                              ? "bg-emerald-400"
                              : "bg-neutral-600"
                          }`}
                        />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <h3 className="text-sm font-bold text-white">
                            {activeFriend.name}
                          </h3>
                          {activeFriend.handle && (
                            <span className="rounded-md border border-[#3A3A3A] bg-[#222222] px-2 py-0.5 font-mono text-[11px] font-semibold text-white">
                              @{activeFriend.handle.replace(/^@/, "")}
                            </span>
                          )}
                        </div>
                        <p className="text-[11px] text-[#A3A3A3] flex items-center gap-1.5 mt-0.5">
                          <span>Accountability Partner</span>
                          <span>•</span>
                          {(() => {
                            const isFriendOnline = Boolean(
                              (activeFriend.handle && onlineHandles.has(activeFriend.handle.toLowerCase().replace(/^@/, ""))) ||
                              (activeFriend.email && onlineEmails.has(activeFriend.email.trim().toLowerCase()))
                            );
                            return (
                              <span className={`flex items-center gap-1 font-medium ${isFriendOnline ? "text-emerald-400" : "text-neutral-400"}`}>
                                <span className={`h-1.5 w-1.5 rounded-full ${isFriendOnline ? "bg-emerald-400 animate-pulse" : "bg-neutral-500"}`} />
                                {isFriendOnline ? "Online" : "Offline"}
                              </span>
                            );
                          })()}
                        </p>
                      </div>
                    </div>
                  </div>

                  {/* Message Stream */}
                  <div className="flex-1 space-y-3 overflow-y-auto p-4 max-h-[420px]">
                    {chatMessages.length === 0 ? (
                      <div className="flex h-full flex-col items-center justify-center py-16 text-center">
                        <MessageSquare size={32} className="text-[#444444] mb-2" />
                        <p className="text-xs font-medium text-[#A3A3A3]">
                          No messages yet. Send a message to coordinate your goals!
                        </p>
                      </div>
                    ) : (
                      chatMessages.map((msg) => {
                        const isMe = msg.sender === "me";
                        return (
                          <div
                            key={msg.id}
                            className={`flex ${isMe ? "justify-end" : "justify-start"}`}
                          >
                            <div
                              className={`max-w-[75%] rounded-2xl px-4 py-2.5 text-xs ${
                                isMe
                                  ? "bg-white text-black font-medium"
                                  : "border border-[#333333] bg-[#222222] text-white"
                              }`}
                            >
                              <p className="leading-relaxed">{msg.text}</p>
                              <span
                                className={`mt-1 block text-[9px] text-right ${
                                  isMe ? "text-neutral-500" : "text-[#737373]"
                                }`}
                              >
                                {msg.time}
                              </span>
                            </div>
                          </div>
                        );
                      })
                    )}
                    <div ref={chatEndRef} />
                  </div>

                  {/* Chat Input */}
                  <form
                    onSubmit={handleSendMessage}
                    className="flex items-center gap-2 border-t border-[#2A2A2A] bg-[#181818] p-3"
                  >
                    <input
                      type="text"
                      placeholder="Type a message or share goal progress..."
                      value={messageInput}
                      onChange={(e) => setMessageInput(e.target.value)}
                      className="flex-1 rounded-xl border border-[#333333] bg-[#121212] px-4 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                    />
                    <button
                      type="submit"
                      disabled={!messageInput.trim()}
                      className="flex h-9 w-9 items-center justify-center rounded-xl bg-white text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
                    >
                      <Send size={15} />
                    </button>
                  </form>
                </>
              ) : (
                <div className="flex h-full flex-col items-center justify-center p-8 text-center">
                  <Users size={40} className="text-[#444444] mb-3" />
                  <h3 className="text-sm font-bold text-white">No Friend Selected</h3>
                  <p className="mt-1 text-xs text-[#737373]">
                    Select a friend on the left or add a new friend to start your accountability chat.
                  </p>
                </div>
              )}
            </div>
          </div>
        )}

        {/* ============================================================
            CREATE COMMUNITY MODAL (Black & White)
        ============================================================ */}
        {showCreateModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-lg rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl">
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <h3 className="text-base font-bold text-white">
                  Create Quorum Community
                </h3>
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="rounded-lg p-1 text-[#737373] hover:text-white"
                >
                  <X size={18} />
                </button>
              </div>

              {createError && (
                <div className="mt-4 rounded-xl border border-[#444444] bg-[#252525] p-3 text-xs text-white">
                  {createError}
                </div>
              )}

              <form onSubmit={handleCreateCommunitySubmit} className="mt-4 space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                    Community Name
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="e.g. DSA Booster"
                    value={newCommName}
                    onChange={(e) => setNewCommName(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] px-3.5 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                    Category
                  </label>
                  <select
                    value={newCommCategory}
                    onChange={(e) => setNewCommCategory(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] px-3.5 py-2.5 text-xs text-white focus:border-white focus:outline-none"
                  >
                    <option value="Coding & Algorithms">Coding & Algorithms</option>
                    <option value="Software Architecture">Software Architecture</option>
                    <option value="Writing & Content">Writing & Content</option>
                    <option value="Deep Work & Discipline">Deep Work & Discipline</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                    Quorum Description & Accountability Pledge
                  </label>
                  <textarea
                    rows={3}
                    required
                    placeholder="Describe what members lock out and how proofs are verified..."
                    value={newCommDesc}
                    onChange={(e) => setNewCommDesc(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] px-3.5 py-2 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                    Required Peer Approvals
                  </label>
                  <div className="flex gap-2">
                    {[1, 2, 3].map((num) => (
                      <button
                        key={num}
                        type="button"
                        onClick={() => setNewCommApprovals(num)}
                        className={`flex-1 rounded-xl border py-2 text-xs font-bold ${
                          newCommApprovals === num
                            ? "border-white bg-white text-black"
                            : "border-[#333333] bg-[#121212] text-[#A3A3A3] hover:text-white"
                        }`}
                      >
                        {num} {num === 1 ? "Peer" : "Peers"}
                      </button>
                    ))}
                  </div>
                </div>

                <div className="flex justify-end gap-2 pt-3 border-t border-[#2A2A2A]">
                  <button
                    type="button"
                    onClick={() => setShowCreateModal(false)}
                    className="rounded-xl border border-[#333333] px-4 py-2 text-xs font-semibold text-[#A3A3A3] hover:text-white"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    className="rounded-xl bg-white px-5 py-2 text-xs font-bold text-black hover:bg-neutral-200"
                  >
                    Create Quorum
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}

        {/* ============================================================
            ADD FRIEND MODAL (100% Privacy Protection via @handle)
        ============================================================ */}
        {showAddFriendModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-md rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl">
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <div>
                  <h3 className="text-base font-bold text-white">
                    Add Accountability Friend
                  </h3>
                  <p className="mt-0.5 text-xs text-[#A3A3A3]">
                    Connect using their unique public handle (@tag).
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => setShowAddFriendModal(false)}
                  className="rounded-lg p-1 text-[#737373] hover:text-white"
                >
                  <X size={18} />
                </button>
              </div>

              {/* Privacy Shield Banner */}
              <div className="mt-4 flex items-start gap-2.5 rounded-xl border border-[#333333] bg-[#141414] p-3 text-[11px] text-[#A3A3A3] leading-relaxed">
                <ShieldCheck size={16} className="text-white flex-shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold text-white">100% Privacy Protected:</span> Connect with friends using their public ONUSLY handle. No personal Google email address is exposed.
                </div>
              </div>

              {friendError && (
                <div className="mt-4 rounded-xl border border-red-900/50 bg-red-950/40 p-3 text-xs text-red-300">
                  {friendError}
                </div>
              )}

              <form onSubmit={handleAddFriendSubmit} className="mt-4 space-y-4">
                {/* Handle Input with Verification */}
                <div>
                  <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                    Friend's ONUSLY Tag (@handle)
                  </label>
                  <div className="flex gap-2">
                    <div className="relative flex-1">
                      <span className="absolute left-3.5 top-2.5 font-mono text-xs text-[#737373]">
                        @
                      </span>
                      <input
                        type="text"
                        required
                        placeholder="e.g. robert_01"
                        value={newFriendHandle}
                        onChange={(e) => {
                          setNewFriendHandle(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ""));
                          setVerifiedPartner(null);
                        }}
                        className="w-full rounded-xl border border-[#333333] bg-[#121212] pl-8 pr-3.5 py-2.5 font-mono text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                      />
                    </div>
                    <button
                      type="button"
                      onClick={handleVerifyTag}
                      disabled={!newFriendHandle.trim() || verifyingHandle}
                      className="rounded-xl border border-[#3A3A3A] bg-[#222222] px-3.5 py-2 text-xs font-semibold text-white transition hover:border-white disabled:opacity-40"
                    >
                      {verifyingHandle ? <Loader2 size={14} className="animate-spin" /> : "Verify Tag"}
                    </button>
                  </div>
                </div>

                {/* Verified Partner Badge */}
                {verifiedPartner && (
                  <div className="flex items-center gap-3 rounded-xl border border-white/20 bg-white/5 p-3">
                    <div className="flex h-9 w-9 items-center justify-center rounded-full bg-white text-black font-bold text-xs overflow-hidden">
                      {verifiedPartner.picture ? (
                        <img src={verifiedPartner.picture} alt={verifiedPartner.name} className="h-full w-full object-cover" referrerPolicy="no-referrer" />
                      ) : (
                        verifiedPartner.name?.[0]?.toUpperCase() || "U"
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-xs font-bold text-white flex items-center gap-1.5">
                        {verifiedPartner.name}
                        <Check size={13} className="text-emerald-400" />
                      </p>
                      <p className="font-mono text-[10px] text-[#A3A3A3]">
                        @{verifiedPartner.handle}
                      </p>
                    </div>
                  </div>
                )}

                {/* Friend's Name (Optional Customization) */}
                <div>
                  <label className="block text-xs font-semibold text-[#A3A3A3] mb-1">
                    Display Nickname (Optional)
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. Alex"
                    value={newFriendName}
                    onChange={(e) => setNewFriendName(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#121212] px-3.5 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                <div className="flex justify-end gap-2 pt-3 border-t border-[#2A2A2A]">
                  <button
                    type="button"
                    onClick={() => {
                      setShowAddFriendModal(false);
                      setVerifiedPartner(null);
                    }}
                    className="rounded-xl border border-[#333333] px-4 py-2 text-xs font-semibold text-[#A3A3A3] hover:text-white"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    className="rounded-xl bg-white px-5 py-2 text-xs font-bold text-black hover:bg-neutral-200"
                  >
                    Add to Friends
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}
