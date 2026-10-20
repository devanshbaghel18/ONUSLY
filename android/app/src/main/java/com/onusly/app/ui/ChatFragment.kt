package com.onusly.app.ui

import android.app.AlertDialog
import android.app.Dialog
import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.ImageButton
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.ChatMessage
import com.onusly.app.data.model.CommunityItem
import com.onusly.app.data.model.ConversationItem
import com.onusly.app.data.model.FriendItem
import com.onusly.app.data.model.FriendRequestItem
import com.onusly.app.data.realtime.RealtimeEvent
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ChatFragment : Fragment() {

    // View 1: Unified Conversations List
    private lateinit var layoutConversationsView: LinearLayout
    private lateinit var tvChatsTitle: TextView
    private lateinit var btnOpenExplore: MaterialButton
    private lateinit var btnOpenAddFriend: MaterialButton
    private lateinit var btnRefreshConversations: ImageButton
    private lateinit var chipFilterAll: TextView
    private lateinit var chipFilterFriends: TextView
    private lateinit var chipFilterCommunities: TextView
    private lateinit var etSearchConversations: EditText
    private lateinit var pbConversationsLoading: ProgressBar
    private lateinit var layoutEmptyConversations: LinearLayout
    private lateinit var btnEmptyAddFriend: MaterialButton
    private lateinit var btnEmptyExplore: MaterialButton
    private lateinit var rvConversations: RecyclerView
    private lateinit var conversationsAdapter: ConversationsAdapter

    // View 2: Active Chat Thread
    private lateinit var layoutChatThreadView: LinearLayout
    private lateinit var btnBackToConversations: ImageButton
    private lateinit var tvPartnerName: TextView
    private lateinit var tvPartnerSubtitle: TextView
    private lateinit var btnRefreshChat: ImageButton
    private lateinit var pbLoading: ProgressBar
    private lateinit var layoutEmpty: LinearLayout
    private lateinit var rvMessages: RecyclerView
    private lateinit var etMessageInput: EditText
    private lateinit var btnSend: ImageButton
    private lateinit var messagesAdapter: ChatAdapter

    // State
    private var allConversations = mutableListOf<ConversationItem>()
    private var activeFilter: String = "all" // "all" | "friends" | "communities"
    private var currentPeer: String = ""
    private var currentConversation: ConversationItem? = null

    // Polling & Realtime Jobs
    private var chatPollJob: Job? = null
    private var conversationsPollJob: Job? = null
    private var realtimeEventsJob: Job? = null

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View? {
        val view = inflater.inflate(R.layout.fragment_chat, container, false)
        bindViews(view)
        setupRecyclerViews()
        setupListeners()
        return view
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        observeRealtimeEvents()
    }

    override fun onResume() {
        super.onResume()
        if (currentConversation != null) {
            loadMessages(currentPeer)
        } else {
            loadConversations()
        }
        startActivePoll()
    }

    override fun onPause() {
        super.onPause()
        stopActivePoll()
    }

    override fun onDestroyView() {
        super.onDestroyView()
        stopActivePoll()
        realtimeEventsJob?.cancel()
        realtimeEventsJob = null
    }

    private fun bindViews(view: View) {
        // View 1
        layoutConversationsView = view.findViewById(R.id.layoutConversationsView)
        tvChatsTitle = view.findViewById(R.id.tvChatsTitle)
        btnOpenExplore = view.findViewById(R.id.btnOpenExplore)
        btnOpenAddFriend = view.findViewById(R.id.btnOpenAddFriend)
        btnRefreshConversations = view.findViewById(R.id.btnRefreshConversations)
        chipFilterAll = view.findViewById(R.id.chipFilterAll)
        chipFilterFriends = view.findViewById(R.id.chipFilterFriends)
        chipFilterCommunities = view.findViewById(R.id.chipFilterCommunities)
        etSearchConversations = view.findViewById(R.id.etSearchConversations)
        pbConversationsLoading = view.findViewById(R.id.pbConversationsLoading)
        layoutEmptyConversations = view.findViewById(R.id.layoutEmptyConversations)
        btnEmptyAddFriend = view.findViewById(R.id.btnEmptyAddFriend)
        btnEmptyExplore = view.findViewById(R.id.btnEmptyExplore)
        rvConversations = view.findViewById(R.id.rvConversations)

        // View 2
        layoutChatThreadView = view.findViewById(R.id.layoutChatThreadView)
        btnBackToConversations = view.findViewById(R.id.btnBackToConversations)
        tvPartnerName = view.findViewById(R.id.tvChatPartnerName)
        tvPartnerSubtitle = view.findViewById(R.id.tvChatPartnerSubtitle)
        btnRefreshChat = view.findViewById(R.id.btnRefreshChat)
        pbLoading = view.findViewById(R.id.pbChatLoading)
        layoutEmpty = view.findViewById(R.id.layoutEmptyChat)
        rvMessages = view.findViewById(R.id.rvChatMessages)
        etMessageInput = view.findViewById(R.id.etChatMessageInput)
        btnSend = view.findViewById(R.id.btnSendChatMessage)
    }

    private fun setupRecyclerViews() {
        val app = requireActivity().application as OnuslyApplication
        val session = app.authManager.getSession()

        val userId = session?.userId ?: ""
        val email = session?.email ?: ""
        val handle = ""

        // 1. Conversations List Adapter
        conversationsAdapter = ConversationsAdapter { item ->
            openConversation(item)
        }
        rvConversations.layoutManager = LinearLayoutManager(context)
        rvConversations.adapter = conversationsAdapter

        // 2. Chat Thread Messages Adapter
        messagesAdapter = ChatAdapter(
            currentUserId = userId,
            currentUserEmail = email,
            currentUserHandle = handle,
            onApproveClicked = { msg -> handleApproveProof(msg) },
            onRejectClicked = { msg -> handleRejectProof(msg) }
        )
        rvMessages.layoutManager = LinearLayoutManager(context).apply {
            stackFromEnd = true
        }
        rvMessages.adapter = messagesAdapter
    }

    private fun setupListeners() {
        // Conversations header buttons
        btnOpenAddFriend.setOnClickListener { showAddFriendDialog() }
        btnEmptyAddFriend.setOnClickListener { showAddFriendDialog() }
        btnOpenExplore.setOnClickListener { showExploreCommunitiesDialog() }
        btnEmptyExplore.setOnClickListener { showExploreCommunitiesDialog() }
        btnRefreshConversations.setOnClickListener { loadConversations() }

        // Filter chips
        chipFilterAll.setOnClickListener { setFilter("all") }
        chipFilterFriends.setOnClickListener { setFilter("friends") }
        chipFilterCommunities.setOnClickListener { setFilter("communities") }

        // Search text watcher
        etSearchConversations.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                applyConversationFilters()
            }
            override fun afterTextChanged(s: Editable?) {}
        })

        // Thread header buttons
        btnBackToConversations.setOnClickListener { closeConversation() }
        btnRefreshChat.setOnClickListener {
            if (currentPeer.isNotBlank()) loadMessages(currentPeer)
        }

        // Send message
        btnSend.setOnClickListener {
            val text = etMessageInput.text.toString().trim()
            if (text.isBlank()) return@setOnClickListener
            if (currentPeer.isBlank()) return@setOnClickListener
            sendMessage(text)
        }
    }

    private fun setFilter(filter: String) {
        activeFilter = filter
        val activeBg = R.drawable.bg_filter_chip_active
        val inactiveBg = R.drawable.bg_filter_chip
        val activeColor = 0xFF000000.toInt()
        val inactiveColor = 0xFFA1A1AA.toInt()

        chipFilterAll.setBackgroundResource(if (filter == "all") activeBg else inactiveBg)
        chipFilterAll.setTextColor(if (filter == "all") activeColor else inactiveColor)

        chipFilterFriends.setBackgroundResource(if (filter == "friends") activeBg else inactiveBg)
        chipFilterFriends.setTextColor(if (filter == "friends") activeColor else inactiveColor)

        chipFilterCommunities.setBackgroundResource(if (filter == "communities") activeBg else inactiveBg)
        chipFilterCommunities.setTextColor(if (filter == "communities") activeColor else inactiveColor)

        applyConversationFilters()
    }

    private fun observeRealtimeEvents() {
        val app = requireActivity().application as OnuslyApplication
        realtimeEventsJob?.cancel()
        realtimeEventsJob = viewLifecycleOwner.lifecycleScope.launch {
            app.realtimeWebSocketManager.events.collect { event ->
                when (event) {
                    is RealtimeEvent.ChatMessageReceived -> {
                        val msg = event.message
                        val peerMatch = currentPeer.isNotBlank() && (
                            currentPeer.equals(msg.senderEmail, ignoreCase = true) ||
                            currentPeer.equals("@${msg.senderHandle.removePrefix("@")}", ignoreCase = true) ||
                            currentPeer.equals(msg.senderId, ignoreCase = true) ||
                            currentPeer.equals(msg.recipientEmail, ignoreCase = true) ||
                            currentPeer.equals("@${msg.recipientHandle.removePrefix("@")}", ignoreCase = true)
                        )
                        if (peerMatch) {
                            loadMessages(currentPeer, silent = true)
                        } else {
                            loadConversations(silent = true)
                        }
                    }
                    is RealtimeEvent.CommunityMessageReceived -> {
                        if (currentPeer.equals(event.communityId, ignoreCase = true)) {
                            loadMessages(currentPeer, silent = true)
                        } else {
                            loadConversations(silent = true)
                        }
                    }
                    is RealtimeEvent.FriendRequestReceived,
                    is RealtimeEvent.FriendAccepted -> {
                        loadConversations(silent = true)
                    }
                }
            }
        }
    }

    private fun startActivePoll() {
        stopActivePoll()
        if (currentConversation != null && currentPeer.isNotBlank()) {
            chatPollJob = viewLifecycleOwner.lifecycleScope.launch {
                while (isActive) {
                    delay(1500)
                    if (currentConversation != null && currentPeer.isNotBlank()) {
                        loadMessages(currentPeer, silent = true)
                    }
                }
            }
        } else {
            conversationsPollJob = viewLifecycleOwner.lifecycleScope.launch {
                while (isActive) {
                    delay(3500)
                    if (currentConversation == null) {
                        loadConversations(silent = true)
                    }
                }
            }
        }
    }

    private fun stopActivePoll() {
        chatPollJob?.cancel()
        chatPollJob = null
        conversationsPollJob?.cancel()
        conversationsPollJob = null
    }

    private fun loadConversations(silent: Boolean = false) {
        val app = requireActivity().application as OnuslyApplication
        if (!silent) {
            pbConversationsLoading.visibility = View.VISIBLE
        }

        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val friendsRes = app.chatRepository.getFriends()
            val commsRes = app.chatRepository.getJoinedCommunities()

            val combined = mutableListOf<ConversationItem>()

            friendsRes.onSuccess { friends ->
                friends.forEach { f ->
                    combined.add(ConversationItem.FriendConversation(f))
                }
            }

            commsRes.onSuccess { comms ->
                comms.forEach { c ->
                    combined.add(ConversationItem.CommunityConversation(c))
                }
            }

            // Fallback to goal approver if user has no friends/communities yet
            if (combined.isEmpty()) {
                val goals = app.goalsRepository.goalsFlow.value.ifEmpty {
                    app.goalsRepository.fetchGoals().getOrNull() ?: emptyList()
                }
                val approvers = goals.mapNotNull { it.approverEmail.takeIf { e -> e.isNotBlank() } }.distinct()
                approvers.forEach { email ->
                    combined.add(
                        ConversationItem.FriendConversation(
                            FriendItem(
                                userId = "",
                                friendId = email,
                                friendHandle = "",
                                friendEmail = email,
                                friendName = email,
                                createdAt = ""
                            )
                        )
                    )
                }
            }

            withContext(Dispatchers.Main) {
                if (!silent) {
                    pbConversationsLoading.visibility = View.GONE
                }
                val isDifferent = allConversations.size != combined.size ||
                        allConversations.zip(combined).any { (a, b) -> a.id != b.id || a.subtitle != b.subtitle }
                if (isDifferent) {
                    allConversations.clear()
                    allConversations.addAll(combined)
                    tvChatsTitle.text = "Chats (${allConversations.size})"
                    applyConversationFilters()
                }
            }
        }
    }

    private fun applyConversationFilters() {
        val query = etSearchConversations.text.toString().trim().lowercase()

        val filtered = allConversations.filter { item ->
            val matchFilter = when (activeFilter) {
                "friends" -> item is ConversationItem.FriendConversation
                "communities" -> item is ConversationItem.CommunityConversation
                else -> true
            }

            val matchQuery = if (query.isBlank()) {
                true
            } else {
                item.name.lowercase().contains(query) || item.subtitle.lowercase().contains(query)
            }

            matchFilter && matchQuery
        }

        if (filtered.isEmpty()) {
            layoutEmptyConversations.visibility = View.VISIBLE
            rvConversations.visibility = View.GONE
        } else {
            layoutEmptyConversations.visibility = View.GONE
            rvConversations.visibility = View.VISIBLE
            conversationsAdapter.setItems(filtered)
        }
    }

    private fun openConversation(item: ConversationItem) {
        currentConversation = item
        currentPeer = when (item) {
            is ConversationItem.FriendConversation -> {
                if (item.friend.friendHandle.isNotBlank()) "@${item.friend.friendHandle.removePrefix("@")}" else item.friend.friendEmail
            }
            is ConversationItem.CommunityConversation -> item.community.id
        }

        tvPartnerName.text = item.name
        tvPartnerSubtitle.text = when (item) {
            is ConversationItem.FriendConversation -> "Accountability Partner • Online"
            is ConversationItem.CommunityConversation -> "Community Group • ${item.community.members} members"
        }

        layoutConversationsView.visibility = View.GONE
        layoutChatThreadView.visibility = View.VISIBLE

        if (item is ConversationItem.CommunityConversation) {
            val app = requireActivity().application as OnuslyApplication
            app.realtimeWebSocketManager.joinCommunity(item.community.id)
        }

        loadMessages(currentPeer)
        startActivePoll()
    }

    private fun closeConversation() {
        currentConversation = null
        currentPeer = ""
        layoutChatThreadView.visibility = View.GONE
        layoutConversationsView.visibility = View.VISIBLE
        loadConversations()
        startActivePoll()
    }

    private fun loadMessages(peer: String, silent: Boolean = false) {
        val app = requireActivity().application as OnuslyApplication
        if (!silent) {
            pbLoading.visibility = View.VISIBLE
        }

        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val result = app.chatRepository.getChatHistory(peer)
            withContext(Dispatchers.Main) {
                if (!silent) {
                    pbLoading.visibility = View.GONE
                }
                result.onSuccess { msgs ->
                    if (msgs.isEmpty()) {
                        layoutEmpty.visibility = View.VISIBLE
                        rvMessages.visibility = View.GONE
                    } else {
                        layoutEmpty.visibility = View.GONE
                        rvMessages.visibility = View.VISIBLE
                        val changed = messagesAdapter.setMessages(msgs)
                        if (changed) {
                            rvMessages.scrollToPosition(msgs.size - 1)
                        }
                    }
                }.onFailure { err ->
                    if (!silent) {
                        Toast.makeText(context, "Failed to load chat: ${err.message}", Toast.LENGTH_SHORT).show()
                    }
                }
            }
        }
    }

    private fun sendMessage(text: String) {
        val app = requireActivity().application as OnuslyApplication
        btnSend.isEnabled = false

        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val result = app.chatRepository.sendMessage(
                recipient = currentPeer,
                text = text
            )
            withContext(Dispatchers.Main) {
                btnSend.isEnabled = true
                result.onSuccess {
                    etMessageInput.text.clear()
                    loadMessages(currentPeer)
                }.onFailure { err ->
                    Toast.makeText(context, "Send failed: ${err.message}", Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    private fun handleApproveProof(msg: ChatMessage) {
        val app = requireActivity().application as OnuslyApplication
        val proofId = if (msg.id.startsWith("proof-")) msg.id.removePrefix("proof-") else msg.id

        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val res = app.goalsRepository.decideApproval(
                goalId = msg.goalId,
                proofId = proofId,
                request = ApprovalDecisionRequest(
                    status = "approved",
                    comment = "Approved via Android Chat",
                    ownerId = msg.ownerId
                )
            )
            withContext(Dispatchers.Main) {
                res.onSuccess {
                    Toast.makeText(context, "Goal Approved & Unlocked!", Toast.LENGTH_SHORT).show()
                    loadMessages(currentPeer)
                }.onFailure { err ->
                    Toast.makeText(context, "Approval failed: ${err.message}", Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    private fun handleRejectProof(msg: ChatMessage) {
        val app = requireActivity().application as OnuslyApplication
        val proofId = if (msg.id.startsWith("proof-")) msg.id.removePrefix("proof-") else msg.id

        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val res = app.goalsRepository.decideApproval(
                goalId = msg.goalId,
                proofId = proofId,
                request = ApprovalDecisionRequest(
                    status = "rejected",
                    comment = "Rejected via Android Chat",
                    ownerId = msg.ownerId
                )
            )
            withContext(Dispatchers.Main) {
                res.onSuccess {
                    Toast.makeText(context, "Proof Rejected. Goal remains active.", Toast.LENGTH_SHORT).show()
                    loadMessages(currentPeer)
                }.onFailure { err ->
                    Toast.makeText(context, "Rejection failed: ${err.message}", Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    // ============================================================
    // DIALOG: ADD FRIEND & FRIEND REQUESTS
    // ============================================================
    private fun showAddFriendDialog() {
        val dialog = Dialog(requireContext())
        val dialogView = LayoutInflater.from(context).inflate(R.layout.dialog_add_friend, null)
        dialog.setContentView(dialogView)
        dialog.window?.setBackgroundDrawableResource(android.R.color.transparent)

        val btnClose = dialogView.findViewById<ImageButton>(R.id.btnCloseAddFriend)
        val btnTabSend = dialogView.findViewById<MaterialButton>(R.id.btnTabSendRequest)
        val btnTabReqs = dialogView.findViewById<MaterialButton>(R.id.btnTabFriendRequests)
        val layoutSend = dialogView.findViewById<LinearLayout>(R.id.layoutSendRequestTab)
        val layoutReqs = dialogView.findViewById<LinearLayout>(R.id.layoutFriendRequestsTab)
        val etTarget = dialogView.findViewById<EditText>(R.id.etFriendTargetInput)
        val btnSubmit = dialogView.findViewById<MaterialButton>(R.id.btnSubmitFriendRequest)
        val pbReqs = dialogView.findViewById<ProgressBar>(R.id.pbRequestsLoading)
        val tvEmptyReqs = dialogView.findViewById<TextView>(R.id.tvEmptyRequests)
        val rvReqs = dialogView.findViewById<RecyclerView>(R.id.rvIncomingRequests)

        btnClose.setOnClickListener { dialog.dismiss() }

        fun switchTab(isSend: Boolean) {
            if (isSend) {
                btnTabSend.setBackgroundColor(0xFFFFFFFF.toInt())
                btnTabSend.setTextColor(0xFF000000.toInt())
                btnTabReqs.setBackgroundColor(0x00000000)
                btnTabReqs.setTextColor(0xFFFFFFFF.toInt())
                layoutSend.visibility = View.VISIBLE
                layoutReqs.visibility = View.GONE
            } else {
                btnTabReqs.setBackgroundColor(0xFFFFFFFF.toInt())
                btnTabReqs.setTextColor(0xFF000000.toInt())
                btnTabSend.setBackgroundColor(0x00000000)
                btnTabSend.setTextColor(0xFFFFFFFF.toInt())
                layoutSend.visibility = View.GONE
                layoutReqs.visibility = View.VISIBLE

                // Load incoming requests
                val app = requireActivity().application as OnuslyApplication
                pbReqs.visibility = View.VISIBLE
                viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
                    val result = app.chatRepository.getFriendRequests()
                    withContext(Dispatchers.Main) {
                        pbReqs.visibility = View.GONE
                        result.onSuccess { data ->
                            if (data.incoming.isEmpty()) {
                                tvEmptyReqs.visibility = View.VISIBLE
                                rvReqs.visibility = View.GONE
                            } else {
                                tvEmptyReqs.visibility = View.GONE
                                rvReqs.visibility = View.VISIBLE
                                rvReqs.layoutManager = LinearLayoutManager(context)
                                rvReqs.adapter = FriendRequestsAdapter(
                                    items = data.incoming,
                                    onAccept = { req ->
                                        respondFriendRequest(req.id, "accept", dialog)
                                    },
                                    onDecline = { req ->
                                        respondFriendRequest(req.id, "reject", dialog)
                                    }
                                )
                            }
                        }.onFailure { err ->
                            Toast.makeText(context, "Error: ${err.message}", Toast.LENGTH_SHORT).show()
                        }
                    }
                }
            }
        }

        btnTabSend.setOnClickListener { switchTab(true) }
        btnTabReqs.setOnClickListener { switchTab(false) }

        btnSubmit.setOnClickListener {
            val target = etTarget.text.toString().trim()
            if (target.isBlank()) {
                Toast.makeText(context, "Please enter a handle or email", Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }
            btnSubmit.isEnabled = false
            val app = requireActivity().application as OnuslyApplication
            viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
                val res = app.chatRepository.sendFriendRequest(target)
                withContext(Dispatchers.Main) {
                    btnSubmit.isEnabled = true
                    res.onSuccess {
                        Toast.makeText(context, "Friend request sent to $target!", Toast.LENGTH_LONG).show()
                        dialog.dismiss()
                    }.onFailure { err ->
                        Toast.makeText(context, "Failed: ${err.message}", Toast.LENGTH_LONG).show()
                    }
                }
            }
        }

        dialog.show()
    }

    private fun respondFriendRequest(requestId: String, action: String, dialog: Dialog) {
        val app = requireActivity().application as OnuslyApplication
        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val res = app.chatRepository.respondFriendRequest(requestId, action)
            withContext(Dispatchers.Main) {
                res.onSuccess {
                    Toast.makeText(context, "Request ${action}ed!", Toast.LENGTH_SHORT).show()
                    dialog.dismiss()
                    loadConversations()
                }.onFailure { err ->
                    Toast.makeText(context, "Error: ${err.message}", Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    // ============================================================
    // DIALOG: EXPLORE TRENDING COMMUNITIES
    // ============================================================
    private fun showExploreCommunitiesDialog() {
        val dialog = Dialog(requireContext())
        val dialogView = LayoutInflater.from(context).inflate(R.layout.dialog_explore_communities, null)
        dialog.setContentView(dialogView)
        dialog.window?.setBackgroundDrawableResource(android.R.color.transparent)

        val btnClose = dialogView.findViewById<ImageButton>(R.id.btnCloseExplore)
        val etSearch = dialogView.findViewById<EditText>(R.id.etSearchCommunities)
        val pbLoading = dialogView.findViewById<ProgressBar>(R.id.pbExploreLoading)
        val tvEmpty = dialogView.findViewById<TextView>(R.id.tvEmptyExplore)
        val rvExplore = dialogView.findViewById<RecyclerView>(R.id.rvExploreCommunities)

        btnClose.setOnClickListener { dialog.dismiss() }

        val app = requireActivity().application as OnuslyApplication
        pbLoading.visibility = View.VISIBLE

        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val result = app.chatRepository.getTrendingCommunities()
            withContext(Dispatchers.Main) {
                pbLoading.visibility = View.GONE
                result.onSuccess { communities ->
                    if (communities.isEmpty()) {
                        tvEmpty.visibility = View.VISIBLE
                        rvExplore.visibility = View.GONE
                    } else {
                        tvEmpty.visibility = View.GONE
                        rvExplore.visibility = View.VISIBLE
                        rvExplore.layoutManager = LinearLayoutManager(context)
                        val adapter = CommunitiesExploreAdapter(communities) { comm ->
                            joinCommunity(comm.id, dialog)
                        }
                        rvExplore.adapter = adapter

                        etSearch.addTextChangedListener(object : TextWatcher {
                            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
                            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                                val q = s.toString().trim().lowercase()
                                val filtered = communities.filter {
                                    it.name.lowercase().contains(q) || it.description.lowercase().contains(q)
                                }
                                adapter.setItems(filtered)
                            }
                            override fun afterTextChanged(s: Editable?) {}
                        })
                    }
                }.onFailure { err ->
                    Toast.makeText(context, "Failed to load communities: ${err.message}", Toast.LENGTH_SHORT).show()
                }
            }
        }

        dialog.show()
    }

    private fun joinCommunity(communityId: String, dialog: Dialog) {
        val app = requireActivity().application as OnuslyApplication
        viewLifecycleOwner.lifecycleScope.launch(Dispatchers.IO) {
            val res = app.chatRepository.joinCommunity(communityId)
            withContext(Dispatchers.Main) {
                res.onSuccess {
                    Toast.makeText(context, "Joined community!", Toast.LENGTH_SHORT).show()
                    dialog.dismiss()
                    loadConversations()
                }.onFailure { err ->
                    Toast.makeText(context, "Join failed: ${err.message}", Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    // ============================================================
    // ADAPTER: FRIEND REQUESTS
    // ============================================================
    private class FriendRequestsAdapter(
        private val items: List<FriendRequestItem>,
        private val onAccept: (FriendRequestItem) -> Unit,
        private val onDecline: (FriendRequestItem) -> Unit
    ) : RecyclerView.Adapter<FriendRequestsAdapter.RequestViewHolder>() {

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RequestViewHolder {
            val v = LayoutInflater.from(parent.context).inflate(R.layout.item_friend_request, parent, false)
            return RequestViewHolder(v)
        }

        override fun onBindViewHolder(holder: RequestViewHolder, position: Int) {
            val item = items[position]
            holder.tvName.text = item.senderName.ifBlank { "@${item.senderHandle.removePrefix("@")}" }
            holder.tvHandle.text = if (item.senderHandle.isNotBlank()) "@${item.senderHandle.removePrefix("@")}" else item.senderEmail
            holder.btnAccept.setOnClickListener { onAccept(item) }
            holder.btnDecline.setOnClickListener { onDecline(item) }
        }

        override fun getItemCount(): Int = items.size

        class RequestViewHolder(v: View) : RecyclerView.ViewHolder(v) {
            val tvName: TextView = v.findViewById(R.id.tvReqSenderName)
            val tvHandle: TextView = v.findViewById(R.id.tvReqSenderHandle)
            val btnAccept: MaterialButton = v.findViewById(R.id.btnAcceptReq)
            val btnDecline: MaterialButton = v.findViewById(R.id.btnDeclineReq)
        }
    }

    // ============================================================
    // ADAPTER: COMMUNITIES EXPLORE
    // ============================================================
    private class CommunitiesExploreAdapter(
        initialItems: List<CommunityItem>,
        private val onJoin: (CommunityItem) -> Unit
    ) : RecyclerView.Adapter<CommunitiesExploreAdapter.CommunityViewHolder>() {

        private val items = initialItems.toMutableList()

        fun setItems(newItems: List<CommunityItem>) {
            items.clear()
            items.addAll(newItems)
            notifyDataSetChanged()
        }

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): CommunityViewHolder {
            val v = LayoutInflater.from(parent.context).inflate(R.layout.item_community_card, parent, false)
            return CommunityViewHolder(v)
        }

        override fun onBindViewHolder(holder: CommunityViewHolder, position: Int) {
            val item = items[position]
            holder.tvName.text = item.name
            holder.tvCategory.text = item.category.uppercase()
            holder.tvDesc.text = item.description
            holder.tvMembers.text = "${item.members} members • ${item.streak} streak"
            holder.btnJoin.isEnabled = !item.isJoined
            holder.btnJoin.text = if (item.isJoined) "Joined" else "Join"
            holder.btnJoin.setOnClickListener { onJoin(item) }
        }

        override fun getItemCount(): Int = items.size

        class CommunityViewHolder(v: View) : RecyclerView.ViewHolder(v) {
            val tvName: TextView = v.findViewById(R.id.tvCommCardName)
            val tvCategory: TextView = v.findViewById(R.id.tvCommCardCategory)
            val tvDesc: TextView = v.findViewById(R.id.tvCommCardDesc)
            val tvMembers: TextView = v.findViewById(R.id.tvCommCardMembers)
            val btnJoin: MaterialButton = v.findViewById(R.id.btnJoinCommunity)
        }
    }
}
