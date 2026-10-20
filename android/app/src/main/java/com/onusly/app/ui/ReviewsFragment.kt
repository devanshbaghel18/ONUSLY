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
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.Proof
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ReviewsFragment : Fragment() {

    private lateinit var tvReviewsHeader: TextView
    private lateinit var btnLookupGoal: MaterialButton
    private lateinit var layoutReviewsOfflineBanner: LinearLayout
    private lateinit var btnRetryReviewsNetwork: MaterialButton
    private lateinit var pbLoadingReviews: ProgressBar
    private lateinit var layoutEmptyReviews: LinearLayout
    private lateinit var btnEmptyLookupGoal: MaterialButton
    private lateinit var rvReviews: RecyclerView

    private lateinit var adapter: ReviewsAdapter
    private val reviewItems = mutableListOf<ReviewItem>()
    private val manuallyLookedUpGoals = mutableListOf<ReviewItem>()

    data class ReviewItem(
        val goal: Goal,
        val proof: Proof
    )

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

    override fun onResume() {
        super.onResume()
        refreshReviews()
    }

    private fun bindViews(view: View) {
        tvReviewsHeader = view.findViewById(R.id.tvReviewsHeader)
        btnLookupGoal = view.findViewById(R.id.btnLookupGoal)
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
    }

    private fun observeOfflineState() {
        viewLifecycleOwner.lifecycleScope.launch {
            val app = requireActivity().application as OnuslyApplication
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
        if (!app.authManager.isSignedIn()) {
            clearLocalData()
            return
        }

        pbLoadingReviews.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            val goalsResult = withContext(Dispatchers.IO) { app.goalsRepository.fetchGoals() }
            if (goalsResult.isFailure) {
                pbLoadingReviews.visibility = View.GONE
                return@launch
            }
            val goals = goalsResult.getOrDefault(emptyList())

            val items = mutableListOf<ReviewItem>()

            // Find goals that have proof_submitted
            val submittedGoals = goals.filter { it.isProofSubmitted }
            for (goal in submittedGoals) {
                val proofsResult = withContext(Dispatchers.IO) {
                    app.goalsRepository.getProofs(goal.id, goal.ownerId)
                }
                val proofs = proofsResult.getOrDefault(emptyList())
                if (proofs.isNotEmpty()) {
                    items.add(ReviewItem(goal, proofs.last()))
                }
            }

            // Include any manually looked-up goals that are still in review
            for (item in manuallyLookedUpGoals) {
                if (items.none { it.goal.id == item.goal.id }) {
                    items.add(item)
                }
            }

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
                            if (proofs.isEmpty()) {
                                Toast.makeText(context, "No proofs found for this goal", Toast.LENGTH_SHORT).show()
                            } else {
                                val item = ReviewItem(goal, proofs.last())
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

    private fun promptDecision(item: ReviewItem, status: String) {
        val app = (activity?.application as? OnuslyApplication) ?: return
        val currentUserId = app.authManager.getUserId()

        // Guard against self-approval
        if (currentUserId != null && currentUserId == item.goal.ownerId && status == "approved") {
            AlertDialog.Builder(requireContext())
                .setTitle("Self-Approval Forbidden")
                .setMessage("Goal owners cannot approve their own proof. Your designated accountability partner (${item.goal.approverEmail}) must verify and approve this goal.")
                .setPositiveButton("OK", null)
                .show()
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
                tvReviewOwner.text = "Owner: ${item.goal.ownerId}"

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

                if (isSelf) {
                    tvSelfApprovalNotice.visibility = View.VISIBLE
                    tvSelfApprovalNotice.text = "You are the goal owner. Only your accountability partner (${item.goal.approverEmail.ifBlank { "assigned friend" }}) can verify and approve."
                    btnApprove.isEnabled = false
                    btnApprove.alpha = 0.5f
                    btnReject.visibility = View.GONE
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
