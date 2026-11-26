package com.onusly.app.ui

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
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
import androidx.activity.result.contract.ActivityResultContracts
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
import com.onusly.app.dns.DnsEngineStats
import com.onusly.app.dns.DnsShieldManager
import com.onusly.app.engine.DomainMatcher
import com.onusly.app.service.OnuslyAccessibilityService
import com.onusly.app.wellbeing.AppUsageTracker
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
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

    private lateinit var cardDnsShield: MaterialCardView
    private lateinit var ivDnsIcon: ImageView
    private lateinit var tvDnsTitle: TextView
    private lateinit var tvDnsBlockedBadge: TextView
    private lateinit var tvDnsDesc: TextView
    private lateinit var tvDnsStats: TextView
    private lateinit var tvPrivateDnsWarning: TextView
    private lateinit var btnToggleDnsShield: MaterialButton

    private val vpnPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        val ctx = context ?: return@registerForActivityResult
        if (result.resultCode == Activity.RESULT_OK) {
            DnsShieldManager.startShield(ctx)
            updateDnsShieldUi(true, DnsShieldManager.stats.value)
        } else {
            DnsShieldManager.setUserEnabled(ctx, false)
            Toast.makeText(ctx, "VPN permission is required to enable Mobile DNS blocking", Toast.LENGTH_LONG).show()
            updateDnsShieldUi(false, DnsShieldManager.stats.value)
        }
    }

    private val notificationPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { _ ->
        proceedWithVpnStart()
    }

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

    private lateinit var tvWellbeingScreenTime: TextView
    private lateinit var tvWellbeingSummary: TextView
    private lateinit var btnGrantUsageAccess: MaterialButton
    private lateinit var btnViewWellbeingBreakdown: MaterialButton

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View? {
        val view = inflater.inflate(R.layout.fragment_home, container, false)
        bindViews(view)
        setupListeners()
        return view
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        observeData()
    }

    override fun onResume() {
        super.onResume()
        updateAccessibilityState()
        updateAccountState()
        updateDnsShieldUi(DnsShieldManager.isRunning.value, DnsShieldManager.stats.value)
        updateWellbeingCard()
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

        cardDnsShield = view.findViewById(R.id.cardDnsShield)
        ivDnsIcon = view.findViewById(R.id.ivDnsIcon)
        tvDnsTitle = view.findViewById(R.id.tvDnsTitle)
        tvDnsBlockedBadge = view.findViewById(R.id.tvDnsBlockedBadge)
        tvDnsDesc = view.findViewById(R.id.tvDnsDesc)
        tvDnsStats = view.findViewById(R.id.tvDnsStats)
        tvPrivateDnsWarning = view.findViewById(R.id.tvPrivateDnsWarning)
        btnToggleDnsShield = view.findViewById(R.id.btnToggleDnsShield)

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

        tvWellbeingScreenTime = view.findViewById(R.id.tvWellbeingScreenTime)
        tvWellbeingSummary = view.findViewById(R.id.tvWellbeingSummary)
        btnGrantUsageAccess = view.findViewById(R.id.btnGrantUsageAccess)
        btnViewWellbeingBreakdown = view.findViewById(R.id.btnViewWellbeingBreakdown)
    }

    private fun setupListeners() {
        btnEnableAccessibility.setOnClickListener {
            val intent = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)
            startActivity(intent)
        }

        btnToggleDnsShield.setOnClickListener {
            val ctx = context ?: return@setOnClickListener
            if (DnsShieldManager.isRunning.value) {
                DnsShieldManager.stopShield(ctx)
                updateDnsShieldUi(false, DnsShieldManager.stats.value)
            } else {
                requestDnsShieldStart()
            }
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

        btnGrantUsageAccess.setOnClickListener {
            try {
                startActivity(AppUsageTracker.createUsageAccessIntent())
            } catch (e: Exception) {
                Toast.makeText(context, "Could not open Usage Access settings: ${e.message}", Toast.LENGTH_SHORT).show()
            }
        }

        btnViewWellbeingBreakdown.setOnClickListener {
            val ctx = context ?: return@setOnClickListener
            WellbeingBreakdownDialog(ctx).show()
        }
    }

    private fun updateWellbeingCard() {
        if (view == null || !::tvWellbeingScreenTime.isInitialized) return
        val ctx = context ?: return
        viewLifecycleOwner.lifecycleScope.launch {
            val result = withContext(Dispatchers.IO) {
                val todayKey = AppUsageTracker.getTodayDateKey()
                val liveHits = DnsShieldManager.domainHits.value
                val todayLive = liveHits.filter {
                    it.lastSeenMillis == 0L || com.onusly.app.dns.DnsHitStore.dateKeyFor(it.lastSeenMillis) == todayKey
                }
                val dnsHits = if (todayLive.isNotEmpty()) todayLive else com.onusly.app.dns.DnsHitStore.create(ctx).loadDay(todayKey)
                val totalWebOpens = dnsHits.sumOf { it.opens }
                val totalBlocked = dnsHits.sumOf { it.blockedAttempts }
                val hasPerm = AppUsageTracker.hasUsageStatsPermission(ctx)
                val snap = if (hasPerm) AppUsageTracker.collectTodaySnapshot(ctx, dnsHits) else null
                listOf(hasPerm, totalWebOpens, totalBlocked, snap)
            }
            if (!isAdded || view == null) return@launch
            val hasPerm = result[0] as Boolean
            val totalWebOpens = result[1] as Long
            val totalBlocked = result[2] as Long
            val snap = result[3] as? com.onusly.app.wellbeing.DailyWellbeingSnapshot

            if (!hasPerm || snap == null) {
                tvWellbeingScreenTime.text = "$totalWebOpens web"
                tvWellbeingSummary.text = "Grant Usage Access so ONUSLY can track daily mobile app screen time alongside your DNS website opens ($totalWebOpens opened, $totalBlocked blocked today)."
                btnGrantUsageAccess.visibility = View.VISIBLE
                return@launch
            }

            btnGrantUsageAccess.visibility = View.GONE
            val totalMin = snap.totalScreenSeconds / 60
            val hrs = totalMin / 60
            val mins = totalMin % 60
            tvWellbeingScreenTime.text = if (hrs > 0) "${hrs}h ${mins}m" else "${mins}m"

            val topAppText = snap.apps.firstOrNull()?.let { app ->
                val appMin = app.seconds / 60
                "Top app: ${app.displayName} (${appMin}m, ${app.opens} opens)"
            } ?: "No app usage recorded yet today"

            tvWellbeingSummary.text = "$topAppText • Websites opened: $totalWebOpens ($totalBlocked blocked)"
        }
    }

    private fun observeData() {
        viewLifecycleOwner.lifecycleScope.launch {
            val app = requireActivity().application as OnuslyApplication
            app.syncRepository.cachedBlocklistFlow.collect { cached ->
                updateBlocklistUi(cached)
                updateDnsShieldUi(DnsShieldManager.isRunning.value, DnsShieldManager.stats.value)
            }
        }
        viewLifecycleOwner.lifecycleScope.launch {
            DnsShieldManager.isRunning.collect { running ->
                updateDnsShieldUi(running, DnsShieldManager.stats.value)
            }
        }
        viewLifecycleOwner.lifecycleScope.launch {
            DnsShieldManager.stats.collect { stats ->
                updateDnsShieldUi(DnsShieldManager.isRunning.value, stats)
            }
        }
    }

    private fun requestDnsShieldStart() {
        val ctx = context ?: return
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(ctx, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            return
        }
        proceedWithVpnStart()
    }

    private fun proceedWithVpnStart() {
        val ctx = context ?: return
        val prepareIntent = DnsShieldManager.prepareVpnIntent(ctx)
        if (prepareIntent != null) {
            vpnPermissionLauncher.launch(prepareIntent)
        } else {
            DnsShieldManager.startShield(ctx)
            updateDnsShieldUi(true, DnsShieldManager.stats.value)
        }
    }

    fun updateDnsShieldUi(running: Boolean, stats: DnsEngineStats) {
        if (view == null || !::cardDnsShield.isInitialized) return
        val ctx = context ?: return
        val app = activity?.application as? OnuslyApplication
        val cache = app?.syncRepository?.getActiveCache()
        val apiHost = DomainMatcher.extractHostFromUrl(app?.syncRepository?.getBaseUrl())
        val extraProtected = if (apiHost.isNotBlank()) setOf(apiHost) else emptySet()
        val activeDomainsCount = DomainMatcher.getActiveBlockedDomains(cache, extraProtected).size

        tvDnsBlockedBadge.text = "$activeDomainsCount ${if (activeDomainsCount == 1) "domain" else "domains"}"
        tvPrivateDnsWarning.visibility = if (DnsShieldManager.isStrictPrivateDnsActive(ctx)) View.VISIBLE else View.GONE

        if (running) {
            cardDnsShield.setCardBackgroundColor(ContextCompat.getColor(ctx, R.color.success_bg))
            cardDnsShield.strokeColor = ContextCompat.getColor(ctx, R.color.success)
            ivDnsIcon.setColorFilter(ContextCompat.getColor(ctx, R.color.success))
            tvDnsTitle.text = getString(R.string.dns_shield_enabled_title)
            tvDnsTitle.setTextColor(ContextCompat.getColor(ctx, R.color.success))
            tvDnsDesc.text = if (activeDomainsCount > 0) {
                "Local VPN DNS resolver is intercepting and blocking $activeDomainsCount goal domain(s) across Chrome, Firefox, and all apps."
            } else {
                "Local VPN DNS resolver is active and forwarding queries normally. Websites added to active goals will be blocked automatically."
            }
            val lastBlocked = stats.lastBlockedDomain?.let { " • Last: $it" } ?: ""
            tvDnsStats.text = "Blocked: ${stats.queriesBlocked}  •  Forwarded: ${stats.queriesForwarded}  •  Cached: ${stats.queriesCached}$lastBlocked"
            tvDnsStats.visibility = View.VISIBLE
            btnToggleDnsShield.text = getString(R.string.dns_shield_disable_button)
            btnToggleDnsShield.backgroundTintList = ContextCompat.getColorStateList(ctx, R.color.surface_card)
        } else {
            cardDnsShield.setCardBackgroundColor(ContextCompat.getColor(ctx, R.color.surface))
            cardDnsShield.strokeColor = ContextCompat.getColor(ctx, R.color.surface_card)
            ivDnsIcon.setColorFilter(ContextCompat.getColor(ctx, R.color.warning))
            tvDnsTitle.text = getString(R.string.dns_shield_disabled_title)
            tvDnsTitle.setTextColor(ContextCompat.getColor(ctx, R.color.warning))
            tvDnsDesc.text = getString(R.string.dns_shield_disabled_desc)
            tvDnsStats.visibility = View.GONE
            btnToggleDnsShield.text = getString(R.string.dns_shield_enable_button)
            btnToggleDnsShield.backgroundTintList = ContextCompat.getColorStateList(ctx, R.color.primary)
        }
    }

    fun updateAccessibilityState() {
        if (view == null || !::cardAccessibility.isInitialized) return
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
        if (view == null || !::layoutSignedIn.isInitialized) return
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
        if (!isAdded || view == null || !::tvBlockedCountBadge.isInitialized) return
        if (cached == null || cached.targets.isEmpty()) {
            tvBlockedCountBadge.text = "0"
            tvBlockedList.text = "No apps or websites currently blocked."
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
        val uniqueDomains = mutableSetOf<String>()
        val sb = java.lang.StringBuilder()

        for (target in cached.targets) {
            val statusTag = if (target.status.equals("active", ignoreCase = true)) "[ACTIVE]" else "[WAITING REVIEW]"
            sb.append("• ").append(target.goalTitle).append(" ").append(statusTag).append("\n")
            if (target.apps.isEmpty() && target.domains.isEmpty()) {
                sb.append("   (no apps or domains configured)\n")
            } else {
                for (appPkg in target.apps) {
                    uniqueApps.add(appPkg)
                    sb.append("   - [App] ").append(appPkg).append("\n")
                }
                for (dom in target.domains) {
                    val cleanDom = DomainMatcher.normalizeDomain(dom)
                    if (cleanDom.isNotBlank()) {
                        uniqueDomains.add(cleanDom)
                        sb.append("   - [Web] ").append(cleanDom).append("\n")
                    }
                }
            }
            sb.append("\n")
        }

        tvBlockedCountBadge.text = (uniqueApps.size + uniqueDomains.size).toString()
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
