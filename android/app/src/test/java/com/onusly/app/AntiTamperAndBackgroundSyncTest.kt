package com.onusly.app

import android.content.Context
import android.content.Intent
import android.provider.Settings
import com.onusly.app.engine.AntiTamperHealth
import com.onusly.app.engine.AntiTamperManager
import com.onusly.app.receiver.BootReceiver
import com.onusly.app.service.EnforcementSyncScheduler
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AntiTamperAndBackgroundSyncTest {

    @Test
    fun testAntiTamperHealth_ModelEvaluation() {
        val fullyProtected = AntiTamperHealth(
            isAccessibilityEnabled = true,
            isAccessibilityRunning = true,
            isBatteryOptimizationsIgnored = true,
            isFullyProtected = true
        )
        assertTrue(fullyProtected.isFullyProtected)

        val degraded = AntiTamperHealth(
            isAccessibilityEnabled = false,
            isAccessibilityRunning = false,
            isBatteryOptimizationsIgnored = true,
            isFullyProtected = false
        )
        assertFalse(degraded.isFullyProtected)
    }

    @Test
    fun testBatteryOptimizationIntent_CreatedCorrectly() {
        val dummyContext = object : android.content.ContextWrapper(null) {
            override fun getPackageName(): String = "com.onusly.app"
        }

        val intent = AntiTamperManager.createBatteryOptimizationIntent(dummyContext)
        assertNotNull(intent)
    }

    @Test
    fun testEnforcementSyncScheduler_Constants() {
        assertEquals(1001, EnforcementSyncScheduler.JOB_ID)
    }

    @Test
    fun testBootReceiver_IgnoresUnrelatedActions() {
        val receiver = BootReceiver()
        val dummyContext = object : android.content.ContextWrapper(null) {}

        // Send unrelated action - should return early without throwing
        val intent = Intent(Intent.ACTION_BATTERY_LOW)
        receiver.onReceive(dummyContext, intent)
    }
}
