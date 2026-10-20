package com.onusly.app.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.onusly.app.R
import com.onusly.app.data.model.Goal

class GoalsAdapter(
    private val onSubmitProofClick: (Goal) -> Unit,
    private val onEditGoalClick: (Goal) -> Unit,
    private val onDeleteGoalClick: (Goal) -> Unit,
    private val onGoalClick: (Goal) -> Unit,
    private val onProofHistoryClick: (Goal) -> Unit
) : ListAdapter<Goal, GoalsAdapter.GoalViewHolder>(GoalDiffCallback()) {

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): GoalViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_goal_card, parent, false)
        return GoalViewHolder(view)
    }

    override fun onBindViewHolder(holder: GoalViewHolder, position: Int) {
        holder.bind(getItem(position))
    }

    inner class GoalViewHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val tvGoalTitle: TextView = itemView.findViewById(R.id.tvGoalTitle)
        private val tvGoalStatus: TextView = itemView.findViewById(R.id.tvGoalStatus)
        private val tvGoalDesc: TextView = itemView.findViewById(R.id.tvGoalDesc)
        private val tvGoalApprover: TextView = itemView.findViewById(R.id.tvGoalApprover)
        private val layoutTargets: LinearLayout = itemView.findViewById(R.id.layoutTargets)
        private val layoutBlockedApps: LinearLayout = itemView.findViewById(R.id.layoutBlockedApps)
        private val tvBlockedAppsList: TextView = itemView.findViewById(R.id.tvBlockedAppsList)
        private val layoutBlockedDomains: LinearLayout = itemView.findViewById(R.id.layoutBlockedDomains)
        private val tvBlockedDomainsList: TextView = itemView.findViewById(R.id.tvBlockedDomainsList)
        private val btnSubmitProof: MaterialButton = itemView.findViewById(R.id.btnSubmitProof)
        private val btnWaitingReview: MaterialButton = itemView.findViewById(R.id.btnWaitingReview)
        private val btnCompletedBadge: MaterialButton = itemView.findViewById(R.id.btnCompletedBadge)
        private val btnEditGoal: MaterialButton = itemView.findViewById(R.id.btnEditGoal)
        private val btnDeleteGoal: MaterialButton = itemView.findViewById(R.id.btnDeleteGoal)

        fun bind(goal: Goal) {
            val context = itemView.context
            tvGoalTitle.text = goal.title

            // Description
            if (goal.description.isNotBlank()) {
                tvGoalDesc.visibility = View.VISIBLE
                tvGoalDesc.text = goal.description
            } else {
                tvGoalDesc.visibility = View.GONE
            }

            // Status chip
            when {
                goal.isActive -> {
                    tvGoalStatus.text = "ACTIVE"
                    tvGoalStatus.setTextColor(ContextCompat.getColor(context, R.color.primary))
                }
                goal.isProofSubmitted -> {
                    tvGoalStatus.text = "WAITING REVIEW"
                    tvGoalStatus.setTextColor(ContextCompat.getColor(context, R.color.warning))
                }
                goal.isCompleted -> {
                    tvGoalStatus.text = "COMPLETED"
                    tvGoalStatus.setTextColor(ContextCompat.getColor(context, R.color.success))
                }
                else -> {
                    tvGoalStatus.text = goal.status.uppercase()
                    tvGoalStatus.setTextColor(ContextCompat.getColor(context, R.color.text_secondary))
                }
            }

            // Approver info
            val approverDisplay = when {
                goal.approverEmail.isNotBlank() -> goal.approverEmail
                goal.approvalType.equals("friend", ignoreCase = true) -> "Friend Approver"
                goal.approvalType.equals("community", ignoreCase = true) -> "Community"
                else -> "None"
            }
            tvGoalApprover.text = "Approver: $approverDisplay"

            // Distraction targets
            if (goal.isTargeted) {
                layoutTargets.visibility = View.VISIBLE
                if (goal.blockedApps.isNotEmpty()) {
                    layoutBlockedApps.visibility = View.VISIBLE
                    tvBlockedAppsList.text = goal.blockedApps.joinToString(", ")
                } else {
                    layoutBlockedApps.visibility = View.GONE
                }

                if (goal.blockedDomains.isNotEmpty()) {
                    layoutBlockedDomains.visibility = View.VISIBLE
                    tvBlockedDomainsList.text = goal.blockedDomains.joinToString(", ")
                } else {
                    layoutBlockedDomains.visibility = View.GONE
                }
            } else {
                layoutTargets.visibility = View.GONE
            }

            // Buttons based on status
            when {
                goal.isActive -> {
                    btnSubmitProof.visibility = View.VISIBLE
                    btnWaitingReview.visibility = View.GONE
                    btnCompletedBadge.visibility = View.GONE
                    btnSubmitProof.setOnClickListener { onSubmitProofClick(goal) }
                }
                goal.isProofSubmitted -> {
                    btnSubmitProof.visibility = View.GONE
                    btnWaitingReview.visibility = View.VISIBLE
                    btnCompletedBadge.visibility = View.GONE
                    btnWaitingReview.isEnabled = true
                    btnWaitingReview.text = "In Review (View Proof)"
                    btnWaitingReview.setOnClickListener { onProofHistoryClick(goal) }
                }
                goal.isCompleted -> {
                    btnSubmitProof.visibility = View.GONE
                    btnWaitingReview.visibility = View.GONE
                    btnCompletedBadge.visibility = View.VISIBLE
                    btnCompletedBadge.isEnabled = true
                    btnCompletedBadge.text = "Completed (View Proof)"
                    btnCompletedBadge.setOnClickListener { onProofHistoryClick(goal) }
                }
            }

            if (goal.isActive) {
                btnEditGoal.visibility = View.VISIBLE
                btnEditGoal.setOnClickListener { onEditGoalClick(goal) }
            } else {
                btnEditGoal.visibility = View.GONE
            }

            btnDeleteGoal.setOnClickListener { onDeleteGoalClick(goal) }
            itemView.setOnClickListener { onGoalClick(goal) }
        }
    }

    class GoalDiffCallback : DiffUtil.ItemCallback<Goal>() {
        override fun areItemsTheSame(oldItem: Goal, newItem: Goal): Boolean = oldItem.id == newItem.id
        override fun areContentsTheSame(oldItem: Goal, newItem: Goal): Boolean = oldItem == newItem
    }
}
