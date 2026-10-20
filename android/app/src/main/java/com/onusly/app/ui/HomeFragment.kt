package com.onusly.app.ui

import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import com.google.android.material.button.MaterialButton
import com.google.android.material.card.MaterialCardView
import com.onusly.app.BuildConfig
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.SyncResult
import com.onusly.app.service.OnuslyAccessibilityService
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class HomeFragment : Fragment() {

    private lateinit var cardAccessibility: MaterialCardView
    private lateinit var ivAccessibilityIcon: ImageView
    private lateinit var tvAccessibilityTitle: TextView
    private lateinit var tvAccessibilityDesc: TextView
    private lateinit var tvRestrictedSettingsNote: TextView
    private lateinit var btnEnableAccessibility: MaterialButton

    private lateinit var layoutSignedIn: LinearLayout
    private lateinit var layoutSignedOut: LinearLayout
    private lateinit var tvUserName: TextView
    private lateinit var tvUserEmail: TextView
    private lateinit var btnSignOut: MaterialButton
    private lateinit var btnSignInGoogle: MaterialButton
    private lateinit var btnManualToken: MaterialButton

    private lateinit var pbSyncing: ProgressBar
    private lateinit var tvLastSyncTime: TextView
    private lateinit var tvVersion: TextView
    private lateinit var btnSyncNow: MaterialButton

    private lateinit var tvBlockedCountBadge: TextView
    private lateinit var tvBlockedList: TextView

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View? {
        val view = inflater.inflate(R.layout.fragment_home, container, false)
        bindViews(view)
        setupListeners()
        observeData()
        return view
    }

    override fun onResume() {
        super.onResume()
        updateAccessibilityState()
        updateAccountState()
        val app = requireActivity().application as OnuslyApplication
        updateBlocklistUi(app.syncRepository.getActiveCache())
    }

    private fun bindViews(view: View) {
        cardAccessibility = view.findViewById(R.id.cardAccessibility)
        ivAccessibilityIcon = view.findViewById(R.id.ivAccessibilityIcon)
        tvAccessibilityTitle = view.findViewById(R.id.tvAccessibilityTitle)
        tvAccessibilityDesc = view.findViewById(R.id.tvAccessibilityDesc)
        tvRestrictedSettingsNote = view.findViewById(R.id.tvRestrictedSettingsNote)
        btnEnableAccessibility = view.findViewById(R.id.btnEnableAccessibility)

        layoutSignedIn = view.findViewById(R.id.layoutSignedIn)
        layoutSignedOut = view.findViewById(R.id.layoutSignedOut)
        tvUserName = view.findViewById(R.id.tvUserName)
        tvUserEmail = view.findViewById(R.id.tvUserEmail)
        btnSignOut = view.findViewById(R.id.btnSignOut)
        btnSignInGoogle = view.findViewById(R.id.btnSignInGoogle)
        btnManualToken = view.findViewById(R.id.btnManualToken)
        btnManualToken.visibility = if (BuildConfig.DEBUG) View.VISIBLE else View.GONE

        pbSyncing = view.findViewById(R.id.pbSyncing)
        tvLastSyncTime = view.findViewById(R.id.tvLastSyncTime)
        tvVersion = view.findViewById(R.id.tvVersion)
        btnSyncNow = view.findViewById(R.id.btnSyncNow)

        tvBlockedCountBadge = view.findViewById(R.id.tvBlockedCountBadge)
        tvBlockedList = view.findViewById(R.id.tvBlockedList)
    }

    private fun setupListeners() {
        btnEnableAccessibility.setOnClickListener {
            val intent = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)
            startActivity(intent)
        }

        btnSignInGoogle.setOnClickListener {
            (activity as? MainActivity)?.launchGoogleSignIn()
        }

        btnSignOut.setOnClickListener {
            val mainActivity = activity as? MainActivity
            if (mainActivity != null) {
                mainActivity.onUserLoggedOut()
            } else {
                val app = requireActivity().application as OnuslyApplication
                app.syncRepository.onUserSignOut()
                app.goalsRepository.clearLocalData()
                updateAccountState()
                updateBlocklistUi(null)
            }
            Toast.makeText(context, "Signed out", Toast.LENGTH_SHORT).show()
        }

        btnManualToken.setOnClickListener {
            if (BuildConfig.DEBUG) {
                (activity as? MainActivity)?.showManualTokenDialog()
            }
        }

        btnSyncNow.setOnClickListener {
            performSync()
        }
    }

    private fun observeData() {
        viewLifecycleOwner.lifecycleScope.launch {
            val app = requireActivity().application as OnuslyApplication
            app.syncRepository.cachedBlocklistFlow.collect { cached ->
                updateBlocklistUi(cached)
            }
        }
    }

    fun updateAccessibilityState() {
        val context = context ?: return
        val isEnabled = OnuslyAccessibilityService.isServiceEnabled(context)
        if (isEnabled) {
            cardAccessibility.setCardBackgroundColor(ContextCompat.getColor(context, R.color.success_bg))
            cardAccessibility.strokeColor = ContextCompat.getColor(context, R.color.success)
            ivAccessibilityIcon.setImageResource(android.R.drawable.checkbox_on_background)
            ivAccessibilityIcon.setColorFilter(ContextCompat.getColor(context, R.color.success))
            tvAccessibilityTitle.text = getString(R.string.accessibility_enabled_title)
            tvAccessibilityTitle.setTextColor(ContextCompat.getColor(context, R.color.success))
            tvAccessibilityDesc.text = getString(R.string.accessibility_enabled_desc)
            tvRestrictedSettingsNote.visibility = View.GONE
            btnEnableAccessibility.visibility = View.GONE
        } else {
            cardAccessibility.setCardBackgroundColor(ContextCompat.getColor(context, R.color.warning_bg))
            cardAccessibility.strokeColor = ContextCompat.getColor(context, R.color.warning)
            ivAccessibilityIcon.setImageResource(android.R.drawable.ic_dialog_alert)
            ivAccessibilityIcon.setColorFilter(ContextCompat.getColor(context, R.color.warning))
            tvAccessibilityTitle.text = getString(R.string.accessibility_disabled_title)
            tvAccessibilityTitle.setTextColor(ContextCompat.getColor(context, R.color.warning))
            tvAccessibilityDesc.text = getString(R.string.accessibility_disabled_desc)
            tvRestrictedSettingsNote.visibility = View.VISIBLE
            btnEnableAccessibility.visibility = View.VISIBLE
        }
    }

    fun updateAccountState() {
        val app = (activity?.application as? OnuslyApplication) ?: return
        val session = app.authManager.getSession()
        if (session != null && session.userId.isNotBlank()) {
            layoutSignedIn.visibility = View.VISIBLE
            layoutSignedOut.visibility = View.GONE
            tvUserName.text = if (session.name.isNotBlank()) session.name else "Authenticated User"
            tvUserEmail.text = session.email
        } else {
            layoutSignedIn.visibility = View.GONE
            layoutSignedOut.visibility = View.VISIBLE
        }
    }

    fun updateBlocklistUi(cached: CachedBlocklist?) {
        if (!isAdded) return
        if (cached == null || cached.targets.isEmpty()) {
            tvBlockedCountBadge.text = "0"
            tvBlockedList.text = "No apps currently blocked."
            tvVersion.text = "None"
            tvLastSyncTime.text = if (cached?.lastSyncTimestamp ?: 0 > 0) {
                formatTimestamp(cached!!.lastSyncTimestamp)
            } else {
                "Never"
            }
            return
        }

        tvVersion.text = cached.version
        tvLastSyncTime.text = formatTimestamp(cached.lastSyncTimestamp)

        val uniqueApps = mutableSetOf<String>()
        val sb = java.lang.StringBuilder()

        for (target in cached.targets) {
            val statusTag = if (target.status.equals("active", ignoreCase = true)) "[ACTIVE]" else "[WAITING REVIEW]"
            sb.append("• ").append(target.goalTitle).append(" ").append(statusTag).append("\n")
            if (target.apps.isEmpty()) {
                sb.append("   (no package names configured)\n")
            } else {
                for (appPkg in target.apps) {
                    uniqueApps.add(appPkg)
                    sb.append("   - ").append(appPkg).append("\n")
                }
            }
            sb.append("\n")
        }

        tvBlockedCountBadge.text = uniqueApps.size.toString()
        tvBlockedList.text = sb.toString().trim()
    }

    private fun formatTimestamp(timestamp: Long): String {
        if (timestamp <= 0) return "Never"
        val sdf = SimpleDateFormat("HH:mm:ss (dd MMM)", Locale.getDefault())
        return sdf.format(Date(timestamp))
    }

    fun performSync() {
        val app = (activity?.application as? OnuslyApplication) ?: return
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(context, "Please sign in first", Toast.LENGTH_SHORT).show()
            return
        }

        pbSyncing.visibility = View.VISIBLE
        btnSyncNow.isEnabled = false

        viewLifecycleOwner.lifecycleScope.launch {
            val result = app.syncRepository.syncBlocklist()
            pbSyncing.visibility = View.GONE
            btnSyncNow.isEnabled = true

            when (result) {
                is SyncResult.Success -> {
                    Toast.makeText(context, "Blocklist synced: ${result.response.version}", Toast.LENGTH_SHORT).show()
                }
                is SyncResult.NotModified -> {
                    Toast.makeText(context, "Blocklist up-to-date (304 Not Modified)", Toast.LENGTH_SHORT).show()
                }
                is SyncResult.Error -> {
                    Toast.makeText(context, "Sync failed: ${result.message}", Toast.LENGTH_LONG).show()
                }
            }
            updateBlocklistUi(app.syncRepository.getActiveCache())
        }
    }
}
