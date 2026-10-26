package com.onusly.app

import com.onusly.app.data.model.InstalledAppInfo
import com.onusly.app.engine.BlockerEngine
import com.onusly.app.engine.InstalledAppsManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class InstalledAppsManagerTest {

    @Test
    fun testIsSystemOrExcluded_AlwaysAllowedPackages() {
        for (pkg in BlockerEngine.ALWAYS_ALLOWED_PACKAGES) {
            assertTrue(
                "Package $pkg in ALWAYS_ALLOWED_PACKAGES must be excluded from blockable apps",
                InstalledAppsManager.isSystemOrExcluded(pkg)
            )
        }
    }

    @Test
    fun testIsSystemOrExcluded_CriticalSystemSubstrings() {
        val systemPackages = listOf(
            "com.android.settings",
            "com.samsung.android.settings",
            "com.google.android.apps.nexuslauncher",
            "com.miui.home.launcher",
            "com.android.systemui",
            "com.google.android.dialer",
            "com.samsung.android.dialer",
            "com.android.server.telecom",
            "com.google.android.emergency",
            "",
            "   "
        )

        for (pkg in systemPackages) {
            assertTrue(
                "System package '$pkg' must be excluded from selection",
                InstalledAppsManager.isSystemOrExcluded(pkg)
            )
        }
    }

    @Test
    fun testIsSystemOrExcluded_NormalDistractionAppsAreSelectable() {
        val userApps = listOf(
            "com.instagram.android",
            "com.twitter.android",
            "com.zhiliaoapp.musically",
            "com.reddit.frontpage",
            "com.google.android.youtube",
            "com.netflix.mediaclient",
            "com.spotify.music"
        )

        for (pkg in userApps) {
            assertFalse(
                "User app '$pkg' must NOT be excluded",
                InstalledAppsManager.isSystemOrExcluded(pkg)
            )
        }
    }

    @Test
    fun testIsSystemOrExcluded_ThirdPartyAppsWithEmergencyOrTelecom_AreSelectable() {
        val thirdPartyApps = listOf(
            "com.emergency.response.simulator",
            "com.telecom.customerapp",
            "com.telecom.billing",
            "com.random.settingswidget",
            "com.fake.dialer"
        )

        for (pkg in thirdPartyApps) {
            assertFalse(
                "Third-party app '$pkg' must NOT be excluded as a system app",
                InstalledAppsManager.isSystemOrExcluded(pkg)
            )
        }
    }

    @Test
    fun testFilterApps_EmptyOrNullQueryReturnsAll() {
        val list = listOf(
            InstalledAppInfo("com.instagram.android", "Instagram", null),
            InstalledAppInfo("com.twitter.android", "X", null),
            InstalledAppInfo("com.reddit.frontpage", "Reddit", null)
        )

        assertEquals(3, InstalledAppsManager.filterApps(list, null).size)
        assertEquals(3, InstalledAppsManager.filterApps(list, "").size)
        assertEquals(3, InstalledAppsManager.filterApps(list, "   ").size)
    }

    @Test
    fun testFilterApps_ByLabelCaseInsensitive() {
        val list = listOf(
            InstalledAppInfo("com.instagram.android", "Instagram", null),
            InstalledAppInfo("com.twitter.android", "X", null),
            InstalledAppInfo("com.reddit.frontpage", "Reddit", null)
        )

        val filtered = InstalledAppsManager.filterApps(list, "gram")
        assertEquals(1, filtered.size)
        assertEquals("Instagram", filtered[0].label)

        val filteredUpper = InstalledAppsManager.filterApps(list, "REDDIT")
        assertEquals(1, filteredUpper.size)
        assertEquals("Reddit", filteredUpper[0].label)
    }

    @Test
    fun testFilterApps_ByPackageName() {
        val list = listOf(
            InstalledAppInfo("com.instagram.android", "Instagram", null),
            InstalledAppInfo("com.twitter.android", "X", null),
            InstalledAppInfo("com.reddit.frontpage", "Reddit", null)
        )

        // Searching for "twitter" finds the app even if the label is "X"
        val filtered = InstalledAppsManager.filterApps(list, "twitter")
        assertEquals(1, filtered.size)
        assertEquals("com.twitter.android", filtered[0].packageName)
    }

    @Test
    fun testFilterApps_NoMatchesReturnsEmpty() {
        val list = listOf(
            InstalledAppInfo("com.instagram.android", "Instagram", null),
            InstalledAppInfo("com.twitter.android", "X", null)
        )

        val filtered = InstalledAppsManager.filterApps(list, "tiktok")
        assertTrue(filtered.isEmpty())
    }
}
