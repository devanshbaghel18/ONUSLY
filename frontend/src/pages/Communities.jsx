import { useState, useMemo, useEffect, useRef } from "react";
import { Link } from "react-router-dom";
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
  Lock,
  Target,
  CheckCheck,
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
} from "../lib/communities";
import {
  getStoredFriends,
  addFriend,
  deleteFriend,
  getChatMessages,
  sendChatMessage,
  receiveChatMessage,
  deleteChatMessage,
  deleteChatMessageGlobally,
  mergeChatHistory,
} from "../lib/friendsChat";
import { getChatHistory, lookupUser, getGoals, submitProof } from "../lib/api";
import { getUser } from "../lib/auth";
import { useWebSocket } from "../hooks/useWebSocket";

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
  // Main mode: 'communities' | 'friends'
  const [mainView, setMainView] = useState("communities");

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

  // Confirm delete modals/prompts
  const [friendToDelete, setFriendToDelete] = useState(null);
  const [commToDelete, setCommToDelete] = useState(null);
  const [commToLeave, setCommToLeave] = useState(null);

  // Lightbox Preview State
  const [lightboxImage, setLightboxImage] = useState(null);

  // ==========================================
  // WHATSAPP "+" PROOF SUBMISSION MODAL STATE
  // ==========================================
  const [showProofModal, setShowProofModal] = useState(false);
  const [proofTargetType, setProofTargetType] = useState("friend"); // 'friend' | 'community'
  const [userGoals, setUserGoals] = useState([]);
  const [selectedGoalId, setSelectedGoalId] = useState("");
  const [proofExplanation, setProofExplanation] = useState("");
  const [proofExternalLink, setProofExternalLink] = useState("");
  const [proofFiles, setProofFiles] = useState([]); // Array of { file, previewUrl, name }
  const [submittingProof, setSubmittingProof] = useState(false);
  const [proofError, setProofError] = useState("");
  const [proofSuccess, setProofSuccess] = useState("");

  // Refs for auto-scroll
  const chatEndRef = useRef(null);
  const commChatEndRef = useRef(null);

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
        images: p.images,
        externalLink: p.externalLink,
      });

      const updatedFriends = getStoredFriends();
      setFriends(updatedFriends);

      const currentActiveId = activeFriendIdRef.current;
      const currentActive = updatedFriends.find((f) => f.id === currentActiveId);
      const isViewing =
        currentActiveId === friend?.id ||
        (currentActive?.handle && friend?.handle && currentActive.handle.toLowerCase().replace(/[@\s]/g, "") === friend.handle.toLowerCase().replace(/[@\s]/g, "")) ||
        (currentActive?.email && friend?.email && currentActive.email.toLowerCase() === friend.email.toLowerCase()) ||
        !currentActiveId;

      if (friend && isViewing) {
        if (!currentActiveId) setActiveFriendId(friend.id);
        setChatMessages(messages);
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
          images: p.images,
          externalLink: p.externalLink,
          time: p.time,
        });
        setCommunities(getStoredCommunities());
        if (selectedCommunityIdRef.current === p.communityId) {
          setCommChatMessages(updated);
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
        send({
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
        send({
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
    setCommChatMessages(getCommunityMessages(selectedCommunity.id));

    return () => {
      send({
        type: "community.leave_room",
        payload: { communityId: selectedCommunity.id },
      });
    };
  }, [selectedCommunity?.id, selectedCommunity?.joined, isConnected, send]);

  // Auto-scroll 1-on-1 chat
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [chatMessages]);

  // Auto-scroll community chat
  useEffect(() => {
    commChatEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [commChatMessages]);

  // Load goals when proof modal is opened
  const openProofModal = (targetType) => {
    setProofTargetType(targetType);
    setProofError("");
    setProofSuccess("");
    setProofFiles([]);
    setProofExplanation("");
    setProofExternalLink("");
    setShowProofModal(true);

    getGoals()
      .then((data) => {
        const active = (data || []).filter(
          (g) => g.status === "active" || g.status === "proof_submitted"
        );
        setUserGoals(active);
        if (active.length > 0) {
          setSelectedGoalId(active[0].id);
        }
      })
      .catch((err) => {
        console.error("Failed to load goals for proof:", err);
      });
  };

  const handleSelectFiles = (e) => {
    const files = Array.from(e.target.files || []);
    if (!files.length) return;

    files.forEach((file) => {
      const reader = new FileReader();
      reader.onload = () => {
        setProofFiles((prev) => [
          ...prev,
          {
            file,
            name: file.name,
            previewUrl: reader.result,
          },
        ]);
      };
      reader.readAsDataURL(file);
    });
  };

  const removeProofFile = (index) => {
    setProofFiles((prev) => prev.filter((_, idx) => idx !== index));
  };

  // Submit proof via API and share into chat
  const handleShareProofSubmit = async (e) => {
    e.preventDefault();
    setProofError("");
    setProofSuccess("");

    if (!selectedGoalId) {
      setProofError("Please select a goal to submit proof for.");
      return;
    }

    if (proofFiles.length === 0 && !proofExplanation.trim() && !proofExternalLink.trim()) {
      setProofError("Please add photos, an explanation, or a verification link.");
      return;
    }

    const currentGoal = userGoals.find((g) => g.id === selectedGoalId);
    const goalTitle = currentGoal?.title || "Accountability Goal";

    setSubmittingProof(true);
    try {
      // 1. Submit proof to backend API
      const summaryText = [
        proofExplanation.trim(),
        proofFiles.length > 0 ? `[Photos Attached: ${proofFiles.map((f) => f.name).join(", ")}]` : "",
      ]
        .filter(Boolean)
        .join(" | ");

      const effectiveProofType = proofFiles.length > 0 ? "photo" : (proofExternalLink.trim() ? "link" : "text");
      const effectivePhotoUrl = proofFiles.length > 0 ? (proofFiles[0]?.name || "screenshot.png") : "";

      await submitProof(selectedGoalId, {
        proofType: effectiveProofType,
        textExplanation: summaryText || "Goal evidence submitted.",
        externalLink: proofExternalLink.trim(),
        photoUrl: effectivePhotoUrl,
      });

      const imageUrls = proofFiles.map((f) => f.previewUrl);
      const senderEmail = currentUser?.email || "";
      const senderHandle = currentUser?.handle || "";
      const senderName = currentUser?.name || (senderHandle ? `@${senderHandle}` : "You");

      if (proofTargetType === "friend" && activeFriendId && activeFriend) {
        // Share in 1-on-1 friend chat
        send({
          type: "chat.message",
          payload: {
            recipientEmail: activeFriend.email || "",
            recipientHandle: activeFriend.handle || "",
            senderEmail,
            senderHandle,
            text: proofExplanation.trim(),
            isProof: true,
            goalId: selectedGoalId,
            goalTitle,
            images: imageUrls,
            externalLink: proofExternalLink.trim(),
          },
        });

        const updated = sendChatMessage(
          activeFriendId,
          {
            text: proofExplanation.trim(),
            isProof: true,
            goalId: selectedGoalId,
            goalTitle,
            images: imageUrls,
            externalLink: proofExternalLink.trim(),
          },
          "me"
        );
        setChatMessages(updated);
        setFriends(getStoredFriends());
      } else if (proofTargetType === "community" && selectedCommunity) {
        // Share in Community group chat
        const commMsgId = `comm-msg-${Date.now()}-${Math.random().toString(36).substr(2, 6)}`;

        send({
          type: "community.message",
          payload: {
            id: commMsgId,
            communityId: selectedCommunity.id,
            communityName: selectedCommunity.name || "Community",
            communityCode: selectedCommunity.code || "",
            senderName,
            senderEmail,
            senderHandle,
            text: proofExplanation.trim(),
            isProof: true,
            goalId: selectedGoalId,
            goalTitle,
            images: imageUrls,
            externalLink: proofExternalLink.trim(),
          },
        });

        const updated = sendCommunityMessage(selectedCommunity.id, {
          id: commMsgId,
          senderName: "You",
          senderHandle,
          text: proofExplanation.trim(),
          isProof: true,
          goalId: selectedGoalId,
          goalTitle,
          images: imageUrls,
          externalLink: proofExternalLink.trim(),
        });
        setCommChatMessages(updated);
        setCommunities(getStoredCommunities());
      }

      setProofSuccess("Proof submitted and shared in chat successfully!");
      setTimeout(() => {
        setShowProofModal(false);
      }, 1000);
    } catch (err) {
      console.error("Failed to submit proof:", err);
      setProofError(err.message || "Failed to submit verification proof");
    } finally {
      setSubmittingProof(false);
    }
  };

  // Select a friend in 1-on-1 chat
  const selectFriend = (id) => {
    setActiveFriendId(id);
    setChatMessages(getChatMessages(id));

    const f = friends.find((item) => item.id === id);
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

  // Filtered lists
  const filteredCommunities = useMemo(() => {
    return communities.filter((c) => {
      const q = searchQuery.toLowerCase();
      return c.name.toLowerCase().includes(q) || c.description.toLowerCase().includes(q);
    });
  }, [communities, searchQuery]);

  const filteredFriends = useMemo(() => {
    return friends.filter((f) =>
      f.name.toLowerCase().includes(friendSearch.toLowerCase()) ||
      (f.handle && f.handle.toLowerCase().includes(friendSearch.toLowerCase()))
    );
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

    send({
      type: "chat.message",
      payload: {
        recipientEmail: currentFriend?.email || "",
        recipientHandle: recHandle,
        senderEmail,
        senderHandle,
        senderName,
        text,
      },
    });

    const updatedMessages = sendChatMessage(activeFriendId, text, "me");
    setChatMessages(updatedMessages);
    setMessageInput("");
    setFriends(getStoredFriends());
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

    setCommMessageInput("");

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
      },
    });

    const updated = sendCommunityMessage(selectedCommunity.id, {
      id: msgId,
      senderName: "You",
      senderHandle,
      text,
    });
    setCommChatMessages(updated);
    setCommunities(getStoredCommunities());
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
              Communities and 1-on-1 friend chat with instant goal proof sharing.
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
                        <h2 className="text-lg font-bold text-white">
                          {selectedCommunity.name}
                        </h2>
                        <span className="rounded-full border border-[#444444] px-2 py-0.5 text-[10px] uppercase font-bold text-[#A3A3A3]">
                          {selectedCommunity.category}
                        </span>
                      </div>
                      <p className="text-xs text-[#A3A3A3] mt-0.5">
                        {selectedCommunity.membersCount} Members • {selectedCommunity.requiredApprovals} Member Approvals Required
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

                    {selectedCommunity.joined ? (
                      <>
                        <button
                          type="button"
                          onClick={() => setCommToLeave(selectedCommunity)}
                          className="rounded-xl border border-[#444444] bg-transparent px-3 py-1.5 text-xs font-semibold text-neutral-300 hover:text-white hover:border-white transition"
                        >
                          Leave Community
                        </button>
                        <button
                          type="button"
                          onClick={() => setCommToDelete(selectedCommunity)}
                          className="rounded-xl border border-red-900/50 bg-red-950/20 px-3 py-1.5 text-xs font-semibold text-red-400 hover:bg-red-900/30 transition"
                          title="Delete Community"
                        >
                          <Trash2 size={13} className="inline mr-1" />
                          Delete
                        </button>
                      </>
                    ) : (
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
                  <div className="flex flex-col rounded-2xl border border-[#2B2B2B] bg-[#121212] overflow-hidden min-h-[560px]">
                    {/* Community Description Banner */}
                    <div className="border-b border-[#222222] bg-[#161616] px-5 py-2.5 flex items-center justify-between text-xs text-[#A3A3A3]">
                      <span className="truncate max-w-xl">
                        📌 {selectedCommunity.description}
                      </span>
                      <span className="font-mono text-[11px] text-[#737373]">
                        Code: {selectedCommunity.code}
                      </span>
                    </div>

                    {/* Community Chat Stream */}
                    <div className="flex-1 space-y-3 overflow-y-auto p-4 max-h-[460px]">
                      {commChatMessages.length === 0 ? (
                        <div className="flex h-full flex-col items-center justify-center py-16 text-center">
                          <Users size={36} className="text-[#444444] mb-2" />
                          <p className="text-xs font-semibold text-white">
                            Community Chat Active
                          </p>
                          <p className="mt-1 text-xs text-[#737373] max-w-sm">
                            Coordinate with members here, or click the <strong className="text-white">+</strong> icon below to submit and share your goal proof!
                          </p>
                        </div>
                      ) : (
                        commChatMessages.map((msg) => {
                          const isMe = msg.sender === "me";
                          return (
                            <div
                              key={msg.id}
                              className={`group flex ${isMe ? "justify-end" : "justify-start"}`}
                            >
                              <div
                                className={`relative max-w-[80%] sm:max-w-[70%] rounded-2xl px-4 py-3 text-xs ${
                                  isMe
                                    ? "bg-white text-black font-medium"
                                    : "border border-[#333333] bg-[#1E1E1E] text-white"
                                }`}
                              >
                                {/* Sender badge for peers */}
                                {!isMe && (
                                  <div className="mb-1 flex items-center gap-1.5 font-bold text-[11px] text-emerald-400">
                                    <span>{msg.senderName || "Member"}</span>
                                    {msg.senderHandle && (
                                      <span className="font-mono text-[10px] text-[#A3A3A3]">
                                        @{msg.senderHandle.replace(/^@/, "")}
                                      </span>
                                    )}
                                  </div>
                                )}

                                {/* Proof Evidence Card */}
                                {msg.isProof && (
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
                                        <span>Goal Proof Evidence</span>
                                      </div>
                                      {msg.goalId && (
                                        <Link
                                          to={`/goals/${msg.goalId}`}
                                          className={`flex items-center gap-1 text-[11px] font-bold underline hover:opacity-80 ${
                                            isMe ? "text-black" : "text-white"
                                          }`}
                                        >
                                          <span>Review Goal</span>
                                          <ExternalLink size={11} />
                                        </Link>
                                      )}
                                    </div>
                                    {msg.goalTitle && (
                                      <h4
                                        className={`text-xs font-bold mb-1 ${
                                          isMe ? "text-black" : "text-white"
                                        }`}
                                      >
                                        🎯 {msg.goalTitle}
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
                                        <span className="truncate max-w-[200px]">
                                          {msg.externalLink}
                                        </span>
                                      </a>
                                    )}
                                    {msg.images && msg.images.length > 0 && (
                                      <WhatsAppImageGrid
                                        images={msg.images}
                                        onImageClick={setLightboxImage}
                                      />
                                    )}
                                  </div>
                                )}

                                {/* Normal text */}
                                {!msg.isProof && (
                                  <p className="leading-relaxed">{msg.text}</p>
                                )}

                                <div className="mt-1 flex items-center justify-end gap-2 text-[9px]">
                                  <span
                                    className={isMe ? "text-neutral-500" : "text-[#737373]"}
                                  >
                                    {msg.time}
                                  </span>
                                  {isMe && <CheckCheck size={12} className="text-neutral-500" />}

                                  {/* Delete message icon */}
                                  <button
                                    type="button"
                                    onClick={() => handleDeleteCommMessage(msg.id)}
                                    className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 text-neutral-400 hover:text-red-500"
                                    title="Delete message"
                                  >
                                    <Trash2 size={11} />
                                  </button>
                                </div>
                              </div>
                            </div>
                          );
                        })
                      )}
                      <div ref={commChatEndRef} />
                    </div>

                    {/* Community Chat Input with WhatsApp "+" Proof Icon */}
                    <form
                      onSubmit={handleSendCommMessage}
                      className="flex items-center gap-2 border-t border-[#2A2A2A] bg-[#181818] p-3"
                    >
                      <button
                        type="button"
                        onClick={() => openProofModal("community")}
                        title="Upload & Share Goal Proof"
                        className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-[#3A3A3A] bg-[#222222] text-white hover:border-white hover:bg-[#2C2C2C] transition"
                      >
                        <Plus size={18} />
                      </button>

                      <input
                        type="text"
                        placeholder="Type a message or click + to share goal proof..."
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
                            <h3 className="text-base font-bold text-white">{c.name}</h3>
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
          <div className="grid grid-cols-1 overflow-hidden rounded-2xl border border-[#2B2B2B] bg-[#181818] shadow-2xl md:grid-cols-12 min-h-[580px]">
            {/* Left Column: Friends List */}
            <div className="border-b border-[#2A2A2A] bg-[#141414] p-4 md:col-span-4 md:border-b-0 md:border-r">
              <div className="flex items-center justify-between pb-3">
                <span className="text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
                  Friends List ({friends.length})
                </span>
                <button
                  type="button"
                  onClick={() => setShowAddFriendModal(true)}
                  className="flex items-center gap-1.5 rounded-lg border border-[#444444] bg-[#222222] px-2.5 py-1.5 text-xs font-semibold text-white hover:border-white transition"
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

            {/* Right Column: 1-on-1 Chat Box */}
            <div className="flex flex-col justify-between bg-[#121212] md:col-span-8">
              {activeFriend ? (
                <>
                  {/* Chat Header */}
                  <div className="flex items-center justify-between border-b border-[#2A2A2A] bg-[#181818] p-4">
                    <div className="flex items-center gap-3">
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
                      <button
                        type="button"
                        onClick={() => setFriendToDelete(activeFriend)}
                        className="rounded-xl border border-red-900/40 bg-red-950/20 p-2 text-red-400 hover:bg-red-900/40 transition"
                        title="Delete Friend & Clear Chat"
                      >
                        <Trash2 size={15} />
                      </button>
                    </div>
                  </div>

                  {/* Message Stream */}
                  <div className="flex-1 space-y-3 overflow-y-auto p-4 max-h-[420px]">
                    {chatMessages.length === 0 ? (
                      <div className="flex h-full flex-col items-center justify-center py-16 text-center">
                        <MessageSquare size={32} className="text-[#444444] mb-2" />
                        <p className="text-xs font-medium text-[#A3A3A3]">
                          No messages yet. Send a message or click <strong className="text-white">+</strong> to share goal evidence!
                        </p>
                      </div>
                    ) : (
                      chatMessages.map((msg) => {
                        const isMe = msg.sender === "me";
                        return (
                          <div
                            key={msg.id}
                            className={`group flex ${isMe ? "justify-end" : "justify-start"}`}
                          >
                            <div
                              className={`relative max-w-[80%] sm:max-w-[70%] rounded-2xl px-4 py-3 text-xs ${
                                isMe
                                  ? "bg-white text-black font-medium"
                                  : "border border-[#333333] bg-[#222222] text-white"
                              }`}
                            >
                              {/* Proof Card */}
                              {msg.isProof && (
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
                                      <span>Goal Proof Evidence</span>
                                    </div>
                                    {msg.goalId && (
                                      <Link
                                        to={`/goals/${msg.goalId}`}
                                        className={`flex items-center gap-1 text-[11px] font-bold underline hover:opacity-80 ${
                                          isMe ? "text-black" : "text-white"
                                        }`}
                                      >
                                        <span>Review Goal</span>
                                        <ExternalLink size={11} />
                                      </Link>
                                    )}
                                  </div>
                                  {msg.goalTitle && (
                                    <h4
                                      className={`text-xs font-bold mb-1 ${
                                        isMe ? "text-black" : "text-white"
                                      }`}
                                    >
                                      🎯 {msg.goalTitle}
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
                                      <span className="truncate max-w-[200px]">
                                        {msg.externalLink}
                                      </span>
                                    </a>
                                  )}
                                  {msg.images && msg.images.length > 0 && (
                                    <WhatsAppImageGrid
                                      images={msg.images}
                                      onImageClick={setLightboxImage}
                                    />
                                  )}
                                </div>
                              )}

                              {/* Normal message */}
                              {!msg.isProof && <p className="leading-relaxed">{msg.text}</p>}

                              <div className="mt-1 flex items-center justify-end gap-2 text-[9px]">
                                <span className={isMe ? "text-neutral-500" : "text-[#737373]"}>
                                  {msg.time}
                                </span>
                                {isMe && <CheckCheck size={12} className="text-neutral-500" />}

                                {/* Delete message button */}
                                <button
                                  type="button"
                                  onClick={() => handleDeleteChatMessage(msg.id)}
                                  className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 text-neutral-400 hover:text-red-500"
                                  title="Delete message"
                                >
                                  <Trash2 size={11} />
                                </button>
                              </div>
                            </div>
                          </div>
                        );
                      })
                    )}
                    <div ref={chatEndRef} />
                  </div>

                  {/* Chat Input with WhatsApp "+" Proof Icon */}
                  <form
                    onSubmit={handleSendMessage}
                    className="flex items-center gap-2 border-t border-[#2A2A2A] bg-[#181818] p-3"
                  >
                    <button
                      type="button"
                      onClick={() => openProofModal("friend")}
                      title="Upload & Share Goal Proof"
                      className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-[#3A3A3A] bg-[#222222] text-white hover:border-white hover:bg-[#2C2C2C] transition"
                    >
                      <Plus size={18} />
                    </button>

                    <input
                      type="text"
                      placeholder="Type a message or click + to share goal proof..."
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
            WHATSAPP "+" PROOF SUBMISSION MODAL
        ============================================================ */}
        {showProofModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
            <div className="w-full max-w-lg rounded-2xl border border-[#333333] bg-[#1A1A1A] p-6 shadow-2xl max-h-[90vh] overflow-y-auto">
              <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
                <div className="flex items-center gap-2">
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-white text-black">
                    <ShieldCheck size={18} />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-white">
                      Share Goal Proof
                    </h3>
                    <p className="text-[11px] text-[#A3A3A3]">
                      {proofTargetType === "friend"
                        ? `Sharing directly with ${activeFriend?.name || "Friend"}`
                        : `Sharing with ${selectedCommunity?.name || "Community"}`}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setShowProofModal(false)}
                  className="rounded-lg p-1 text-[#737373] hover:text-white"
                >
                  <X size={18} />
                </button>
              </div>

              {proofError && (
                <div className="mt-4 rounded-xl border border-red-900 bg-red-950/40 p-3 text-xs text-red-300">
                  {proofError}
                </div>
              )}
              {proofSuccess && (
                <div className="mt-4 rounded-xl border border-emerald-900 bg-emerald-950/40 p-3 text-xs font-semibold text-emerald-300">
                  {proofSuccess}
                </div>
              )}

              <form onSubmit={handleShareProofSubmit} className="mt-4 space-y-4">
                {/* 1. Goal Selector */}
                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    1. Select Active Goal
                  </label>
                  {userGoals.length === 0 ? (
                    <div className="rounded-xl border border-[#333333] bg-[#141414] p-3 text-xs text-[#A3A3A3]">
                      No active goals found. Please{" "}
                      <Link to="/goals" className="text-white underline font-semibold">
                        create a goal
                      </Link>{" "}
                      first.
                    </div>
                  ) : (
                    <select
                      value={selectedGoalId}
                      onChange={(e) => setSelectedGoalId(e.target.value)}
                      className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white focus:border-white focus:outline-none"
                    >
                      {userGoals.map((g) => (
                        <option key={g.id} value={g.id}>
                          {g.title} ({g.status === "proof_submitted" ? "Proof Pending" : "Active"})
                        </option>
                      ))}
                    </select>
                  )}
                </div>

                {/* 2. Photo / Document Evidence */}
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label className="text-xs font-bold text-white">
                      2. Attach Photos / Screenshots
                    </label>
                    <span className="text-[11px] text-[#737373]">
                      {proofFiles.length} photo{proofFiles.length !== 1 ? "s" : ""} selected
                    </span>
                  </div>

                  <label className="flex cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed border-[#3A3A3A] bg-[#141414] p-4 text-center hover:border-white transition">
                    <Paperclip size={20} className="text-[#A3A3A3] mb-1" />
                    <span className="text-xs font-semibold text-white">
                      Click to choose multiple photos
                    </span>
                    <span className="text-[10px] text-[#737373] mt-0.5">
                      PNG, JPG, screenshots supported
                    </span>
                    <input
                      type="file"
                      multiple
                      accept="image/*"
                      onChange={handleSelectFiles}
                      className="hidden"
                    />
                  </label>

                  {/* Thumbnail Strip */}
                  {proofFiles.length > 0 && (
                    <div className="mt-3 grid grid-cols-4 gap-2">
                      {proofFiles.map((item, idx) => (
                        <div
                          key={idx}
                          className="relative aspect-square overflow-hidden rounded-lg border border-[#333333] bg-[#111111]"
                        >
                          <img
                            src={item.previewUrl}
                            alt="Preview"
                            className="h-full w-full object-cover"
                          />
                          <button
                            type="button"
                            onClick={() => removeProofFile(idx)}
                            className="absolute top-1 right-1 rounded-full bg-black/80 p-1 text-white hover:bg-black"
                          >
                            <X size={10} />
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* 3. External Link */}
                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    3. Verification Link (Optional)
                  </label>
                  <input
                    type="url"
                    placeholder="https://github.com/pull/... or Loom URL"
                    value={proofExternalLink}
                    onChange={(e) => setProofExternalLink(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                {/* 4. Verification Note */}
                <div>
                  <label className="block text-xs font-bold text-white mb-1.5">
                    4. Verification Note / Milestone Summary
                  </label>
                  <textarea
                    rows={3}
                    placeholder="Describe what you completed, milestone achievements, notes for review..."
                    value={proofExplanation}
                    onChange={(e) => setProofExplanation(e.target.value)}
                    className="w-full rounded-xl border border-[#333333] bg-[#141414] p-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                  />
                </div>

                {/* Submit Action */}
                <div className="pt-2">
                  <button
                    type="submit"
                    disabled={submittingProof || userGoals.length === 0}
                    className="flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-xs font-bold text-black hover:bg-neutral-200 disabled:opacity-40 transition"
                  >
                    {submittingProof ? (
                      <>
                        <Loader2 size={14} className="animate-spin" />
                        Submitting Proof & Sharing in Chat...
                      </>
                    ) : (
                      <>
                        <ShieldCheck size={15} />
                        Submit Proof & Drop in Chat
                      </>
                    )}
                  </button>
                </div>
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
