package com.onusly.app.ui

import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.EditText
import android.widget.ImageButton
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import com.google.android.gms.auth.api.signin.GoogleSignIn
import com.google.android.gms.common.api.ApiException
import com.google.android.material.bottomnavigation.BottomNavigationView
import com.onusly.app.BuildConfig
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.SyncResult
import com.onusly.app.data.model.UserSession
import kotlinx.coroutines.launch

class MainActivity : AppCompatActivity() {

    private lateinit var bottomNavigation: BottomNavigationView
    private lateinit var headerProgress: ProgressBar
    private lateinit var btnHeaderSync: ImageButton

    private var homeFragment = HomeFragment()
    private var goalsFragment = GoalsFragment()
    private var reviewsFragment = ReviewsFragment()
    private var chatFragment = ChatFragment()
    private var settingsFragment = SettingsFragment()

    private var activeFragment: Fragment = homeFragment

    private val googleSignInLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        val task = GoogleSignIn.getSignedInAccountFromIntent(result.data)
        try {
            val account = task.getResult(ApiException::class.java)
            val idToken = account?.idToken
            if (!idToken.isNullOrBlank()) {
                handleGoogleToken(idToken)
            } else {
                Toast.makeText(this, "Google Sign-In returned no ID token", Toast.LENGTH_LONG).show()
            }
        } catch (e: ApiException) {
            Toast.makeText(this, "Google Sign-In failed (Code ${e.statusCode}): ${e.message}", Toast.LENGTH_LONG).show()
        } catch (e: Exception) {
            Toast.makeText(this, "Google Sign-In error: ${e.message}", Toast.LENGTH_LONG).show()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        bindViews()
        setupFragments(savedInstanceState)
        setupNavigation()

        val app = application as OnuslyApplication
        app.syncRepository.addSignOutListener {
            runOnUiThread {
                onUserLoggedOut()
            }
        }
    }

    private fun bindViews() {
        bottomNavigation = findViewById(R.id.bottom_navigation)
        headerProgress = findViewById(R.id.headerProgress)
        btnHeaderSync = findViewById(R.id.btnHeaderSync)

        btnHeaderSync.setOnClickListener {
            performQuickSync()
        }
    }

    private fun setupFragments(savedInstanceState: Bundle?) {
        if (savedInstanceState == null) {
            supportFragmentManager.beginTransaction()
                .add(R.id.container_fragment, homeFragment, "HOME")
                .add(R.id.container_fragment, goalsFragment, "GOALS").hide(goalsFragment)
                .add(R.id.container_fragment, reviewsFragment, "REVIEWS").hide(reviewsFragment)
                .add(R.id.container_fragment, chatFragment, "CHAT").hide(chatFragment)
                .add(R.id.container_fragment, settingsFragment, "SETTINGS").hide(settingsFragment)
                .commit()
            activeFragment = homeFragment
        } else {
            homeFragment = (supportFragmentManager.findFragmentByTag("HOME") as? HomeFragment) ?: homeFragment
            goalsFragment = (supportFragmentManager.findFragmentByTag("GOALS") as? GoalsFragment) ?: goalsFragment
            reviewsFragment = (supportFragmentManager.findFragmentByTag("REVIEWS") as? ReviewsFragment) ?: reviewsFragment
            chatFragment = (supportFragmentManager.findFragmentByTag("CHAT") as? ChatFragment) ?: chatFragment
            settingsFragment = (supportFragmentManager.findFragmentByTag("SETTINGS") as? SettingsFragment) ?: settingsFragment
            val activeTag = savedInstanceState.getString("KEY_ACTIVE_TAG", "HOME")
            activeFragment = when (activeTag) {
                "GOALS" -> goalsFragment
                "REVIEWS" -> reviewsFragment
                "CHAT" -> chatFragment
                "SETTINGS" -> settingsFragment
                else -> homeFragment
            }
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        val activeTag = when (activeFragment) {
            goalsFragment -> "GOALS"
            reviewsFragment -> "REVIEWS"
            chatFragment -> "CHAT"
            settingsFragment -> "SETTINGS"
            else -> "HOME"
        }
        outState.putString("KEY_ACTIVE_TAG", activeTag)
    }

    private fun setupNavigation() {
        bottomNavigation.setOnItemSelectedListener { item ->
            val targetFragment = when (item.itemId) {
                R.id.nav_home -> homeFragment
                R.id.nav_goals -> goalsFragment
                R.id.nav_reviews -> reviewsFragment
                R.id.nav_chat -> chatFragment
                R.id.nav_settings -> settingsFragment
                else -> null
            }

            if (targetFragment != null && targetFragment != activeFragment) {
                supportFragmentManager.beginTransaction()
                    .hide(activeFragment)
                    .show(targetFragment)
                    .commit()
                activeFragment = targetFragment
                true
            } else {
                false
            }
        }
    }

    fun launchGoogleSignIn() {
        val app = application as OnuslyApplication
        val client = app.authManager.getGoogleSignInClient()
        googleSignInLauncher.launch(client.signInIntent)
    }

    fun handleGoogleToken(idToken: String) {
        val app = application as OnuslyApplication
        val existingUserId = app.authManager.getUserId()
        headerProgress.visibility = View.VISIBLE

        lifecycleScope.launch {
            val result = app.authManager.exchangeGoogleIdToken(
                apiBaseUrl = app.syncRepository.getBaseUrl(),
                googleIdToken = idToken
            )
            headerProgress.visibility = View.GONE

            result.onSuccess { session ->
                if (!existingUserId.isNullOrBlank() && existingUserId != session.userId) {
                    // Direct account switch without signout: clean previous user state & cache!
                    app.syncRepository.onUserSignOut()
                    app.goalsRepository.clearLocalData()
                    chatFragment.clearLocalData()
                    reviewsFragment.clearLocalData()
                    app.authManager.saveSession(session)
                }
                app.realtimeWebSocketManager.connect()
                Toast.makeText(this@MainActivity, "Signed in as ${session.email}", Toast.LENGTH_SHORT).show()
                homeFragment.updateAccountState()
                performQuickSync()
                goalsFragment.refreshGoals()
            }.onFailure { err ->
                Toast.makeText(this@MainActivity, "Login failed: ${err.message}", Toast.LENGTH_LONG).show()
            }
        }
    }

    fun performQuickSync() {
        val app = application as OnuslyApplication
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(this, "Please sign in first", Toast.LENGTH_SHORT).show()
            return
        }

        headerProgress.visibility = View.VISIBLE
        btnHeaderSync.isEnabled = false

        lifecycleScope.launch {
            val result = app.syncRepository.syncBlocklist()
            headerProgress.visibility = View.GONE
            btnHeaderSync.isEnabled = true

            when (result) {
                is SyncResult.Success -> {
                    Toast.makeText(this@MainActivity, "Blocklist synced: ${result.response.version}", Toast.LENGTH_SHORT).show()
                }
                is SyncResult.NotModified -> {
                    Toast.makeText(this@MainActivity, "Up-to-date (304 Not Modified)", Toast.LENGTH_SHORT).show()
                }
                is SyncResult.Error -> {
                    Toast.makeText(this@MainActivity, "Sync failed: ${result.message}", Toast.LENGTH_LONG).show()
                }
            }
            homeFragment.updateBlocklistUi(app.syncRepository.getActiveCache())
        }
    }

    fun showManualTokenDialog() {
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
            .setMessage("Paste an existing ONUSLY JWT token:")
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
                        homeFragment.updateAccountState()
                        performQuickSync()
                        goalsFragment.refreshGoals()
                    }
                }
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    // --- Goal Creation & Editing (Vertical Slice 2) ---

    fun showCreateGoalDialog() {
        val app = application as OnuslyApplication
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(this, "Sign in first to create a goal", Toast.LENGTH_SHORT).show()
            return
        }

        GoalEditorDialog(
            context = this,
            scope = lifecycleScope,
            goalsRepository = app.goalsRepository,
            existingGoal = null
        ) { goal, syncSucceeded, syncError ->
            Toast.makeText(this, "Goal created: ${goal.title}", Toast.LENGTH_SHORT).show()
            if (!syncSucceeded) {
                showSyncFailedDialog(syncError)
            }
        }.show()
    }

    fun showEditGoalDialog(goal: Goal) {
        val app = application as OnuslyApplication
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(this, "Sign in first to edit a goal", Toast.LENGTH_SHORT).show()
            return
        }

        GoalEditorDialog(
            context = this,
            scope = lifecycleScope,
            goalsRepository = app.goalsRepository,
            existingGoal = goal
        ) { updatedGoal, syncSucceeded, syncError ->
            Toast.makeText(this, "Goal updated: ${updatedGoal.title}", Toast.LENGTH_SHORT).show()
            if (!syncSucceeded) {
                showSyncFailedDialog(syncError)
            }
        }.show()
    }

    fun showSyncFailedDialog(syncError: String?) {
        AlertDialog.Builder(this)
            .setTitle("Blocklist Sync Pending")
            .setMessage("Your update was saved on the server, but local blocklist update failed (${syncError ?: "Network error"}). Local enforcement will retry automatically, or you can retry now.")
            .setPositiveButton("Retry Sync Now") { _, _ ->
                val app = application as OnuslyApplication
                lifecycleScope.launch {
                    val result = app.goalsRepository.retryBlocklistSync()
                    if (result.isSuccess) {
                        Toast.makeText(this@MainActivity, "Blocklist synchronized successfully", Toast.LENGTH_SHORT).show()
                    } else {
                        Toast.makeText(this@MainActivity, "Sync retry failed: ${result.exceptionOrNull()?.message}", Toast.LENGTH_LONG).show()
                    }
                }
            }
            .setNegativeButton("Dismiss", null)
            .show()
    }

    fun showSubmitProofDialog(goal: Goal) {
        val app = application as OnuslyApplication
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(this, "Sign in first to submit proof", Toast.LENGTH_SHORT).show()
            return
        }

        SubmitProofDialog(
            context = this,
            scope = lifecycleScope,
            goalsRepository = app.goalsRepository,
            goal = goal
        ) { _, syncSucceeded, syncError ->
            Toast.makeText(this, "Proof submitted for \"${goal.title}\"", Toast.LENGTH_SHORT).show()
            if (!syncSucceeded) {
                showSyncFailedDialog(syncError)
            }
        }.show()
    }

    fun showProofHistoryDialog(goal: Goal) {
        val app = application as OnuslyApplication
        if (!app.authManager.isSignedIn()) {
            Toast.makeText(this, "Sign in first to view proof history", Toast.LENGTH_SHORT).show()
            return
        }

        ProofHistoryDialog(
            context = this,
            scope = lifecycleScope,
            goalsRepository = app.goalsRepository,
            goal = goal
        ).show()
    }

    private var isLoggingOut = false

    fun onUserLoggedOut() {
        if (isLoggingOut) return
        isLoggingOut = true
        try {
            val app = application as OnuslyApplication
            chatFragment.clearLocalData()
            app.realtimeWebSocketManager.disconnect()
            app.syncRepository.onUserSignOut()
            app.goalsRepository.clearLocalData()
            homeFragment.updateAccountState()
            homeFragment.updateBlocklistUi(null)
            goalsFragment.refreshGoals()
            reviewsFragment.clearLocalData()
            settingsFragment.updateProfileUi()
        } finally {
            isLoggingOut = false
        }
    }
}
