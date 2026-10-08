package com.onusly.app.ui

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.view.View
import android.widget.ImageView
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.google.android.material.button.MaterialButton
import com.google.android.material.card.MaterialCardView
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.SyncResult
import com.onusly.app.engine.BlockerEngine
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class LockActivity : AppCompatActivity() {

    companion object {
        const val EXTRA_BLOCKED_PACKAGE = "extra_blocked_package"
        const val EXTRA_GOAL_TITLES = "extra_goal_titles"
        const val EXTRA_GOAL_STATUSES = "extra_goal_statuses"
    }

    private var blockedPackage: String? = null
    private var goalTitles: ArrayList<String> = arrayListOf()
    private var goalStatuses: ArrayList<String> = arrayListOf()

    private lateinit var ivLockIcon: ImageView
    private lateinit var tvBlockedPackage: TextView
    private lateinit var cardInstruction: MaterialCardView
    private lateinit var tvStatusHeader: TextView
    private lateinit var tvInstructionText: TextView
    private lateinit var tvGoalTitles: TextView
    private lateinit var tvSyncStatus: TextView
    private lateinit var progressBar: ProgressBar
    private lateinit var btnCheckAgain: MaterialButton
    private lateinit var btnOpenDashboard: MaterialButton
    private lateinit var btnGoHome: MaterialButton

    private var autoCheckJob: Job? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_lock)

        // Custom back button handler: ALWAYS navigate to Home screen, never back to the blocked app
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                exitToHomeScreen()
            }
        })

        bindViews()
        parseIntent(intent)
        setupListeners()
        updateUiState()
        checkIfStillBlocked()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        parseIntent(intent)
        updateUiState()
        checkIfStillBlocked()
    }

    override fun onResume() {
        super.onResume()
        startAutoCheckTicker()
        checkIfStillBlocked()
    }

    override fun onPause() {
        super.onPause()
        autoCheckJob?.cancel()
    }

    private fun bindViews() {
        ivLockIcon = findViewById(R.id.ivLockIcon)
        tvBlockedPackage = findViewById(R.id.tvBlockedPackage)
        cardInstruction = findViewById(R.id.cardInstruction)
        tvStatusHeader = findViewById(R.id.tvStatusHeader)
        tvInstructionText = findViewById(R.id.tvInstructionText)
        tvGoalTitles = findViewById(R.id.tvGoalTitles)
        tvSyncStatus = findViewById(R.id.tvSyncStatus)
        progressBar = findViewById(R.id.progressBar)
        btnCheckAgain = findViewById(R.id.btnCheckAgain)
        btnOpenDashboard = findViewById(R.id.btnOpenDashboard)
        btnGoHome = findViewById(R.id.btnGoHome)
    }

    private fun parseIntent(intent: Intent?) {
        if (intent == null) return
        blockedPackage = intent.getStringExtra(EXTRA_BLOCKED_PACKAGE)
        val titles = intent.getStringArrayListExtra(EXTRA_GOAL_TITLES)
        if (titles != null) {
            goalTitles = titles
        }
        val statuses = intent.getStringArrayListExtra(EXTRA_GOAL_STATUSES)
        if (statuses != null) {
            goalStatuses = statuses
        }
    }

    private fun setupListeners() {
        btnCheckAgain.setOnClickListener {
            performSyncCheck(manual = true)
        }

        btnOpenDashboard.setOnClickListener {
            val app = application as OnuslyApplication
            var url = app.syncRepository.getBaseUrl()
            // If baseUrl is backend 8080, direct to web frontend on 5173 if local, or same origin
            val dashboardUrl = if (url.contains(":8080")) {
                url.replace(":8080", ":5173")
            } else {
                url
            }
            try {
                val browserIntent = Intent(Intent.ACTION_VIEW, Uri.parse(dashboardUrl)).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK
                }
                startActivity(browserIntent)
            } catch (e: Exception) {
                Toast.makeText(this, "Could not open browser: ${e.message}", Toast.LENGTH_SHORT).show()
            }
        }

        btnGoHome.setOnClickListener {
            exitToHomeScreen()
        }
    }

    private fun exitToHomeScreen() {
        val homeIntent = Intent(Intent.ACTION_MAIN).apply {
            addCategory(Intent.CATEGORY_HOME)
            flags = Intent.FLAG_ACTIVITY_NEW_TASK
        }
        startActivity(homeIntent)
        finish()
    }

    private fun updateUiState() {
        tvBlockedPackage.text = blockedPackage ?: "Restricted App"

        if (goalTitles.isNotEmpty()) {
            val sb = java.lang.StringBuilder()
            for ((index, title) in goalTitles.withIndex()) {
                sb.append("• ").append(title)
                if (index < goalTitles.size - 1) {
                    sb.append("\n")
                }
            }
            tvGoalTitles.text = sb.toString()
        } else {
            tvGoalTitles.text = "Active ONUSLY Commitment"
        }

        val hasActive = goalStatuses.any { it.equals("active", ignoreCase = true) }
        val isProofSubmitted = !hasActive && goalStatuses.all { it.equals("proof_submitted", ignoreCase = true) }

        if (isProofSubmitted) {
            tvStatusHeader.text = "PROOF SUBMITTED"
            tvInstructionText.text = getString(R.string.proof_submitted_instruction)
            cardInstruction.setCardBackgroundColor(ContextCompat.getColor(this, R.color.surface))
            cardInstruction.strokeColor = ContextCompat.getColor(this, R.color.primary)
            tvStatusHeader.setTextColor(ContextCompat.getColor(this, R.color.primary))
            ivLockIcon.setColorFilter(ContextCompat.getColor(this, R.color.primary))
        } else {
            tvStatusHeader.text = "GOAL IN PROGRESS"
            tvInstructionText.text = getString(R.string.active_goal_instruction)
            cardInstruction.setCardBackgroundColor(ContextCompat.getColor(this, R.color.warning_bg))
            cardInstruction.strokeColor = ContextCompat.getColor(this, R.color.warning)
            tvStatusHeader.setTextColor(ContextCompat.getColor(this, R.color.warning))
            ivLockIcon.setColorFilter(ContextCompat.getColor(this, R.color.warning))
        }

        val app = application as OnuslyApplication
        val cache = app.syncRepository.getActiveCache()
        if (cache != null) {
            val dateStr = if (cache.lastSyncTimestamp > 0) {
                val sdf = SimpleDateFormat("HH:mm:ss", Locale.getDefault())
                sdf.format(Date(cache.lastSyncTimestamp))
            } else "Never"
            tvSyncStatus.text = "Last sync: $dateStr • Version: ${cache.version}"
        } else {
            tvSyncStatus.text = "No blocklist cached"
        }
    }

    private fun checkIfStillBlocked() {
        val app = application as OnuslyApplication
        val activeCache = app.syncRepository.getActiveCache()
        val pkg = blockedPackage ?: return

        val evaluation = BlockerEngine.evaluatePackage(pkg, activeCache)
        if (!evaluation.isBlocked) {
            // Unlocked!
            Toast.makeText(this, "Goal completed! App unlocked.", Toast.LENGTH_SHORT).show()
            finish()
        } else {
            // Update titles/statuses in case they changed
            goalTitles = ArrayList(evaluation.matchingTargets.map { it.goalTitle })
            goalStatuses = ArrayList(evaluation.matchingTargets.map { it.status })
            updateUiState()
        }
    }

    private fun performSyncCheck(manual: Boolean) {
        if (manual) {
            progressBar.visibility = View.VISIBLE
            btnCheckAgain.isEnabled = false
        }

        lifecycleScope.launch {
            val app = application as OnuslyApplication
            val result = app.syncRepository.syncBlocklist()

            if (manual) {
                progressBar.visibility = View.GONE
                btnCheckAgain.isEnabled = true
            }

            when (result) {
                is SyncResult.Success, is SyncResult.NotModified -> {
                    checkIfStillBlocked()
                }
                is SyncResult.Error -> {
                    if (manual) {
                        Toast.makeText(this@LockActivity, "Sync failed: ${result.message}", Toast.LENGTH_SHORT).show()
                    }
                    checkIfStillBlocked()
                }
            }
        }
    }

    private fun startAutoCheckTicker() {
        autoCheckJob?.cancel()
        autoCheckJob = lifecycleScope.launch {
            while (isActive) {
                delay(10_000)
                performSyncCheck(manual = false)
            }
        }
    }
}
