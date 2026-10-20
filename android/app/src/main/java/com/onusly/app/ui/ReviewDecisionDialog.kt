package com.onusly.app.ui

import android.app.Dialog
import android.content.Context
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.ImageView
import android.widget.ProgressBar
import android.widget.TextView
import androidx.core.content.ContextCompat
import com.google.android.material.button.MaterialButton
import com.onusly.app.R
import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.ApprovalResult
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.Proof
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ReviewDecisionDialog(
    private val context: Context,
    private val scope: CoroutineScope,
    private val goalsRepository: GoalsRepository,
    private val goal: Goal,
    private val proof: Proof,
    private val decisionStatus: String, // "approved" | "rejected"
    private val onDecisionSubmitted: (ApprovalResult, Boolean, String?) -> Unit
) {

    private val dialog: Dialog = Dialog(context, android.R.style.Theme_Material_Dialog)

    private lateinit var tvDecisionTitle: TextView
    private lateinit var tvDecisionSubtitle: TextView
    private lateinit var tvDecisionError: TextView
    private lateinit var ivDecisionImpactIcon: ImageView
    private lateinit var tvDecisionImpactText: TextView
    private lateinit var etDecisionComment: EditText
    private lateinit var pbDecisionSubmitting: ProgressBar
    private lateinit var btnCancelDecision: MaterialButton
    private lateinit var btnConfirmDecision: MaterialButton

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_review_decision, null)
        dialog.setContentView(view)
        dialog.window?.setLayout(
            (context.resources.displayMetrics.widthPixels * 0.95).toInt(),
            ViewGroup.LayoutParams.WRAP_CONTENT
        )

        bindViews(view)
        setupInitialData()
        setupListeners()

        dialog.show()
    }

    private fun bindViews(view: View) {
        tvDecisionTitle = view.findViewById(R.id.tvDecisionTitle)
        tvDecisionSubtitle = view.findViewById(R.id.tvDecisionSubtitle)
        tvDecisionError = view.findViewById(R.id.tvDecisionError)
        ivDecisionImpactIcon = view.findViewById(R.id.ivDecisionImpactIcon)
        tvDecisionImpactText = view.findViewById(R.id.tvDecisionImpactText)
        etDecisionComment = view.findViewById(R.id.etDecisionComment)
        pbDecisionSubmitting = view.findViewById(R.id.pbDecisionSubmitting)
        btnCancelDecision = view.findViewById(R.id.btnCancelDecision)
        btnConfirmDecision = view.findViewById(R.id.btnConfirmDecision)
    }

    private fun setupInitialData() {
        val isApprove = decisionStatus.equals("approved", ignoreCase = true)
        tvDecisionSubtitle.text = "Goal: ${goal.title}"

        if (isApprove) {
            tvDecisionTitle.text = "Approve Goal Proof"
            ivDecisionImpactIcon.setColorFilter(ContextCompat.getColor(context, R.color.success))
            tvDecisionImpactText.text = "Approving marks this goal as completed and releases all distraction locks."
            btnConfirmDecision.text = "Confirm Approval"
            btnConfirmDecision.backgroundTintList = ContextCompat.getColorStateList(context, R.color.success)
        } else {
            tvDecisionTitle.text = "Reject Goal Proof"
            ivDecisionImpactIcon.setColorFilter(ContextCompat.getColor(context, R.color.error))
            tvDecisionImpactText.text = "Rejecting will return this goal to active and keep all distraction targets locked."
            btnConfirmDecision.text = "Confirm Rejection"
            btnConfirmDecision.backgroundTintList = ContextCompat.getColorStateList(context, R.color.error)
        }
    }

    private fun setupListeners() {
        btnCancelDecision.setOnClickListener {
            dialog.dismiss()
        }

        btnConfirmDecision.setOnClickListener {
            submitDecision()
        }
    }

    private fun submitDecision() {
        hideError()
        val comment = etDecisionComment.text.toString().trim()

        if (comment.length > 2000) {
            showError("Comment cannot exceed 2000 characters.")
            return
        }

        val request = ApprovalDecisionRequest(
            ownerId = goal.ownerId,
            status = decisionStatus,
            comment = comment
        )

        setSubmittingState(true)

        scope.launch(Dispatchers.IO) {
            val result = goalsRepository.decideApproval(goal.id, proof.id, request)
            withContext(Dispatchers.Main) {
                setSubmittingState(false)
                result.onSuccess { mutation ->
                    dialog.dismiss()
                    onDecisionSubmitted(mutation.value, mutation.syncSucceeded, mutation.syncError)
                }.onFailure { err ->
                    showError(formatErrorMessage(err))
                }
            }
        }
    }

    private fun setSubmittingState(submitting: Boolean) {
        pbDecisionSubmitting.visibility = if (submitting) View.VISIBLE else View.GONE
        btnConfirmDecision.isEnabled = !submitting
        btnCancelDecision.isEnabled = !submitting
        etDecisionComment.isEnabled = !submitting
    }

    private fun showError(message: String) {
        tvDecisionError.text = message
        tvDecisionError.visibility = View.VISIBLE
    }

    private fun hideError() {
        tvDecisionError.visibility = View.GONE
    }

    private fun formatErrorMessage(err: Throwable): String {
        return when (err) {
            is OnuslyApiException -> {
                when (err.statusCode) {
                    403 -> "Forbidden: You are not authorized to decide this goal, or goal owners cannot self-approve."
                    409 -> "Proof has already been decided."
                    404 -> "Goal or proof not found."
                    401 -> "Session expired. Please sign in again."
                    else -> err.message
                }
            }
            else -> err.message ?: "Failed to submit decision. Please check your network connection."
        }
    }
}
