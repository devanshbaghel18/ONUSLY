import { useState, useMemo, useEffect, useRef } from "react";
import { Link, useLocation } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  Users,
  Search,
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
  Compass,
  UserCheck,
  Target,
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
  mergeCommunityHistory,
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
  reconcileFriendsWithServer,
} from "../lib/friendsChat";
import {
  getChatHistory,
  lookupUser,
  clearChatHistory,
  sendFriendRequest,
  getFriendRequests,
  respondFriendRequest,
  getFriendsList,
  getTrendingCommunities,
  joinCommunityApi,
  sendChatMessageApi,
} from "../lib/api";

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

  // ==========================================
  // COMMUNITIES STATE
  // ==========================================
  const [communities, setCommunities] = useState(() => getStoredCommunities());
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
  const [friends, setFriends] = useState(() => reconcileFriendsWithServer([], getUser()).friends);
  const friendsRef = useRef(friends);
  useEffect(() => {
    friendsRef.current = friends;
  }, [friends]);
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
  const [friendRequests, setFriendRequests] = useState({ incoming: [], outgoing: [] });
  const [friendModalTab, setFriendModalTab] = useState("add"); // 'add' | 'requests'
  const [friendSuccess, setFriendSuccess] = useState("");
  const [sendingRequest, setSendingRequest] = useState(false);

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

  // Add friend modal state (declared before effects that reference setters)
  const [showAddFriendModal, setShowAddFriendModal] = useState(false);
  const [newFriendHandle, setNewFriendHandle] = useState("");
  const [newFriendName, setNewFriendName] = useState("");
  const [newFriendEmail, setNewFriendEmail] = useState("");
  const [verifiedPartner, setVerifiedPartner] = useState(null);
  const [verifyingHandle, setVerifyingHandle] = useState(false);
  const [friendError, setFriendError] = useState("");

  // Navigation state handler (e.g. redirected from "Share for Verification")
  useEffect(() => {
    if (!location.state) return;
    const timer = setTimeout(() => {
      if (location.state?.view === "friends" && location.state?.openRequests) {
        setSelectedCommunity(null);
        setFriendModalTab("requests");
        setShowAddFriendModal(true);
        getFriendRequests().then(setFriendRequests).catch(() => {});
      } else if (location.state?.view === "friends" && location.state?.targetId) {
        setSelectedCommunity(null);
        setActiveFriendId(location.state.targetId);
        const updated = clearUnreadCount(location.state.targetId);
        setFriends(updated);
        setChatMessages(getChatMessages(location.state.targetId));
      }
      if (location.state?.view === "communities" && location.state?.targetId) {
        setActiveFriendId(null);
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

      const me = getUser();
      const myId = String(me?.id || me?.sub || "").trim();
      const myEmail = (me?.email || "").trim().toLowerCase();
      const myHandle = (me?.handle || "").replace(/[@\s]/g, "").toLowerCase();

      const senderEmail = (p.senderEmail || "").trim().toLowerCase();
      const senderHandle = (p.senderHandle || "").replace(/[@\s]/g, "").toLowerCase();
      const senderId = String(p.senderId || "").trim();

      const isFromMe = Boolean(
        (myId && senderId && senderId === myId) ||
        (myEmail && senderEmail && senderEmail === myEmail) ||
        (myHandle && senderHandle && senderHandle === myHandle)
      );

      // For messages I sent (multi-tab sync), the peer is the RECIPIENT, NOT the sender.
      const peerId = isFromMe ? String(p.recipientId || "").trim() : senderId;
      const peerEmail = isFromMe ? (p.recipientEmail || "") : (p.senderEmail || "");
      const peerHandle = isFromMe ? (p.recipientHandle || "") : (p.senderHandle || "");
      const peerName = isFromMe ? "" : (p.senderName || "");

      const normPeerEmail = peerEmail.trim().toLowerCase();
      const normPeerHandle = peerHandle.replace(/[@\s]/g, "").toLowerCase();

      const currentActiveId = activeFriendIdRef.current;
      const currentFriendsList = getStoredFriends();
      const currentActive = currentFriendsList.find((f) => f.id === currentActiveId);

      // Check whether user is currently looking at this peer's chat
      const isViewing =
        Boolean(currentActiveId) &&
        ((peerId && currentActiveId === peerId) ||
          (currentActive?.handle && normPeerHandle && currentActive.handle.toLowerCase().replace(/[@\s]/g, "") === normPeerHandle) ||
          (currentActive?.email && normPeerEmail && currentActive.email.toLowerCase() === normPeerEmail));

      const { friend, messages } = receiveChatMessage({
        messageId: p.id,
        friendId: peerId,
        friendEmail: peerEmail,
        friendHandle: peerHandle,
        friendName: peerName,
        text: p.text,
        time: timeFormatted,
        sender: isFromMe ? "me" : "friend",
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

    // Real-time Friend Request received
    if (event?.type === "friend.request") {
      getFriendRequests()
        .then((data) => {
          if (data) setFriendRequests(data);
        })
        .catch(() => {});
    }

    // Real-time Friend Request Accepted
    if (event?.type === "friend.accepted") {
      getFriendRequests()
        .then((data) => {
          if (data) setFriendRequests(data);
        })
        .catch(() => {});
      getFriendsList()
        .then((list) => {
          if (Array.isArray(list)) {
            const { friends: reconciled, remappedIds } = reconcileFriendsWithServer(list, getUser());
            setFriends(reconciled);
            if (activeFriendIdRef.current && remappedIds[activeFriendIdRef.current]) {
              const nextId = remappedIds[activeFriendIdRef.current];
              setActiveFriendId(nextId);
              setChatMessages(getChatMessages(nextId));
            }
          }
        })
        .catch(() => {});
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
    if (!activeFriendId && !selectedCommunity?.id) return;

    const syncHistory = () => {
      if (activeFriendId) {
        const f = friendsRef.current.find((item) => item.id === activeFriendId);
        const peerKey = f?.handle ? `@${f.handle.replace(/^@/, "")}` : f?.email || "";
        if (peerKey) {
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
        }
      }

      if (selectedCommunity?.id) {
        getChatHistory(selectedCommunity.id)
          .then((serverMsgs) => {
            if (serverMsgs && serverMsgs.length > 0) {
              const merged = mergeCommunityHistory(selectedCommunity.id, serverMsgs, currentUser?.email, currentUser?.handle);
              setCommChatMessages((prev) => {
                if (prev.length !== merged.length || (prev.length > 0 && merged.length > 0 && prev[prev.length - 1]?.id !== merged[merged.length - 1]?.id)) {
                  return merged;
                }
                return prev;
              });
            }
          })
          .catch(() => {});
      }
    };

    // Immediate sync on mount / friend or community switch
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
  }, [activeFriendId, selectedCommunity?.id, currentUser?.email, currentUser?.handle]);

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
        const msgId = generateMsgId("msg");
        const wsSent = send({
          type: "chat.message",
          payload: {
            id: msgId,
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

        if (!wsSent) {
          const peerTarget = activeFriend.handle ? `@${activeFriend.handle.replace(/^@/, "")}` : activeFriend.email || activeFriend.id;
          if (peerTarget) {
            sendChatMessageApi(peerTarget, text || "Sent an attachment", { replyTo: currentReplyTo }).catch(() => {});
          }
        }

        const updated = sendChatMessage(
          activeFriendId,
          {
            id: msgId,
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
        const wsSent = send({
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

        if (!wsSent) {
          sendChatMessageApi(selectedCommunity.id, text || "Sent an attachment", {
            replyTo: currentReplyTo,
          }).catch(() => {});
        }

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

        const proofs = await getProofs(resolvedGoalId, ownerId);
        const sortedProofs = [...(Array.isArray(proofs) ? proofs : [])].sort(
          (a, b) => new Date(b.submittedAt || 0).getTime() - new Date(a.submittedAt || 0).getTime()
        );
        const targetProofId =
          msg.proofId ||
          sortedProofs.find((p) => p.status === "pending" || p.status === "proof_submitted")?.id ||
          sortedProofs[0]?.id;

        if (!targetProofId) {
          throw new Error("No submitted proof found for this goal.");
        }

        await decideApproval(resolvedGoalId, targetProofId, {
          ownerId: ownerId,
          status: "approved",
          comment: "Approved from chat",
        });
      } else {
        throw new Error("Could not resolve goal ID for this proof message.");
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
    setSelectedCommunity(null);
    setActiveFriendId(id);
    const updated = clearUnreadCount(id);
    setFriends(updated);
    setChatMessages(getChatMessages(id));

    const f = updated.find((item) => item.id === id);
    const peerKey = f?.handle ? `@${f.handle.replace(/^@/, "")}` : f?.email || "";
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

  const selectCommunity = (c) => {
    setActiveFriendId(null);
    setSelectedCommunity(c);
    setCommChatMessages(getCommunityMessages(c.id));
    if (c?.id) {
      getChatHistory(c.id)
        .then((serverMsgs) => {
          if (serverMsgs && serverMsgs.length > 0) {
            const merged = mergeCommunityHistory(c.id, serverMsgs, currentUser?.email, currentUser?.handle);
            setCommChatMessages(merged);
          }
        })
        .catch(() => {});
    }
  };

  const handleSelectConversation = (conv) => {
    if (conv.type === "friend") {
      selectFriend(conv.id);
    } else if (conv.type === "community") {
      selectCommunity(conv.raw);
    }
  };

  // WhatsApp-Style Unified Chat & Community Discovery State
  const [chatFilter, setChatFilter] = useState("all"); // 'all' | 'unread' | 'friends' | 'communities'
  const [unifiedSearch, setUnifiedSearch] = useState("");
  const [showExploreModal, setShowExploreModal] = useState(false);
  const [exploreSearch, setExploreSearch] = useState("");
  const [exploreCategory, setExploreCategory] = useState("All");
  const [trendingCommunities, setTrendingCommunities] = useState([]);

  // Sync Friends, Requests, and Trending Communities from server with automatic 4-second polling
  useEffect(() => {
    const fetchFriendsAndRequests = () => {
      getFriendsList()
        .then((backendFriends) => {
          if (Array.isArray(backendFriends)) {
            const { friends: reconciled, remappedIds } = reconcileFriendsWithServer(backendFriends, getUser());
            setFriends(reconciled);
            if (activeFriendIdRef.current && remappedIds[activeFriendIdRef.current]) {
              const nextId = remappedIds[activeFriendIdRef.current];
              setActiveFriendId(nextId);
              setChatMessages(getChatMessages(nextId));
            }
          }
        })
        .catch(() => {});

      getFriendRequests()
        .then((data) => {
          if (data && (data.incoming || data.outgoing)) {
            setFriendRequests(data);
          }
        })
        .catch(() => {});
    };

    fetchFriendsAndRequests();
    const interval = setInterval(fetchFriendsAndRequests, 4000);

    getTrendingCommunities()
      .then((list) => {
        if (Array.isArray(list) && list.length > 0) {
          setTrendingCommunities(list);
        }
      })
      .catch(() => {});

    return () => clearInterval(interval);
  }, []);

  // Compute unified conversation list (WhatsApp style)
  const unifiedConversations = useMemo(() => {
    // 1. Friends mapped to unified conversation shape
    const friendConvos = friends.map((f) => ({
      id: f.id,
      type: "friend",
      name: f.name || `@${f.handle || "friend"}`,
      handle: f.handle || "",
      email: f.email || "",
      picture: f.picture || "",
      lastMessage: f.lastMessage || "No messages yet",
      lastMessageTime: f.lastMessageTime || "",
      unreadCount: Number(f.unreadCount) || 0,
      isFavorite: Boolean(f.isFavorite),
      raw: f,
    }));

    // 2. Joined communities mapped to unified conversation shape
    const joinedComms = communities
      .filter((c) => c.joined)
      .map((c) => {
        const msgs = getCommunityMessages(c.id);
        const lastMsg = msgs.length > 0 ? msgs[msgs.length - 1] : null;
        return {
          id: c.id,
          type: "community",
          name: c.name,
          category: c.category || "General",
          membersCount: c.membersCount || c.members?.length || 1,
          code: c.code || "",
          lastMessage: lastMsg
            ? lastMsg.isProof
              ? `[Proof] ${lastMsg.goalTitle || "Goal"}`
              : lastMsg.text
            : c.description || "Community group chat",
          lastMessageTime: lastMsg?.time || "",
          unreadCount: 0,
          isFavorite: Boolean(c.isFavorite),
          raw: c,
        };
      });

    // 3. Combine and sort
    const all = [...friendConvos, ...joinedComms];
    all.sort((a, b) => {
      if (a.isFavorite && !b.isFavorite) return -1;
      if (!a.isFavorite && b.isFavorite) return 1;
      return 0;
    });
    return all;
  }, [friends, communities]);

  // Filtered unified conversations
  const filteredConversations = useMemo(() => {
    let list = unifiedConversations;

    // Filter chip tab
    if (chatFilter === "unread") {
      list = list.filter((c) => c.unreadCount > 0);
    } else if (chatFilter === "friends") {
      list = list.filter((c) => c.type === "friend");
    } else if (chatFilter === "communities") {
      list = list.filter((c) => c.type === "community");
    }

    // Search query
    if (unifiedSearch.trim()) {
      const q = unifiedSearch.toLowerCase();
      list = list.filter(
        (c) =>
          c.name.toLowerCase().includes(q) ||
          (c.handle && c.handle.toLowerCase().includes(q)) ||
          (c.category && c.category.toLowerCase().includes(q)) ||
          (c.lastMessage && c.lastMessage.toLowerCase().includes(q))
      );
    }

    return list;
  }, [unifiedConversations, chatFilter, unifiedSearch]);

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

  const handleSendFriendRequest = async (e) => {
    e.preventDefault();
    const cleanHandle = newFriendHandle.replace(/[@\s]/g, "").toLowerCase();
    const cleanEmail = newFriendEmail.trim().toLowerCase();
    const target = cleanHandle ? `@${cleanHandle}` : cleanEmail;
    if (!target) {
      setFriendError("Please enter an ONUSLY handle (e.g. @bob) or email");
      return;
    }

    setSendingRequest(true);
    setFriendError("");
    setFriendSuccess("");

    try {
      await sendFriendRequest(target);
      setFriendSuccess(`Friend request sent to ${target}! Once they accept, they will appear in your chat list.`);
      setNewFriendHandle("");
      setNewFriendEmail("");
      setVerifiedPartner(null);
      getFriendRequests().then(setFriendRequests).catch(() => {});
    } catch (err) {
      setFriendError(err.message || "Failed to send friend request");
    } finally {
      setSendingRequest(false);
    }
  };

  const handleRespondFriendRequest = async (requestId, action, reqData) => {
    try {
      await respondFriendRequest(requestId, action);
      if (action === "accept" && reqData) {
        const updated = addFriend({
          id: reqData.senderId || "",
          name: reqData.senderName || `@${reqData.senderHandle || "friend"}`,
          handle: reqData.senderHandle,
          email: reqData.senderEmail,
        });
        setFriends(updated);
        const added = updated.find(
          (f) =>
            (reqData.senderId && f.id === reqData.senderId) ||
            (reqData.senderHandle && f.handle && f.handle.toLowerCase() === reqData.senderHandle.toLowerCase()) ||
            (reqData.senderEmail && f.email && f.email.toLowerCase() === reqData.senderEmail.toLowerCase())
        );
        if (added) {
          selectFriend(added.id);
          setShowAddFriendModal(false);
        }
      }
      const refreshedReqs = await getFriendRequests();
      setFriendRequests(refreshedReqs);
      window.dispatchEvent(new Event("onusly:inbox-updated"));
    } catch (err) {
      setFriendError(err.message || `Failed to ${action} request`);
    }
  };

  const handleJoinTrendingCommunity = async (comm) => {
    try {
      await joinCommunityApi(comm.id);
    } catch (err) {
      console.warn("Server join error, joining locally:", err);
    }
    const updated = addOrUpdateCommunity({
      id: comm.id,
      name: comm.name,
      description: comm.description,
      category: comm.category,
      membersCount: comm.members || 1,
      joined: true,
      code: comm.id.toUpperCase(),
    });
    setCommunities(updated);
    const target = updated.find((c) => c.id === comm.id);
    if (target) {
      selectCommunity(target);
    }
    setShowExploreModal(false);
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
    const recipientId = currentFriend?.id && !String(currentFriend.id).startsWith("friend-") ? currentFriend.id : "";

    const wsSent = send({
      type: "chat.message",
      payload: {
        id: msgId,
        recipientId,
        recipientEmail: currentFriend?.email || "",
        recipientHandle: recHandle,
        senderEmail,
        senderHandle,
        senderName,
        text,
        replyTo: currentReplyTo,
      },
    });

    // Only fall back to REST HTTP POST when WebSocket is disconnected —
    // prevents double delivery and duplicate message IDs in DynamoDB & UI.
    if (!wsSent) {
      const peerTarget = recHandle ? `@${recHandle}` : currentFriend?.email || currentFriend?.id;
      if (peerTarget) {
        sendChatMessageApi(peerTarget, text, { id: msgId, replyTo: currentReplyTo }).catch(() => {});
      }
    }

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

    const wsSent = send({
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

    if (!wsSent) {
      sendChatMessageApi(selectedCommunity.id, text, { id: msgId, replyTo: currentReplyTo }).catch(() => {});
    }

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

      const f = friends.find((item) => item.id === activeFriendId);
      const peerKey = f?.handle ? `@${f.handle}` : f?.email || "";
      if (peerKey) {
        clearChatHistory(peerKey).catch(() => {});
      }
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
          className={`relative max-w-[80%] sm:max-w-[70%] rounded-lg px-4 py-3 text-xs transition-all duration-300 ${
            isHighlighted
              ? "ring-1 ring-[#ff7843] shadow-[0_0_24px_rgba(255,120,67,0.25)] scale-[1.01] bg-[#1e2732]"
              : ""
          } ${
            isMe
              ? "bg-[#1a222a] border border-[#2e3b4a] border-t-[#3f5062] text-[#f2eee7] shadow-sm"
              : "border border-[#232c36] border-t-[#2d3946] bg-[#12161c] text-[#e0ded6] shadow-sm"
          }`}
        >
          {/* Sender badge for peers */}
          {showSenderBadge && !isMe && (
            <div className="mb-1 flex items-center gap-1.5 font-bold text-[11px] text-white">
              <span>{msg.senderName || "Member"}</span>
              {msg.senderHandle && (
                <span className="font-mono text-[10px] text-neutral-400">
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
              className={`mb-2 rounded border-l-2 p-2 text-[11px] select-none cursor-pointer transition-all ${
                isMe
                  ? "border-l-[#ff7843] bg-[#11161d] text-neutral-300 hover:bg-[#141a22]"
                  : "border-l-[#384656] bg-[#0c0f13] text-neutral-300 hover:bg-[#0f1318]"
              }`}
              title="Click to jump to quoted message"
            >
              <div className="flex items-center justify-between font-bold text-[10px] text-white mb-0.5">
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
            <div className="mb-2 rounded border border-[#ff7843]/30 bg-[#141920] p-3 text-neutral-200 shadow-md">
              <div className="flex items-center justify-between gap-2 border-b border-[#2d3744] pb-2 mb-2">
                <div className="flex items-center gap-1.5 text-[11px] font-mono font-bold uppercase tracking-wider text-[#ff9769]">
                  <ShieldCheck size={14} className="text-[#ff7843]" />
                  <span>GOAL VERIFICATION REQUEST</span>
                </div>
                <div className="flex items-center gap-2">
                  {msg.approved ? (
                    <span className="flex items-center gap-1 text-[11px] font-mono font-bold text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/30">
                      <CheckCircle2 size={12} />
                      <span>RELEASED</span>
                    </span>
                  ) : (
                    <span className="text-[10px] text-amber-300 font-mono font-bold uppercase bg-amber-500/10 px-2 py-0.5 rounded border border-amber-500/30">
                      PENDING WITNESS
                    </span>
                  )}
                  {msg.goalId && (
                    <Link
                      to={`/goals/${msg.goalId}${msg.ownerId ? `?ownerId=${msg.ownerId}` : ""}`}
                      className="flex items-center gap-1 text-[11px] font-mono font-bold text-neutral-300 hover:text-white underline"
                    >
                      <span>DETAILS</span>
                      <ExternalLink size={11} />
                    </Link>
                  )}
                </div>
              </div>
              {displayGoalTitle && (
                <div className="flex items-center gap-1.5 text-xs font-bold text-white mb-1">
                  <Target size={13} className="text-[#ff7843] shrink-0" />
                  <span>{displayGoalTitle}</span>
                </div>
              )}
              {msg.text && (
                <p className="text-xs leading-relaxed mb-2 text-neutral-200">
                  {msg.text}
                </p>
              )}
              {msg.externalLink && (
                <a
                  href={msg.externalLink}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-[11px] text-neutral-300 hover:text-white underline underline-offset-2 mb-2"
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
                      className="flex items-center gap-2.5 rounded border border-[#2b3644] bg-[#10141a] p-2.5 text-xs text-white transition hover:bg-[#161c24] max-w-sm"
                    >
                      <div className="flex h-8 w-8 items-center justify-center rounded bg-[#182028] text-white border border-[#2b3644] shrink-0">
                        <FileText size={16} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="font-semibold truncate text-[11px]">{f.name || "Attached File"}</p>
                        <span className="text-[10px] text-neutral-400">Click to view / download</span>
                      </div>
                    </a>
                  ))}
                </div>
              )}

              {/* If approved, show banner. If not approved and not me, show Approve button */}
              {msg.approved ? (
                <div className="mt-2.5 flex items-center gap-1.5 rounded border border-emerald-500/30 bg-emerald-500/10 px-3 py-1.5 text-[11px] font-mono font-bold text-emerald-400 shadow-sm">
                  <CheckCircle2 size={14} />
                  <span>VERIFIED & UNLOCKED // TARGET RESTRICTIONS RELEASED</span>
                </div>
              ) : !isMe ? (
                <button
                  type="button"
                  disabled={approvingMsgId === msg.id}
                  onClick={() => handleApproveChatProof({ ...msg, goalTitle: displayGoalTitle })}
                  className="mt-2.5 flex items-center gap-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-bold px-3.5 py-1.5 text-xs transition disabled:opacity-50 shadow-md shadow-emerald-950/40"
                >
                  {approvingMsgId === msg.id ? (
                    <Loader2 size={13} className="animate-spin" />
                  ) : (
                    <ThumbsUp size={13} />
                  )}
                  <span>Approve & Unlock Goal</span>
                </button>
              ) : (
                <div className="mt-2 flex items-center gap-1.5 text-[10px] font-mono text-neutral-400">
                  <span className="h-1.5 w-1.5 rounded-full bg-neutral-400 animate-pulse" />
                  <span>AWAITING PARTNER REVIEW & VERIFICATION</span>
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
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-white/10 text-white border border-white/20 shrink-0">
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
              className="inline-flex items-center gap-1 text-[11px] text-neutral-300 hover:text-white underline underline-offset-2 mb-1"
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
                  text: (isVerification && displayGoalTitle ? `Goal: ${displayGoalTitle}` : "") || msg.text || (msg.images?.length ? "[Image Attachment]" : "[Attachment]"),
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
              className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 text-neutral-400 hover:text-white"
              title="Delete message"
            >
              <Trash2 size={11} />
            </button>
          </div>
        </div>
      </div>
    );
  };

  const renderCommunityChatPane = () => {
    if (!selectedCommunity) return null;
    if (!selectedCommunity.joined) {
      return (
        <div className="flex-1 min-h-0 flex flex-col justify-center h-full p-8 text-center space-y-4 bg-[#121212]">
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
      );
    }

    return (
      <div className="flex-1 min-h-0 flex flex-col h-full overflow-hidden bg-[#0e1216]">
        {/* Integrated Community Chat Header */}
        <div className="shrink-0 flex flex-col gap-3 border-b border-[#26313d] bg-[#11151a] p-3 sm:px-4 sm:py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => setSelectedCommunity(null)}
              className="rounded border border-[#2b3542] bg-[#161c23] p-1.5 text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
              title="Back to all chats"
            >
              <ArrowLeft size={15} />
            </button>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-sm sm:text-base font-bold text-white flex items-center gap-2">
                  <span>{selectedCommunity.name}</span>
                  {selectedCommunity.isFavorite && (
                    <Star size={13} className="fill-[#ff7843] text-[#ff7843] shrink-0" title="Favorited Community" />
                  )}
                </h2>
                <span className="rounded border border-[#2d3a49] bg-[#161d26] px-2 py-0.5 text-[9px] font-mono uppercase font-bold text-[#ff7843]">
                  {selectedCommunity.category}
                </span>
              </div>
              <p className="text-[11px] font-mono text-neutral-400 mt-0.5">
                {selectedCommunity.membersCount || 1} MEMBERS • {selectedCommunity.requiredApprovals || 2} WITNESS APPROVALS REQ.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            <button
              type="button"
              onClick={() => handleCopyCode(selectedCommunity.code)}
              className="flex items-center gap-1.5 rounded border border-[#2b3542] bg-[#161c23] px-2.5 py-1 text-xs font-mono font-semibold text-neutral-300 hover:border-[#3d4c5e] hover:text-white transition"
              title="Share Invite Code"
            >
              <Copy size={12} />
              <span>{copiedCode ? "COPIED" : `CODE: ${selectedCommunity.code}`}</span>
            </button>

            {/* WhatsApp-style 3-Dot Menu */}
            <div className="relative" ref={commMenuRef}>
              <button
                type="button"
                onClick={() => setShowCommMenu((prev) => !prev)}
                className="rounded border border-[#2b3542] bg-[#161c23] p-1.5 text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                title="More options"
              >
                <MoreVertical size={15} />
              </button>

              {showCommMenu && (
                <div className="absolute right-0 top-full mt-1.5 w-48 rounded-lg border border-[#2d3744] bg-[#141920] py-1 shadow-2xl z-40 backdrop-blur-md">
                  <button
                    type="button"
                    onClick={handleToggleFavoriteActiveCommunity}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-neutral-200 hover:bg-[#1d252f] hover:text-white text-left transition"
                  >
                    <Star size={13} className={selectedCommunity.isFavorite ? "fill-[#ff7843] text-[#ff7843]" : "text-neutral-400"} />
                    <span>{selectedCommunity.isFavorite ? "Remove Star" : "Add to Favorites"}</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setShowCommMenu(false);
                      setClearChatModal({ isOpen: true, type: "community", targetName: selectedCommunity.name });
                    }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-neutral-200 hover:bg-[#1d252f] hover:text-white text-left transition"
                  >
                    <RotateCcw size={13} className="text-neutral-400" />
                    <span>Clear Chat</span>
                  </button>
                  {selectedCommunity.joined && (
                    <>
                      <div className="my-1 border-t border-[#232c36]" />
                      <button
                        type="button"
                        onClick={() => {
                          setShowCommMenu(false);
                          setCommToLeave(selectedCommunity);
                        }}
                        className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-neutral-200 hover:bg-[#1d252f] hover:text-white text-left transition"
                      >
                        <ArrowLeft size={13} className="text-neutral-400" />
                        <span>Leave Community</span>
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setShowCommMenu(false);
                          setCommToDelete(selectedCommunity);
                        }}
                        className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-rose-400 hover:bg-rose-500/10 text-left transition"
                      >
                        <Trash2 size={13} className="text-rose-400" />
                        <span>Delete Community</span>
                      </button>
                    </>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Community Description Banner */}
        <div className="shrink-0 border-b border-[#202933] bg-[#0c0f13] px-4 py-2 flex items-center justify-between text-xs text-neutral-400">
          <span className="truncate max-w-xl font-mono text-[11px]">
            {selectedCommunity.description}
          </span>
          <span className="font-mono text-[10px] text-neutral-500 uppercase shrink-0 ml-2">
            CHANNEL // {selectedCommunity.code}
          </span>
        </div>

        {/* Community Chat Stream */}
        <div
          ref={commChatContainerRef}
          onScroll={(e) => {
            const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
            isCommAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 100;
          }}
          className="flex-1 min-h-0 space-y-3 overflow-y-auto no-scrollbar p-4 overscroll-contain bg-[#0e1216]"
        >
          {commChatMessages.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center py-16 text-center">
              <Users size={32} className="text-neutral-600 mb-2" />
              <p className="text-xs font-semibold text-white">Channel Standby</p>
              <p className="mt-1 text-xs font-mono text-neutral-400 max-w-sm">
                Broadcast proof, verified telemetry, or messages to the cluster using the attachment trigger.
              </p>
            </div>
          ) : (
            commChatMessages.map((msg) => {
              const u = getUser();
              const myEmail = (u?.email || "").toLowerCase();
              const myHandle = (u?.handle || "").replace(/[@\s]/g, "").toLowerCase();
              const msgSenderEmail = (msg.senderEmail || "").toLowerCase();
              const msgSenderHandle = (msg.senderHandle || "").replace(/[@\s]/g, "").toLowerCase();

              const isMe =
                msg.sender === "me" ||
                (Boolean(myEmail) && msgSenderEmail === myEmail) ||
                (Boolean(myHandle) && msgSenderHandle === myHandle);

              return (
                <ChatBubble
                  key={msg.id}
                  msg={msg}
                  isMe={isMe}
                  showSenderBadge={true}
                  onDelete={(m, isCurrentUser) => requestDeleteMessage(m, isCurrentUser, "community")}
                  onReply={(replyContext) => setReplyingTo(replyContext)}
                  onJumpToMessage={handleJumpToMessage}
                  isHighlighted={highlightedMsgId === msg.id}
                  onImageClick={(imgUrl) => setLightboxImage(imgUrl)}
                />
              );
            })
          )}
          <div ref={commChatEndRef} />
        </div>

        {/* Community Chat Input */}
        <form
          onSubmit={handleSendCommMessage}
          className="shrink-0 sticky bottom-0 z-20 flex items-center gap-2 border-t border-[#26313d] bg-[#11151a] p-3"
        >
          <button
            type="button"
            onClick={() => openAttachModal("community")}
            title="Attach photos, files or links"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded border border-[#2b3542] bg-[#161c23] text-neutral-400 hover:border-[#ff7843]/50 hover:text-white transition"
          >
            <Paperclip size={15} />
          </button>

          <input
            type="text"
            placeholder="Broadcast telemetry to channel..."
            value={commMessageInput}
            onChange={(e) => setCommMessageInput(e.target.value)}
            className="flex-1 rounded border border-[#26313d] bg-[#0a0d10] px-3.5 py-2 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
          />

          <button
            type="submit"
            disabled={!commMessageInput.trim()}
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded border border-[#ff7843]/40 bg-[#ff7843] text-black transition-all hover:bg-[#ff956b] disabled:opacity-30 disabled:hover:bg-[#ff7843]"
          >
            <Send size={14} />
          </button>
        </form>
      </div>
    );
  };

  return (
    <AppLayout>
      <div className="mx-auto w-full max-w-[1720px] flex-1 flex flex-col min-h-0 h-full">
        {/* Compact Pro Command Bar */}
        <div className="shrink-0 flex items-center justify-between gap-3 border-b border-[#26313d] bg-[#11151a] px-4 py-2.5 rounded-t-lg mb-0">
          <div className="flex items-center gap-3">
            <span className="instrument-label"><i /> DISPATCH</span>
            <h1 className="text-sm sm:text-base font-bold text-white tracking-tight flex items-center gap-1.5">
              <span>Conversations</span>
              <span className="text-[#ff7843]">.</span>
            </h1>
            <span className="hidden md:inline-block font-mono text-[10px] text-neutral-400 border-l border-[#26313d] pl-3 uppercase">
              PEER VERIFICATION // LIVE SYNC
            </span>
          </div>
          {/* Quick Action Buttons */}
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setShowExploreModal(true)}
              className="flex items-center gap-1.5 rounded border border-[#2b3542] bg-[#161c23] hover:bg-[#1f2732] hover:border-[#3d4c5e] px-3 py-1.5 text-xs font-semibold text-neutral-200 transition"
              title="Explore and join accountability communities"
            >
              <Compass size={13} className="text-neutral-400" />
              <span>Explore</span>
            </button>
            <button
              type="button"
              onClick={() => {
                setFriendModalTab(friendRequests.incoming?.length > 0 ? "requests" : "add");
                setShowAddFriendModal(true);
              }}
              className="flex items-center gap-1.5 rounded border border-[#ff7843]/40 bg-[#ff7843] px-3 py-1.5 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-sm"
              title="Add friend or view pending friend requests"
            >
              <UserPlus size={13} />
              <span>Add Friend</span>
              {friendRequests.incoming?.length > 0 && (
                <span className="flex h-3.5 min-w-[14px] items-center justify-center rounded bg-black text-[9px] font-black text-white px-0.5">
                  {friendRequests.incoming.length}
                </span>
              )}
            </button>
          </div>
        </div>

        {/* ============================================================
            UNIFIED CHAT WORKSPACE (Friends + Joined Communities)
        ============================================================ */}
        <div className="flex-1 min-h-0 grid grid-cols-1 overflow-hidden rounded-b-lg border border-[#26313d] bg-[#0c0f13] shadow-2xl md:grid-cols-12">
          {/* Left Column: Unified Conversations List */}
          <div
            className={`${
              activeFriend || selectedCommunity ? "hidden md:flex" : "flex"
            } flex-col h-full min-h-0 border-b border-[#26313d] bg-[#0f1318] p-3 sm:p-3.5 md:col-span-4 md:border-b-0 md:border-r md:border-[#26313d] overflow-hidden`}
          >
            {/* Header: Chat count & quick actions */}
            <div className="shrink-0 flex items-center justify-between pb-2 border-b border-[#1c232a]">
              <span className="text-[11px] font-mono font-bold uppercase tracking-wider text-neutral-400">
                CHANNELS ({filteredConversations.length})
              </span>
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => setShowExploreModal(true)}
                  className="rounded border border-[#26313d] bg-[#141920] px-2 py-0.5 text-[10px] font-mono font-semibold text-neutral-300 hover:text-white hover:border-[#384654] transition"
                  title="Explore and join communities"
                >
                  EXPLORE
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setFriendModalTab(friendRequests.incoming?.length > 0 ? "requests" : "add");
                    setShowAddFriendModal(true);
                  }}
                  className="inline-flex items-center gap-1 rounded border border-[#ff7843]/40 bg-[#ff7843] px-2 py-0.5 text-[10px] font-bold text-black shadow-sm"
                  title="Add friend"
                >
                  <UserPlus size={11} />
                  <span>ADD</span>
                  {friendRequests.incoming?.length > 0 && (
                    <span className="flex h-3 min-w-[12px] items-center justify-center rounded bg-black text-[8px] font-black text-white px-0.5">
                      {friendRequests.incoming.length}
                    </span>
                  )}
                </button>
              </div>
            </div>

            {/* Filter Tabs: All | Unread | Friends | Communities */}
            <div className="shrink-0 flex items-center gap-1.5 py-2 overflow-x-auto no-scrollbar">
              {[
                { id: "all", label: "All" },
                {
                  id: "unread",
                  label: "Unread",
                  badge: unifiedConversations.reduce((sum, c) => sum + (c.unreadCount || 0), 0),
                },
                { id: "friends", label: "Friends" },
                { id: "communities", label: "Communities" },
              ].map((chip) => {
                const isActive = chatFilter === chip.id;
                return (
                  <button
                    key={chip.id}
                    type="button"
                    onClick={() => setChatFilter(chip.id)}
                    className={`shrink-0 flex items-center gap-1.5 rounded px-2.5 py-1 text-[11px] font-mono font-semibold transition ${
                      isActive
                        ? "bg-[#1f2732] text-white border border-[#3b4756] shadow-sm font-bold"
                        : "bg-[#141920] text-neutral-400 hover:text-white hover:bg-[#1a212a] border border-[#222933]"
                    }`}
                  >
                    <span>{chip.label}</span>
                    {chip.badge > 0 && (
                      <span
                        className={`rounded px-1 text-[9px] font-bold ${
                          isActive ? "bg-[#ff7843] text-black" : "bg-[#273240] text-neutral-200"
                        }`}
                      >
                        {chip.badge}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>

            {/* Unified Search Input */}
            <div className="shrink-0 relative mt-1">
              <Search
                size={13}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-neutral-500"
              />
              <input
                type="text"
                placeholder="Search channels, handles..."
                value={unifiedSearch}
                onChange={(e) => setUnifiedSearch(e.target.value)}
                className="w-full rounded border border-[#242d38] bg-[#0a0d10] py-1.5 pl-8 pr-3 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
              />
            </div>

            {/* Incoming Friend Request Alert Banner */}
            {friendRequests.incoming && friendRequests.incoming.length > 0 && (
              <div
                onClick={() => {
                  setFriendModalTab("requests");
                  setShowAddFriendModal(true);
                }}
                className="shrink-0 mt-2.5 flex cursor-pointer items-center justify-between gap-2 rounded-xl border border-white/20 bg-white/10 px-3 py-2 text-xs transition hover:bg-white/15 shadow-sm"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className="flex h-2 w-2 rounded-full bg-emerald-400 animate-pulse shrink-0" />
                  <p className="font-semibold text-white truncate text-[11px]">
                    {friendRequests.incoming.length} incoming friend {friendRequests.incoming.length === 1 ? "request" : "requests"}
                  </p>
                </div>
                <span className="btn-glow shrink-0 rounded-lg px-2 py-0.5 text-[10px] font-bold text-black">
                  Review
                </span>
              </div>
            )}

            {/* Unified Conversations List items */}
            <div className="flex-1 min-h-0 mt-3 space-y-1 overflow-y-auto no-scrollbar">
              {filteredConversations.length === 0 ? (
                <div className="flex h-48 flex-col items-center justify-center p-4 text-center">
                  <MessageSquare size={28} className="text-[#444444] mb-2" />
                  <p className="text-xs font-semibold text-neutral-300">
                    No conversations found
                  </p>
                  <p className="text-[11px] text-[#737373] max-w-xs mt-1">
                    {unifiedSearch
                      ? "Try a different search keyword"
                      : "Add an accountability friend or join a trending community to get started"}
                  </p>
                  <div className="flex items-center gap-2 mt-4">
                    <button
                      type="button"
                      onClick={() => setShowExploreModal(true)}
                      className="rounded-lg border border-[#333333] bg-[#1E1E1E] px-3 py-1.5 text-xs font-medium text-white hover:border-white transition"
                    >
                      Explore Communities
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setFriendModalTab("add");
                        setShowAddFriendModal(true);
                      }}
                      className="btn-glow rounded-lg px-3 py-1.5 text-xs font-bold text-black"
                    >
                      Add Friend
                    </button>
                  </div>
                </div>
              ) : (
                filteredConversations.map((item) => {
                  const isFriend = item.type === "friend";
                  const isSelected = isFriend
                    ? activeFriendId === item.id
                    : selectedCommunity?.id === item.id;
                  const hasUnread = item.unreadCount > 0;

                  return (
                    <div
                      key={`${item.type}-${item.id}`}
                      data-convo-id={item.id}
                      onClick={() => handleSelectConversation(item)}
                      className={`relative flex cursor-pointer items-center justify-between rounded p-2.5 transition-colors border ${
                        isSelected
                          ? "bg-[#171e26] border-[#384656] shadow-sm before:absolute before:left-0 before:top-1.5 before:bottom-1.5 before:w-1 before:bg-[#ff7843] before:rounded-r"
                          : "border-transparent hover:bg-[#12161c] hover:border-[#1f2833]"
                      }`}
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        {/* Avatar */}
                        {isFriend ? (
                          <div className="relative flex h-9 w-9 flex-shrink-0 items-center justify-center rounded border border-[#2b3542] bg-[#161c23] text-xs font-mono font-bold text-neutral-200 overflow-hidden">
                            {item.picture ? (
                              <img
                                src={item.picture}
                                alt={item.name}
                                className="h-full w-full object-cover"
                                referrerPolicy="no-referrer"
                              />
                            ) : (
                              item.name[0]?.toUpperCase() || "F"
                            )}
                            <span
                              className={`absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border border-[#11151a] ${
                                (item.handle &&
                                  onlineHandles.has(
                                    item.handle.toLowerCase().replace(/^@/, "")
                                  )) ||
                                (item.email &&
                                  onlineEmails.has(item.email.trim().toLowerCase()))
                                  ? "bg-[#10b981]"
                                  : "bg-neutral-600"
                              }`}
                              title={
                                (item.handle &&
                                  onlineHandles.has(
                                    item.handle.toLowerCase().replace(/^@/, "")
                                  )) ||
                                (item.email &&
                                  onlineEmails.has(item.email.trim().toLowerCase()))
                                  ? "Online"
                                  : "Offline"
                              }
                            />
                          </div>
                        ) : (
                          <div className="relative flex h-9 w-9 flex-shrink-0 items-center justify-center rounded border border-[#2b3542] bg-[#161c23] text-neutral-300">
                            <Users size={16} className="text-[#ff7843]" />
                          </div>
                        )}

                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-1.5">
                            <p
                              className={`truncate text-xs ${
                                hasUnread ? "font-bold text-white" : "font-semibold text-neutral-200"
                              }`}
                            >
                              {item.name}
                            </p>
                            {item.isFavorite && (
                              <Star
                                size={11}
                                className="fill-[#ff7843] text-[#ff7843] shrink-0"
                                title="Favorited"
                              />
                            )}
                            {isFriend && item.handle && (
                              <span className="font-mono text-[10px] text-neutral-400">
                                @{item.handle.replace(/^@/, "")}
                              </span>
                            )}
                            {!isFriend && (
                              <span className="rounded border border-[#26313d] bg-[#12161c] px-1.5 py-0.2 text-[9px] font-mono font-bold text-neutral-400 uppercase">
                                {item.category}
                              </span>
                            )}
                          </div>
                          <p
                            className={`truncate text-[11px] font-mono ${
                              hasUnread ? "font-medium text-neutral-200" : "text-neutral-500"
                            }`}
                          >
                            {item.lastMessage || (isFriend ? "Select to open channel" : "Community channel")}
                          </p>
                        </div>
                      </div>

                      <div className="flex flex-col items-end gap-1 flex-shrink-0 ml-2">
                        {item.lastMessageTime && (
                          <span
                            className={`text-[10px] font-mono ${
                              hasUnread ? "font-bold text-[#ff7843]" : "text-neutral-500"
                            }`}
                          >
                            {item.lastMessageTime}
                          </span>
                        )}
                        {hasUnread && (
                          <span className="flex h-4 min-w-[18px] items-center justify-center rounded bg-[#ff7843] px-1 text-[9px] font-mono font-bold text-black shadow-sm">
                            {item.unreadCount > 99 ? "99+" : item.unreadCount}
                          </span>
                        )}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>

          {/* Right Column: Chat Box */}
          <div
            className={`${
              activeFriend || selectedCommunity ? "flex" : "hidden md:flex"
            } flex-col h-full min-h-0 overflow-hidden bg-[#0e1216] md:col-span-8`}
          >
              {activeFriend ? (
                <>
                  {/* Chat Header */}
                  <div className="shrink-0 flex items-center justify-between border-b border-[#26313d] bg-[#11151a] px-4 py-3">
                    <div className="flex items-center gap-3">
                      <button
                        type="button"
                        onClick={() => {
                          setActiveFriendId(null);
                          setChatMessages([]);
                        }}
                        className="rounded border border-[#2b3542] bg-[#161c23] p-1.5 text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                        title="Back to Share Proof / All Chats"
                      >
                        <ArrowLeft size={15} />
                      </button>
                      <div className="relative flex h-9 w-9 items-center justify-center rounded border border-[#2b3542] bg-[#161c23] text-sm font-mono font-bold text-white overflow-hidden">
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
                          className={`absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border border-[#11151a] ${
                            (activeFriend.handle &&
                              onlineHandles.has(
                                activeFriend.handle.toLowerCase().replace(/^@/, "")
                              )) ||
                            (activeFriend.email &&
                              onlineEmails.has(activeFriend.email.trim().toLowerCase()))
                              ? "bg-[#10b981]"
                              : "bg-neutral-600"
                          }`}
                        />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <h3 className="text-sm font-bold text-white flex items-center gap-1.5">
                            <span>{activeFriend.name}</span>
                            {activeFriend.isFavorite && (
                              <Star size={13} className="fill-[#ff7843] text-[#ff7843] shrink-0" title="Starred Friend" />
                            )}
                          </h3>
                          {activeFriend.handle && (
                            <span className="rounded border border-[#26313d] bg-[#141920] px-1.5 py-0.2 font-mono text-[10px] font-semibold text-neutral-300">
                              @{activeFriend.handle.replace(/^@/, "")}
                            </span>
                          )}
                        </div>
                        <p className="text-[10px] font-mono text-neutral-400 flex items-center gap-1.5 mt-0.5 uppercase">
                          <span>WITNESS PARTNER</span>
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
                                className={`flex items-center gap-1 font-mono ${
                                  isOnline ? "text-[#10b981]" : "text-neutral-500"
                                }`}
                              >
                                <span
                                  className={`h-1.5 w-1.5 rounded-full ${
                                    isOnline ? "bg-[#10b981]" : "bg-neutral-500"
                                  }`}
                                />
                                {isOnline ? "CONNECTED" : "OFFLINE"}
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
                          className="rounded border border-[#2b3542] bg-[#161c23] p-1.5 text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                          title="More options"
                        >
                          <MoreVertical size={15} />
                        </button>

                        {showFriendMenu && (
                          <div className="absolute right-0 top-full mt-1.5 w-48 rounded-lg border border-[#2d3744] bg-[#141920] py-1 shadow-2xl z-40 backdrop-blur-md">
                            <button
                              type="button"
                              onClick={handleToggleFavoriteActiveFriend}
                              className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-neutral-200 hover:bg-[#1d252f] hover:text-white text-left transition"
                            >
                              <Star size={13} className={activeFriend.isFavorite ? "fill-[#ff7843] text-[#ff7843]" : "text-neutral-400"} />
                              <span>{activeFriend.isFavorite ? "Remove Star" : "Add to Favorites"}</span>
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                setShowFriendMenu(false);
                                setClearChatModal({ isOpen: true, type: "friend", targetName: activeFriend.name });
                              }}
                              className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-neutral-200 hover:bg-[#1d252f] hover:text-white text-left transition"
                            >
                              <RotateCcw size={13} className="text-neutral-400" />
                              <span>Clear Chat</span>
                            </button>
                            <div className="my-1 border-t border-[#232c36]" />
                            <button
                              type="button"
                              onClick={() => {
                                setShowFriendMenu(false);
                                setFriendToDelete(activeFriend);
                              }}
                              className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-rose-400 hover:bg-rose-500/10 text-left transition"
                            >
                              <Trash2 size={13} className="text-rose-400" />
                              <span>Delete Witness Contact</span>
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
                        <p className="text-xs font-mono text-neutral-400">
                          No messages yet. Transmit a message or select attachments to share verification.
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
                    <div className="shrink-0 flex items-center justify-between border-t border-l-4 border-l-white border-t-[#2A2A2A] bg-[#1E1E1E] px-4 py-2 text-xs">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 text-[10px] font-bold text-white">
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

                  {/* Chat Input - Engineered Hardware Cockpit */}
                  <form
                    onSubmit={handleSendMessage}
                    className="shrink-0 sticky bottom-0 z-20 flex items-center gap-2 border-t border-[#26313d] bg-[#11151a] p-3"
                  >
                    <button
                      type="button"
                      onClick={() => openAttachModal("friend")}
                      title="Attach photos, files or links"
                      className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded border border-[#2b3542] bg-[#161c23] text-neutral-400 hover:border-[#3d4c5e] hover:text-white transition"
                    >
                      <Paperclip size={16} />
                    </button>

                    <input
                      type="text"
                      placeholder="Type a message or verification update..."
                      value={messageInput}
                      onChange={(e) => setMessageInput(e.target.value)}
                      className="flex-1 rounded border border-[#26313d] bg-[#0a0d10] px-3.5 py-2 text-xs text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                    />

                    <button
                      type="submit"
                      disabled={!messageInput.trim()}
                      className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded border border-[#ff7843]/40 bg-[#ff7843] text-black font-bold transition-all hover:bg-[#ff956b] disabled:opacity-30 disabled:hover:bg-[#ff7843]"
                    >
                      <Send size={14} />
                    </button>
                  </form>
                </>
              ) : selectedCommunity ? (
                renderCommunityChatPane()
              ) : (
                <div className="relative flex h-full flex-col items-center justify-center p-8 text-center select-none overflow-hidden bg-[#0c0e12]">
                  {/* Subtle radial ambient background glow */}
                  <div className="pointer-events-none absolute h-72 w-72 rounded-full bg-[#ff7843]/[0.02] blur-3xl" />

                  {/* Machined Terminal Emblem */}
                  <div className="relative flex h-16 w-16 items-center justify-center rounded-lg border border-[#2d3744] bg-[#12161c] shadow-xl">
                    <ShieldCheck size={32} className="text-[#ff7843]" />
                    <span className="absolute -bottom-1 -right-1 flex h-5 w-5 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-neutral-400">
                      <Paperclip size={10} />
                    </span>
                  </div>

                  <div className="mt-4 flex items-center gap-1.5 text-[10px] font-mono text-neutral-400 uppercase tracking-widest">
                    <span className="h-1.5 w-1.5 rounded-full bg-[#10b981]" />
                    <span>DISPATCH ENGINE // STANDBY</span>
                  </div>

                  {/* Heading & Tagline */}
                  <h2 className="mt-2 text-xl font-bold tracking-tight text-white">
                    Accountability Dispatch
                  </h2>
                  <p className="mt-2 max-w-sm text-xs leading-relaxed text-neutral-400">
                    Select an accountability witness or community channel on the left to review proof, confirm completions, and enforce active locks.
                  </p>

                  <div className="mt-6 flex flex-wrap items-center justify-center gap-3">
                    <button
                      type="button"
                      onClick={() => {
                        setFriendModalTab(friendRequests.incoming?.length > 0 ? "requests" : "add");
                        setShowAddFriendModal(true);
                      }}
                      className="flex items-center gap-1.5 rounded border border-[#ff7843]/40 bg-[#ff7843] px-4 py-2 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-md"
                    >
                      <UserPlus size={14} />
                      <span>Add Witness Partner</span>
                      {friendRequests.incoming?.length > 0 && (
                        <span className="flex h-4 min-w-[16px] items-center justify-center rounded bg-black text-[9px] font-black text-white px-1">
                          {friendRequests.incoming.length}
                        </span>
                      )}
                    </button>
                    <button
                      type="button"
                      onClick={() => setShowExploreModal(true)}
                      className="flex items-center gap-1.5 rounded border border-[#2b3542] bg-[#161c23] px-4 py-2 text-xs font-semibold text-neutral-200 hover:border-[#3d4c5e] hover:text-white transition"
                    >
                      <Users size={14} />
                      <span>Explore Communities</span>
                    </button>
                  </div>

                  {/* Telemetry footer */}
                  <div className="mt-10 flex items-center gap-2 text-[10px] font-mono text-neutral-500 uppercase tracking-wider">
                    <ShieldCheck size={12} className="text-neutral-500" />
                    <span>END-TO-END PEER VERIFICATION • DIRECT LIVE SYNC</span>
                  </div>
                </div>
              )}
            </div>
          </div>

        {/* ============================================================
            WHATSAPP-STYLE ATTACHMENT MODAL (No goal required)
        ============================================================ */}
        {showAttachModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-md rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl max-h-[85vh] overflow-y-auto">
              <div className="flex items-center justify-between border-b border-[#232c36] pb-3">
                <div className="flex items-center gap-2.5">
                  <div className="flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                    <Paperclip size={15} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white tracking-tight">TRANSMIT ATTACHMENT</h3>
                    <p className="text-[10px] font-mono text-neutral-400">
                      {attachTargetType === "friend"
                        ? `WITNESS: ${activeFriend?.name || "PARTNER"}`
                        : `CHANNEL: ${selectedCommunity?.name || "COMMUNITY"}`}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setShowAttachModal(false)}
                  className="rounded p-1 text-neutral-400 hover:text-white hover:bg-[#1a212a] transition"
                >
                  <X size={16} />
                </button>
              </div>

              <form onSubmit={handleSendAttachment} className="mt-4 space-y-4">
                {/* Photo / File Picker */}
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label className="text-[11px] font-mono font-bold uppercase text-neutral-300 flex items-center gap-1.5">
                      <ImageIcon size={13} className="text-[#ff7843]" />
                      <span>EVIDENCE & TELEMETRY FILES</span>
                    </label>
                    <span className="text-[10px] font-mono text-neutral-500">
                      {attachFiles.length} selected
                    </span>
                  </div>

                  <label className="flex cursor-pointer flex-col items-center justify-center rounded border border-dashed border-[#2b3644] bg-[#0a0d10] p-4 text-center hover:border-[#ff7843]/50 transition">
                    <Paperclip size={18} className="text-neutral-400 mb-1" />
                    <span className="text-xs font-semibold text-neutral-200">
                      Select proof images or document files
                    </span>
                    <span className="text-[10px] font-mono text-neutral-500 mt-0.5">
                      PNG, JPG, PDF, TXT
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
                          className="relative aspect-square overflow-hidden rounded border border-[#26313d] bg-[#0c0f13] flex items-center justify-center p-1"
                        >
                          {item.isImage ? (
                            <img
                              src={item.previewUrl}
                              alt="Preview"
                              className="h-full w-full object-cover rounded"
                            />
                          ) : (
                            <div className="flex flex-col items-center justify-center text-center p-1">
                              <FileText size={18} className="text-neutral-300 mb-1" />
                              <span className="text-[9px] font-mono text-neutral-300 truncate max-w-full">
                                {item.name}
                              </span>
                            </div>
                          )}
                          <button
                            type="button"
                            onClick={() => removeAttachFile(idx)}
                            className="absolute top-1 right-1 rounded bg-black/80 p-0.5 text-white hover:bg-black"
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
                  <label className="text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5 flex items-center gap-1.5">
                    <LinkIcon size={13} className="text-neutral-400" />
                    <span>EXTERNAL EVIDENCE LINK</span>
                  </label>
                  <input
                    type="url"
                    placeholder="https://github.com/... or proof url"
                    value={attachLink}
                    onChange={(e) => setAttachLink(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                  />
                </div>

                {/* Caption */}
                <div>
                  <label className="text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5 block">
                    TRANSMISSION NOTE (OPTIONAL)
                  </label>
                  <textarea
                    rows={2}
                    placeholder="Add notes for your witness..."
                    value={attachCaption}
                    onChange={(e) => setAttachCaption(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none resize-none"
                  />
                </div>

                {/* Send */}
                <button
                  type="submit"
                  disabled={attachSending || (attachFiles.length === 0 && !attachCaption.trim() && !attachLink.trim())}
                  className="flex w-full items-center justify-center gap-2 rounded border border-[#ff7843]/40 bg-[#ff7843] py-2.5 text-xs font-bold text-black hover:bg-[#ff956b] disabled:opacity-40 transition shadow-md"
                >
                  {attachSending ? (
                    <>
                      <Loader2 size={13} className="animate-spin" />
                      <span>Transmitting...</span>
                    </>
                  ) : (
                    <>
                      <Send size={13} />
                      <span>Transmit Proof to Chat</span>
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
            <div className="w-full max-w-lg rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl max-h-[90vh] overflow-y-auto">
              <div className="flex items-center justify-between border-b border-[#232c36] pb-3">
                <div className="flex items-center gap-2">
                  <span className="instrument-label"><i /> CHANNEL INIT</span>
                  <h3 className="text-sm font-bold text-white tracking-tight">
                    Create Community Hub
                  </h3>
                </div>
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="rounded p-1 text-neutral-400 hover:text-white hover:bg-[#1a212a] transition"
                >
                  <X size={16} />
                </button>
              </div>

              {createError && (
                <div className="mt-3 rounded border border-rose-500/30 bg-rose-500/10 p-2.5 text-xs font-mono text-rose-300">
                  {createError}
                </div>
              )}

              <form onSubmit={handleCreateCommunitySubmit} className="mt-4 space-y-4">
                <div>
                  <label className="block text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5">
                    COMMUNITY IDENTIFIER / NAME
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. 5AM Founders Club"
                    value={newCommName}
                    onChange={(e) => setNewCommName(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                    required
                  />
                </div>

                <div>
                  <label className="block text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5">
                    CLUSTER CATEGORY
                  </label>
                  <select
                    value={newCommCategory}
                    onChange={(e) => setNewCommCategory(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white focus:border-[#ff7843] focus:outline-none"
                  >
                    <option value="General Focus">General Focus</option>
                    <option value="Coding & Tech">Coding & Tech</option>
                    <option value="Health & Fitness">Health & Fitness</option>
                    <option value="Academic Study">Academic Study</option>
                    <option value="Daily Routine">Daily Routine</option>
                  </select>
                </div>

                <div>
                  <label className="block text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5">
                    VERIFICATION PROTOCOL & GUIDELINES
                  </label>
                  <textarea
                    rows={3}
                    placeholder="Explain the mission and evidence standards required to unlock goals..."
                    value={newCommDesc}
                    onChange={(e) => setNewCommDesc(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                    required
                  />
                </div>

                <div>
                  <label className="block text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5">
                    REQUIRED WITNESS APPROVALS FOR RELEASE
                  </label>
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={newCommApprovals}
                    onChange={(e) => setNewCommApprovals(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white focus:border-[#ff7843] focus:outline-none"
                  />
                </div>

                {/* Add Friends to Community Checkbox Section */}
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label className="text-[11px] font-mono font-bold uppercase text-neutral-300">
                      INVITE REGISTERED WITNESSES
                    </label>
                    <span className="text-[10px] font-mono text-neutral-500">
                      {selectedFriendsForCommunity.length} selected
                    </span>
                  </div>

                  {friends.length === 0 ? (
                    <div className="rounded border border-[#26313d] bg-[#0a0d10] p-3 text-xs font-mono text-neutral-400">
                      No witnesses connected yet. You can share your channel code after creation.
                    </div>
                  ) : (
                    <div className="max-h-40 overflow-y-auto rounded border border-[#26313d] bg-[#0a0d10] p-1.5 space-y-1">
                      {friends.map((f) => {
                        const isChecked = selectedFriendsForCommunity.includes(f.id);
                        return (
                          <label
                            key={f.id}
                            className={`flex cursor-pointer items-center justify-between rounded p-2 transition ${
                              isChecked ? "bg-[#1f2732] border border-[#3b4756]" : "border border-transparent hover:bg-[#141920]"
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
                                className="rounded border-[#3b4756] text-[#ff7843] focus:ring-0"
                              />
                              <div>
                                <p className="text-xs font-semibold text-white">{f.name}</p>
                                {f.handle && (
                                  <p className="font-mono text-[10px] text-neutral-400">
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
                    className="w-full rounded border border-[#ff7843]/40 bg-[#ff7843] py-2.5 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-md"
                  >
                    Initialize Channel & Launch Chat
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}

        {/* ============================================================
            EXPLORE COMMUNITIES MODAL (WhatsApp Discovery & Trending)
        ============================================================ */}
        {showExploreModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-2xl rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl max-h-[85vh] flex flex-col">
              {/* Header */}
              <div className="shrink-0 flex items-center justify-between border-b border-[#232c36] pb-3">
                <div className="flex items-center gap-2.5">
                  <div className="flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                    <Compass size={16} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white tracking-tight">COMMUNITY DISCOVERY</h3>
                    <p className="text-[10px] font-mono text-neutral-400">
                      JOIN PEER ACCOUNTABILITY CLUSTERS OR ENTER INVITE TOKEN
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setShowExploreModal(false)}
                  className="rounded p-1 text-neutral-400 hover:text-white hover:bg-[#1a212a] transition"
                >
                  <X size={16} />
                </button>
              </div>

              {/* Private Code Quick Join bar */}
              <form onSubmit={handleJoinByCode} className="shrink-0 mt-3.5 flex gap-2">
                <input
                  type="text"
                  placeholder="ENTER ACCESS CODE (E.G. FOCUS5)"
                  value={privateCode}
                  onChange={(e) => setPrivateCode(e.target.value.toUpperCase())}
                  className="flex-1 rounded border border-[#26313d] bg-[#0a0d10] px-3.5 py-1.5 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                />
                <button
                  type="submit"
                  disabled={!privateCode.trim()}
                  className="rounded border border-[#ff7843]/40 bg-[#ff7843] px-3.5 py-1.5 text-xs font-bold text-black hover:bg-[#ff956b] transition disabled:opacity-30"
                >
                  Join Code
                </button>
              </form>
              {codeNotice.error && (
                <p className="mt-1 text-xs font-mono text-rose-400">{codeNotice.error}</p>
              )}
              {codeNotice.success && (
                <p className="mt-1 text-xs font-mono text-emerald-400">{codeNotice.success}</p>
              )}

              {/* Category Pills & Search */}
              <div className="shrink-0 mt-3 space-y-2">
                <div className="relative">
                  <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-neutral-500" />
                  <input
                    type="text"
                    placeholder="Filter communities by keyword or topic..."
                    value={exploreSearch}
                    onChange={(e) => setExploreSearch(e.target.value)}
                    className="w-full rounded border border-[#26313d] bg-[#0a0d10] py-1.5 pl-8 pr-3 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                  />
                </div>

                <div className="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
                  {["All", "Coding & Tech", "Health & Fitness", "Academic Study", "Daily Routine", "General Focus"].map((cat) => {
                    const isSel = exploreCategory === cat;
                    return (
                      <button
                        key={cat}
                        type="button"
                        onClick={() => setExploreCategory(cat)}
                        className={`shrink-0 rounded px-2.5 py-1 text-[11px] font-mono font-semibold transition ${
                          isSel
                            ? "bg-[#1f2732] text-white border border-[#3b4756] shadow-sm font-bold"
                            : "bg-[#141920] text-neutral-400 hover:text-white hover:bg-[#1a212a] border border-[#222933]"
                        }`}
                      >
                        {cat}
                      </button>
                    );
                  })}
                </div>
              </div>

              {/* Communities List */}
              <div className="flex-1 min-h-0 mt-3.5 overflow-y-auto no-scrollbar space-y-2 pr-1">
                {(() => {
                  const items = (trendingCommunities.length > 0 ? trendingCommunities : communities).filter((comm) => {
                    const matchCat = exploreCategory === "All" || comm.category === exploreCategory;
                    const matchSearch =
                      !exploreSearch.trim() ||
                      comm.name.toLowerCase().includes(exploreSearch.toLowerCase()) ||
                      (comm.description && comm.description.toLowerCase().includes(exploreSearch.toLowerCase()));
                    return matchCat && matchSearch;
                  });

                  if (items.length === 0) {
                    return (
                      <div className="py-12 text-center">
                        <Users size={28} className="mx-auto text-neutral-600 mb-2" />
                        <p className="text-xs font-semibold text-neutral-300">No clusters match criteria</p>
                        <p className="text-[11px] font-mono text-neutral-500 mt-1">Try another category or search filter</p>
                      </div>
                    );
                  }

                  return items.map((comm) => {
                    const isAlreadyJoined = communities.some((c) => c.id === comm.id && c.joined);
                    return (
                      <div
                        key={comm.id}
                        className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded border border-[#26313d] bg-[#0f1318] p-3 hover:border-[#384656] transition"
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <h4 className="text-xs font-bold text-white truncate">{comm.name}</h4>
                            <span className="rounded border border-[#2d3a49] bg-[#161d26] px-1.5 py-0.2 text-[9px] font-mono font-bold text-[#ff7843] uppercase">
                              {comm.category}
                            </span>
                          </div>
                          <p className="text-xs text-neutral-400 mt-0.5 line-clamp-2">
                            {comm.description || "Accountability community"}
                          </p>
                          <div className="flex items-center gap-3 mt-1.5 text-[10px] font-mono text-neutral-500 uppercase">
                            <span>{comm.members || comm.membersCount || 1} MEMBERS</span>
                            <span>•</span>
                            <span>{comm.requiredApprovals || 2} WITNESS APPROVALS</span>
                          </div>
                        </div>

                        <div className="shrink-0 flex items-center gap-2">
                          {isAlreadyJoined ? (
                            <button
                              type="button"
                              onClick={() => {
                                const found = communities.find((c) => c.id === comm.id);
                                if (found) {
                                  setActiveFriendId(null);
                                  setSelectedCommunity(found);
                                  setShowExploreModal(false);
                                }
                              }}
                              className="rounded border border-[#2b3542] bg-[#161c23] px-3 py-1.5 text-xs font-mono font-semibold text-neutral-200 hover:text-white hover:border-[#3d4c5e] transition"
                            >
                              Open Chat
                            </button>
                          ) : (
                            <button
                              type="button"
                              onClick={() => handleJoinTrendingCommunity(comm)}
                              className="rounded border border-[#ff7843]/40 bg-[#ff7843] px-3.5 py-1 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-sm"
                            >
                              Join Cluster
                            </button>
                          )}
                        </div>
                      </div>
                    );
                  });
                })()}
              </div>

              {/* Footer: Create Community prompt */}
              <div className="shrink-0 mt-3 pt-3 border-t border-[#232c36] flex items-center justify-between">
                <span className="text-xs font-mono text-neutral-400">Need a specialized verification group?</span>
                <button
                  type="button"
                  onClick={() => {
                    setShowExploreModal(false);
                    setShowCreateModal(true);
                  }}
                  className="rounded border border-[#2b3542] bg-[#161c23] px-3 py-1.5 text-xs font-semibold text-neutral-200 hover:border-[#3d4c5e] hover:text-white transition"
                >
                  + Create Custom Cluster
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ============================================================
            ADD FRIEND & REQUESTS MODAL
        ============================================================ */}
        {showAddFriendModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-md rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl flex flex-col max-h-[90vh]">
              {/* Header */}
              <div className="shrink-0 flex items-center justify-between border-b border-[#232c36] pb-3">
                <div className="flex items-center gap-2.5">
                  <div className="flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                    <UserPlus size={15} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white tracking-tight">WITNESS PAIRING</h3>
                    <p className="text-[10px] font-mono text-neutral-400">CONNECT DIRECT ACCOUNTABILITY PARTNERS</p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setShowAddFriendModal(false);
                    setFriendError("");
                    setFriendSuccess("");
                  }}
                  className="rounded p-1 text-neutral-400 hover:text-white hover:bg-[#1a212a] transition"
                >
                  <X size={16} />
                </button>
              </div>

              {/* Tab Switcher: Add Friend vs Pending Requests */}
              <div className="shrink-0 flex border-b border-[#232c36] mt-3">
                <button
                  type="button"
                  onClick={() => setFriendModalTab("add")}
                  className={`flex-1 py-2 text-xs font-mono font-semibold text-center border-b-2 transition ${
                    friendModalTab === "add"
                      ? "border-[#ff7843] text-white font-bold"
                      : "border-transparent text-neutral-400 hover:text-white"
                  }`}
                >
                  CONNECT WITNESS
                </button>
                <button
                  type="button"
                  onClick={() => setFriendModalTab("requests")}
                  className={`flex-1 py-2 text-xs font-mono font-semibold text-center border-b-2 flex items-center justify-center gap-1.5 transition ${
                    friendModalTab === "requests"
                      ? "border-[#ff7843] text-white font-bold"
                      : "border-transparent text-neutral-400 hover:text-white"
                  }`}
                >
                  <span>PENDING REQUESTS</span>
                  {friendRequests.incoming?.length > 0 ? (
                    <span className="flex h-4 min-w-[16px] items-center justify-center rounded bg-[#ff7843] px-1 text-[9px] font-mono font-bold text-black">
                      {friendRequests.incoming.length}
                    </span>
                  ) : friendRequests.outgoing?.length > 0 ? (
                    <span className="flex h-4 min-w-[16px] items-center justify-center rounded bg-[#273240] px-1 text-[9px] font-mono font-bold text-neutral-300">
                      {friendRequests.outgoing.length}
                    </span>
                  ) : null}
                </button>
              </div>

              {/* Status alerts */}
              {friendError && (
                <div className="mt-3 rounded border border-rose-500/30 bg-rose-500/10 p-2.5 text-xs font-mono text-rose-300">
                  {friendError}
                </div>
              )}
              {friendSuccess && (
                <div className="mt-3 rounded border border-emerald-500/30 bg-emerald-500/10 p-2.5 text-xs font-mono text-emerald-300">
                  {friendSuccess}
                </div>
              )}

              {/* TAB 1: SEND FRIEND REQUEST */}
              {friendModalTab === "add" && (
                <form onSubmit={handleSendFriendRequest} className="mt-4 space-y-4 overflow-y-auto pr-1">
                  <div>
                    <label className="block text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5">
                      WITNESS ONUSLY HANDLE (@TAG)
                    </label>
                    <div className="flex gap-2">
                      <div className="relative flex-1">
                        <AtSign
                          size={13}
                          className="absolute left-3 top-1/2 -translate-y-1/2 text-neutral-500"
                        />
                        <input
                          type="text"
                          placeholder="e.g. himanshu or @robert"
                          value={newFriendHandle}
                          onChange={(e) => {
                            setNewFriendHandle(e.target.value);
                            setVerifiedPartner(null);
                          }}
                          className="w-full rounded border border-[#26313d] bg-[#0a0d10] py-1.5 pl-8 pr-3 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={handleVerifyTag}
                        disabled={verifyingHandle || !newFriendHandle.trim()}
                        className="rounded border border-[#2b3542] bg-[#161c23] px-3 py-1.5 text-xs font-mono font-semibold text-neutral-200 hover:border-[#3d4c5e] hover:text-white disabled:opacity-40 transition"
                      >
                        {verifyingHandle ? <Loader2 size={12} className="animate-spin" /> : "Verify"}
                      </button>
                    </div>
                  </div>

                  {verifiedPartner && (
                    <div className="flex items-center gap-3 rounded border border-[#384656] bg-[#161d25] p-2.5">
                      <div className="h-8 w-8 rounded border border-[#2b3542] overflow-hidden bg-[#0c0f13] flex items-center justify-center text-xs font-mono font-bold text-white">
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
                        <p className="text-xs font-bold text-white truncate">{verifiedPartner.name}</p>
                        <p className="font-mono text-[10px] text-neutral-400">@{verifiedPartner.handle}</p>
                      </div>
                      <Check size={14} className="text-[#10b981]" />
                    </div>
                  )}

                  <div className="relative flex items-center justify-center py-1">
                    <div className="absolute inset-0 flex items-center">
                      <div className="w-full border-t border-[#232c36]" />
                    </div>
                    <span className="relative bg-[#12161c] px-2 text-[9px] font-mono uppercase font-bold text-neutral-500">
                      OR DIRECT BY EMAIL
                    </span>
                  </div>

                  <div>
                    <label className="block text-[11px] font-mono font-bold uppercase text-neutral-300 mb-1.5">
                      WITNESS REGISTERED EMAIL
                    </label>
                    <input
                      type="email"
                      placeholder="e.g. partner@gmail.com"
                      value={newFriendEmail}
                      onChange={(e) => setNewFriendEmail(e.target.value)}
                      className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-2 text-xs font-mono text-white placeholder-neutral-500 focus:border-[#ff7843] focus:outline-none"
                    />
                  </div>

                  <div className="pt-2">
                    <button
                      type="submit"
                      disabled={sendingRequest || (!newFriendHandle.trim() && !newFriendEmail.trim())}
                      className="w-full rounded border border-[#ff7843]/40 bg-[#ff7843] py-2.5 text-xs font-bold text-black hover:bg-[#ff956b] disabled:opacity-40 transition flex items-center justify-center gap-2 shadow-md"
                    >
                      {sendingRequest ? (
                        <>
                          <Loader2 size={13} className="animate-spin" />
                          <span>Dispatching Pairing Request...</span>
                        </>
                      ) : (
                        <>
                          <Send size={13} />
                          <span>Send Witness Pairing Request</span>
                        </>
                      )}
                    </button>
                  </div>
                </form>
              )}

              {/* TAB 2: INCOMING & OUTGOING REQUESTS */}
              {friendModalTab === "requests" && (
                <div className="mt-4 space-y-4 overflow-y-auto pr-1">
                  {/* Incoming Requests */}
                  <div>
                    <h4 className="text-[10px] font-mono font-bold uppercase tracking-wider text-neutral-400 mb-2">
                      INCOMING REQUESTS ({friendRequests.incoming?.length || 0})
                    </h4>
                    {(!friendRequests.incoming || friendRequests.incoming.length === 0) ? (
                      <div className="rounded border border-[#232c36] bg-[#0a0d10] p-4 text-center">
                        <UserCheck size={22} className="mx-auto text-neutral-600 mb-1" />
                        <p className="text-xs font-mono text-neutral-400">No pending witness requests</p>
                      </div>
                    ) : (
                      <div className="space-y-2">
                        {friendRequests.incoming.map((req) => (
                          <div
                            key={req.id}
                            className="flex items-center justify-between gap-3 rounded border border-[#26313d] bg-[#0f1318] p-2.5"
                          >
                            <div className="min-w-0 flex-1">
                              <p className="text-xs font-bold text-white truncate">
                                {req.senderName || `@${req.senderHandle || "friend"}`}
                              </p>
                              {req.senderHandle && (
                                <p className="font-mono text-[10px] text-neutral-400">
                                  @{req.senderHandle.replace(/^@/, "")}
                                </p>
                              )}
                              <p className="text-[10px] font-mono text-neutral-500 mt-0.5">
                                Sent {req.createdAt ? new Date(req.createdAt).toLocaleDateString() : "recently"}
                              </p>
                            </div>
                            <div className="flex items-center gap-1.5 shrink-0">
                              <button
                                type="button"
                                onClick={() => handleRespondFriendRequest(req.id, "accept", req)}
                                className="rounded border border-[#ff7843]/40 bg-[#ff7843] px-2.5 py-1 text-xs font-bold text-black hover:bg-[#ff956b] shadow-sm transition"
                              >
                                Accept
                              </button>
                              <button
                                type="button"
                                onClick={() => handleRespondFriendRequest(req.id, "decline", req)}
                                className="rounded border border-[#2b3542] bg-[#161c23] px-2.5 py-1 text-xs font-mono text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                              >
                                Decline
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* Outgoing Requests */}
                  {friendRequests.outgoing && friendRequests.outgoing.length > 0 && (
                    <div className="pt-2 border-t border-[#232c36]">
                      <h4 className="text-[10px] font-mono font-bold uppercase tracking-wider text-neutral-400 mb-2">
                        DISPATCHED REQUESTS ({friendRequests.outgoing.length})
                      </h4>
                      <div className="space-y-1.5">
                        {friendRequests.outgoing.map((req) => (
                          <div
                            key={req.id}
                            className="flex items-center justify-between gap-2 rounded border border-[#232c36] bg-[#0a0d10] p-2 text-xs"
                          >
                            <div className="min-w-0 flex-1">
                              <p className="font-semibold text-neutral-200 truncate">
                                {req.recipientName || (req.recipientHandle ? `@${req.recipientHandle.replace(/^@/, '')}` : '') || req.recipientEmail || req.recipientId}
                              </p>
                              {req.recipientEmail && (
                                <p className="font-mono text-[10px] text-neutral-400 truncate">
                                  {req.recipientEmail}
                                </p>
                              )}
                              <span className="text-[10px] font-mono text-neutral-500">
                                {req.createdAt ? new Date(req.createdAt).toLocaleDateString() : "Pending"}
                              </span>
                            </div>
                            <span className="rounded border border-[#26313d] bg-[#12161c] px-2 py-0.5 text-[10px] font-mono text-neutral-400">
                              PENDING
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}

        {/* ============================================================
            CONFIRM DELETE FRIEND MODAL
        ============================================================ */}
        {friendToDelete && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-sm rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl">
              <h3 className="text-sm font-bold text-white tracking-tight">TERMINATE WITNESS PAIRING</h3>
              <p className="mt-2 text-xs font-mono text-neutral-400 leading-relaxed">
                Sever witness connection with <strong className="text-white">{friendToDelete.name}</strong>? Local cached telemetry and chat records will be purged.
              </p>
              <div className="mt-4 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setFriendToDelete(null)}
                  className="rounded border border-[#2b3542] bg-[#161c23] px-3.5 py-1.5 text-xs font-mono text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmDeleteFriend}
                  className="rounded border border-rose-500/40 bg-rose-500/20 px-3.5 py-1.5 text-xs font-mono font-bold text-rose-300 hover:bg-rose-500/30 transition shadow-sm"
                >
                  Terminate Pairing
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
            <div className="w-full max-w-sm rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl">
              <h3 className="text-sm font-bold text-white tracking-tight">DISCONNECT FROM CHANNEL</h3>
              <p className="mt-2 text-xs font-mono text-neutral-400 leading-relaxed">
                Disconnect from <strong className="text-white">{commToLeave.name}</strong>? You will cease receiving group proof broadcasts.
              </p>
              <div className="mt-4 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setCommToLeave(null)}
                  className="rounded border border-[#2b3542] bg-[#161c23] px-3.5 py-1.5 text-xs font-mono text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmLeaveCommunity}
                  className="rounded border border-[#ff7843]/40 bg-[#ff7843] px-3.5 py-1.5 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-sm"
                >
                  Disconnect
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
            <div className="w-full max-w-sm rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl">
              <h3 className="text-sm font-bold text-white tracking-tight">PERMANENTLY PURGE CHANNEL</h3>
              <p className="mt-2 text-xs font-mono text-neutral-400 leading-relaxed">
                Permanently purge <strong className="text-white">{commToDelete.name}</strong> and all broadcast logs? This action is irrevocable.
              </p>
              <div className="mt-4 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setCommToDelete(null)}
                  className="rounded border border-[#2b3542] bg-[#161c23] px-3.5 py-1.5 text-xs font-mono text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmDeleteCommunity}
                  className="rounded border border-rose-500/40 bg-rose-500/20 px-3.5 py-1.5 text-xs font-mono font-bold text-rose-300 hover:bg-rose-500/30 transition shadow-sm"
                >
                  Purge Channel
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
            <div className="w-full max-w-sm rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl space-y-3.5">
              <div className="flex items-center gap-2.5">
                <div className="flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-rose-400">
                  <Trash2 size={15} />
                </div>
                <div>
                  <h3 className="text-sm font-bold text-white tracking-tight">DELETE TRANSMISSION</h3>
                  <p className="text-[10px] font-mono text-neutral-400">
                    {deleteMsgModal.isMe ? "CHOOSE DELETION SCOPE" : "PURGE FROM LOCAL BUFFER"}
                  </p>
                </div>
              </div>

              {deleteMsgModal.msg?.text && (
                <div className="rounded border border-[#26313d] bg-[#0a0d10] p-2.5 text-xs font-mono text-neutral-300 italic line-clamp-2">
                  "{deleteMsgModal.msg.text}"
                </div>
              )}

              <div className="pt-2 flex flex-col gap-2">
                {deleteMsgModal.isMe && (
                  <button
                    type="button"
                    onClick={handleConfirmDeleteForEveryone}
                    className="w-full rounded border border-rose-500/40 bg-rose-500/20 px-4 py-2 text-xs font-mono font-bold text-rose-300 hover:bg-rose-500/30 transition shadow-sm"
                  >
                    Delete for everyone
                  </button>
                )}
                <button
                  type="button"
                  onClick={handleConfirmDeleteForMe}
                  className={`w-full rounded px-4 py-2 text-xs font-mono font-semibold transition ${
                    deleteMsgModal.isMe
                      ? "border border-[#2b3542] bg-[#161c23] text-neutral-300 hover:text-white hover:border-[#3d4c5e]"
                      : "border border-rose-500/40 bg-rose-500/20 text-rose-300 hover:bg-rose-500/30"
                  }`}
                >
                  Delete for me
                </button>
                <button
                  type="button"
                  onClick={() => setDeleteMsgModal({ isOpen: false, msg: null, isMe: false, type: "friend" })}
                  className="w-full rounded border border-transparent px-4 py-1.5 text-xs font-mono text-neutral-500 hover:text-neutral-300 transition"
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
            <div className="w-full max-w-sm rounded-lg border border-[#2b3542] bg-[#12161c] p-5 shadow-2xl space-y-3.5">
              <div className="flex items-center gap-2.5">
                <div className="flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                  <RotateCcw size={15} />
                </div>
                <div>
                  <h3 className="text-sm font-bold text-white tracking-tight">CLEAR CHANNEL BUFFER</h3>
                  <p className="text-[10px] font-mono text-neutral-400">
                    {clearChatModal.targetName ? `TARGET: ${clearChatModal.targetName}` : "CURRENT CONVERSATION"}
                  </p>
                </div>
              </div>

              <p className="text-xs font-mono text-neutral-400 leading-relaxed">
                Purge all messages and media records from your local chat buffer?
              </p>

              <div className="pt-2 flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setClearChatModal({ isOpen: false, type: "friend", targetName: "" })}
                  className="rounded border border-[#2b3542] bg-[#161c23] px-3.5 py-1.5 text-xs font-mono text-neutral-300 hover:text-white hover:border-[#3d4c5e] transition"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleConfirmClearChat}
                  className="rounded border border-[#ff7843]/40 bg-[#ff7843] px-3.5 py-1.5 text-xs font-bold text-black hover:bg-[#ff956b] transition shadow-sm"
                >
                  Clear Buffer
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
