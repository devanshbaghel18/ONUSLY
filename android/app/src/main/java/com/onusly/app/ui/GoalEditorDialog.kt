package com.onusly.app.ui

import android.app.Dialog
import android.content.Context
import android.content.res.ColorStateList
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.core.content.ContextCompat
import com.google.android.material.button.MaterialButton
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.onusly.app.R
import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.TargetCatalog
import com.onusly.app.data.model.UpdateGoalRequest
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class GoalEditorDialog(
    private val context: Context,
    private val scope: CoroutineScope,
    private val goalsRepository: GoalsRepository,
    private val existingGoal: Goal? = null,
    private val onGoalSaved: (Goal, Boolean, String?) -> Unit
) {

    private val dialog: Dialog = Dialog(context, android.R.style.Theme_Material_Dialog)

    private val existingApps = existingGoal?.blockedApps?.toSet() ?: emptySet()
    private val existingDomains = existingGoal?.blockedDomains?.toSet() ?: emptySet()

    private val newlySelectedApps = mutableSetOf<String>()
    private val newlyAddedDomains = mutableSetOf<String>()

    private lateinit var tvEditorModalTitle: TextView
    private lateinit var tvEditorModalSubtitle: TextView
    private lateinit var tvEditorError: TextView
    private lateinit var etGoalTitle: EditText
    private lateinit var etGoalDesc: EditText
    private lateinit var btnPickApps: MaterialButton
    private lateinit var chipGroupBlockedApps: ChipGroup
    private lateinit var etDomainInput: EditText
    private lateinit var btnAddDomain: MaterialButton
    private lateinit var chipGroupBlockedDomains: ChipGroup
    private lateinit var bannerAccountabilityRequired: View
    private lateinit var tvApproverLabel: TextView
    private lateinit var etApproverEmail: EditText
    private lateinit var pbEditorSaving: ProgressBar
    private lateinit var btnCancelGoal: MaterialButton
    private lateinit var btnSaveGoal: MaterialButton

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_goal_editor, null)
        dialog.setContentView(view)
        dialog.window?.setLayout(
            (context.resources.displayMetrics.widthPixels * 0.95).toInt(),
            ViewGroup.LayoutParams.WRAP_CONTENT
        )

        bindViews(view)
        setupInitialData()
        setupListeners()
        updateAccountabilityState()

        dialog.show()
    }

    private fun bindViews(view: View) {
        tvEditorModalTitle = view.findViewById(R.id.tvEditorModalTitle)
        tvEditorModalSubtitle = view.findViewById(R.id.tvEditorModalSubtitle)
        tvEditorError = view.findViewById(R.id.tvEditorError)
        etGoalTitle = view.findViewById(R.id.etGoalTitle)
        etGoalDesc = view.findViewById(R.id.etGoalDesc)
        btnPickApps = view.findViewById(R.id.btnPickApps)
        chipGroupBlockedApps = view.findViewById(R.id.chipGroupBlockedApps)
        etDomainInput = view.findViewById(R.id.etDomainInput)
        btnAddDomain = view.findViewById(R.id.btnAddDomain)
        chipGroupBlockedDomains = view.findViewById(R.id.chipGroupBlockedDomains)
        bannerAccountabilityRequired = view.findViewById(R.id.bannerAccountabilityRequired)
        tvApproverLabel = view.findViewById(R.id.tvApproverLabel)
        etApproverEmail = view.findViewById(R.id.etApproverEmail)
        pbEditorSaving = view.findViewById(R.id.pbEditorSaving)
        btnCancelGoal = view.findViewById(R.id.btnCancelGoal)
        btnSaveGoal = view.findViewById(R.id.btnSaveGoal)
    }

    private fun setupInitialData() {
        etDomainInput.hint = "e.g. reddit.com (enforced via Chrome extension)"
        if (existingGoal != null) {
            tvEditorModalTitle.text = "Edit Goal"
            tvEditorModalSubtitle.text = "Add new distraction targets or update goal details. Existing targets are locked."
            etGoalTitle.setText(existingGoal.title)
            etGoalDesc.setText(existingGoal.description)
            etApproverEmail.setText(existingGoal.approverEmail)
            // If editing an existing goal that has NO approver, keep enabled so user can assign when upgrading to targeted
            etApproverEmail.isEnabled = existingGoal.approverEmail.isBlank()

            // Render existing locked app chips
            for (app in existingApps) {
                addAppChip(app, isLocked = true)
            }

            // Render existing locked domain chips
            for (domain in existingDomains) {
                addDomainChip(domain, isLocked = true)
            }
        } else {
            tvEditorModalTitle.text = "Create New Goal"
            tvEditorModalSubtitle.text = "Define your commitment. Distraction targets require friend verification."
        }
    }

    private fun setupListeners() {
        btnPickApps.setOnClickListener {
            val allCurrentApps = existingApps + newlySelectedApps
            showInstalledAppsPicker(allCurrentApps)
        }

        btnAddDomain.setOnClickListener {
            val domain = etDomainInput.text.toString().trim().lowercase()
                .removePrefix("http://")
                .removePrefix("https://")
                .trimEnd('/')

            if (domain.isBlank()) return@setOnClickListener
            if (existingDomains.contains(domain) || newlyAddedDomains.contains(domain)) {
                Toast.makeText(context, "Domain already added", Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }

            newlyAddedDomains.add(domain)
            addDomainChip(domain, isLocked = false)
            etDomainInput.text.clear()
            updateAccountabilityState()
        }

        btnCancelGoal.setOnClickListener {
            dialog.dismiss()
        }

        btnSaveGoal.setOnClickListener {
            saveGoal()
        }
    }

    private fun showInstalledAppsPicker(currentlySelected: Set<String>) {
        val picker = InstalledAppsPickerDialog(
            context = context,
            initialSelected = currentlySelected,
            onSelected = { selectedSet ->
                // Keep existing locked apps
                val newApps = selectedSet.filterNot { existingApps.contains(it) }.toSet()
                newlySelectedApps.clear()
                newlySelectedApps.addAll(newApps)

                // Refresh app chips
                chipGroupBlockedApps.removeAllViews()
                for (app in existingApps) {
                    addAppChip(app, isLocked = true)
                }
                for (app in newlySelectedApps) {
                    addAppChip(app, isLocked = false)
                    val associatedDomains = TargetCatalog.getDomainsForApp(app)
                    for (domain in associatedDomains) {
                        if (!existingDomains.contains(domain) && !newlyAddedDomains.contains(domain)) {
                            newlyAddedDomains.add(domain)
                            addDomainChip(domain, isLocked = false)
                        }
                    }
                }
                updateAccountabilityState()
            }
        )
        picker.show()
    }

    private fun addAppChip(packageName: String, isLocked: Boolean) {
        val chip = Chip(context).apply {
            text = packageName
            isCloseIconVisible = !isLocked
            setChipBackgroundColorResource(if (isLocked) R.color.surface_card else R.color.surface)
            setTextColor(ContextCompat.getColor(context, if (isLocked) R.color.warning else R.color.text_primary))
            chipStrokeColor = ColorStateList.valueOf(
                ContextCompat.getColor(context, if (isLocked) R.color.warning else R.color.primary)
            )
            chipStrokeWidth = 2f

            if (isLocked) {
                setChipIconResource(R.drawable.ic_nav_reviews)
                setChipIconTintResource(R.color.warning)
            } else {
                setCloseIconTintResource(R.color.text_muted)
                setOnCloseIconClickListener {
                    newlySelectedApps.remove(packageName)
                    chipGroupBlockedApps.removeView(this)
                    updateAccountabilityState()
                }
            }
        }
        chipGroupBlockedApps.addView(chip)
    }

    private fun addDomainChip(domain: String, isLocked: Boolean) {
        val chip = Chip(context).apply {
            text = domain
            isCloseIconVisible = !isLocked
            setChipBackgroundColorResource(if (isLocked) R.color.surface_card else R.color.surface)
            setTextColor(ContextCompat.getColor(context, if (isLocked) R.color.warning else R.color.text_primary))
            chipStrokeColor = ColorStateList.valueOf(
                ContextCompat.getColor(context, if (isLocked) R.color.warning else R.color.primary)
            )
            chipStrokeWidth = 2f

            if (isLocked) {
                setChipIconResource(R.drawable.ic_nav_reviews)
                setChipIconTintResource(R.color.warning)
            } else {
                setCloseIconTintResource(R.color.text_muted)
                setOnCloseIconClickListener {
                    newlyAddedDomains.remove(domain)
                    chipGroupBlockedDomains.removeView(this)
                    updateAccountabilityState()
                }
            }
        }
        chipGroupBlockedDomains.addView(chip)
    }

    private fun updateAccountabilityState() {
        val hasTargets = (existingApps.isNotEmpty() || newlySelectedApps.isNotEmpty() ||
                existingDomains.isNotEmpty() || newlyAddedDomains.isNotEmpty())

        if (hasTargets) {
            bannerAccountabilityRequired.visibility = View.VISIBLE
            tvApproverLabel.text = "FRIEND APPROVER *"
            tvApproverLabel.setTextColor(ContextCompat.getColor(context, R.color.warning))
        } else {
            bannerAccountabilityRequired.visibility = View.GONE
            tvApproverLabel.text = "FRIEND APPROVER (OPTIONAL)"
            tvApproverLabel.setTextColor(ContextCompat.getColor(context, R.color.text_muted))
        }
    }

    private fun saveGoal() {
        val title = etGoalTitle.text.toString().trim()
        val desc = etGoalDesc.text.toString().trim()
        val approver = etApproverEmail.text.toString().trim()

        tvEditorError.visibility = View.GONE

        if (title.isBlank()) {
            showError("Goal title cannot be empty.")
            return
        }

        // Ensure associated domains from TargetCatalog for all selected apps are added to newlyAddedDomains
        for (app in newlySelectedApps) {
            val associatedDomains = TargetCatalog.getDomainsForApp(app)
            for (domain in associatedDomains) {
                if (!existingDomains.contains(domain) && !newlyAddedDomains.contains(domain)) {
                    newlyAddedDomains.add(domain)
                }
            }
        }

        val allApps = (existingApps + newlySelectedApps).toList()
        val allDomains = (existingDomains + newlyAddedDomains).toList()
        val hasTargets = allApps.isNotEmpty() || allDomains.isNotEmpty()

        if (hasTargets && approver.isBlank()) {
            showError("Friend Approver email or @handle is required when distraction targets are set.")
            return
        }

        setSavingState(true)

        scope.launch {
            if (existingGoal == null) {
                // CREATE GOAL
                val approvalType = if (hasTargets) "friend" else if (approver.isNotBlank()) "friend" else "none"
                val req = CreateGoalRequest(
                    title = title,
                    description = desc,
                    approvalType = approvalType,
                    approverEmail = approver.ifBlank { null },
                    blockedApps = allApps,
                    blockedDomains = allDomains
                )

                val result = goalsRepository.createGoal(req)
                withContext(Dispatchers.Main) {
                    setSavingState(false)
                    result.onSuccess { mutation ->
                        dialog.dismiss()
                        onGoalSaved(mutation.value, mutation.syncSucceeded, mutation.syncError)
                    }.onFailure { err ->
                        showError(formatErrorMessage(err))
                    }
                }
            } else {
                // UPDATE GOAL (Add-Only targets)
                val shouldSendAccountability = existingGoal.approverEmail.isBlank() && (hasTargets || approver.isNotBlank())
                val req = UpdateGoalRequest(
                    title = title,
                    description = desc,
                    approvalType = if (shouldSendAccountability) "friend" else null,
                    approverEmail = if (shouldSendAccountability) approver.ifBlank { null } else null,
                    blockedApps = allApps,
                    blockedDomains = allDomains
                )

                val result = goalsRepository.updateGoal(existingGoal.id, req)
                withContext(Dispatchers.Main) {
                    setSavingState(false)
                    result.onSuccess { mutation ->
                        dialog.dismiss()
                        onGoalSaved(mutation.value, mutation.syncSucceeded, mutation.syncError)
                    }.onFailure { err ->
                        showError(formatErrorMessage(err))
                    }
                }
            }
        }
    }

    private fun setSavingState(saving: Boolean) {
        pbEditorSaving.visibility = if (saving) View.VISIBLE else View.GONE
        btnSaveGoal.isEnabled = !saving
        btnCancelGoal.isEnabled = !saving
        btnPickApps.isEnabled = !saving
        btnAddDomain.isEnabled = !saving
    }

    private fun showError(message: String) {
        tvEditorError.text = message
        tvEditorError.visibility = View.VISIBLE
    }

    private fun formatErrorMessage(err: Throwable): String {
        return when (err) {
            is OnuslyApiException -> {
                when (err.message.trim()) {
                    "accountability_required" -> "Accountability Required: Distraction targets cannot be set with approvalType 'none'. Please provide a valid friend approver."
                    "targets_locked" -> "Targets are locked: Existing distraction targets on an active goal cannot be removed."
                    "goal_locked" -> "Goal is locked: Active goals with targets cannot be modified in an invalid state."
                    else -> err.message.trim()
                }
            }
            else -> err.message?.trim() ?: "Failed to save goal"
        }
    }
}
