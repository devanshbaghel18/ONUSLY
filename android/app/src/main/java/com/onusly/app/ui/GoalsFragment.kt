package com.onusly.app.ui

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
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
import com.google.android.material.floatingactionbutton.FloatingActionButton
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.Goal
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class GoalsFragment : Fragment() {

    private enum class GoalFilter { ALL, ACTIVE, REVIEW, COMPLETED }

    private var currentFilter = GoalFilter.ALL

    private lateinit var btnFilterAll: MaterialButton
    private lateinit var btnFilterActive: MaterialButton
    private lateinit var btnFilterReview: MaterialButton
    private lateinit var btnFilterCompleted: MaterialButton

    private lateinit var pbLoadingGoals: ProgressBar
    private lateinit var layoutGoalsOfflineBanner: LinearLayout
    private lateinit var btnRetryGoalsNetwork: MaterialButton
    private lateinit var layoutEmptyGoals: LinearLayout
    private lateinit var tvEmptyGoalsTitle: TextView
    private lateinit var tvEmptyGoalsDesc: TextView
    private lateinit var rvGoals: RecyclerView
    private lateinit var fabCreateGoal: FloatingActionButton

    private lateinit var adapter: GoalsAdapter
    private var allGoals: List<Goal> = emptyList()
    private var pollJob: Job? = null

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View? {
        val view = inflater.inflate(R.layout.fragment_goals, container, false)
        bindViews(view)
        setupRecyclerView()
        setupListeners()
        observeGoals()
        return view
    }

    override fun onResume() {
        super.onResume()
        if (!isHidden) {
            refreshGoals()
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
            refreshGoals()
            startPolling()
        }
    }

    override fun onDestroyView() {
        super.onDestroyView()
        stopPolling()
    }

    private fun startPolling() {
        stopPolling()
        pollJob = viewLifecycleOwner.lifecycleScope.launch {
            while (isActive) {
                delay(30000)
                val app = (activity?.application as? OnuslyApplication) ?: break
                if (!app.authManager.isSignedIn()) break
                app.goalsRepository.fetchGoals()
            }
        }
    }

    private fun stopPolling() {
        pollJob?.cancel()
        pollJob = null
    }

    private fun bindViews(view: View) {
        btnFilterAll = view.findViewById(R.id.btnFilterAll)
        btnFilterActive = view.findViewById(R.id.btnFilterActive)
        btnFilterReview = view.findViewById(R.id.btnFilterReview)
        btnFilterCompleted = view.findViewById(R.id.btnFilterCompleted)

        layoutGoalsOfflineBanner = view.findViewById(R.id.layoutGoalsOfflineBanner)
        btnRetryGoalsNetwork = view.findViewById(R.id.btnRetryGoalsNetwork)
        pbLoadingGoals = view.findViewById(R.id.pbLoadingGoals)
        layoutEmptyGoals = view.findViewById(R.id.layoutEmptyGoals)
        tvEmptyGoalsTitle = view.findViewById(R.id.tvEmptyGoalsTitle)
        tvEmptyGoalsDesc = view.findViewById(R.id.tvEmptyGoalsDesc)
        rvGoals = view.findViewById(R.id.rvGoals)
        fabCreateGoal = view.findViewById(R.id.fabCreateGoal)
    }

    private fun setupRecyclerView() {
        adapter = GoalsAdapter(
            onSubmitProofClick = { goal ->
                (activity as? MainActivity)?.showSubmitProofDialog(goal)
            },
            onEditGoalClick = { goal ->
                (activity as? MainActivity)?.showEditGoalDialog(goal)
            },
            onDeleteGoalClick = { goal ->
                confirmDeleteGoal(goal)
            },
            onGoalClick = { goal ->
                if (goal.isActive) {
                    (activity as? MainActivity)?.showEditGoalDialog(goal)
                } else {
                    (activity as? MainActivity)?.showProofHistoryDialog(goal)
                }
            },
            onProofHistoryClick = { goal ->
                (activity as? MainActivity)?.showProofHistoryDialog(goal)
            }
        )
        rvGoals.layoutManager = LinearLayoutManager(context)
        rvGoals.adapter = adapter
    }

    private fun setupListeners() {
        btnFilterAll.setOnClickListener { setFilter(GoalFilter.ALL) }
        btnFilterActive.setOnClickListener { setFilter(GoalFilter.ACTIVE) }
        btnFilterReview.setOnClickListener { setFilter(GoalFilter.REVIEW) }
        btnFilterCompleted.setOnClickListener { setFilter(GoalFilter.COMPLETED) }

        btnRetryGoalsNetwork.setOnClickListener {
            refreshGoals()
        }

        fabCreateGoal.setOnClickListener {
            (activity as? MainActivity)?.showCreateGoalDialog()
        }
    }

    private fun setFilter(filter: GoalFilter) {
        currentFilter = filter
        updateFilterButtonStyles()
        applyFilter()
    }

    private fun updateFilterButtonStyles() {
        val context = context ?: return
        val activeBg = ContextCompat.getColor(context, R.color.primary)
        val inactiveText = ContextCompat.getColor(context, R.color.text_secondary)
        val activeText = ContextCompat.getColor(context, R.color.text_primary)

        val buttons = listOf(
            Triple(btnFilterAll, GoalFilter.ALL, "All"),
            Triple(btnFilterActive, GoalFilter.ACTIVE, "Active"),
            Triple(btnFilterReview, GoalFilter.REVIEW, "In Review"),
            Triple(btnFilterCompleted, GoalFilter.COMPLETED, "Completed")
        )

        for ((btn, filter, _) in buttons) {
            if (filter == currentFilter) {
                btn.setBackgroundColor(activeBg)
                btn.setTextColor(activeText)
                btn.strokeWidth = 0
            } else {
                btn.setBackgroundColor(ContextCompat.getColor(context, android.R.color.transparent))
                btn.setTextColor(inactiveText)
                btn.strokeColor = ContextCompat.getColorStateList(context, R.color.surface_card)
                btn.strokeWidth = 2
            }
        }
    }

    private fun observeGoals() {
        viewLifecycleOwner.lifecycleScope.launch {
            val app = requireActivity().application as OnuslyApplication
            app.goalsRepository.goalsFlow.collect { goals ->
                allGoals = goals
                applyFilter()
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            val app = requireActivity().application as OnuslyApplication
            app.goalsRepository.isOfflineFlow.collect { isOffline ->
                layoutGoalsOfflineBanner.visibility = if (isOffline) View.VISIBLE else View.GONE
            }
        }
    }

    private fun applyFilter() {
        val filtered = when (currentFilter) {
            GoalFilter.ALL -> allGoals
            GoalFilter.ACTIVE -> allGoals.filter { it.isActive }
            GoalFilter.REVIEW -> allGoals.filter { it.isProofSubmitted }
            GoalFilter.COMPLETED -> allGoals.filter { it.isCompleted }
        }

        adapter.submitList(filtered)

        if (filtered.isEmpty()) {
            layoutEmptyGoals.visibility = View.VISIBLE
            rvGoals.visibility = View.GONE
            when (currentFilter) {
                GoalFilter.ALL -> {
                    tvEmptyGoalsTitle.text = "No Goals Yet"
                    tvEmptyGoalsDesc.text = "Tap the + button below to create your first accountability goal."
                }
                GoalFilter.ACTIVE -> {
                    tvEmptyGoalsTitle.text = "No Active Goals"
                    tvEmptyGoalsDesc.text = "You have no active goals in progress."
                }
                GoalFilter.REVIEW -> {
                    tvEmptyGoalsTitle.text = "No Goals in Review"
                    tvEmptyGoalsDesc.text = "No goals currently waiting for partner review."
                }
                GoalFilter.COMPLETED -> {
                    tvEmptyGoalsTitle.text = "No Completed Goals"
                    tvEmptyGoalsDesc.text = "Goals verified by your friend approver will appear here."
                }
            }
        } else {
            layoutEmptyGoals.visibility = View.GONE
            rvGoals.visibility = View.VISIBLE
        }
    }

    fun refreshGoals() {
        val app = (activity?.application as? OnuslyApplication) ?: return
        if (!app.authManager.isSignedIn()) {
            allGoals = emptyList()
            applyFilter()
            layoutGoalsOfflineBanner.visibility = View.GONE
            return
        }

        pbLoadingGoals.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            val result = app.goalsRepository.fetchGoals()
            pbLoadingGoals.visibility = View.GONE
            result.onFailure { err ->
                context?.let {
                    Toast.makeText(it, "Could not load goals: ${err.message}", Toast.LENGTH_SHORT).show()
                }
            }
        }
    }

    private fun showError(message: String) {
        val ctx = context ?: return
        AlertDialog.Builder(ctx)
            .setTitle("Cannot Delete Goal")
            .setMessage(message)
            .setPositiveButton("OK", null)
            .show()
    }

    companion object {
        fun canDeleteGoal(goal: Goal): Boolean {
            return !(goal.isActive || goal.isProofSubmitted)
        }
    }

    private fun confirmDeleteGoal(goal: Goal) {
        val context = context ?: return

        // Prevent deletion of active or in-review goals
        if (!canDeleteGoal(goal)) {
            showError("Cannot delete an active or in-review goal.")
            return
        }

        AlertDialog.Builder(context)
            .setTitle("Delete Goal")
            .setMessage("Are you sure you want to delete \"${goal.title}\"?")
            .setPositiveButton("Delete") { _, _ ->
                deleteGoal(goal)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun deleteGoal(goal: Goal) {
        val app = (activity?.application as? OnuslyApplication) ?: return
        viewLifecycleOwner.lifecycleScope.launch {
            val result = app.goalsRepository.deleteGoal(goal.id)
            result.onSuccess { mutation ->
                if (!mutation.syncSucceeded) {
                    context?.let {
                        AlertDialog.Builder(it)
                            .setTitle("Goal Deleted (Sync Pending)")
                            .setMessage("Goal deleted on server, but local blocklist sync failed (${mutation.syncError ?: "network error"}). Tap 'Sync Now' on Home to retry.")
                            .setPositiveButton("Retry Sync Now") { _, _ ->
                                viewLifecycleOwner.lifecycleScope.launch { app.goalsRepository.retryBlocklistSync() }
                            }
                            .setNegativeButton("Later", null)
                            .show()
                    }
                } else {
                    Toast.makeText(context, "Goal deleted and blocklist synced", Toast.LENGTH_SHORT).show()
                }
            }.onFailure { err ->
                context?.let {
                    AlertDialog.Builder(it)
                        .setTitle("Cannot Delete Goal")
                        .setMessage(err.message ?: "Failed to delete goal")
                        .setPositiveButton("OK", null)
                        .show()
                }
            }
        }
    }
}
