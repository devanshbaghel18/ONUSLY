import { useState, useMemo, useEffect, useRef } from "react";
import { Link, useLocation } from "react-router-dom";
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
  Paperclip,
  Trash2,
  ExternalLink,
  CheckCheck,
  Image as ImageIcon,
  LinkIcon,
  ThumbsUp,
  FileText,
  CheckCircle2,
  MoreVertical,
  Star,
  Reply,
  RotateCcw,
} from "lucide-react";
import {
  getStoredCommunities,
  joinCommunity,
  leaveCommunity,
  deleteCommunity,
  joinByCode,
  addOrUpdateCommunity,
  createCommunity,
  getCommunityMessages,
  sendCommunityMessage,
  receiveCommunityMessage,
  deleteCommunityMessage,
  deleteCommunityMessageForMe,
  clearCommunityChat,
  toggleFavoriteCommunity,
  markCommunityMessageApproved,
} from "../lib/communities";
import {
  getStoredFriends,
  addFriend,
  deleteFriend,
  getChatMessages,
  sendChatMessage,
  receiveChatMessage,
  deleteChatMessageGlobally,
  deleteChatMessageForMe,
  clearFriendChat,
  toggleFavoriteFriend,
  mergeChatHistory,
  markChatMessageApproved,
  clearUnreadCount,
} from "../lib/friendsChat";
import { getChatHistory, lookupUser } from "../lib/api";
import { getUser } from "../lib/auth";
import { useWebSocket } from "../hooks/useWebSocket";
import { wsManager } from "../lib/websocket";

function generateMsgId(prefix = "msg") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
}

// ==========================================
// WHATSAPP-STYLE IMAGE COLLAGE GRID
// ==========================================
function WhatsAppImageGrid({ images, onImageClick }) {
  if (!images || images.length === 0) return null;

  if (images.length === 1) {
    return (
      <div
        className="mt-2 overflow-hidden rounded-xl border border-white/10 max-w-sm cursor-pointer hover:opacity-95 transition"
        onClick={() => onImageClick(images[0])}
      >
        <img
          src={images[0]}
          alt="Proof"
          className="w-full max-h-72 object-cover rounded-xl"
        />
      </div>
    );
  }

  if (images.length === 2) {
    return (
      <div className="mt-2 grid grid-cols-2 gap-1.5 max-w-sm rounded-xl overflow-hidden border border-white/10">
        {images.map((img, i) => (
          <div
            key={i}
            className="aspect-square cursor-pointer hover:opacity-90 overflow-hidden"
            onClick={() => onImageClick(img)}
          >
            <img src={img} alt={`Proof ${i + 1}`} className="h-full w-full object-cover" />
          </div>
        ))}
      </div>
    );
  }

  if (images.length === 3) {
    return (
      <div className="mt-2 grid grid-cols-2 gap-1.5 max-w-sm rounded-xl overflow-hidden border border-white/10">
        <div
          className="col-span-2 aspect-[16/9] cursor-pointer hover:opacity-90 overflow-hidden"
          onClick={() => onImageClick(images[0])}
        >
          <img src={images[0]} alt="Proof 1" className="h-full w-full object-cover" />
        </div>
        <div
          className="aspect-square cursor-pointer hover:opacity-90 overflow-hidden"
          onClick={() => onImageClick(images[1])}
        >
          <img src={images[1]} alt="Proof 2" className="h-full w-full object-cover" />
        </div>
        <div
          className="aspect-square cursor-pointer hover:opacity-90 overflow-hidden"
          onClick={() => onImageClick(images[2])}
        >
          <img src={images[2]} alt="Proof 3" className="h-full w-full object-cover" />
        </div>
      </div>
    );
  }

  // 4 or more images: WhatsApp 2x2 grid with +N overlay on 4th image
  const displayImages = images.slice(0, 4);
  const remainingCount = images.length - 4;

  return (
    <div className="mt-2 grid grid-cols-2 gap-1.5 max-w-sm rounded-xl overflow-hidden border border-white/10">
      {displayImages.map((img, i) => {
        const isFourth = i === 3 && remainingCount > 0;
        return (
          <div
            key={i}
            className="relative aspect-square cursor-pointer hover:opacity-90 overflow-hidden bg-black/40"
            onClick={() => onImageClick(img)}
          >
            <img src={img} alt={`Proof ${i + 1}`} className="h-full w-full object-cover" />
            {isFourth && (
              <div className="absolute inset-0 flex items-center justify-center bg-black/65 backdrop-blur-[2px]">
                <span className="text-xl font-extrabold text-white">+{remainingCount}</span>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

export default function Communities() {
  const location = useLocation();
  // Main mode: 'communities' | 'friends'
  const [mainView, setMainView] = useState(() => {
    if (location.state?.view) return location.state.view;
    if (location.pathname === "/community-friends") return "friends";
    return "communities";
  });

  // ==========================================
  // COMMUNITIES STATE
  // ==========================================
  const [communities, setCommunities] = useState(() => getStoredCommunities());
  const [searchQuery, setSearchQuery] = useState("");
  const [privateCode, setPrivateCode] = useState("");
  const [codeNotice, setCodeNotice] = useState({ error: "", success: "" });

  const [selectedCommunity, setSelectedCommunity] = useState(null);
  const selectedCommunityIdRef = useRef(null);
  useEffect(() => {
    selectedCommunityIdRef.current = selectedCommunity?.id || null;
  }, [selectedCommunity]);

  const [commChatMessages, setCommChatMessages] = useState([]);
  const [commMessageInput, setCommMessageInput] = useState("");
  const [copiedCode, setCopiedCode] = useState(false);

  // Create Community Modal State
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newCommName, setNewCommName] = useState("");
  const [newCommDesc, setNewCommDesc] = useState("");
  const [newCommCategory, setNewCommCategory] = useState("General Focus");
  const [newCommApprovals, setNewCommApprovals] = useState(2);
  const [selectedFriendsForCommunity, setSelectedFriendsForCommunity] = useState([]);
  const [createError, setCreateError] = useState("");

  // ==========================================
  // FRIENDS & 1-ON-1 CHAT STATE
  // ==========================================
  const [friends, setFriends] = useState(() => getStoredFriends());
  const [friendSearch, setFriendSearch] = useState("");
  const [activeFriendId, setActiveFriendId] = useState(() => {
    if (location.state?.view === "friends" && location.state?.targetId) {
      return location.state.targetId;
    }
    return null;
  });
  const activeFriend = friends.find((f) => f.id === activeFriendId);
  const activeFriendIdRef = useRef(activeFriendId);
  useEffect(() => {
    activeFriendIdRef.current = activeFriendId;
  }, [activeFriendId]);

  const currentUser = getUser();
  const [chatMessages, setChatMessages] = useState(() => {
    if (location.state?.view === "friends" && location.state?.targetId) {
      return getChatMessages(location.state.targetId);
    }
    return [];
  });
  const [messageInput, setMessageInput] = useState("");
  const [onlineEmails, setOnlineEmails] = useState(new Set());
  const [onlineHandles, setOnlineHandles] = useState(new Set());

  // Confirm delete modals/prompts
  const [friendToDelete, setFriendToDelete] = useState(null);
  const [commToDelete, setCommToDelete] = useState(null);
  const [commToLeave, setCommToLeave] = useState(null);

  // WhatsApp-style Features: Reply, 3-Dot Dropdown, Delete Modals, Favorites
  const [replyingTo, setReplyingTo] = useState(null);
  const [showFriendMenu, setShowFriendMenu] = useState(false);
  const [showCommMenu, setShowCommMenu] = useState(false);
  const [deleteMsgModal, setDeleteMsgModal] = useState({
    isOpen: false,
    msg: null,
    isMe: false,
    type: "friend",
  });
  const [clearChatModal, setClearChatModal] = useState({
    isOpen: false,
    type: "friend",
    targetName: "",
  });
  const friendMenuRef = useRef(null);
  const commMenuRef = useRef(null);

  // Jump to quoted message with highlight animation (WhatsApp style)
  const [highlightedMsgId, setHighlightedMsgId] = useState(null);

  const handleJumpToMessage = (targetMsgId) => {
    if (!targetMsgId) return;
    const element = document.getElementById(`msg-${targetMsgId}`);
    if (element) {
      element.scrollIntoView({ behavior: "smooth", block: "center" });
      setHighlightedMsgId(targetMsgId);
      setTimeout(() => {
        setHighlightedMsgId((curr) => (curr === targetMsgId ? null : curr));
      }, 2200);
    }
  };

  // Close menus on click outside
  useEffect(() => {
    const handleClickOutside = (e) => {
      if (friendMenuRef.current && !friendMenuRef.current.contains(e.target)) {
        setShowFriendMenu(false);
      }
      if (commMenuRef.current && !commMenuRef.current.contains(e.target)) {
        setShowCommMenu(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Lightbox Preview State
  const [lightboxImage, setLightboxImage] = useState(null);

  // ==========================================
  // WHATSAPP-STYLE ATTACHMENT STATE (simplified - no mandatory goal)
  // ==========================================
  const [showAttachModal, setShowAttachModal] = useState(false);
  const [attachTargetType, setAttachTargetType] = useState("friend"); // 'friend' | 'community'
  const [attachFiles, setAttachFiles] = useState([]); // Array of { file, previewUrl, name }
  const [attachCaption, setAttachCaption] = useState("");
  const [attachLink, setAttachLink] = useState("");
  const [attachSending, setAttachSending] = useState(false);

  // Approval in-chat state
  const [approvingMsgId, setApprovingMsgId] = useState(null);

  // Navigation state handler (e.g. redirected from "Share for Verification")
  useEffect(() => {
    if (!location.state) return;
    const timer = setTimeout(() => {
      if (location.state?.view) {
        setMainView(location.state.view);
      }
      if (location.state?.view === "friends" && location.state?.targetId) {
        setActiveFriendId(location.state.targetId);
        const updated = clearUnreadCount(location.state.targetId);
        setFriends(updated);
        setChatMessages(getChatMessages(location.state.targetId));
      }
      if (location.state?.view === "communities" && location.state?.targetId) {
        const allComms = getStoredCommunities();
        const targetComm = allComms.find((c) => c.id === location.state.targetId);
        if (targetComm) {
          setSelectedCommunity(targetComm);
          setCommChatMessages(getCommunityMessages(targetComm.id));
        }
      }
    }, 0);
    return () => clearTimeout(timer);
  }, [location.state]);

  // Refs for auto-scroll and container scrolling
  const chatEndRef = useRef(null);
  const commChatEndRef = useRef(null);
  const chatContainerRef = useRef(null);
  const commChatContainerRef = useRef(null);
  const isAtBottomRef = useRef(true);
  const isCommAtBottomRef = useRef(true);

  // ==========================================
  // WEBSOCKET REALTIME CONNECTION
  // ==========================================
  const { isConnected, send } = useWebSocket((event) => {
    if (event?.type === "presence.list") {
      const list = event.payload?.onlineEmails || [];
      const newEmails = new Set(list.map((e) => String(e).toLowerCase()));
      const handles = event.payload?.onlineHandles || [];
      const newHandles = new Set(handles.map((h) => String(h).toLowerCase().replace(/[@\s]/g, "")));

      setOnlineEmails((prev) => {
        if (prev.size === newEmails.size && [...newEmails].every((e) => prev.has(e))) {
          return prev;
        }
        return newEmails;
      });

      setOnlineHandles((prev) => {
        if (prev.size === newHandles.size && [...newHandles].every((h) => prev.has(h))) {
          return prev;
        }
        return newHandles;
      });
    }

    // 1-on-1 Chat message received
    if (event?.type === "chat.message") {
      const p = event.payload || {};
      const timeFormatted = p.time
        ? new Date(p.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
        : new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

      const currentActiveId = activeFriendIdRef.current;
      const currentFriendsList = getStoredFriends();
      const currentActive = currentFriendsList.find((f) => f.id === currentActiveId);

      const senderEmail = (p.senderEmail || "").trim().toLowerCase();
      const senderHandle = (p.senderHandle || "").replace(/[@\s]/g, "").toLowerCase();

      // Check whether user is currently looking at this sender's chat
      const isViewing =
        Boolean(currentActiveId) &&
        (currentActiveId === p.id ||
          (currentActive?.handle && senderHandle && currentActive.handle.toLowerCase().replace(/[@\s]/g, "") === senderHandle) ||
          (currentActive?.email && senderEmail && currentActive.email.toLowerCase() === senderEmail));

      const { friend, messages } = receiveChatMessage({
        messageId: p.id,
        friendEmail: p.senderEmail,
        friendHandle: p.senderHandle,
        friendName: p.senderName,
        text: p.text,
        time: timeFormatted,
        sender: "friend",
        isProof: p.isProof,
        goalId: p.goalId,
        goalTitle: p.goalTitle,
        ownerId: p.ownerId,
        approved: p.approved,
        images: p.images,
        files: p.files,
        externalLink: p.externalLink,
        isCurrentlyActive: isViewing,
        replyTo: p.replyTo || null,
      });

      const updatedFriends = getStoredFriends();
      setFriends(updatedFriends);

      // Only update messages if user is actively viewing this friend. NEVER auto-open or switch chat!
      if (friend && isViewing) {
        setChatMessages(messages);
        setTimeout(() => {
          if (isAtBottomRef.current && chatContainerRef.current) {
            chatContainerRef.current.scrollTo({
              top: chatContainerRef.current.scrollHeight,
              behavior: "smooth",
            });
          }
        }, 10);
      }
    }

    // 1-on-1 Message deleted (WhatsApp style - deletes on both sides immediately)
    if (event?.type === "chat.delete") {
      const delId = event.payload?.id;
      if (delId) {
        deleteChatMessageGlobally(delId);
        setChatMessages((prev) => prev.filter((m) => m.id !== delId));
        setFriends(getStoredFriends());
      }
    }

    // Real-time approval sync across chats
    if (event?.type === "chat.message.approved" || event?.type === "goal.unlocked") {
      const { messageId, goalId } = event.payload || {};
      if (messageId || goalId) {
        setChatMessages((prev) =>
          prev.map((m) =>
            (messageId && m.id === messageId) || (goalId && m.goalId === goalId)
              ? { ...m, approved: true }
              : m
          )
        );
        setCommChatMessages((prev) =>
          prev.map((m) =>
            (messageId && m.id === messageId) || (goalId && m.goalId === goalId)
              ? { ...m, approved: true }
              : m
          )
        );
        markChatMessageApproved(goalId, messageId);
        markCommunityMessageApproved(goalId, messageId);
      }
    }

    // Community Group Chat message received
    if (event?.type === "community.message") {
      const p = event.payload || {};
      if (p.communityId) {
        // Auto-create community card if recipient doesn't have it yet
        if (!getStoredCommunities().some((c) => c.id === p.communityId)) {
          const autoComm = {
            id: p.communityId,
            name: p.communityName || "Community",
            code: p.communityCode || "",
            category: "General Focus",
            joined: true,
            membersCount: 2,
            requiredApprovals: 2,
          };
          const updatedCommList = addOrUpdateCommunity(autoComm);
          setCommunities(updatedCommList);
        }

        const isFromMe = Boolean(
          (currentUser?.id && p.senderId && p.senderId === currentUser.id) ||
          (currentUser?.email && p.senderEmail && p.senderEmail.toLowerCase() === currentUser.email.toLowerCase()) ||
          (currentUser?.handle && p.senderHandle && p.senderHandle.toLowerCase().replace(/^@/, "") === currentUser.handle.toLowerCase().replace(/^@/, ""))
        );

        const updated = receiveCommunityMessage(p.communityId, {
          id: p.id,
          sender: isFromMe ? "me" : "member",
          senderName: p.senderName,
          senderHandle: p.senderHandle,
          text: p.text,
          isProof: p.isProof,
          goalId: p.goalId,
          goalTitle: p.goalTitle,
          ownerId: p.ownerId,
          approved: p.approved,
          images: p.images,
          files: p.files,
          externalLink: p.externalLink,
          time: p.time,
          replyTo: p.replyTo || null,
        });
        setCommunities(getStoredCommunities());
        if (selectedCommunityIdRef.current === p.communityId) {
          setCommChatMessages(updated);
          setTimeout(() => {
            if (isCommAtBottomRef.current && commChatContainerRef.current) {
              commChatContainerRef.current.scrollTo({
                top: commChatContainerRef.current.scrollHeight,
                behavior: "smooth",
              });
            }
          }, 10);
        }
      }
    }

    // Community Message deleted
    if (event?.type === "community.delete_message") {
      const { communityId, messageId } = event.payload || {};
      if (communityId && messageId) {
        deleteCommunityMessage(communityId, messageId);
        if (selectedCommunityIdRef.current === communityId) {
          setCommChatMessages((prev) => prev.filter((m) => m.id !== messageId));
        }
        setCommunities(getStoredCommunities());
      }
    }

    // Community Join By Code Success from Realtime Gateway
    if (event?.type === "community.join_success") {
      const commData = event.payload?.community;
      if (commData) {
        const updated = addOrUpdateCommunity({ ...commData, joined: true });
        setCommunities(updated);
        setSelectedCommunity(commData);
        setPrivateCode("");
        setCodeNotice({ error: "", success: `Joined "${commData.name}" successfully!` });
        setTimeout(() => setCodeNotice({ error: "", success: "" }), 3000);
        wsManager.send({
          type: "community.join_room",
          payload: { communityId: commData.id },
        });
      }
    }

    // Community Join By Code Error from Realtime Gateway
    if (event?.type === "community.join_error") {
      const errMsg = event.payload?.error || "No community found with that invite code.";
      setCodeNotice({ error: errMsg, success: "" });
    }

    // Community Auto-Join / Realtime Sync from friend who created community
    if (event?.type === "community.sync" || event?.type === "community.auto_join") {
      const commData = event.payload?.community;
      if (commData) {
        const updated = addOrUpdateCommunity({ ...commData, joined: true });
        setCommunities(updated);
        // Automatically join room so messages stream live
        wsManager.send({
          type: "community.join_room",
          payload: { communityId: commData.id },
        });
      }
    }
  });

  // Query online presence and announce communities on connect
  useEffect(() => {
    if (!isConnected) return;
    send({ type: "presence.query" });

    // Sync all existing local communities with backend Hub so their invite codes are immediately searchable
    const localComms = getStoredCommunities();
    localComms.forEach((comm) => {
      send({
        type: "community.register",
        payload: {
          community: comm,
          members: (comm.members || []).map((m) => ({
            handle: m.handle || "",
            email: m.email || "",
            name: m.name || "",
          })),
        },
      });
    });

    const interval = setInterval(() => {
      send({ type: "presence.query" });
    }, 6000);

    return () => clearInterval(interval);
  }, [isConnected, send]);

  // Join/leave community room when active community changes
  useEffect(() => {
    if (!selectedCommunity || !selectedCommunity.joined || !isConnected) return;
    send({
      type: "community.join_room",
      payload: { communityId: selectedCommunity.id },
    });
    const timer = setTimeout(() => {
      setCommChatMessages(getCommunityMessages(selectedCommunity.id));
    }, 0);

    return () => {
      clearTimeout(timer);
      send({
        type: "community.leave_room",
        payload: { communityId: selectedCommunity.id },
      });
    };
  }, [selectedCommunity, isConnected, send]);

  // Auto-scroll 1-on-1 chat on message change if user is near bottom
  useEffect(() => {
    if (isAtBottomRef.current && chatContainerRef.current) {
      chatContainerRef.current.scrollTo({
        top: chatContainerRef.current.scrollHeight,
        behavior: "smooth",
      });
    }
  }, [chatMessages]);

  // Auto-scroll community chat on message change if user is near bottom
  useEffect(() => {
    if (isCommAtBottomRef.current && commChatContainerRef.current) {
      commChatContainerRef.current.scrollTo({
        top: commChatContainerRef.current.scrollHeight,
        behavior: "smooth",
      });
    }
  }, [commChatMessages]);

  // Instant scroll to bottom when selecting a different friend
  useEffect(() => {
    if (chatContainerRef.current) {
      chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
    }
    isAtBottomRef.current = true;
  }, [activeFriendId]);

  // Instant scroll to bottom when selecting a different community
  useEffect(() => {
    if (commChatContainerRef.current) {
      commChatContainerRef.current.scrollTop = commChatContainerRef.current.scrollHeight;
    }
    isCommAtBottomRef.current = true;
  }, [selectedCommunity?.id]);

  // Background heartbeat & window focus sync (ensures latest chats with zero manual refresh)
  useEffect(() => {
    if (!activeFriendId) return;

    const syncHistory = () => {
      const f = friends.find((item) => item.id === activeFriendId);
      const peerKey = f?.handle ? `@${f.handle}` : f?.email || "";
      if (!peerKey) return;

      getChatHistory(peerKey)
        .then((serverMsgs) => {
          if (serverMsgs && serverMsgs.length > 0) {
            const merged = mergeChatHistory(activeFriendId, serverMsgs, currentUser?.email, currentUser?.handle);
            setChatMessages((prev) => {
              if (prev.length !== merged.length || (prev.length > 0 && merged.length > 0 && prev[prev.length - 1]?.id !== merged[merged.length - 1]?.id)) {
                return merged;
              }
              return prev;
            });
          }
        })
        .catch(() => {});
    };

    // Immediate sync on mount / friend switch
    syncHistory();

    // 3-second heartbeat sync for zero-loss real-time assurance
    const interval = setInterval(syncHistory, 3000);

    // Sync on window focus / tab switch
    const onFocus = () => syncHistory();
    window.addEventListener("focus", onFocus);

    return () => {
      clearInterval(interval);
      window.removeEventListener("focus", onFocus);
    };
  }, [activeFriendId, friends, currentUser?.email, currentUser?.handle]);

  // ==========================================
  // ATTACHMENT MODAL HANDLERS (WhatsApp-style)
  // ==========================================
  const openAttachModal = (targetType) => {
    setAttachTargetType(targetType);
    setAttachFiles([]);
    setAttachCaption("");
    setAttachLink("");
    setShowAttachModal(true);
  };

  const handleSelectAttachFiles = (e) => {
    const files = Array.from(e.target.files || []);
    if (!files.length) return;
    files.forEach((file) => {
      const isImg = file.type.startsWith("image/");
      const reader = new FileReader();
      reader.onload = () => {
        setAttachFiles((prev) => [
          ...prev,
          {
            file,
            name: file.name,
            type: file.type,
            isImage: isImg,
            previewUrl: reader.result,
          },
        ]);
      };
      reader.readAsDataURL(file);
    });
    // Reset input so the same file can be re-selected
    e.target.value = "";
  };

  const removeAttachFile = (index) => {
    setAttachFiles((prev) => prev.filter((_, idx) => idx !== index));
  };

  // Send attachment (images/docs/links) as a chat message - NO goal selection required
  const handleSendAttachment = async (e) => {
    e.preventDefault();
    if (attachFiles.length === 0 && !attachCaption.trim() && !attachLink.trim()) return;

    setAttachSending(true);
    const imageUrls = attachFiles
      .filter((f) => f.isImage || f.type?.startsWith("image/"))
      .map((f) => f.previewUrl);
    const docFiles = attachFiles
      .filter((f) => !f.isImage && !f.type?.startsWith("image/"))
      .map((f) => ({ name: f.name, url: f.previewUrl, type: f.type }));

    const senderEmail = currentUser?.email || "";
    const senderHandle = currentUser?.handle || "";
    const senderName = currentUser?.name || (senderHandle ? `@${senderHandle}` : "You");
    const text = attachCaption.trim();
    const externalLink = attachLink.trim();
    const currentReplyTo = replyingTo ? { ...replyingTo } : null;

    try {
      if (attachTargetType === "friend" && activeFriendId && activeFriend) {
        send({
          type: "chat.message",
          payload: {
            recipientEmail: activeFriend.email || "",
            recipientHandle: activeFriend.handle || "",
            senderEmail,
            senderHandle,
            senderName,
            text,
            images: imageUrls,
            files: docFiles,
            externalLink,
            replyTo: currentReplyTo,
          },
        });

        const updated = sendChatMessage(
          activeFriendId,
          {
            text,
            images: imageUrls,
            files: docFiles,
            externalLink,
            replyTo: currentReplyTo,
          },
          "me"
        );
        setChatMessages(updated);
        setFriends(getStoredFriends());
        setReplyingTo(null);
        isAtBottomRef.current = true;
        setTimeout(() => {
          if (chatContainerRef.current) {
            chatContainerRef.current.scrollTo({
              top: chatContainerRef.current.scrollHeight,
              behavior: "smooth",
            });
          }
        }, 10);
      } else if (attachTargetType === "community" && selectedCommunity) {
        const msgId = generateMsgId("comm-msg");
        send({
          type: "community.message",
          payload: {
            id: msgId,
            communityId: selectedCommunity.id,
            communityName: selectedCommunity.name || "Community",
            communityCode: selectedCommunity.code || "",
            senderName,
            senderEmail,
            senderHandle,
            text,
            images: imageUrls,
            files: docFiles,
            externalLink,
            replyTo: currentReplyTo,
          },
        });

        const updated = sendCommunityMessage(selectedCommunity.id, {
          id: msgId,
          senderName: "You",
          senderHandle,
          text,
          images: imageUrls,
          files: docFiles,
          externalLink,
          replyTo: currentReplyTo,
        });
        setCommChatMessages(updated);
        setCommunities(getStoredCommunities());
        setReplyingTo(null);
        isCommAtBottomRef.current = true;
        setTimeout(() => {
          if (commChatContainerRef.current) {
            commChatContainerRef.current.scrollTo({
              top: commChatContainerRef.current.scrollHeight,
              behavior: "smooth",
            });
          }
        }, 10);
      }
    } catch (err) {
      console.error("Failed to send attachment:", err);
    } finally {
      setAttachSending(false);
      setShowAttachModal(false);
    }
  };

  // ==========================================
  // IN-CHAT APPROVE HANDLER
  // ==========================================
  const handleApproveChatProof = async (msg) => {
    setApprovingMsgId(msg.id);
    try {
      const { getProofs, getGoal, getGoals, decideApproval } = await import("../lib/api");
      let resolvedGoalId = msg.goalId;
      let ownerId = msg.ownerId || "";

      // If goalId is missing, attempt to resolve it by matching goalTitle against active goals
      if (!resolvedGoalId) {
        try {
          const allGoals = await getGoals().catch(() => []);
          const targetTitle = (msg.goalTitle || "").toLowerCase().trim();
          const matched = allGoals.find((g) => {
            return targetTitle && g.title && g.title.toLowerCase().trim() === targetTitle;
          }) || allGoals.find((g) => g.status === "active" || g.status === "proof_submitted");

          if (matched) {
            resolvedGoalId = matched.id;
            ownerId = matched.ownerId || ownerId;
          }
        } catch (findErr) {
          console.warn("Could not find matching goal by title:", findErr);
        }
      }

      if (resolvedGoalId) {
        const goal = await getGoal(resolvedGoalId, ownerId).catch(() => null);
        if (goal && !ownerId) {
          ownerId = goal.ownerId || "";
        }

        let proofs = await getProofs(resolvedGoalId, ownerId).catch(() => []);
        let targetProofId = proofs.find((p) => p.status === "pending" || p.status === "proof_submitted")?.id || proofs[0]?.id || "direct-approval";

        await decideApproval(resolvedGoalId, targetProofId, {
          ownerId: ownerId,
          status: "approved",
          comment: "Approved from chat",
        }).catch((decideErr) => {
          console.warn("decideApproval returned:", decideErr.message);
        });
      }

      // Mark locally as approved
      setChatMessages((prev) =>
        prev.map((m) => (m.id === msg.id || m.goalId === msg.goalId ? { ...m, approved: true } : m))
      );
      setCommChatMessages((prev) =>
        prev.map((m) => (m.id === msg.id || m.goalId === msg.goalId ? { ...m, approved: true } : m))
      );
      markChatMessageApproved(msg.goalId, msg.id);
      markCommunityMessageApproved(msg.goalId, msg.id);

      // Broadcast over WebSocket so owner instantly gets goal.unlocked / approved
      send({
        type: "chat.message.approved",
        payload: {
          messageId: msg.id,
          goalId: msg.goalId,
          goalTitle: msg.goalTitle,
          approverName: currentUser?.name || (currentUser?.handle ? `@${currentUser.handle}` : "Partner"),
        },
      });
    } catch (err) {
      console.error("Failed to approve in chat:", err);
    } finally {
      setApprovingMsgId(null);
    }
  };

  // Select a friend in 1-on-1 chat
  const selectFriend = (id) => {
    setActiveFriendId(id);
    const updated = clearUnreadCount(id);
    setFriends(updated);
    setChatMessages(getChatMessages(id));

    const f = updated.find((item) => item.id === id);
    const peerKey = f?.handle ? `@${f.handle}` : f?.email || "";
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

  const totalUnreadFriends = useMemo(() => {
    return friends.reduce((acc, f) => acc + (Number(f.unreadCount) || 0), 0);
  }, [friends]);

  // Filtered lists (Favorited items are sorted to the top)
  const filteredCommunities = useMemo(() => {
    const list = communities.filter((c) => {
      const q = searchQuery.toLowerCase();
      return c.name.toLowerCase().includes(q) || c.description.toLowerCase().includes(q);
    });
    return [...list].sort((a, b) => {
      if (a.isFavorite && !b.isFavorite) return -1;
      if (!a.isFavorite && b.isFavorite) return 1;
      return 0;
    });
  }, [communities, searchQuery]);

  const filteredFriends = useMemo(() => {
    const list = friends.filter((f) =>
      f.name.toLowerCase().includes(friendSearch.toLowerCase()) ||
      (f.handle && f.handle.toLowerCase().includes(friendSearch.toLowerCase()))
    );
    return [...list].sort((a, b) => {
      if (a.isFavorite && !b.isFavorite) return -1;
      if (!a.isFavorite && b.isFavorite) return 1;
      return 0;
    });
  }, [friends, friendSearch]);

  // Join / Leave / Delete Community actions
  const handleJoin = (id, e) => {
    e?.stopPropagation();
    const updated = joinCommunity(id);
    setCommunities(updated);
    const target = updated.find((c) => c.id === id);
    if (target) {
      setSelectedCommunity(target);
    }
  };

  const handleConfirmLeaveCommunity = () => {
    if (!commToLeave) return;
    const updated = leaveCommunity(commToLeave.id);
    setCommunities(updated);
    if (selectedCommunity?.id === commToLeave.id) {
      setSelectedCommunity(null);
    }
    setCommToLeave(null);
  };

  const handleConfirmDeleteCommunity = () => {
    if (!commToDelete) return;
    const updated = deleteCommunity(commToDelete.id);
    setCommunities(updated);
    if (selectedCommunity?.id === commToDelete.id) {
      setSelectedCommunity(null);
    }
    setCommToDelete(null);
  };

  const handleJoinByCode = (e) => {
    e.preventDefault();
    setCodeNotice({ error: "", success: "" });
    const codeClean = privateCode.trim().toUpperCase();
    if (!codeClean) return;

    // 1. Try local storage first
    try {
      const updated = joinByCode(codeClean);
      setCommunities(updated);
      const target = updated.find((c) => c.code?.toUpperCase() === codeClean);
      if (target) {
        setSelectedCommunity(target);
      }
      setPrivateCode("");
      setCodeNotice({ error: "", success: "Successfully joined community!" });
      setTimeout(() => setCodeNotice({ error: "", success: "" }), 3000);
      return;
    } catch {
      // Not in local storage, query realtime gateway
    }

    // 2. Query realtime WebSocket gateway
    if (isConnected) {
      send({
        type: "community.join_by_code",
        payload: { code: codeClean },
      });
      setCodeNotice({ error: "", success: "Searching for community..." });
    } else {
      setCodeNotice({ error: `No community found with invite code "${codeClean}".`, success: "" });
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

    const friendsToAdd = friends.filter((f) => selectedFriendsForCommunity.includes(f.id));

    const updated = createCommunity({
      name: newCommName,
      description: newCommDesc,
      category: newCommCategory,
      requiredApprovals: newCommApprovals,
      isPrivate: true,
      selectedFriends: friendsToAdd,
    });

    const createdCommunity = updated[0];

    // Realtime Sync with Hub and auto-join selected friends
    send({
      type: "community.register",
      payload: {
        community: createdCommunity,
        members: friendsToAdd.map((f) => ({
          handle: f.handle ? f.handle.replace(/[@\s]/g, "").toLowerCase() : "",
          email: f.email ? f.email.trim().toLowerCase() : "",
          name: f.name || f.handle || "Friend",
        })),
      },
    });

    send({
      type: "community.join_room",
      payload: { communityId: createdCommunity.id },
    });

    setCommunities(updated);
    setShowCreateModal(false);
    setNewCommName("");
    setNewCommDesc("");
    setSelectedFriendsForCommunity([]);
    setCreateError("");

    // Automatically open the new community group chat
    if (updated.length > 0) {
      setSelectedCommunity(updated[0]);
    }
  };

  // Add friend modal state
  const [showAddFriendModal, setShowAddFriendModal] = useState(false);
  const [newFriendHandle, setNewFriendHandle] = useState("");
  const [newFriendName, setNewFriendName] = useState("");
  const [newFriendEmail, setNewFriendEmail] = useState("");
  const [verifiedPartner, setVerifiedPartner] = useState(null);
  const [verifyingHandle, setVerifyingHandle] = useState(false);
  const [friendError, setFriendError] = useState("");

  const handleVerifyTag = async () => {
    const clean = newFriendHandle.replace(/[@\s]/g, "").toLowerCase();
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
    const cleanHandle = newFriendHandle.replace(/[@\s]/g, "").toLowerCase();
    if (!cleanHandle && !newFriendEmail.trim()) {
      setFriendError("Friend's ONUSLY handle is required (e.g. @himanshu)");
      return;
    }

    let friendData = {
      name: newFriendName.trim() || verifiedPartner?.name || (cleanHandle ? `@${cleanHandle}` : "Friend"),
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
        // Fallback
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

  // Delete friend confirmation
  const handleConfirmDeleteFriend = () => {
    if (!friendToDelete) return;
    const updated = deleteFriend(friendToDelete.id);
    setFriends(updated);
    if (activeFriendId === friendToDelete.id) {
      if (updated.length > 0) {
        selectFriend(updated[0].id);
      } else {
        setActiveFriendId(null);
        setChatMessages([]);
      }
    }
    setFriendToDelete(null);
  };

  // Send 1-on-1 Chat message
  const handleSendMessage = (e) => {
    e.preventDefault();
    if (!messageInput.trim() || !activeFriendId) return;

    const text = messageInput.trim();
    const currentFriend = friends.find((f) => f.id === activeFriendId);
    const u = getUser();
    const senderEmail = u?.email || "";
    const senderHandle = u?.handle ? u.handle.replace(/[@\s]/g, "").toLowerCase() : "";
    const senderName = u?.name || (senderHandle ? `@${senderHandle}` : "Friend");
    const recHandle = currentFriend?.handle ? currentFriend.handle.replace(/[@\s]/g, "").toLowerCase() : "";

    const msgId = generateMsgId("msg");
    const currentReplyTo = replyingTo ? { ...replyingTo } : null;

    send({
      type: "chat.message",
      payload: {
        id: msgId,
        recipientEmail: currentFriend?.email || "",
        recipientHandle: recHandle,
        senderEmail,
        senderHandle,
        senderName,
        text,
        replyTo: currentReplyTo,
      },
    });

    const updatedMessages = sendChatMessage(
      activeFriendId,
      { id: msgId, text, replyTo: currentReplyTo },
      "me"
    );
    setChatMessages(updatedMessages);
    setMessageInput("");
    setReplyingTo(null);
    setFriends(getStoredFriends());

    setTimeout(() => {
      if (chatContainerRef.current) {
        chatContainerRef.current.scrollTo({
          top: chatContainerRef.current.scrollHeight,
          behavior: "smooth",
        });
      }
    }, 10);
  };

  // Delete individual 1-on-1 message (WhatsApp style - deletes for both sides)
  const handleDeleteChatMessage = (msgId) => {
    if (!activeFriendId) return;
    const currentFriend = friends.find((f) => f.id === activeFriendId);

    const cleanH = currentFriend?.handle ? currentFriend.handle.replace(/[@\s]/g, "").toLowerCase() : "";
    const cleanE = currentFriend?.email ? currentFriend.email.trim().toLowerCase() : "";

    // Notify peer over WebSocket
    send({
      type: "chat.delete",
      payload: {
        id: msgId,
        recipientEmail: cleanE,
        recipientHandle: cleanH,
      },
    });

    deleteChatMessageGlobally(msgId);
    setChatMessages((prev) => prev.filter((m) => m.id !== msgId));
    setFriends(getStoredFriends());
  };

  // Send Community Group Chat message
  const handleSendCommMessage = (e) => {
    e.preventDefault();
    if (!commMessageInput.trim() || !selectedCommunity) return;

    const text = commMessageInput.trim();
    const senderEmail = currentUser?.email || "";
    const senderHandle = currentUser?.handle || "";
    const senderName = currentUser?.name || (senderHandle ? `@${senderHandle}` : "You");
    const msgId = `comm-msg-${Date.now()}-${Math.random().toString(36).substr(2, 6)}`;
    const currentReplyTo = replyingTo ? { ...replyingTo } : null;

    setCommMessageInput("");
    setReplyingTo(null);

    send({
      type: "community.message",
      payload: {
        id: msgId,
        communityId: selectedCommunity.id,
        communityName: selectedCommunity.name || "Community",
        communityCode: selectedCommunity.code || "",
        senderName,
        senderEmail,
        senderHandle,
        text,
        replyTo: currentReplyTo,
      },
    });

    const updated = sendCommunityMessage(selectedCommunity.id, {
      id: msgId,
      senderName: "You",
      senderHandle,
      text,
      replyTo: currentReplyTo,
    });
    setCommChatMessages(updated);
    setCommunities(getStoredCommunities());

    isCommAtBottomRef.current = true;
    setTimeout(() => {
      if (commChatContainerRef.current) {
        commChatContainerRef.current.scrollTo({
          top: commChatContainerRef.current.scrollHeight,
          behavior: "smooth",
        });
      }
    }, 10);
  };

  // Delete individual community message
  const handleDeleteCommMessage = (msgId) => {
    if (!selectedCommunity) return;

    send({
      type: "community.delete_message",
      payload: {
        communityId: selectedCommunity.id,
        messageId: msgId,
      },
    });

    deleteCommunityMessage(selectedCommunity.id, msgId);
    setCommChatMessages((prev) => prev.filter((m) => m.id !== msgId));
    setCommunities(getStoredCommunities());
  };

  // Open modal for message deletion (Delete for me vs Delete for everyone)
  const requestDeleteMessage = (msg, isMe, type = "friend") => {
    setDeleteMsgModal({
      isOpen: true,
      msg,
      isMe,
      type,
    });
  };

  const handleConfirmDeleteForEveryone = () => {
    const { msg, type } = deleteMsgModal;
    if (!msg) return;
    if (type === "friend") {
      handleDeleteChatMessage(msg.id);
    } else {
      handleDeleteCommMessage(msg.id);
    }
    setDeleteMsgModal({ isOpen: false, msg: null, isMe: false, type: "friend" });
  };

  const handleConfirmDeleteForMe = () => {
    const { msg, type } = deleteMsgModal;
    if (!msg) return;
    if (type === "friend" && activeFriendId) {
      const updated = deleteChatMessageForMe(activeFriendId, msg.id);
      setChatMessages(updated);
      setFriends(getStoredFriends());
    } else if (type === "community" && selectedCommunity) {
      const updated = deleteCommunityMessageForMe(selectedCommunity.id, msg.id);
      setCommChatMessages(updated);
      setCommunities(getStoredCommunities());
    }
    setDeleteMsgModal({ isOpen: false, msg: null, isMe: false, type: "friend" });
  };

  // Clear chat confirm handler
  const handleConfirmClearChat = () => {
    if (clearChatModal.type === "friend" && activeFriendId) {
      clearFriendChat(activeFriendId);
      setChatMessages([]);
      setFriends(getStoredFriends());
    } else if (clearChatModal.type === "community" && selectedCommunity) {
      clearCommunityChat(selectedCommunity.id);
      setCommChatMessages([]);
      setCommunities(getStoredCommunities());
    }
    setClearChatModal({ isOpen: false, type: "friend", targetName: "" });
  };

  // Star / Favorite toggle handler
  const handleToggleFavoriteActiveFriend = () => {
    if (!activeFriendId) return;
    const updated = toggleFavoriteFriend(activeFriendId);
    setFriends(updated);
    setShowFriendMenu(false);
  };

  const handleToggleFavoriteActiveCommunity = () => {
    if (!selectedCommunity) return;
    const updated = toggleFavoriteCommunity(selectedCommunity.id);
    setCommunities(updated);
    const refreshed = updated.find((c) => c.id === selectedCommunity.id);
    if (refreshed) setSelectedCommunity(refreshed);
    setShowCommMenu(false);
  };

  // ==========================================
  // CHAT MESSAGE BUBBLE COMPONENT
  // ==========================================
  const ChatBubble = ({
    msg,
    isMe,
    onDelete,
    onReply,
    onJumpToMessage,
    isHighlighted = false,
    onImageClick,
    showSenderBadge = false,
  }) => {
    const textStr = (msg.text || "").trim();
    const textMatch = textStr.match(/verify my goal:\s*["“]?([^"”\n\r]+)["”]?/i);
    const isVerification = Boolean(
      msg.isProof ||
      textMatch ||
      textStr.toLowerCase().includes("verify my goal") ||
      textStr.toLowerCase().includes("please verify")
    );
    const displayGoalTitle = msg.goalTitle || (textMatch ? textMatch[1].trim() : "");

    return (
      <div
        id={`msg-${msg.id}`}
        className={`group flex transition-all duration-300 scroll-mt-20 ${
          isMe ? "justify-end" : "justify-start"
        }`}
      >
        <div
          className={`relative max-w-[80%] sm:max-w-[70%] rounded-2xl px-4 py-3 text-xs transition-all duration-300 ${
            isHighlighted
              ? "ring-2 ring-emerald-400 shadow-[0_0_24px_rgba(16,185,129,0.6)] scale-[1.02] bg-emerald-500/15"
              : ""
          } ${
            isMe
              ? "bg-white text-black font-medium"
              : "border border-[#333333] bg-[#1E1E1E] text-white"
          }`}
        >
          {/* Sender badge for peers */}
          {showSenderBadge && !isMe && (
            <div className="mb-1 flex items-center gap-1.5 font-bold text-[11px] text-emerald-400">
              <span>{msg.senderName || "Member"}</span>
              {msg.senderHandle && (
                <span className="font-mono text-[10px] text-[#A3A3A3]">
                  @{msg.senderHandle.replace(/^@/, "")}
                </span>
              )}
            </div>
          )}

          {/* Quoted Reply context with WhatsApp-style jump to message */}
          {msg.replyTo && (
            <div
              onClick={(e) => {
                e.stopPropagation();
                if (msg.replyTo.id && onJumpToMessage) {
                  onJumpToMessage(msg.replyTo.id);
                }
              }}
              className={`mb-2 rounded-lg border-l-4 p-2 text-[11px] select-none cursor-pointer transition-all hover:opacity-90 active:scale-[0.98] ${
                isMe
                  ? "border-emerald-600 bg-black/10 hover:bg-black/15 text-neutral-800"
                  : "border-emerald-400 bg-white/10 hover:bg-white/15 text-neutral-300"
              }`}
              title="Click to jump to quoted message"
            >
              <div className="flex items-center justify-between font-bold text-[10px] text-emerald-500 mb-0.5">
                <div className="flex items-center gap-1">
                  <Reply size={10} />
                  <span>{msg.replyTo.senderName || (msg.replyTo.sender === "me" ? "You" : "Message")}</span>
                </div>
                <span className="text-[9px] font-normal opacity-70 underline ml-2">Jump ↑</span>
              </div>
              <p className="line-clamp-2 italic opacity-90 text-[11px]">{msg.replyTo.text}</p>
            </div>
          )}

          {/* Goal Verification Card */}
          {isVerification && (
            <div
              className={`mb-2 rounded-xl p-3 ${
                isMe
                  ? "border border-black/15 bg-black/5"
                  : "border border-white/15 bg-white/5"
              }`}
            >
              <div className="flex items-center justify-between gap-2 border-b border-current/15 pb-2 mb-2">
                <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-emerald-400">
                  <ShieldCheck size={14} />
                  <span>Goal Verification</span>
                </div>
                <div className="flex items-center gap-2">
                  {msg.approved ? (
                    <span className="flex items-center gap-1 text-[11px] font-bold text-emerald-400">
                      <CheckCircle2 size={13} />
                      <span>Approved</span>
                    </span>
                  ) : (
                    <span className="text-[10px] text-amber-400 font-semibold bg-amber-400/10 px-2 py-0.5 rounded-full border border-amber-400/20">
                      Pending
                    </span>
                  )}
                  {msg.goalId && (
                    <Link
                      to={`/goals/${msg.goalId}${msg.ownerId ? `?ownerId=${msg.ownerId}` : ""}`}
                      className={`flex items-center gap-1 text-[11px] font-bold underline hover:opacity-80 ${
                        isMe ? "text-black" : "text-white"
                      }`}
                    >
                      <span>Details</span>
                      <ExternalLink size={11} />
                    </Link>
                  )}
                </div>
              </div>
              {displayGoalTitle && (
                <h4
                  className={`text-xs font-bold mb-1 ${
                    isMe ? "text-black" : "text-white"
                  }`}
                >
                  🎯 {displayGoalTitle}
                </h4>
              )}
              {msg.text && (
                <p
                  className={`text-xs leading-relaxed mb-2 ${
                    isMe ? "text-neutral-800" : "text-neutral-200"
                  }`}
                >
                  {msg.text}
                </p>
              )}
              {msg.externalLink && (
                <a
                  href={msg.externalLink}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-[11px] text-blue-400 hover:underline mb-2"
                >
                  <ExternalLink size={12} />
                  <span className="truncate max-w-[200px]">{msg.externalLink}</span>
                </a>
              )}
              {msg.images && msg.images.length > 0 && (
                <WhatsAppImageGrid images={msg.images} onImageClick={onImageClick} />
              )}
              {msg.files && msg.files.length > 0 && (
                <div className="mt-2 space-y-1.5">
                  {msg.files.map((f, i) => (
                    <a
                      key={i}
                      href={f.url}
                      download={f.name || `file-${i}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={`flex items-center gap-2.5 rounded-xl p-2.5 text-xs transition max-w-sm ${
                        isMe
                          ? "border border-black/15 bg-black/5 hover:bg-black/10 text-neutral-900"
                          : "border border-white/10 bg-white/5 hover:bg-white/10 text-white"
                      }`}
                    >
                      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-red-500/20 text-red-400 border border-red-500/30 shrink-0">
                        <FileText size={16} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="font-semibold truncate text-[11px]">{f.name || "Attached File"}</p>
                        <span className="text-[10px] opacity-70">Click to view / download</span>
                      </div>
                    </a>
                  ))}
                </div>
              )}

              {/* If approved, show banner. If not approved and not me, show Approve button */}
              {msg.approved ? (
                <div className="mt-2.5 flex items-center gap-1.5 rounded-lg border border-emerald-500/40 bg-emerald-500/15 px-3 py-1.5 text-[11px] font-bold text-emerald-400">
                  <CheckCircle2 size={14} />
                  <span>Verified & Unlocked! Apps are released.</span>
                </div>
              ) : !isMe ? (
                <button
                  type="button"
                  disabled={approvingMsgId === msg.id}
                  onClick={() => handleApproveChatProof({ ...msg, goalTitle: displayGoalTitle })}
                  className="mt-2.5 flex items-center gap-1.5 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-black px-3.5 py-1.5 text-[11px] font-bold transition disabled:opacity-50 shadow-sm"
                >
                  {approvingMsgId === msg.id ? (
                    <Loader2 size={13} className="animate-spin" />
                  ) : (
                    <ThumbsUp size={13} />
                  )}
                  <span>Approve & Unlock Goal</span>
                </button>
              ) : (
                <div className="mt-2 flex items-center gap-1.5 text-[10px] text-neutral-400">
                  <span className="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse" />
                  <span>Awaiting partner review & verification</span>
                </div>
              )}
            </div>
          )}

          {/* Images attached (non-verification messages) */}
          {!isVerification && msg.images && msg.images.length > 0 && (
            <WhatsAppImageGrid images={msg.images} onImageClick={onImageClick} />
          )}

          {/* Files attached (non-verification messages) */}
          {!isVerification && msg.files && msg.files.length > 0 && (
            <div className="mt-2 space-y-1.5">
              {msg.files.map((f, i) => (
                <a
                  key={i}
                  href={f.url}
                  download={f.name || `file-${i}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={`flex items-center gap-2.5 rounded-xl p-2.5 text-xs transition max-w-sm ${
                    isMe
                      ? "border border-black/15 bg-black/5 hover:bg-black/10 text-neutral-900"
                      : "border border-white/10 bg-white/5 hover:bg-white/10 text-white"
                  }`}
                >
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-red-500/20 text-red-400 border border-red-500/30 shrink-0">
                    <FileText size={16} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-semibold truncate text-[11px]">{f.name || "Attached File"}</p>
                    <span className="text-[10px] opacity-70">Click to view / download</span>
                  </div>
                </a>
              ))}
            </div>
          )}

          {/* External link (non-verification) */}
          {!isVerification && msg.externalLink && (
            <a
              href={msg.externalLink}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-[11px] text-blue-400 hover:underline mb-1"
            >
              <ExternalLink size={12} />
              <span className="truncate max-w-[200px]">{msg.externalLink}</span>
            </a>
          )}

          {/* Normal text (non-verification) */}
          {!isVerification && msg.text && (
            <p className="leading-relaxed">{msg.text}</p>
          )}

          <div className="mt-1 flex items-center justify-end gap-2 text-[9px]">
            <span className={isMe ? "text-neutral-500" : "text-[#737373]"}>
              {msg.time}
            </span>
            {isMe && <CheckCheck size={12} className="text-neutral-500" />}

            {/* Reply action icon */}
            <button
              type="button"
              onClick={() =>
                onReply &&
                onReply({
                  id: msg.id,
                  text: (isVerification && displayGoalTitle ? `🎯 Goal: ${displayGoalTitle}` : "") || msg.text || (msg.images?.length ? "📸 Photo attachment" : "📎 Attachment"),
                  senderName: msg.senderName || (isMe ? "You" : (activeFriend?.name || "Member")),
                  sender: msg.sender,
                })
              }
              className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 text-neutral-400 hover:text-white"
              title="Reply to message"
            >
              <Reply size={11} />
            </button>

            {/* Delete message icon */}
            <button
              type="button"
              onClick={() => onDelete && onDelete(msg, isMe)}
              className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 text-neutral-400 hover:text-red-500"
              title="Delete message"
            >
              <Trash2 size={11} />
            </button>
          </div>
        </div>
      </div>
    );
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
              Chat, share files, and verify goals with your accountability partners.
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
              Communities ({communities.length})
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
              <span>Friends ({friends.length})</span>
              {totalUnreadFriends > 0 && (
                <span className="flex h-4 min-w-[16px] items-center justify-center rounded-full bg-emerald-500 px-1 text-[9px] font-black text-black shadow-[0_0_8px_rgba(16,185,129,0.5)] animate-pulse">
                  {totalUnreadFriends > 99 ? "99+" : totalUnreadFriends}
                </span>
              )}
            </button>
          </div>
        </div>

        {/* ============================================================
            VIEW 1: COMMUNITIES SECTION
        ============================================================ */}
        {mainView === "communities" && (
          <div>
            {selectedCommunity ? (
              /* Selected Community: Group Chat or Rules Preview */
              <div className="space-y-4">
                {/* Community Chat Header */}
                <div className="flex flex-col gap-3 rounded-2xl border border-[#2B2B2B] bg-[#181818] p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex items-center gap-3">
                    <button
                      type="button"
                      onClick={() => setSelectedCommunity(null)}
                      className="rounded-xl border border-[#333333] bg-[#141414] p-2 text-white hover:border-white transition"
                      title="Back to communities"
                    >
                      <ArrowLeft size={16} />
                    </button>
                    <div>
                      <div className="flex items-center gap-2">
                        <h2 className="text-lg font-bold text-white flex items-center gap-2">
                          <span>{selectedCommunity.name}</span>
                          {selectedCommunity.isFavorite && (
                            <Star size={15} className="fill-amber-400 text-amber-400 shrink-0" title="Favorited Community" />
                          )}
                        </h2>
                        <span className="rounded-full border border-[#444444] px-2 py-0.5 text-[10px] uppercase font-bold text-[#A3A3A3]">
                          {selectedCommunity.category}
                        </span>
                      </div>
                      <p className="text-xs text-[#A3A3A3] mt-0.5">
                        {selectedCommunity.membersCount} Members • {selectedCommunity.requiredApprovals} Approvals Required
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 flex-wrap">
                    <button
                      type="button"
                      onClick={() => handleCopyCode(selectedCommunity.code)}
                      className="flex items-center gap-1.5 rounded-xl border border-[#333333] bg-[#222222] px-3 py-1.5 text-xs font-semibold text-white hover:border-white transition"
                      title="Share Invite Code"
                    >
                      <Copy size={13} />
                      {copiedCode ? "Copied" : selectedCommunity.code}
                    </button>

                    {/* WhatsApp-style 3-Dot Menu */}
                    <div className="relative" ref={commMenuRef}>
                      <button
                        type="button"
                        onClick={() => setShowCommMenu((prev) => !prev)}
                        className="rounded-xl border border-[#333333] bg-[#222222] p-2 text-neutral-300 hover:text-white hover:border-white transition"
                        title="More options"
                      >
                        <MoreVertical size={16} />
                      </button>

                      {showCommMenu && (
                        <div className="absolute right-0 top-full mt-2 w-48 rounded-xl border border-[#333333] bg-[#1E1E1E] py-1 shadow-2xl z-40 backdrop-blur-md">
                          <button
                            type="button"
                            onClick={handleToggleFavoriteActiveCommunity}
                            className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-neutral-200 hover:bg-white/10 hover:text-white text-left transition"
                          >
                            <Star size={14} className={selectedCommunity.isFavorite ? "fill-amber-400 text-amber-400" : "text-neutral-400"} />
                            <span>{selectedCommunity.isFavorite ? "Remove from Starred" : "Add to Favourite"}</span>
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setShowCommMenu(false);
                              setClearChatModal({ isOpen: true, type: "community", targetName: selectedCommunity.name });
                            }}
                            className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-neutral-200 hover:bg-white/10 hover:text-white text-left transition"
                          >
                            <RotateCcw size={14} className="text-neutral-400" />
                            <span>Clear Chat</span>
                          </button>
                          {selectedCommunity.joined && (
                            <>
                              <div className="my-1 border-t border-[#333333]" />
                              <button
                                type="button"
                                onClick={() => {
                                  setShowCommMenu(false);
                                  setCommToLeave(selectedCommunity);
                                }}
                                className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-neutral-200 hover:bg-white/10 hover:text-white text-left transition"
                              >
                                <ArrowLeft size={14} className="text-neutral-400" />
                                <span>Leave Community</span>
                              </button>
                              <button
                                type="button"
                                onClick={() => {
                                  setShowCommMenu(false);
                                  setCommToDelete(selectedCommunity);
                                }}
                                className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-300 text-left transition"
                              >
                                <Trash2 size={14} className="text-red-400" />
                                <span>Delete Community</span>
                              </button>
                            </>
                          )}
                        </div>
                      )}
                    </div>

                    {!selectedCommunity.joined && (
                      <button
                        type="button"
                        onClick={(e) => handleJoin(selectedCommunity.id, e)}
                        className="rounded-xl bg-white px-4 py-1.5 text-xs font-bold text-black hover:bg-neutral-200 transition"
                      >
                        Join Community
                      </button>
                    )}
                  </div>
                </div>

                {/* If joined, show full Community Group Chat */}
                {selectedCommunity.joined ? (
                  <div className="flex flex-col rounded-2xl border border-[#2B2B2B] bg-[#121212] overflow-hidden" style={{ height: "calc(100vh - 320px)", minHeight: "480px" }}>
                    {/* Community Description Banner */}
                    <div className="shrink-0 border-b border-[#222222] bg-[#161616] px-5 py-2.5 flex items-center justify-between text-xs text-[#A3A3A3]">
                      <span className="truncate max-w-xl">
                        📌 {selectedCommunity.description}
                      </span>
                      <span className="font-mono text-[11px] text-[#737373]">
                        Code: {selectedCommunity.code}
                      </span>
                    </div>

                    {/* Community Chat Stream */}
                    <div
                      ref={commChatContainerRef}
                      onScroll={(e) => {
                        const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
                        isCommAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 100;
                      }}
                      className="flex-1 min-h-0 space-y-3 overflow-y-auto no-scrollbar p-4 overscroll-contain"
                    >
                      {commChatMessages.length === 0 ? (
                        <div className="flex h-full flex-col items-center justify-center py-16 text-center">
                          <Users size={36} className="text-[#444444] mb-2" />
                          <p className="text-xs font-semibold text-white">
                            Community Chat Active
                          </p>
                          <p className="mt-1 text-xs text-[#737373] max-w-sm">
                            Share messages, photos, or files with your community using the 📎 icon.
                          </p>
                        </div>
                      ) : (
                        commChatMessages.map((msg) => (
                          <ChatBubble
                            key={msg.id}
                            msg={msg}
                            isMe={msg.sender === "me"}
                            onDelete={(m, isMe) => requestDeleteMessage(m, isMe, "community")}
                            onReply={(replyContext) => setReplyingTo(replyContext)}
                            onJumpToMessage={handleJumpToMessage}
                            isHighlighted={highlightedMsgId === msg.id}
                            onImageClick={setLightboxImage}
                            showSenderBadge={true}
                          />
                        ))
                      )}
                      <div ref={commChatEndRef} />
                    </div>

                    {/* Quoted Reply banner */}
                    {replyingTo && (
                      <div className="shrink-0 flex items-center justify-between border-t border-l-4 border-l-emerald-500 border-t-[#2A2A2A] bg-[#1E1E1E] px-4 py-2 text-xs">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-1.5 text-[10px] font-bold text-emerald-400">
                            <Reply size={12} />
                            <span>Replying to {replyingTo.senderName || "Message"}</span>
                          </div>
                          <p className="truncate text-[11px] text-neutral-300 italic mt-0.5">
                            {replyingTo.text}
                          </p>
                        </div>
                        <button
                          type="button"
                          onClick={() => setReplyingTo(null)}
                          className="ml-3 rounded-full p-1 text-neutral-400 hover:bg-neutral-800 hover:text-white transition"
                          title="Cancel reply"
                        >
                          <X size={14} />
                        </button>
                      </div>
                    )}

                    {/* Community Chat Input - WhatsApp style */}
                    <form
                      onSubmit={handleSendCommMessage}
                      className="shrink-0 flex items-center gap-2 border-t border-[#2A2A2A] bg-[#181818] p-3"
                    >
                      <button
                        type="button"
                        onClick={() => openAttachModal("community")}
                        title="Attach photos, files or links"
                        className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-[#3A3A3A] bg-[#222222] text-[#A3A3A3] hover:border-white hover:text-white hover:bg-[#2C2C2C] transition"
                      >
                        <Paperclip size={18} />
                      </button>

                      <input
                        type="text"
                        placeholder="Type a message..."
                        value={commMessageInput}
                        onChange={(e) => setCommMessageInput(e.target.value)}
                        className="flex-1 rounded-xl border border-[#333333] bg-[#121212] px-4 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                      />

                      <button
                        type="submit"
                        disabled={!commMessageInput.trim()}
                        className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-white text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
                      >
                        <Send size={15} />
                      </button>
                    </form>
                  </div>
                ) : (
                  /* Not joined preview */
                  <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-8 text-center space-y-4">
                    <Users size={40} className="mx-auto text-[#737373]" />
                    <h3 className="text-lg font-bold text-white">Join {selectedCommunity.name}</h3>
                    <p className="text-xs text-[#A3A3A3] max-w-md mx-auto leading-relaxed">
                      {selectedCommunity.description}
                    </p>
                    <div className="pt-2">
                      <button
                        type="button"
                        onClick={(e) => handleJoin(selectedCommunity.id, e)}
                        className="rounded-xl bg-white px-6 py-2.5 text-xs font-bold text-black hover:bg-neutral-200 transition"
                      >
                        Join Community to Enter Chat
                      </button>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              /* Communities Explorer */
              <div className="space-y-6">
                <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex items-center gap-2 flex-wrap">
                    <div className="relative w-64">
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
                        className="rounded-xl border border-[#444444] bg-[#222222] px-3.5 py-2 text-xs font-semibold text-white hover:border-white transition"
                      >
                        Join
                      </button>
                    </form>
                  </div>

                  <button
                    type="button"
                    onClick={() => setShowCreateModal(true)}
                    className="flex items-center justify-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black hover:bg-neutral-200 transition"
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
                      Click "+ Create Community" above to build your own community and add your friends.
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
                          <div className="flex items-center gap-2">
                            <h3 className="text-base font-bold text-white flex items-center gap-1.5">
                              <span>{c.name}</span>
                              {c.isFavorite && (
                                <Star size={13} className="fill-amber-400 text-amber-400 shrink-0" title="Starred Community" />
                              )}
                            </h3>
                            {c.joined && (
                              <span className="rounded-md border border-emerald-900 bg-emerald-950/40 px-2 py-0.5 text-[10px] font-semibold text-emerald-400">
                                Joined
                              </span>
                            )}
                          </div>
                          <span className="rounded-full border border-[#444444] px-2.5 py-0.5 text-[10px] uppercase font-semibold text-[#A3A3A3]">
                            {c.category}
                          </span>
                        </div>
                        <p className="mt-3 text-xs leading-relaxed text-[#A3A3A3] line-clamp-2">
                          {c.description}
                        </p>
                        <div className="mt-5 flex items-center justify-between border-t border-[#2B2B2B] pt-4 text-xs">
                          <span className="text-[#737373]">{c.membersCount} Members</span>
                          <span className="font-semibold text-white">
                            {c.joined ? "Open Chat →" : "View Community →"}
                          </span>
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
          <div className="grid grid-cols-1 overflow-hidden rounded-2xl border border-[#2B2B2B] bg-[#181818] shadow-2xl md:grid-cols-12" style={{ height: "calc(100vh - 220px)", minHeight: "520px" }}>
            {/* Left Column: Friends List */}
            <div className="flex flex-col h-full min-h-0 border-b border-[#2A2A2A] bg-[#141414] p-4 md:col-span-4 md:border-b-0 md:border-r overflow-hidden">
              <div className="shrink-0 flex items-center justify-between pb-3">
                <span className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
                  Friends ({friends.length})
                </span>
                <button
                  type="button"
                  onClick={() => setShowAddFriendModal(true)}
                  className="flex items-center gap-1.5 rounded-lg border border-[#444444] bg-[#222222] px-2.5 py-1.5 text-xs font-semibold text-white hover:border-white transition"
                >
                  <UserPlus size={14} />
                  Add
                </button>
              </div>

              {/* Search Friends */}
              <div className="shrink-0 relative mt-2">
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
              <div className="flex-1 min-h-0 mt-4 space-y-1 overflow-y-auto no-scrollbar">
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
                    const hasUnread = Number(f.unreadCount) > 0 && !isSelected;
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
                              <img
                                src={f.picture}
                                alt={f.name}
                                className="h-full w-full object-cover"
                                referrerPolicy="no-referrer"
                              />
                            ) : (
                              f.name[0]?.toUpperCase() || "F"
                            )}
                            <span
                              className={`absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-[#1E1E1E] ${
                                (f.handle &&
                                  onlineHandles.has(
                                    f.handle.toLowerCase().replace(/^@/, "")
                                  )) ||
                                (f.email && onlineEmails.has(f.email.trim().toLowerCase()))
                                  ? "bg-emerald-400"
                                  : "bg-neutral-600"
                              }`}
                              title={
                                (f.handle &&
                                  onlineHandles.has(
                                    f.handle.toLowerCase().replace(/^@/, "")
                                  )) ||
                                (f.email && onlineEmails.has(f.email.trim().toLowerCase()))
                                  ? "Online"
                                  : "Offline"
                              }
                            />
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-1.5">
                              <p className={`truncate text-xs ${hasUnread ? "font-bold text-white" : "font-semibold text-white"}`}>
                                {f.name}
                              </p>
                              {f.isFavorite && (
                                <Star size={11} className="fill-amber-400 text-amber-400 shrink-0" title="Starred Friend" />
                              )}
                              {f.handle && (
                                <span className="font-mono text-[10px] text-[#A3A3A3]">
                                  @{f.handle.replace(/^@/, "")}
                                </span>
                              )}
                            </div>
                            <p className={`truncate text-[11px] ${hasUnread ? "font-medium text-neutral-200" : "text-[#737373]"}`}>
                              {f.lastMessage || "Click to start chatting"}
                            </p>
                          </div>
                        </div>

                        <div className="flex flex-col items-end gap-1 flex-shrink-0 ml-2">
                          {f.lastMessageTime && (
                            <span className={`text-[10px] ${hasUnread ? "font-bold text-emerald-400" : "text-[#737373]"}`}>
                              {f.lastMessageTime}
                            </span>
                          )}
                          {hasUnread && (
                            <span className="flex h-5 min-w-[20px] items-center justify-center rounded-full bg-emerald-500 px-1.5 text-[10px] font-black text-black shadow-[0_0_10px_rgba(16,185,129,0.5)] animate-in zoom-in-75 duration-200">
                              {f.unreadCount > 99 ? "99+" : f.unreadCount}
                            </span>
                          )}
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </div>

            {/* Right Column: 1-on-1 Chat Box */}
            <div className="flex flex-col h-full min-h-0 overflow-hidden bg-[#121212] md:col-span-8">
              {activeFriend ? (
                <>
                  {/* Chat Header */}
                  <div className="shrink-0 flex items-center justify-between border-b border-[#2A2A2A] bg-[#181818] p-4">
                    <div className="flex items-center gap-3">
                      <button
                        type="button"
                        onClick={() => {
                          setActiveFriendId(null);
                          setChatMessages([]);
                        }}
                        className="rounded-xl border border-[#333333] bg-[#141414] p-2 text-neutral-400 hover:text-white hover:border-white transition"
                        title="Back to Share Proof / All Chats"
                      >
                        <ArrowLeft size={16} />
                      </button>
                      <div className="relative flex h-10 w-10 items-center justify-center rounded-full border border-[#444444] bg-[#282828] text-sm font-bold text-white overflow-hidden">
                        {activeFriend.picture ? (
                          <img
                            src={activeFriend.picture}
                            alt={activeFriend.name}
                            className="h-full w-full object-cover"
                            referrerPolicy="no-referrer"
                          />
                        ) : (
                          activeFriend.name[0]?.toUpperCase()
                        )}
                        <span
                          className={`absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-[#181818] ${
                            (activeFriend.handle &&
                              onlineHandles.has(
                                activeFriend.handle.toLowerCase().replace(/^@/, "")
                              )) ||
                            (activeFriend.email &&
                              onlineEmails.has(activeFriend.email.trim().toLowerCase()))
                              ? "bg-emerald-400"
                              : "bg-neutral-600"
                          }`}
                        />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <h3 className="text-sm font-bold text-white flex items-center gap-1.5">
                            <span>{activeFriend.name}</span>
                            {activeFriend.isFavorite && (
                              <Star size={13} className="fill-amber-400 text-amber-400 shrink-0" title="Starred Friend" />
                            )}
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
                            const isOnline = Boolean(
                              (activeFriend.handle &&
                                onlineHandles.has(
                                  activeFriend.handle.toLowerCase().replace(/^@/, "")
                                )) ||
                              (activeFriend.email &&
                                onlineEmails.has(activeFriend.email.trim().toLowerCase()))
                            );
                            return (
                              <span
                                className={`flex items-center gap-1 font-medium ${
                                  isOnline ? "text-emerald-400" : "text-neutral-400"
                                }`}
                              >
                                <span
                                  className={`h-1.5 w-1.5 rounded-full ${
                                    isOnline ? "bg-emerald-400 animate-pulse" : "bg-neutral-500"
                                  }`}
                                />
                                {isOnline ? "Online" : "Offline"}
                              </span>
                            );
                          })()}
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      {/* WhatsApp-style 3-Dot Dropdown Menu */}
                      <div className="relative" ref={friendMenuRef}>
                        <button
                          type="button"
                          onClick={() => setShowFriendMenu((prev) => !prev)}
                          className="rounded-xl border border-[#333333] bg-[#222222] p-2 text-neutral-300 hover:text-white hover:border-white transition"
                          title="More options"
                        >
                          <MoreVertical size={16} />
                        </button>

                        {showFriendMenu && (
                          <div className="absolute right-0 top-full mt-2 w-48 rounded-xl border border-[#333333] bg-[#1E1E1E] py-1 shadow-2xl z-40 backdrop-blur-md">
                            <button
                              type="button"
                              onClick={handleToggleFavoriteActiveFriend}
                              className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-neutral-200 hover:bg-white/10 hover:text-white text-left transition"
                            >
                              <Star size={14} className={activeFriend.isFavorite ? "fill-amber-400 text-amber-400" : "text-neutral-400"} />
                              <span>{activeFriend.isFavorite ? "Remove from Starred" : "Add to Favourite"}</span>
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                setShowFriendMenu(false);
                                setClearChatModal({ isOpen: true, type: "friend", targetName: activeFriend.name });
                              }}
                              className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-neutral-200 hover:bg-white/10 hover:text-white text-left transition"
                            >
                              <RotateCcw size={14} className="text-neutral-400" />
                              <span>Clear Chat</span>
                            </button>
                            <div className="my-1 border-t border-[#333333]" />
                            <button
                              type="button"
                              onClick={() => {
                                setShowFriendMenu(false);
                                setFriendToDelete(activeFriend);
                              }}
                              className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-red-400 hover:bg-red-500/10 hover:text-red-300 text-left transition"
                            >
                              <Trash2 size={14} className="text-red-400" />
                              <span>Delete Contact</span>
                            </button>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>

                  {/* Message Stream */}
                  <div
                    ref={chatContainerRef}
                    onScroll={(e) => {
                      const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
                      isAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 100;
                    }}
                    className="flex-1 min-h-0 space-y-3 overflow-y-auto no-scrollbar p-4 overscroll-contain"
                  >
                    {chatMessages.length === 0 ? (
                      <div className="flex h-full flex-col items-center justify-center py-16 text-center">
                        <MessageSquare size={32} className="text-[#444444] mb-2" />
                        <p className="text-xs font-medium text-[#A3A3A3]">
                          No messages yet. Send a message or click 📎 to share files!
                        </p>
                      </div>
                    ) : (
                      chatMessages.map((msg) => (
                        <ChatBubble
                          key={msg.id}
                          msg={msg}
                          isMe={msg.sender === "me"}
                          onDelete={(m, isMe) => requestDeleteMessage(m, isMe, "friend")}
                          onReply={(replyContext) => setReplyingTo(replyContext)}
                          onJumpToMessage={handleJumpToMessage}
                          isHighlighted={highlightedMsgId === msg.id}
                          onImageClick={setLightboxImage}
                        />
                      ))
                    )}
                    <div ref={chatEndRef} />
                  </div>

                  {/* Quoted Reply banner */}
                  {replyingTo && (
                    <div className="shrink-0 flex items-center justify-between border-t border-l-4 border-l-emerald-500 border-t-[#2A2A2A] bg-[#1E1E1E] px-4 py-2 text-xs">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 text-[10px] font-bold text-emerald-400">
                          <Reply size={12} />
                          <span>Replying to {replyingTo.senderName || "Message"}</span>
                        </div>
                        <p className="truncate text-[11px] text-neutral-300 italic mt-0.5">
                          {replyingTo.text}
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() => setReplyingTo(null)}
                        className="ml-3 rounded-full p-1 text-neutral-400 hover:bg-neutral-800 hover:text-white transition"
                        title="Cancel reply"
                      >
                        <X size={14} />
                      </button>
                    </div>
                  )}

                  {/* Chat Input - WhatsApp style with paperclip */}
                  <form
                    onSubmit={handleSendMessage}
                    className="shrink-0 flex items-center gap-2 border-t border-[#2A2A2A] bg-[#181818] p-3"
                  >
                    <button
                      type="button"
                      onClick={() => openAttachModal("friend")}
                      title="Attach photos, files or links"
                      className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-[#3A3A3A] bg-[#222222] text-[#A3A3A3] hover:border-white hover:text-white hover:bg-[#2C2C2C] transition"
                    >
                      <Paperclip size={18} />
                    </button>

                    <input
                      type="text"
                      placeholder="Type a message..."
                      value={messageInput}
                      onChange={(e) => setMessageInput(e.target.value)}
                      className="flex-1 rounded-xl border border-[#333333] bg-[#121212] px-4 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                    />

                    <button
                      type="submit"
                      disabled={!messageInput.trim()}
                      className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-white text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
                    >
                      <Send size={15} />
                    </button>
                  </form>
                </>
              ) : (
                <div className="relative flex h-full flex-col items-center justify-center p-8 text-center select-none overflow-hidden bg-[#121212]">
                  {/* Subtle radial ambient background glow */}
                  <div className="pointer-events-none absolute h-72 w-72 rounded-full bg-white/[0.03] blur-3xl" />

                  {/* Icon Emblem */}
                  <div className="relative flex h-20 w-20 items-center justify-center rounded-3xl border border-[#2B2B2B] bg-gradient-to-b from-[#202020] to-[#141414] shadow-2xl">
                    <ShieldCheck size={38} className="text-white drop-shadow-[0_0_12px_rgba(255,255,255,0.2)]" />
                    <span className="absolute -bottom-1 -right-1 flex h-6 w-6 items-center justify-center rounded-full border border-[#2B2B2B] bg-[#161616] text-[#A3A3A3]">
                      <Paperclip size={12} />
                    </span>
                  </div>

                  {/* Heading & Tagline */}
                  <h2 className="mt-6 text-xl sm:text-2xl font-bold tracking-tight text-white">
                    Share proof with friends
                  </h2>
                  <p className="mt-2.5 max-w-sm text-xs sm:text-sm leading-relaxed text-[#A3A3A3]">
                    Select an accountability partner from the left to exchange proof, verify completed goals, and stay locked in together.
                  </p>

                  {/* Quick Action / Guide Pill */}
                  {friends.length === 0 ? (
                    <button
                      type="button"
                      onClick={() => setShowAddFriendModal(true)}
                      className="mt-6 flex items-center gap-2 rounded-xl border border-white bg-white px-4 py-2.5 text-xs font-bold text-black hover:bg-neutral-200 transition shadow-lg"
                    >
                      <UserPlus size={14} />
                      Add your first friend
                    </button>
                  ) : (
                    <div className="mt-6 inline-flex items-center gap-2 rounded-full border border-[#2B2B2B] bg-[#181818] px-4 py-1.5 text-xs text-[#8A8A8A]">
                      <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                      <span>Select a friend from the left sidebar to start</span>
                    </div>
                  )}

                  {/* WhatsApp-style Security & Verification footer */}
                  <div className="mt-12 flex items-center gap-2 text-[11px] text-[#555555]">
                    <ShieldCheck size={13} className="text-emerald-500/80" />
                    <span>End-to-end peer verification • Direct live sync</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* ============================================================
            WHATSAPP-STYLE ATTACHMENT MODAL (No goal required)
        ============================================================ */}
        {showAttachModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-md rounded-2xl border border-[#333333] bg-[#1A1A1A] p-5 shadow-2xl max-h-[85vh] overflow-y-auto">
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-3">
                <div className="flex items-center gap-2">
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-white text-black">
                    <Paperclip size={16} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white">Send Attachment</h3>
                    <p className="text-[11px] text-[#A3A3A3]">
                      {attachTargetType === "friend"
                        ? `To ${activeFriend?.name || "Friend"}`
                        : `To ${selectedCommunity?.name || "Community"}`}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setShowAttachModal(false)}
                  className="rounded-lg p-1 text-[#737373] hover:text-white transition"
                >
                  <X size={18} />
                </button>
              </div>

              <form onSubmit={handleSendAttachment} className="mt-4 space-y-4">
                {/* Photo / File Picker */}
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label className="text-xs font-bold text-white flex items-center gap-1.5">
                      <ImageIcon size={14} className="text-[#A3A3A3]" />
                      Photos & Files
                    </label>
                    <span className="text-[11px] text-[#737373]">
                      {attachFiles.length} selected
                    </span>
                  </div>

                  <label className="flex cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed border-[#3A3A3A] bg-[#141414] p-4 text-center hover:border-white transition">
                    <Paperclip size={20} className="text-[#A3A3A3] mb-1" />
                    <span className="text-xs font-semibold text-white">
                      Tap to choose photos or files
                    </span>
                    <span className="text-[10px] text-[#737373] mt-0.5">
                      PNG, JPG, PDF, screenshots
                    </span>
                    <input
                      type="file"
                      multiple
                      accept="image/*,.pdf,.doc,.docx,.txt"
                      onChange={handleSelectAttachFiles}
                      className="hidden"
                    />
                  </label>

                  {/* Thumbnail Strip */}
                  {attachFiles.length > 0 && (
                    <div className="mt-3 grid grid-cols-4 gap-2">
                      {attachFiles.map((item, idx) => (
                        <div
                          key={idx}
                          className="relative aspect-square overflow-hidden rounded-lg border border-[#333333] bg-[#111111] flex items-center justify-center p-1"
                        >
                          {item.isImage ? (
                            <img
                              src={item.previewUrl}
                              alt="Preview"
                              className="h-full w-full object-cover rounded"
                            />
                          ) : (
                            <div className="flex flex-col items-center justify-center text-center p-1">
                              <FileText size={20} className="text-red-400 mb-1" />
                              <span className="text-[9px] text-neutral-300 truncate max-w-full">
                                {item.name}
                              </span>
                            </div>
                          )}
                          <button
                            type="button"
                            onClick={() => removeAttachFile(idx)}
                            className="absolute top-1 right-1 rounded-full bg-black/80 p-1 text-white hover:bg-black"
                          >
                            <X size={10} />
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Link */}
                <div>
                  <label className="text-xs font-bold text-white mb-1.5 flex items-center gap-1.5">
                    <LinkIcon size={14} className="text-[#A3A3A3]" />
                    Link (Optional)
                  </label>
                  <input
                    type="url"
                    placeholder="https://..."
                    value={attachLink}
                    onChange={(e) => setAttachLink(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                {/* Caption */}
                <div>
                  <label className="text-xs font-bold text-white mb-1.5 block">
                    Caption (Optional)
                  </label>
                  <textarea
                    rows={2}
                    placeholder="Add a caption..."
                    value={attachCaption}
                    onChange={(e) => setAttachCaption(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none resize-none"
                  />
                </div>

                {/* Send */}
                <button
                  type="submit"
                  disabled={attachSending || (attachFiles.length === 0 && !attachCaption.trim() && !attachLink.trim())}
                  className="flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-xs font-bold text-black hover:bg-neutral-200 disabled:opacity-40 transition"
                >
                  {attachSending ? (
                    <>
                      <Loader2 size={14} className="animate-spin" />
                      Sending...
                    </>
                  ) : (
                    <>
                      <Send size={14} />
                      Send
                    </>
                  )}
                </button>
              </form>
            </div>
          </div>
        )}

        {/* ============================================================
            CREATE COMMUNITY MODAL (With Friend Selection)
        ============================================================ */}
        {showCreateModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-lg rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl max-h-[90vh] overflow-y-auto">
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <h3 className="text-base font-bold text-white">
                  Create Community
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
                <div className="mt-4 rounded-xl border border-red-900 bg-red-950/40 p-3 text-xs text-red-300">
                  {createError}
                </div>
              )}

              <form onSubmit={handleCreateCommunitySubmit} className="mt-4 space-y-4">
                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    Community Name
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. 5AM Founders Club"
                    value={newCommName}
                    onChange={(e) => setNewCommName(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                    required
                  />
                </div>

                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    Category
                  </label>
                  <select
                    value={newCommCategory}
                    onChange={(e) => setNewCommCategory(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white focus:border-white focus:outline-none"
                  >
                    <option value="General Focus">General Focus</option>
                    <option value="Coding & Tech">Coding & Tech</option>
                    <option value="Health & Fitness">Health & Fitness</option>
                    <option value="Academic Study">Academic Study</option>
                    <option value="Daily Routine">Daily Routine</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    Community Description & Guidelines
                  </label>
                  <textarea
                    rows={3}
                    placeholder="Explain the mission and evidence requirements..."
                    value={newCommDesc}
                    onChange={(e) => setNewCommDesc(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                    required
                  />
                </div>

                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    Required Member Approvals for Release
                  </label>
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={newCommApprovals}
                    onChange={(e) => setNewCommApprovals(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white focus:border-white focus:outline-none"
                  />
                </div>

                {/* Add Friends to Community Checkbox Section */}
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label className="text-xs font-bold text-white">
                      Add Friends to Community
                    </label>
                    <span className="text-[11px] text-[#737373]">
                      {selectedFriendsForCommunity.length} selected
                    </span>
                  </div>

                  {friends.length === 0 ? (
                    <div className="rounded-xl border border-[#333333] bg-[#141414] p-3 text-xs text-[#737373]">
                      No friends added yet. You can share your invite code after creation.
                    </div>
                  ) : (
                    <div className="max-h-40 overflow-y-auto rounded-xl border border-[#333333] bg-[#141414] p-2 space-y-1">
                      {friends.map((f) => {
                        const isChecked = selectedFriendsForCommunity.includes(f.id);
                        return (
                          <label
                            key={f.id}
                            className={`flex cursor-pointer items-center justify-between rounded-lg p-2 transition ${
                              isChecked ? "bg-[#252525]" : "hover:bg-[#1E1E1E]"
                            }`}
                          >
                            <div className="flex items-center gap-2.5">
                              <input
                                type="checkbox"
                                checked={isChecked}
                                onChange={(e) => {
                                  if (e.target.checked) {
                                    setSelectedFriendsForCommunity((prev) => [...prev, f.id]);
                                  } else {
                                    setSelectedFriendsForCommunity((prev) =>
                                      prev.filter((id) => id !== f.id)
                                    );
                                  }
                                }}
                                className="rounded border-[#444444] text-white focus:ring-0"
                              />
                              <div>
                                <p className="text-xs font-semibold text-white">{f.name}</p>
                                {f.handle && (
                                  <p className="font-mono text-[10px] text-[#737373]">
                                    @{f.handle.replace(/^@/, "")}
                                  </p>
                                )}
                              </div>
                            </div>
                          </label>
                        );
                      })}
                    </div>
                  )}
                </div>

                <div className="pt-2">
                  <button
                    type="submit"
                    className="w-full rounded-xl bg-white py-3 text-xs font-bold text-black hover:bg-neutral-200 transition"
                  >
                    Create Community & Open Chat
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}

        {/* ============================================================
            ADD FRIEND MODAL
        ============================================================ */}
        {showAddFriendModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-md rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl">
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <h3 className="text-base font-bold text-white">
                  Add Accountability Partner
                </h3>
                <button
                  type="button"
                  onClick={() => setShowAddFriendModal(false)}
                  className="rounded-lg p-1 text-[#737373] hover:text-white"
                >
                  <X size={18} />
                </button>
              </div>

              {friendError && (
                <div className="mt-4 rounded-xl border border-red-900 bg-red-950/40 p-3 text-xs text-red-300">
                  {friendError}
                </div>
              )}

              <form onSubmit={handleAddFriendSubmit} className="mt-4 space-y-4">
                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    Friend's ONUSLY Handle (Tag)
                  </label>
                  <div className="flex gap-2">
                    <div className="relative flex-1">
                      <AtSign
                        size={14}
                        className="absolute left-3 top-1/2 -translate-y-1/2 text-[#737373]"
                      />
                      <input
                        type="text"
                        placeholder="e.g. himanshu or @robert"
                        value={newFriendHandle}
                        onChange={(e) => {
                          setNewFriendHandle(e.target.value);
                          setVerifiedPartner(null);
                        }}
                        className="w-full rounded-xl border border-[#333333] bg-[#141414] py-2 pl-8 pr-3 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                        required
                      />
                    </div>
                    <button
                      type="button"
                      onClick={handleVerifyTag}
                      disabled={verifyingHandle || !newFriendHandle.trim()}
                      className="rounded-xl border border-[#444444] bg-[#222222] px-3 py-2 text-xs font-semibold text-white hover:border-white disabled:opacity-40 transition"
                    >
                      {verifyingHandle ? (
                        <Loader2 size={13} className="animate-spin" />
                      ) : (
                        "Verify"
                      )}
                    </button>
                  </div>
                </div>

                {verifiedPartner && (
                  <div className="flex items-center gap-3 rounded-xl border border-white/20 bg-white/5 p-3">
                    <div className="h-9 w-9 rounded-full overflow-hidden bg-neutral-800 border border-white/20 flex items-center justify-center text-xs font-bold text-white">
                      {verifiedPartner.picture ? (
                        <img
                          src={verifiedPartner.picture}
                          alt="avatar"
                          className="h-full w-full object-cover"
                          referrerPolicy="no-referrer"
                        />
                      ) : (
                        verifiedPartner.name?.[0]?.toUpperCase() || "U"
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-bold text-white truncate">
                        {verifiedPartner.name}
                      </p>
                      <p className="font-mono text-[11px] text-neutral-400">
                        @{verifiedPartner.handle}
                      </p>
                    </div>
                    <Check size={16} className="text-emerald-400" />
                  </div>
                )}

                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    Display Name
                  </label>
                  <input
                    type="text"
                    placeholder="Nickname or Real Name"
                    value={newFriendName}
                    onChange={(e) => setNewFriendName(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                <div className="pt-2">
                  <button
                    type="submit"
                    className="w-full rounded-xl bg-white py-3 text-xs font-bold text-black hover:bg-neutral-200 transition"
                  >
                    Add Partner
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}

        {/* ============================================================
            CONFIRM DELETE FRIEND MODAL
        ============================================================ */}
        {friendToDelete && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-sm rounded-2xl border border-red-900/50 bg-[#1A1A1A] p-6 shadow-2xl">
              <h3 className="text-sm font-bold text-white">Delete Friend?</h3>
              <p className="mt-2 text-xs text-[#A3A3A3] leading-relaxed">
                Are you sure you want to remove <strong className="text-white">{friendToDelete.name}</strong> from your friends? All local chat history with this partner will be cleared.
              </p>
              <div className="mt-5 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setFriendToDelete(null)}
                  className="rounded-xl border border-[#444444] px-4 py-2 text-xs font-semibold text-white hover:border-white transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmDeleteFriend}
                  className="rounded-xl bg-red-600 px-4 py-2 text-xs font-bold text-white hover:bg-red-500 transition"
                >
                  Confirm Delete
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ============================================================
            CONFIRM LEAVE COMMUNITY MODAL
        ============================================================ */}
        {commToLeave && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-sm rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl">
              <h3 className="text-sm font-bold text-white">Leave Community?</h3>
              <p className="mt-2 text-xs text-[#A3A3A3] leading-relaxed">
                Are you sure you want to leave <strong className="text-white">{commToLeave.name}</strong>? You will no longer receive community notifications.
              </p>
              <div className="mt-5 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setCommToLeave(null)}
                  className="rounded-xl border border-[#444444] px-4 py-2 text-xs font-semibold text-white hover:border-white transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmLeaveCommunity}
                  className="rounded-xl bg-white px-4 py-2 text-xs font-bold text-black hover:bg-neutral-200 transition"
                >
                  Leave Community
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ============================================================
            CONFIRM DELETE COMMUNITY MODAL
        ============================================================ */}
        {commToDelete && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-sm rounded-2xl border border-red-900/50 bg-[#1A1A1A] p-6 shadow-2xl">
              <h3 className="text-sm font-bold text-white">Delete Community?</h3>
              <p className="mt-2 text-xs text-[#A3A3A3] leading-relaxed">
                Permanently delete <strong className="text-white">{commToDelete.name}</strong> and all its group chat history? This action cannot be undone.
              </p>
              <div className="mt-5 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setCommToDelete(null)}
                  className="rounded-xl border border-[#444444] px-4 py-2 text-xs font-semibold text-white hover:border-white transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmDeleteCommunity}
                  className="rounded-xl bg-red-600 px-4 py-2 text-xs font-bold text-white hover:bg-red-500 transition"
                >
                  Delete Community
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ============================================================
            WHATSAPP-STYLE DELETE MESSAGE MODAL (Delete for me vs Delete for everyone)
        ============================================================ */}
        {deleteMsgModal.isOpen && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-sm rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl space-y-4">
              <div className="flex items-center gap-2.5">
                <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-red-500/15 text-red-400 border border-red-500/30">
                  <Trash2 size={16} />
                </div>
                <div>
                  <h3 className="text-sm font-bold text-white">Delete Message?</h3>
                  <p className="text-[11px] text-[#A3A3A3]">
                    {deleteMsgModal.isMe ? "Choose how you want to delete this message" : "Remove this message from your view"}
                  </p>
                </div>
              </div>

              {deleteMsgModal.msg?.text && (
                <div className="rounded-xl border border-white/10 bg-white/5 p-2.5 text-xs text-neutral-300 italic line-clamp-2">
                  "{deleteMsgModal.msg.text}"
                </div>
              )}

              <div className="pt-2 flex flex-col gap-2">
                {deleteMsgModal.isMe && (
                  <button
                    type="button"
                    onClick={handleConfirmDeleteForEveryone}
                    className="w-full rounded-xl bg-red-600 px-4 py-2.5 text-xs font-bold text-white hover:bg-red-500 transition shadow-sm"
                  >
                    Delete for everyone
                  </button>
                )}
                <button
                  type="button"
                  onClick={handleConfirmDeleteForMe}
                  className={`w-full rounded-xl px-4 py-2.5 text-xs font-bold transition ${
                    deleteMsgModal.isMe
                      ? "border border-[#444444] bg-[#222222] text-white hover:border-white"
                      : "bg-red-600 text-white hover:bg-red-500"
                  }`}
                >
                  Delete for me
                </button>
                <button
                  type="button"
                  onClick={() => setDeleteMsgModal({ isOpen: false, msg: null, isMe: false, type: "friend" })}
                  className="w-full rounded-xl border border-transparent px-4 py-2 text-xs font-semibold text-[#A3A3A3] hover:text-white transition"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ============================================================
            WHATSAPP-STYLE CLEAR CHAT MODAL
        ============================================================ */}
        {clearChatModal.isOpen && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-sm rounded-2xl border border-red-900/40 bg-[#1A1A1A] p-6 shadow-2xl space-y-4">
              <div className="flex items-center gap-2.5">
                <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-red-500/15 text-red-400 border border-red-500/30">
                  <RotateCcw size={16} />
                </div>
                <div>
                  <h3 className="text-sm font-bold text-white">Clear Chat Messages?</h3>
                  <p className="text-[11px] text-[#A3A3A3]">
                    {clearChatModal.targetName ? `Chat with ${clearChatModal.targetName}` : "This conversation"}
                  </p>
                </div>
              </div>

              <p className="text-xs text-[#A3A3A3] leading-relaxed">
                Are you sure you want to clear all messages from this chat? Media and text will be deleted from your view.
              </p>

              <div className="pt-2 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setClearChatModal({ isOpen: false, type: "friend", targetName: "" })}
                  className="rounded-xl border border-[#444444] px-4 py-2 text-xs font-semibold text-white hover:border-white transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmClearChat}
                  className="rounded-xl bg-red-600 px-4 py-2 text-xs font-bold text-white hover:bg-red-500 transition"
                >
                  Clear Chat
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ============================================================
            LIGHTBOX MODAL FOR FULL-SIZE IMAGE PREVIEW
        ============================================================ */}
        {lightboxImage && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/90 p-4 backdrop-blur-md"
            onClick={() => setLightboxImage(null)}
          >
            <button
              type="button"
              onClick={() => setLightboxImage(null)}
              className="absolute top-5 right-5 rounded-full bg-white/10 p-2 text-white hover:bg-white/20 transition"
            >
              <X size={22} />
            </button>
            <img
              src={lightboxImage}
              alt="Full size proof evidence"
              className="max-h-[90vh] max-w-[90vw] object-contain rounded-xl border border-white/10"
              onClick={(e) => e.stopPropagation()}
            />
          </div>
        )}
      </div>
    </AppLayout>
  );
}
