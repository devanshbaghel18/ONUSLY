package com.onusly.app.dns

import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.provider.Settings
import com.onusly.app.service.OnuslyDnsVpnService
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Coordinates the lifecycle, permission state, persisted preference, and live telemetry
 * of the local Mobile DNS Shield (OnuslyDnsVpnService).
 */
object DnsShieldManager {

    private const val SETTINGS_PREFS_NAME = "onusly_settings"
    private const val KEY_DNS_SHIELD_ENABLED = "dns_shield_user_enabled"

    private val _isRunning = MutableStateFlow(false)
    val isRunning: StateFlow<Boolean> = _isRunning.asStateFlow()

    private val _stats = MutableStateFlow(DnsEngineStats())
    val stats: StateFlow<DnsEngineStats> = _stats.asStateFlow()

    private val _domainHits = MutableStateFlow<List<DomainQueryHit>>(emptyList())
    val domainHits: StateFlow<List<DomainQueryHit>> = _domainHits.asStateFlow()

    private val _lastError = MutableStateFlow<String?>(null)
    val lastError: StateFlow<String?> = _lastError.asStateFlow()

    internal fun updateRunning(running: Boolean) {
        _isRunning.value = running
    }

    internal fun updateStats(newStats: DnsEngineStats) {
        _stats.value = newStats
    }

    internal fun updateDomainHits(hits: List<DomainQueryHit>) {
        _domainHits.value = hits
    }

    internal fun updateError(err: String?) {
        _lastError.value = err
    }

    fun isUserEnabled(context: Context): Boolean {
        return try {
            val prefs = context.getSharedPreferences(SETTINGS_PREFS_NAME, Context.MODE_PRIVATE)
            prefs.getBoolean(KEY_DNS_SHIELD_ENABLED, false)
        } catch (_: Exception) {
            false
        }
    }

    fun setUserEnabled(context: Context, enabled: Boolean) {
        try {
            val prefs = context.getSharedPreferences(SETTINGS_PREFS_NAME, Context.MODE_PRIVATE)
            prefs.edit().putBoolean(KEY_DNS_SHIELD_ENABLED, enabled).apply()
        } catch (_: Exception) {
        }
    }

    /**
     * Checks whether system VPN permission is already granted (null = ready to start without UI prompt).
     * Otherwise returns the system Intent to launch with ActivityResultLauncher.
     */
    fun prepareVpnIntent(context: Context): Intent? {
        return try {
            VpnService.prepare(context)
        } catch (_: Exception) {
            null
        }
    }

    /**
     * Starts the local DNS VPN service if permission has already been granted.
     * Returns true if start command was dispatched, false if VPN permission consent is still needed.
     */
    fun startShield(context: Context, markUserEnabled: Boolean = true): Boolean {
        if (markUserEnabled) {
            setUserEnabled(context, true)
        }
        if (prepareVpnIntent(context) != null) {
            return false
        }
        val intent = Intent(context, OnuslyDnsVpnService::class.java).apply {
            action = OnuslyDnsVpnService.ACTION_START
        }
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
            return true
        } catch (e: Exception) {
            updateError(e.message ?: "Failed to start DNS Shield service")
            return false
        }
    }

    fun stopShield(context: Context, clearUserEnabled: Boolean = true) {
        if (clearUserEnabled) {
            setUserEnabled(context, false)
        }
        if (!_isRunning.value) {
            return
        }
        try {
            val intent = Intent(context, OnuslyDnsVpnService::class.java).apply {
                action = OnuslyDnsVpnService.ACTION_STOP
            }
            context.startService(intent)
        } catch (_: Exception) {
        }
    }

    /**
     * Ensures the DNS Shield is started on app startup or boot if the user opted in
     * and VPN consent is already granted.
     */
    fun startIfEnabledAndPermitted(context: Context) {
        if (isUserEnabled(context) && !_isRunning.value && prepareVpnIntent(context) == null) {
            startShield(context, markUserEnabled = false)
        }
    }

    /**
     * Detects Android 9+ Private DNS "hostname" (Strict DoT) mode, which bypasses port-53 local VPN DNS.
     * Automatic ("opportunistic") and "off" modes route cleanly through local VPN DNS.
     */
    fun isStrictPrivateDnsActive(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.P) return false
        return try {
            val mode = Settings.Global.getString(context.contentResolver, "private_dns_mode")
            mode.equals("hostname", ignoreCase = true)
        } catch (_: Exception) {
            false
        }
    }
}
