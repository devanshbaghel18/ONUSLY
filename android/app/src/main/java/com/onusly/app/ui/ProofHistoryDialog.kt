package com.onusly.app.ui

import android.app.Dialog
import android.content.Context
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ProgressBar
import android.widget.TextView
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.onusly.app.R
import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.Proof
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ProofHistoryDialog(
    private val context: Context,
    private val scope: CoroutineScope,
    private val goalsRepository: GoalsRepository,
    private val goal: Goal
) {

    private val dialog: Dialog = Dialog(context, android.R.style.Theme_Material_Dialog)

    private lateinit var tvHistoryModalSubtitle: TextView
    private lateinit var tvHistoryError: TextView
    private lateinit var pbLoadingHistory: ProgressBar
    private lateinit var layoutEmptyHistory: View
    private lateinit var rvProofHistory: RecyclerView
    private lateinit var btnCloseHistory: MaterialButton

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_proof_history, null)
        dialog.setContentView(view)
        dialog.window?.setLayout(
            (context.resources.displayMetrics.widthPixels * 0.95).toInt(),
            ViewGroup.LayoutParams.WRAP_CONTENT
        )

        bindViews(view)
        setupInitialData()
        loadProofs()

        dialog.show()
    }

    private fun bindViews(view: View) {
        tvHistoryModalSubtitle = view.findViewById(R.id.tvHistoryModalSubtitle)
        tvHistoryError = view.findViewById(R.id.tvHistoryError)
        pbLoadingHistory = view.findViewById(R.id.pbLoadingHistory)
        layoutEmptyHistory = view.findViewById(R.id.layoutEmptyHistory)
        rvProofHistory = view.findViewById(R.id.rvProofHistory)
        btnCloseHistory = view.findViewById(R.id.btnCloseHistory)
    }

    private fun setupInitialData() {
        tvHistoryModalSubtitle.text = "Goal: ${goal.title}"
        rvProofHistory.layoutManager = LinearLayoutManager(context)
        btnCloseHistory.setOnClickListener { dialog.dismiss() }
    }

    private fun loadProofs() {
        pbLoadingHistory.visibility = View.VISIBLE
        layoutEmptyHistory.visibility = View.GONE
        rvProofHistory.visibility = View.GONE
        tvHistoryError.visibility = View.GONE

        scope.launch(Dispatchers.IO) {
            val result = goalsRepository.getProofs(goal.id, goal.ownerId)
            withContext(Dispatchers.Main) {
                pbLoadingHistory.visibility = View.GONE
                result.onSuccess { proofs ->
                    if (proofs.isEmpty()) {
                        layoutEmptyHistory.visibility = View.VISIBLE
                        rvProofHistory.visibility = View.GONE
                    } else {
                        layoutEmptyHistory.visibility = View.GONE
                        rvProofHistory.visibility = View.VISIBLE
                        rvProofHistory.adapter = ProofHistoryAdapter(proofs.reversed())
                    }
                }.onFailure { err ->
                    tvHistoryError.text = err.message ?: "Failed to load proof history"
                    tvHistoryError.visibility = View.VISIBLE
                }
            }
        }
    }

    private class ProofHistoryAdapter(
        private val proofs: List<Proof>
    ) : RecyclerView.Adapter<ProofHistoryAdapter.ProofViewHolder>() {

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ProofViewHolder {
            val view = LayoutInflater.from(parent.context)
                .inflate(R.layout.item_proof_history, parent, false)
            return ProofViewHolder(view)
        }

        override fun onBindViewHolder(holder: ProofViewHolder, position: Int) {
            holder.bind(proofs[position])
        }

        override fun getItemCount(): Int = proofs.size

        class ProofViewHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
            private val tvType: TextView = itemView.findViewById(R.id.tvHistoryTypeBadge)
            private val tvTimestamp: TextView = itemView.findViewById(R.id.tvHistoryTimestamp)
            private val tvExplanation: TextView = itemView.findViewById(R.id.tvHistoryExplanation)
            private val tvLink: TextView = itemView.findViewById(R.id.tvHistoryLink)
            private val tvPhotoUrl: TextView = itemView.findViewById(R.id.tvHistoryPhotoUrl)

            fun bind(proof: Proof) {
                tvType.text = proof.proofType.uppercase()
                tvTimestamp.text = if (proof.submittedAt.isNotBlank()) {
                    proof.submittedAt
                } else {
                    "Recently submitted"
                }

                if (proof.textExplanation.isNotBlank()) {
                    tvExplanation.visibility = View.VISIBLE
                    tvExplanation.text = proof.textExplanation
                } else {
                    tvExplanation.visibility = View.GONE
                }

                if (proof.externalLink.isNotBlank()) {
                    tvLink.visibility = View.VISIBLE
                    tvLink.text = proof.externalLink
                } else {
                    tvLink.visibility = View.GONE
                }

                if (proof.photoUrl.isNotBlank()) {
                    tvPhotoUrl.visibility = View.VISIBLE
                    tvPhotoUrl.text = proof.photoUrl
                } else {
                    tvPhotoUrl.visibility = View.GONE
                }
            }
        }
    }
}
