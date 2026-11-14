package com.onusly.app.engine

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.PowerManager
import android.provider.Settings
import com.onusly.app.service.OnuslyAccessibilityService

data class AntiTamperHealth(
    val isAccessibilityEnabled: Boolean,
    val isAccessibilityRunning: Boolean,
    val isBatteryOptimizationsIgnored: Boolean,
    val isFullyProtected: Boolean
)

object AntiTamperManager {

    fun isIgnoringBatteryOptimizations(context: Context): Boolean {
        return try {
            val powerManager = context.getSystemService(Context.POWER_SERVICE) as? PowerManager
            powerManager?.isIgnoringBatteryOptimizations(context.packageName) ?: false
        } catch (_: Exception) {
            false
        }
    }

    fun createBatteryOptimizationIntent(context: Context): Intent {
        return try {
            val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                data = Uri.parse("package:${context.packageName}")
            }
            if (intent.resolveActivity(context.packageManager) != null) {
                intent
            } else {
                Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
            }
        } catch (_: Exception) {
            Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
        }
    }

    fun getHealthSummary(context: Context): AntiTamperHealth {
        val enabled = OnuslyAccessibilityService.isServiceEnabled(context)
        val running = OnuslyAccessibilityService.isServiceRunning
        val batteryIgnored = isIgnoringBatteryOptimizations(context)
        val fullyProtected = enabled && batteryIgnored

        return AntiTamperHealth(
            isAccessibilityEnabled = enabled,
            isAccessibilityRunning = running,
            isBatteryOptimizationsIgnored = batteryIgnored,
            isFullyProtected = fullyProtected
        )
    }
}
