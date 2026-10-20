package com.onusly.app.ui

import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import com.google.android.material.button.MaterialButton
import com.google.android.material.card.MaterialCardView
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.SyncRepository
import com.onusly.app.service.OnuslyAccessibilityService
import kotlinx.coroutines.launch

class SettingsFragment : Fragment() {

    private lateinit var tvProfileName: TextView
    private lateinit var tvProfileEmail: TextView
    private lateinit var tvProfileHandle: TextView
    private lateinit var btnEditHandle: MaterialButton

    private lateinit var tvBaseUrl: TextView
    private lateinit var tvBaseUrlBadge: TextView
    private lateinit var btnEditBaseUrl: MaterialButton

    private lateinit var cardSettingsAccessibility: MaterialCardView
    private lateinit var tvAccessibilityStatus: TextView
    private lateinit var tvAccessibilityDiagnostics: TextView
    private lateinit var btnSettingsAccessibility: MaterialButton

    private lateinit var btnSettingsSignOut: MaterialButton

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View? {
        val view = inflater.inflate(R.layout.fragment_settings, container, false)
        bindViews(view)
        setupListeners()
        return view
    }

    override fun onResume() {
        super.onResume()
        updateProfileUi()
        updateBaseUrlUi()
        updateAccessibilityUi()
    }

    private fun bindViews(view: View) {
        tvProfileName = view.findViewById(R.id.tvProfileName)
        tvProfileEmail = view.findViewById(R.id.tvProfileEmail)
        tvProfileHandle = view.findViewById(R.id.tvProfileHandle)
        btnEditHandle = view.findViewById(R.id.btnEditHandle)

        tvBaseUrl = view.findViewById(R.id.tvBaseUrl)
        tvBaseUrlBadge = view.findViewById(R.id.tvBaseUrlBadge)
        btnEditBaseUrl = view.findViewById(R.id.btnEditBaseUrl)

        cardSettingsAccessibility = view.findViewById(R.id.cardSettingsAccessibility)
        tvAccessibilityStatus = view.findViewById(R.id.tvAccessibilityStatus)
        tvAccessibilityDiagnostics = view.findViewById(R.id.tvAccessibilityDiagnostics)
        btnSettingsAccessibility = view.findViewById(R.id.btnSettingsAccessibility)

        btnSettingsSignOut = view.findViewById(R.id.btnSettingsSignOut)
    }

    private fun setupListeners() {
        btnEditHandle.setOnClickListener {
            showEditHandleDialog()
        }

        btnEditBaseUrl.setOnClickListener {
            showEditBaseUrlDialog()
        }

        btnSettingsAccessibility.setOnClickListener {
            val intent = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)
            startActivity(intent)
        }

        btnSettingsSignOut.setOnClickListener {
            val mainActivity = activity as? MainActivity
            if (mainActivity != null) {
                mainActivity.onUserLoggedOut()
            } else {
                val app = (activity?.application as? OnuslyApplication) ?: return@setOnClickListener
                app.syncRepository.onUserSignOut()
                app.goalsRepository.clearLocalData()
                updateProfileUi()
            }
            Toast.makeText(context, "Signed out", Toast.LENGTH_SHORT).show()
        }
    }

    fun updateProfileUi() {
        val app = (activity?.application as? OnuslyApplication) ?: return
        val session = app.authManager.getSession()
        if (session != null && session.userId.isNotBlank()) {
            tvProfileName.text = if (session.name.isNotBlank()) session.name else "Authenticated User"
            tvProfileEmail.text = session.email

            // Fetch live profile from backend to get handle
            viewLifecycleOwner.lifecycleScope.launch {
                val profileResult = app.goalsRepository.getMyProfile()
                profileResult.onSuccess { profile ->
                    if (profile.handle.isNotBlank()) {
                        tvProfileHandle.text = if (profile.handle.startsWith("@")) profile.handle else "@${profile.handle}"
                    } else {
                        tvProfileHandle.text = "@no_handle_set"
                    }
                    if (profile.name.isNotBlank()) {
                        tvProfileName.text = profile.name
                    }
                }.onFailure {
                    tvProfileHandle.text = "@unknown"
                }
            }
        } else {
            tvProfileName.text = "Not Signed In"
            tvProfileEmail.text = "Sign in from Home to manage your profile."
            tvProfileHandle.text = "—"
        }
    }

    private fun updateBaseUrlUi() {
        val app = (activity?.application as? OnuslyApplication) ?: return
        val context = context ?: return
        val url = app.syncRepository.getBaseUrl()
        tvBaseUrl.text = url

        if (app.syncRepository.isDefaultBaseUrl()) {
            tvBaseUrlBadge.text = "DEFAULT"
            tvBaseUrlBadge.setTextColor(ContextCompat.getColor(context, R.color.primary))
        } else {
            tvBaseUrlBadge.text = "CUSTOM"
            tvBaseUrlBadge.setTextColor(ContextCompat.getColor(context, R.color.warning))
        }
    }

    private fun updateAccessibilityUi() {
        val context = context ?: return
        val isEnabled = OnuslyAccessibilityService.isServiceEnabled(context)
        if (isEnabled) {
            tvAccessibilityStatus.text = "ACTIVE"
            tvAccessibilityStatus.setTextColor(ContextCompat.getColor(context, R.color.success))
            cardSettingsAccessibility.strokeColor = ContextCompat.getColor(context, R.color.success)
            cardSettingsAccessibility.strokeWidth = 2
            tvAccessibilityDiagnostics.text = "Service is active and enforcing blocklists. Window-state monitoring is operational with canRetrieveWindowContent=false to protect your privacy."
        } else {
            tvAccessibilityStatus.text = "DISABLED"
            tvAccessibilityStatus.setTextColor(ContextCompat.getColor(context, R.color.warning))
            cardSettingsAccessibility.strokeColor = ContextCompat.getColor(context, R.color.warning)
            cardSettingsAccessibility.strokeWidth = 2
            tvAccessibilityDiagnostics.text = "Enforcement is currently paused. Android requires users to manually enable Accessibility Services in System Settings.\n\nTap below to open Accessibility Settings, find ONUSLY, and switch it ON.\n\nNote: If Android 13+ restricts settings, open App Info > ⋮ (menu) > 'Allow restricted settings'."
        }
    }

    private fun showEditHandleDialog() {
        val context = context ?: return
        val app = (activity?.application as? OnuslyApplication) ?: return
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(context, "Sign in first to edit your handle", Toast.LENGTH_SHORT).show()
            return
        }

        val dialogView = LayoutInflater.from(context).inflate(R.layout.dialog_edit_handle, null)
        val tvEditHandleError = dialogView.findViewById<TextView>(R.id.tvEditHandleError)
        val etEditHandleInput = dialogView.findViewById<EditText>(R.id.etEditHandleInput)
        val pbSavingHandle = dialogView.findViewById<ProgressBar>(R.id.pbSavingHandle)
        val btnCancelEditHandle = dialogView.findViewById<MaterialButton>(R.id.btnCancelEditHandle)
        val btnSaveEditHandle = dialogView.findViewById<MaterialButton>(R.id.btnSaveEditHandle)

        val currentHandle = tvProfileHandle.text.toString().removePrefix("@")
        if (currentHandle != "no_handle_set" && currentHandle != "—" && currentHandle != "unknown") {
            etEditHandleInput.setText(currentHandle)
            etEditHandleInput.setSelection(currentHandle.length)
        }

        val dialog = AlertDialog.Builder(context)
            .setView(dialogView)
            .create()

        dialog.window?.setBackgroundDrawableResource(android.R.color.transparent)

        btnCancelEditHandle.setOnClickListener {
            dialog.dismiss()
        }

        btnSaveEditHandle.setOnClickListener {
            val rawHandle = etEditHandleInput.text.toString().trim()
            val cleanHandle = rawHandle.removePrefix("@").trim()

            val validHandleRegex = Regex("^[a-zA-Z0-9_.-]{2,30}$")
            if (!validHandleRegex.matches(cleanHandle)) {
                tvEditHandleError.text = "Handle must be 2-30 characters and only contain letters, numbers, _, -, or ."
                tvEditHandleError.visibility = View.VISIBLE
                return@setOnClickListener
            }

            tvEditHandleError.visibility = View.GONE
            pbSavingHandle.visibility = View.VISIBLE
            btnSaveEditHandle.isEnabled = false
            btnCancelEditHandle.isEnabled = false

            viewLifecycleOwner.lifecycleScope.launch {
                val result = app.goalsRepository.updateHandle(cleanHandle)
                pbSavingHandle.visibility = View.GONE
                btnSaveEditHandle.isEnabled = true
                btnCancelEditHandle.isEnabled = true

                result.onSuccess { updated ->
                    tvProfileHandle.text = "@${updated.handle}"
                    Toast.makeText(context, "Handle updated: @${updated.handle}", Toast.LENGTH_SHORT).show()
                    dialog.dismiss()
                }.onFailure { err ->
                    // Preserve input, keep dialog open, display error
                    tvEditHandleError.text = err.message ?: "Failed to update handle"
                    tvEditHandleError.visibility = View.VISIBLE
                }
            }
        }

        dialog.show()
    }

    private fun showEditBaseUrlDialog() {
        val context = context ?: return
        val app = (activity?.application as? OnuslyApplication) ?: return
        val currentUrl = app.syncRepository.getBaseUrl()

        val dialogView = LayoutInflater.from(context).inflate(R.layout.dialog_edit_base_url, null)
        val tvEditBaseUrlError = dialogView.findViewById<TextView>(R.id.tvEditBaseUrlError)
        val etEditBaseUrlInput = dialogView.findViewById<EditText>(R.id.etEditBaseUrlInput)
        val btnResetDefaultUrl = dialogView.findViewById<MaterialButton>(R.id.btnResetDefaultUrl)
        val btnCancelEditBaseUrl = dialogView.findViewById<MaterialButton>(R.id.btnCancelEditBaseUrl)
        val btnSaveEditBaseUrl = dialogView.findViewById<MaterialButton>(R.id.btnSaveEditBaseUrl)

        etEditBaseUrlInput.setText(currentUrl)
        etEditBaseUrlInput.setSelection(currentUrl.length)

        val dialog = AlertDialog.Builder(context)
            .setView(dialogView)
            .create()

        dialog.window?.setBackgroundDrawableResource(android.R.color.transparent)

        btnResetDefaultUrl.setOnClickListener {
            etEditBaseUrlInput.setText(SyncRepository.DEFAULT_BASE_URL)
            etEditBaseUrlInput.setSelection(SyncRepository.DEFAULT_BASE_URL.length)
            tvEditBaseUrlError.visibility = View.GONE
        }

        btnCancelEditBaseUrl.setOnClickListener {
            dialog.dismiss()
        }

        btnSaveEditBaseUrl.setOnClickListener {
            val inputUrl = etEditBaseUrlInput.text.toString().trim()

            if (!SyncRepository.isValidBaseUrl(inputUrl)) {
                tvEditBaseUrlError.text = "Invalid URL. Please enter a valid http:// or https:// URL (e.g. http://10.0.2.2:8080)."
                tvEditBaseUrlError.visibility = View.VISIBLE
                return@setOnClickListener
            }

            if (inputUrl == currentUrl) {
                dialog.dismiss()
                return@setOnClickListener
            }

            AlertDialog.Builder(context)
                .setTitle("Change Server & Sign Out?")
                .setMessage("Connecting to a new server ($inputUrl) will sign you out and clear all local cached blocklists to prevent credential exposure.\n\nAre you sure you want to proceed?")
                .setPositiveButton("Yes, Change Server") { _, _ ->
                    try {
                        app.syncRepository.setBaseUrl(inputUrl)
                        dialog.dismiss()
                        (activity as? MainActivity)?.onUserLoggedOut()
                        updateBaseUrlUi()
                        Toast.makeText(context, "API server updated. Please sign in.", Toast.LENGTH_LONG).show()
                    } catch (e: Exception) {
                        tvEditBaseUrlError.text = e.message ?: "Invalid URL"
                        tvEditBaseUrlError.visibility = View.VISIBLE
                    }
                }
                .setNegativeButton("Cancel", null)
                .show()
        }

        dialog.show()
    }
}
