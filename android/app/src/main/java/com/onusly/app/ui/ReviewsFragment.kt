package com.onusly.app.ui

import android.content.Context
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.ApproverInboxResponse
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.GoalParser
import com.onusly.app.data.model.Proof
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.time.Instant
import java.time.LocalDateTime
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.ZonedDateTime

class ReviewsFragment : Fragment() {

    private lateinit var tvReviewsHeader: TextView
    private lateinit var btnLookupGoal: MaterialButton
    private lateinit var btnFilterPendingReviews: MaterialButton
    private lateinit var btnFilterApprovedReviews: MaterialButton
    private lateinit var btnFilterRejectedReviews: MaterialButton
    private lateinit var btnFilterAllReviews: MaterialButton
    private lateinit var layoutReviewsOfflineBanner: LinearLayout
    private lateinit var btnRetryReviewsNetwork: MaterialButton
    private lateinit var pbLoadingReviews: ProgressBar
    private lateinit var layoutEmptyReviews: LinearLayout
    private lateinit var btnEmptyLookupGoal: MaterialButton
    private lateinit var rvReviews: RecyclerView

    private lateinit var adapter: ReviewsAdapter
    private val reviewItems = mutableListOf<ReviewItem>()
    private val manuallyLookedUpGoals = mutableListOf<ReviewItem>()
    private var pollJob: Job? = null
    private var currentFilter: String = "pending"

    companion object {
        private const val KEY_MANUALLY_LOOKED_UP_GOALS = "key_manually_looked_up_goals"

        fun parseTimestampToEpochMillis(timestamp: String?): Long {
            if (timestamp.isNullOrBlank()) return Long.MIN_VALUE
            val trimmed = timestamp.trim()
            trimmed.toLongOrNull()?.let { return it }
            return try {
                Instant.parse(trimmed).toEpochMilli()
            } catch (_: Exception) {
                try {
                    OffsetDateTime.parse(trimmed).toInstant().toEpochMilli()
                } catch (_: Exception) {
                    try {
                        ZonedDateTime.parse(trimmed).toInstant().toEpochMilli()
                    } catch (_: Exception) {
                        try {
                            LocalDateTime.parse(trimmed).atZone(ZoneOffset.UTC).toInstant().toEpochMilli()
                        } catch (_: Exception) {
                            Long.MIN_VALUE
                        }
                    }
                }
            }
        }

        fun sortProofsDescending(proofs: List<Proof>): List<Proof> {
            return proofs.sortedWith { p1, p2 ->
                val t1 = parseTimestampToEpochMillis(p1.submittedAt)
                val t2 = parseTimestampToEpochMillis(p2.submittedAt)
                if (t1 != t2) {
                    t2.compareTo(t1)
                } else {
                    p1.id.compareTo(p2.id)
                }
            }
        }

        fun findLatestProof(proofs: List<Proof>): Proof? {
            if (proofs.isEmpty()) return null
            return sortProofsDescending(proofs).firstOrNull()
        }
    }

    data class ReviewItem(
        val goal: Goal,
        val proof: Proof,
        val ownerName: String = "",
        val ownerHandle: String = "",
        val ownerEmail: String = "",
        val proofCount: Int = 1
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        restoreLookedUpGoals(savedInstanceState)
    }

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View? {
        val view = inflater.inflate(R.layout.fragment_reviews, container, false)
        bindViews(view)
        setupRecyclerView()
        setupListeners()
        observeOfflineState()
        return view
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        if (savedInstanceState != null && manuallyLookedUpGoals.isEmpty()) {
            restoreLookedUpGoals(savedInstanceState)
        }
    }

    override fun onResume() {
        super.onResume()
        if (!isHidden) {
            refreshReviews()
            startPolling()
        }
    }

    override fun onPause() {
        super.onPause()
        stopPolling()
    }

    override fun onHiddenChanged(hidden: Boolean) {
        super.onHiddenChanged(hidden)
        if (hidden) {
            stopPolling()
        } else {
            refreshReviews()
            startPolling()
        }
    }

    override fun onDestroyView() {
        super.onDestroyView()
        stopPolling()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        try {
            val jsonArr = JSONArray()
            for (item in manuallyLookedUpGoals) {
                val itemObj = JSONObject().apply {
                    put("goal", JSONObject().apply {
                        put("id", item.goal.id)
                        put("ownerId", item.goal.ownerId)
                        put("approverId", item.goal.approverId)
                        put("approverEmail", item.goal.approverEmail)
                        put("approvalType", item.goal.approvalType)
                        put("title", item.goal.title)
                        put("description", item.goal.description)
                        put("status", item.goal.status)
                        put("createdAt", item.goal.createdAt)
                        put("updatedAt", item.goal.updatedAt)
                        put("blockedApps", JSONArray(item.goal.blockedApps))
                        put("blockedDomains", JSONArray(item.goal.blockedDomains))
                    })
                    put("proof", JSONObject().apply {
                        put("id", item.proof.id)
                        put("goalId", item.proof.goalId)
                        put("ownerId", item.proof.ownerId)
                        put("proofType", item.proof.proofType)
                        put("textExplanation", item.proof.textExplanation)
                        put("externalLink", item.proof.externalLink)
                        put("photoUrl", item.proof.photoUrl)
                        put("submittedAt", item.proof.submittedAt)
                    })
                }
                jsonArr.put(itemObj)
            }
            outState.putString(KEY_MANUALLY_LOOKED_UP_GOALS, jsonArr.toString())
        } catch (_: Exception) {}
    }

    private fun restoreLookedUpGoals(bundle: Bundle?) {
        val jsonStr = bundle?.getString(KEY_MANUALLY_LOOKED_UP_GOALS) ?: return
        try {
            val jsonArr = JSONArray(jsonStr)
            manuallyLookedUpGoals.clear()
            for (i in 0 until jsonArr.length()) {
                val itemObj = jsonArr.getJSONObject(i)
                val goal = GoalParser.parseGoal(itemObj.getJSONObject("goal"))
                val proof = GoalParser.parseProof(itemObj.getJSONObject("proof"))
                manuallyLookedUpGoals.add(ReviewItem(goal, proof))
            }
        } catch (_: Exception) {}
    }

    private fun startPolling() {
        stopPolling()
        pollJob = viewLifecycleOwner.lifecycleScope.launch {
            while (isActive) {
                delay(15000)
                refreshReviews()
            }
        }
    }

    private fun stopPolling() {
        pollJob?.cancel()
        pollJob = null
    }

    private fun bindViews(view: View) {
        tvReviewsHeader = view.findViewById(R.id.tvReviewsHeader)
        btnLookupGoal = view.findViewById(R.id.btnLookupGoal)
        btnFilterPendingReviews = view.findViewById(R.id.btnFilterPendingReviews)
        btnFilterApprovedReviews = view.findViewById(R.id.btnFilterApprovedReviews)
        btnFilterRejectedReviews = view.findViewById(R.id.btnFilterRejectedReviews)
        btnFilterAllReviews = view.findViewById(R.id.btnFilterAllReviews)
        layoutReviewsOfflineBanner = view.findViewById(R.id.layoutReviewsOfflineBanner)
        btnRetryReviewsNetwork = view.findViewById(R.id.btnRetryReviewsNetwork)
        pbLoadingReviews = view.findViewById(R.id.pbLoadingReviews)
        layoutEmptyReviews = view.findViewById(R.id.layoutEmptyReviews)
        btnEmptyLookupGoal = view.findViewById(R.id.btnEmptyLookupGoal)
        rvReviews = view.findViewById(R.id.rvReviews)
    }

    private fun setupListeners() {
        btnLookupGoal.setOnClickListener { showLookupGoalDialog() }
        btnEmptyLookupGoal.setOnClickListener { showLookupGoalDialog() }
        btnRetryReviewsNetwork.setOnClickListener { refreshReviews() }

        btnFilterPendingReviews.setOnClickListener { setFilter("pending") }
        btnFilterApprovedReviews.setOnClickListener { setFilter("approved") }
        btnFilterRejectedReviews.setOnClickListener { setFilter("rejected") }
        btnFilterAllReviews.setOnClickListener { setFilter("all") }
    }

    private fun setFilter(filter: String) {
        if (currentFilter == filter) return
        currentFilter = filter
        updateFilterButtonStyles()
        refreshReviews()
    }

    private fun updateFilterButtonStyles() {
        val ctx = context ?: return
        val activeBg = ContextCompat.getColorStateList(ctx, R.color.primary)
        val inactiveBg = ContextCompat.getColorStateList(ctx, android.R.color.transparent)
        val activeText = ContextCompat.getColor(ctx, R.color.text_primary)
        val inactiveText = ContextCompat.getColor(ctx, R.color.text_secondary)
        val cardBorder = ContextCompat.getColorStateList(ctx, R.color.surface_card)

        fun applyStyle(btn: MaterialButton, isActive: Boolean) {
            if (isActive) {
                btn.backgroundTintList = activeBg
                btn.setTextColor(activeText)
                btn.strokeWidth = 0
            } else {
                btn.backgroundTintList = inactiveBg
                btn.setTextColor(inactiveText)
                btn.strokeColor = cardBorder
                btn.strokeWidth = (1 * resources.displayMetrics.density).toInt()
            }
        }

        applyStyle(btnFilterPendingReviews, currentFilter == "pending")
        applyStyle(btnFilterApprovedReviews, currentFilter == "approved")
        applyStyle(btnFilterRejectedReviews, currentFilter == "rejected")
        applyStyle(btnFilterAllReviews, currentFilter == "all")
    }

    private fun observeOfflineState() {
        viewLifecycleOwner.lifecycleScope.launch {
            val app = (activity?.application as? OnuslyApplication) ?: return@launch
            app.goalsRepository.isOfflineFlow.collect { isOffline ->
                layoutReviewsOfflineBanner.visibility = if (isOffline) View.VISIBLE else View.GONE
            }
        }
    }

    private fun setupRecyclerView() {
        adapter = ReviewsAdapter(
            onApprove = { item -> promptDecision(item, "approved") },
            onReject = { item -> promptDecision(item, "rejected") }
        )
        rvReviews.layoutManager = LinearLayoutManager(context)
        rvReviews.adapter = adapter
    }

    fun clearLocalData() {
        stopPolling()
        reviewItems.clear()
        manuallyLookedUpGoals.clear()
        if (::adapter.isInitialized) {
            adapter.notifyDataSetChanged()
        }
        if (::layoutEmptyReviews.isInitialized && ::rvReviews.isInitialized) {
            layoutEmptyReviews.visibility = View.VISIBLE
            rvReviews.visibility = View.GONE
        }
        if (::layoutReviewsOfflineBanner.isInitialized) {
            layoutReviewsOfflineBanner.visibility = View.GONE
        }
    }

    fun refreshReviews() {
        val app = (activity?.application as? OnuslyApplication) ?: return
        if (view == null || !::pbLoadingReviews.isInitialized) return
        val callerUserId = app.authManager.getUserId() ?: return
        if (!app.authManager.isSignedIn()) {
            clearLocalData()
            return
        }

        pbLoadingReviews.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            val inboxResult = withContext(Dispatchers.IO) {
                app.goalsRepository.getApproverInbox(currentFilter)
            }
            if (inboxResult.isFailure) {
                pbLoadingReviews.visibility = View.GONE
                return@launch
            }
            val inboxResponse = inboxResult.getOrDefault(ApproverInboxResponse())

            val items = mutableListOf<ReviewItem>()
            for (inboxItem in inboxResponse.items) {
                val proof = inboxItem.latestProof ?: Proof(
                    id = "proof-placeholder-${inboxItem.goalId}",
                    goalId = inboxItem.goalId,
                    ownerId = inboxItem.ownerId,
                    proofType = "none",
                    textExplanation = "Goal is waiting for proof submission.",
                    submittedAt = if (inboxItem.updatedAt.isNotBlank()) inboxItem.updatedAt else inboxItem.createdAt
                )
                items.add(
                    ReviewItem(
                        goal = inboxItem.toGoal(),
                        proof = proof,
                        ownerName = inboxItem.ownerName,
                        ownerHandle = inboxItem.ownerHandle,
                        ownerEmail = inboxItem.ownerEmail,
                        proofCount = inboxItem.proofCount
                    )
                )
            }

            // Include any manually looked-up goals that match the active filter
            for (item in manuallyLookedUpGoals) {
                if (items.none { it.goal.id == item.goal.id }) {
                    val matches = when (currentFilter.lowercase()) {
                        "pending" -> item.goal.isProofSubmitted
                        "approved" -> item.goal.isCompleted
                        "rejected" -> item.goal.isRejected
                        else -> true
                    }
                    if (matches) {
                        items.add(item)
                    }
                }
            }

            if (app.authManager.getUserId() != callerUserId) return@launch

            pbLoadingReviews.visibility = View.GONE
            reviewItems.clear()
            reviewItems.addAll(items)
            adapter.notifyDataSetChanged()

            if (reviewItems.isEmpty()) {
                layoutEmptyReviews.visibility = View.VISIBLE
                rvReviews.visibility = View.GONE
            } else {
                layoutEmptyReviews.visibility = View.GONE
                rvReviews.visibility = View.VISIBLE
            }
        }
    }

    private fun showLookupGoalDialog() {
        val context = context ?: return
        val app = (activity?.application as? OnuslyApplication) ?: return

        val container = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(50, 20, 50, 10)
        }

        val inputGoalId = EditText(context).apply {
            hint = "Goal ID *"
        }
        val inputOwnerId = EditText(context).apply {
            hint = "Owner ID (optional)"
        }

        container.addView(inputGoalId)
        container.addView(inputOwnerId)

        AlertDialog.Builder(context)
            .setTitle("Lookup Friend's Goal")
            .setMessage("Enter your friend's Goal ID to inspect their submitted proof and make an accountability decision.")
            .setView(container)
            .setPositiveButton("Lookup") { _, _ ->
                val goalId = inputGoalId.text.toString().trim()
                val ownerId = inputOwnerId.text.toString().trim().ifBlank { null }

                if (goalId.isBlank()) {
                    Toast.makeText(context, "Goal ID is required", Toast.LENGTH_SHORT).show()
                    return@setPositiveButton
                }

                pbLoadingReviews.visibility = View.VISIBLE
                viewLifecycleOwner.lifecycleScope.launch {
                    val goalResult = withContext(Dispatchers.IO) {
                        app.goalsRepository.fetchGoal(goalId, ownerId)
                    }
                    pbLoadingReviews.visibility = View.GONE

                    goalResult.onSuccess { goal ->
                        if (!goal.isProofSubmitted) {
                            AlertDialog.Builder(context)
                                .setTitle("Goal Not in Review")
                                .setMessage("\"${goal.title}\" is currently ${goal.status.uppercase()}. Only goals with submitted proof (\"proof_submitted\") can be reviewed.")
                                .setPositiveButton("OK", null)
                                .show()
                            return@onSuccess
                        }

                        // Fetch proofs
                        pbLoadingReviews.visibility = View.VISIBLE
                        val proofsResult = withContext(Dispatchers.IO) {
                            app.goalsRepository.getProofs(goal.id, goal.ownerId)
                        }
                        pbLoadingReviews.visibility = View.GONE

                        proofsResult.onSuccess { proofs ->
                            val latestProof = findLatestProof(proofs)
                            if (latestProof == null) {
                                Toast.makeText(context, "No proofs found for this goal", Toast.LENGTH_SHORT).show()
                            } else {
                                val item = ReviewItem(goal, latestProof)
                                manuallyLookedUpGoals.removeAll { it.goal.id == goal.id }
                                manuallyLookedUpGoals.add(0, item)
                                refreshReviews()
                                Toast.makeText(context, "Loaded goal: ${goal.title}", Toast.LENGTH_SHORT).show()
                            }
                        }.onFailure { err ->
                            Toast.makeText(context, "Failed to load proofs: ${err.message}", Toast.LENGTH_LONG).show()
                        }
                    }.onFailure { err ->
                        AlertDialog.Builder(context)
                            .setTitle("Lookup Failed")
                            .setMessage(err.message ?: "Failed to find goal. Ensure the Goal ID is correct and you are the assigned approver.")
                            .setPositiveButton("OK", null)
                            .show()
                    }
                }
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun showError(message: String) {
        val ctx = context ?: return
        AlertDialog.Builder(ctx)
            .setTitle("Forbidden")
            .setMessage(message)
            .setPositiveButton("OK", null)
            .show()
    }

    private fun promptDecision(item: ReviewItem, status: String) {
        val app = (activity?.application as? OnuslyApplication) ?: return
        val currentUserId = app.authManager.getUserId()

        // Guard against deciding your own goal
        if (item.goal.ownerId == currentUserId) {
            showError("You cannot decide your own goal.")
            return
        }

        ReviewDecisionDialog(
            context = requireContext(),
            scope = viewLifecycleOwner.lifecycleScope,
            goalsRepository = app.goalsRepository,
            goal = item.goal,
            proof = item.proof,
            decisionStatus = status
        ) { decision, syncSucceeded, syncError ->
            val msg = if (decision.status == "approved") {
                "Proof approved! \"${item.goal.title}\" is completed and targets are unlocked."
            } else {
                "Proof rejected. \"${item.goal.title}\" has returned to active status."
            }
            Toast.makeText(context, msg, Toast.LENGTH_LONG).show()

            if (!syncSucceeded) {
                (activity as? MainActivity)?.let { mainAct ->
                    AlertDialog.Builder(mainAct)
                        .setTitle("Blocklist Sync Pending")
                        .setMessage("Decision saved on the server, but local blocklist update failed (${syncError ?: "Network error"}). Local enforcement will retry automatically.")
                        .setPositiveButton("Retry Sync Now") { _, _ ->
                            mainAct.lifecycleScope.launch {
                                app.goalsRepository.retryBlocklistSync()
                            }
                        }
                        .setNegativeButton("Dismiss", null)
                        .show()
                }
            }

            manuallyLookedUpGoals.removeAll { it.goal.id == item.goal.id }
            refreshReviews()
        }.show()
    }

    inner class ReviewsAdapter(
        private val onApprove: (ReviewItem) -> Unit,
        private val onReject: (ReviewItem) -> Unit
    ) : RecyclerView.Adapter<ReviewsAdapter.ReviewViewHolder>() {

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ReviewViewHolder {
            val view = LayoutInflater.from(parent.context).inflate(R.layout.item_review_card, parent, false)
            return ReviewViewHolder(view)
        }

        override fun onBindViewHolder(holder: ReviewViewHolder, position: Int) {
            holder.bind(reviewItems[position])
        }

        override fun getItemCount(): Int = reviewItems.size

        inner class ReviewViewHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
            private val tvReviewGoalTitle: TextView = itemView.findViewById(R.id.tvReviewGoalTitle)
            private val tvReviewOwner: TextView = itemView.findViewById(R.id.tvReviewOwner)
            private val tvReviewTargets: TextView = itemView.findViewById(R.id.tvReviewTargets)
            private val tvProofTypeBadge: TextView = itemView.findViewById(R.id.tvProofTypeBadge)
            private val tvProofSubmittedAt: TextView = itemView.findViewById(R.id.tvProofSubmittedAt)
            private val tvProofContent: TextView = itemView.findViewById(R.id.tvProofContent)
            private val tvProofLink: TextView = itemView.findViewById(R.id.tvProofLink)
            private val tvSelfApprovalNotice: TextView = itemView.findViewById(R.id.tvSelfApprovalNotice)
            private val btnApprove: MaterialButton = itemView.findViewById(R.id.btnApprove)
            private val btnReject: MaterialButton = itemView.findViewById(R.id.btnReject)

            fun bind(item: ReviewItem) {
                val app = itemView.context.applicationContext as OnuslyApplication
                val currentUserId = app.authManager.getUserId()
                val isSelf = currentUserId != null && currentUserId == item.goal.ownerId

                tvReviewGoalTitle.text = item.goal.title
                val ownerDisplayName = when {
                    item.ownerName.isNotBlank() -> item.ownerName
                    item.ownerHandle.isNotBlank() -> "@${item.ownerHandle}"
                    item.ownerEmail.isNotBlank() -> item.ownerEmail
                    else -> item.goal.ownerId
                }
                tvReviewOwner.text = "Owner: $ownerDisplayName"

                if (item.goal.isTargeted) {
                    val allTargets = item.goal.blockedApps + item.goal.blockedDomains
                    tvReviewTargets.text = "Locked Targets: ${allTargets.joinToString(", ")}"
                    tvReviewTargets.visibility = View.VISIBLE
                } else {
                    tvReviewTargets.text = "Locked Targets: None"
                    tvReviewTargets.visibility = View.VISIBLE
                }

                tvProofTypeBadge.text = "PROOF TYPE: ${item.proof.proofType.uppercase()}"
                tvProofSubmittedAt.text = if (item.proof.submittedAt.isNotBlank()) {
                    "Submitted: ${item.proof.submittedAt}"
                } else {
                    "Submitted recently"
                }

                when (item.proof.proofType.lowercase()) {
                    "text" -> {
                        tvProofContent.visibility = View.VISIBLE
                        tvProofContent.text = item.proof.textExplanation
                        tvProofLink.visibility = View.GONE
                    }
                    "link" -> {
                        tvProofContent.visibility = if (item.proof.textExplanation.isNotBlank()) View.VISIBLE else View.GONE
                        tvProofContent.text = item.proof.textExplanation
                        tvProofLink.visibility = View.VISIBLE
                        tvProofLink.text = item.proof.externalLink
                    }
                    "photo" -> {
                        tvProofContent.visibility = if (item.proof.textExplanation.isNotBlank()) View.VISIBLE else View.GONE
                        tvProofContent.text = item.proof.textExplanation
                        tvProofLink.visibility = View.VISIBLE
                        tvProofLink.text = item.proof.photoUrl
                    }
                    else -> {
                        tvProofContent.visibility = View.VISIBLE
                        tvProofContent.text = item.proof.textExplanation
                        tvProofLink.visibility = View.GONE
                    }
                }

                if (item.goal.isCompleted) {
                    tvSelfApprovalNotice.visibility = View.VISIBLE
                    tvSelfApprovalNotice.text = "This goal has been verified and completed."
                    btnApprove.visibility = View.GONE
                    btnReject.visibility = View.GONE
                } else if (item.goal.isRejected) {
                    tvSelfApprovalNotice.visibility = View.VISIBLE
                    tvSelfApprovalNotice.text = "This proof submission was rejected."
                    btnApprove.visibility = View.GONE
                    btnReject.visibility = View.GONE
                } else if (isSelf) {
                    tvSelfApprovalNotice.visibility = View.VISIBLE
                    tvSelfApprovalNotice.text = "You are the goal owner. Only your accountability partner (${item.goal.approverEmail.ifBlank { "assigned friend" }}) can verify and approve."
                    btnApprove.isEnabled = false
                    btnApprove.alpha = 0.5f
                    btnApprove.setOnClickListener(null)
                    btnReject.visibility = View.GONE
                    btnReject.setOnClickListener(null)
                } else {
                    tvSelfApprovalNotice.visibility = View.GONE
                    btnApprove.isEnabled = true
                    btnApprove.alpha = 1.0f
                    btnReject.visibility = View.VISIBLE
                    btnApprove.setOnClickListener { onApprove(item) }
                    btnReject.setOnClickListener { onReject(item) }
                }
            }
        }
    }
}
