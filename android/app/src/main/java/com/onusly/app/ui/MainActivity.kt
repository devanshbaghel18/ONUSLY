package com.onusly.app.ui

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.view.View
import android.widget.EditText
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.google.android.gms.auth.api.signin.GoogleSignIn
import com.google.android.gms.common.api.ApiException
import com.google.android.material.button.MaterialButton
import com.google.android.material.card.MaterialCardView
import com.onusly.app.BuildConfig
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.SyncResult
import com.onusly.app.data.model.UserSession
import com.onusly.app.service.OnuslyAccessibilityService
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class MainActivity : AppCompatActivity() {

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
    private lateinit var btnSettings: ImageButton

    private val googleSignInLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            val task = GoogleSignIn.getSignedInAccountFromIntent(result.data)
            try {
                val account = task.getResult(ApiException::class.java)
                val idToken = account?.idToken
                if (!idToken.isNullOrBlank()) {
                    handleGoogleToken(idToken)
                } else {
                    Toast.makeText(this, "Google Sign-In returned no ID token", Toast.LENGTH_LONG).show()
                }
            } catch (e: Exception) {
                Toast.makeText(this, "Google Sign-In failed: ${e.message}", Toast.LENGTH_LONG).show()
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        bindViews()
        setupListeners()
        observeData()
    }

    override fun onResume() {
        super.onResume()
        updateAccessibilityState()
        updateAccountState()
        updateBlocklistUi((application as OnuslyApplication).syncRepository.getActiveCache())
    }

    private fun bindViews() {
        cardAccessibility = findViewById(R.id.cardAccessibility)
        ivAccessibilityIcon = findViewById(R.id.ivAccessibilityIcon)
        tvAccessibilityTitle = findViewById(R.id.tvAccessibilityTitle)
        tvAccessibilityDesc = findViewById(R.id.tvAccessibilityDesc)
        tvRestrictedSettingsNote = findViewById(R.id.tvRestrictedSettingsNote)
        btnEnableAccessibility = findViewById(R.id.btnEnableAccessibility)

        layoutSignedIn = findViewById(R.id.layoutSignedIn)
        layoutSignedOut = findViewById(R.id.layoutSignedOut)
        tvUserName = findViewById(R.id.tvUserName)
        tvUserEmail = findViewById(R.id.tvUserEmail)
        btnSignOut = findViewById(R.id.btnSignOut)
        btnSignInGoogle = findViewById(R.id.btnSignInGoogle)
        btnManualToken = findViewById(R.id.btnManualToken)
        btnManualToken.visibility = if (BuildConfig.DEBUG) View.VISIBLE else View.GONE

        pbSyncing = findViewById(R.id.pbSyncing)
        tvLastSyncTime = findViewById(R.id.tvLastSyncTime)
        tvVersion = findViewById(R.id.tvVersion)
        btnSyncNow = findViewById(R.id.btnSyncNow)

        tvBlockedCountBadge = findViewById(R.id.tvBlockedCountBadge)
        tvBlockedList = findViewById(R.id.tvBlockedList)
        btnSettings = findViewById(R.id.btnSettings)
    }

    private fun setupListeners() {
        btnEnableAccessibility.setOnClickListener {
            val intent = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)
            startActivity(intent)
        }

        btnSignInGoogle.setOnClickListener {
            val app = application as OnuslyApplication
            val client = app.authManager.getGoogleSignInClient()
            googleSignInLauncher.launch(client.signInIntent)
        }

        btnSignOut.setOnClickListener {
            val app = application as OnuslyApplication
            app.syncRepository.onUserSignOut()
            updateAccountState()
            updateBlocklistUi(null)
            Toast.makeText(this, "Signed out", Toast.LENGTH_SHORT).show()
        }

        btnManualToken.setOnClickListener {
            if (BuildConfig.DEBUG) {
                showManualTokenDialog()
            }
        }

        btnSyncNow.setOnClickListener {
            performSync()
        }

        btnSettings.setOnClickListener {
            showServerSettingsDialog()
        }
    }

    private fun observeData() {
        lifecycleScope.launch {
            val app = application as OnuslyApplication
            app.syncRepository.cachedBlocklistFlow.collect { cached ->
                updateBlocklistUi(cached)
            }
        }
    }

    private fun updateAccessibilityState() {
        val isEnabled = OnuslyAccessibilityService.isServiceEnabled(this)
        if (isEnabled) {
            cardAccessibility.setCardBackgroundColor(ContextCompat.getColor(this, R.color.success_bg))
            cardAccessibility.strokeColor = ContextCompat.getColor(this, R.color.success)
            ivAccessibilityIcon.setImageResource(android.R.drawable.checkbox_on_background)
            ivAccessibilityIcon.setColorFilter(ContextCompat.getColor(this, R.color.success))
            tvAccessibilityTitle.text = getString(R.string.accessibility_enabled_title)
            tvAccessibilityTitle.setTextColor(ContextCompat.getColor(this, R.color.success))
            tvAccessibilityDesc.text = getString(R.string.accessibility_enabled_desc)
            tvRestrictedSettingsNote.visibility = View.GONE
            btnEnableAccessibility.visibility = View.GONE
        } else {
            cardAccessibility.setCardBackgroundColor(ContextCompat.getColor(this, R.color.warning_bg))
            cardAccessibility.strokeColor = ContextCompat.getColor(this, R.color.warning)
            ivAccessibilityIcon.setImageResource(android.R.drawable.ic_dialog_alert)
            ivAccessibilityIcon.setColorFilter(ContextCompat.getColor(this, R.color.warning))
            tvAccessibilityTitle.text = getString(R.string.accessibility_disabled_title)
            tvAccessibilityTitle.setTextColor(ContextCompat.getColor(this, R.color.warning))
            tvAccessibilityDesc.text = getString(R.string.accessibility_disabled_desc)
            tvRestrictedSettingsNote.visibility = View.VISIBLE
            btnEnableAccessibility.visibility = View.VISIBLE
        }
    }

    private fun updateAccountState() {
        val app = application as OnuslyApplication
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

    private fun updateBlocklistUi(cached: CachedBlocklist?) {
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

        // Count unique blocked apps
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

    private fun handleGoogleToken(idToken: String) {
        val app = application as OnuslyApplication
        pbSyncing.visibility = View.VISIBLE
        btnSignInGoogle.isEnabled = false

        lifecycleScope.launch {
            val result = app.authManager.exchangeGoogleIdToken(
                apiBaseUrl = app.syncRepository.getBaseUrl(),
                googleIdToken = idToken
            )
            pbSyncing.visibility = View.GONE
            btnSignInGoogle.isEnabled = true

            result.onSuccess { session ->
                Toast.makeText(this@MainActivity, "Signed in as ${session.email}", Toast.LENGTH_SHORT).show()
                updateAccountState()
                performSync()
            }.onFailure { err ->
                Toast.makeText(this@MainActivity, "Login failed: ${err.message}", Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun performSync() {
        val app = application as OnuslyApplication
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(this, "Please sign in first", Toast.LENGTH_SHORT).show()
            return
        }

        pbSyncing.visibility = View.VISIBLE
        btnSyncNow.isEnabled = false

        lifecycleScope.launch {
            val result = app.syncRepository.syncBlocklist()
            pbSyncing.visibility = View.GONE
            btnSyncNow.isEnabled = true

            when (result) {
                is SyncResult.Success -> {
                    Toast.makeText(this@MainActivity, "Blocklist synced: ${result.response.version}", Toast.LENGTH_SHORT).show()
                }
                is SyncResult.NotModified -> {
                    Toast.makeText(this@MainActivity, "Blocklist up-to-date (304 Not Modified)", Toast.LENGTH_SHORT).show()
                }
                is SyncResult.Error -> {
                    Toast.makeText(this@MainActivity, "Sync failed: ${result.message}", Toast.LENGTH_LONG).show()
                }
            }
            updateBlocklistUi(app.syncRepository.getActiveCache())
        }
    }

    private fun showServerSettingsDialog() {
        val app = application as OnuslyApplication
        val currentUrl = app.syncRepository.getBaseUrl()

        val input = EditText(this).apply {
            setText(currentUrl)
            setSelection(currentUrl.length)
        }

        AlertDialog.Builder(this)
            .setTitle("Backend API Base URL")
            .setMessage("Default for Android Emulator is http://10.0.2.2:8080.\nFor physical devices, use your host's local IP (e.g. http://192.168.1.x:8080).")
            .setView(input)
            .setPositiveButton("Save") { _, _ ->
                val newUrl = input.text.toString().trim()
                if (newUrl.isNotBlank()) {
                    app.syncRepository.setBaseUrl(newUrl)
                    Toast.makeText(this, "Base URL updated: $newUrl", Toast.LENGTH_SHORT).show()
                }
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun showManualTokenDialog() {
        if (!BuildConfig.DEBUG) return
        val app = application as OnuslyApplication
        val container = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(50, 20, 50, 10)
        }

        val inputJwt = EditText(this).apply {
            hint = "Paste ONUSLY JWT Bearer Token"
        }
        val inputEmail = EditText(this).apply {
            hint = "User Email (optional)"
        }
        container.addView(inputJwt)
        container.addView(inputEmail)

        AlertDialog.Builder(this)
            .setTitle("Manual Session Token")
            .setMessage("Paste an existing ONUSLY JWT token (e.g. from web app or curl testing):")
            .setView(container)
            .setPositiveButton("Set Session") { _, _ ->
                val token = inputJwt.text.toString().trim()
                val email = inputEmail.text.toString().trim()
                if (token.isNotBlank()) {
                    val userId = AuthManager.parseUserIdFromJwt(token)
                    if (userId.isNullOrBlank()) {
                        Toast.makeText(this, "Invalid JWT token: could not extract 'sub' claim", Toast.LENGTH_LONG).show()
                    } else {
                        val session = UserSession(
                            userId = userId,
                            email = if (email.isNotBlank()) email else "manual_user@onusly",
                            name = "Dev User",
                            jwtToken = token
                        )
                        app.authManager.saveSession(session)
                        updateAccountState()
                        performSync()
                    }
                }
            }
            .setNegativeButton("Cancel", null)
            .show()
    }
}
